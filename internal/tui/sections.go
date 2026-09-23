package tui

import (
	"context"
	"fmt"
	"slices"
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// projectSections returns the IDs of the sections of a project in their order.
func (m Model) projectSections(projectID string) []string {
	var secs []todoist.Section
	for _, s := range m.snap.Sections {
		if s.ProjectID == projectID && !s.IsArchived {
			secs = append(secs, s)
		}
	}
	sort.SliceStable(secs, func(i, j int) bool { return secs[i].SectionOrder < secs[j].SectionOrder })
	ids := make([]string, len(secs))
	for i, s := range secs {
		ids[i] = s.ID
	}
	return ids
}

// headerSection returns the section of the header under the cursor, or nil.
func (m Model) headerSection() *todoist.Section {
	r := m.currentRow()
	if r == nil || r.task != nil || r.sectionID == "" {
		return nil
	}
	return m.sections[r.sectionID]
}

// sectionKeys handles the section keys in a project view. ok is false for other keys.
func (m Model) sectionKeys(key string) (tea.Model, tea.Cmd, bool) {
	cur := m.currentNav()
	if cur == nil || cur.kind != vkProject {
		return m, nil, false
	}
	if key == "A" {
		next := m.openDialog(inputAddSection, "")
		return m, next, true
	}
	s := m.headerSection()
	if s == nil {
		return m, nil, false
	}
	switch key {
	case "e":
		m.targetSection = s.ID
		next := m.openDialog(inputRenameSection, s.Name)
		return m, next, true
	case "delete", "backspace":
		n := 0
		for _, t := range m.snap.Tasks {
			if t.Section() == s.ID {
				n++
			}
		}
		id, name, client := s.ID, s.Name, m.client
		prompt := fmt.Sprintf("Delete section “%s”? It has no tasks. y/n", name)
		if n > 0 {
			prompt = fmt.Sprintf("Delete section “%s” and its %d task(s)? This cannot be undone. y/n", name, n)
		}
		m.confirm = &confirmPrompt{prompt: prompt, run: func(m *Model) tea.Cmd {
			return m.simpleWrite("Deleted section “"+name+"”", func(ctx context.Context) error {
				return client.DeleteSection(ctx, id)
			})
		}}
		return m, nil, true
	case "[", "]":
		d := -1
		if key == "]" {
			d = 1
		}
		next := m.moveSection(s, d)
		return m, next, true
	}
	return m, nil, false
}

// addSection creates a section and puts it right after the section under the cursor.
// If the cursor is above all sections, the new section goes first.
func (m *Model) addSection(name string) tea.Cmd {
	cur := m.currentNav()
	if cur == nil || cur.kind != vkProject || name == "" {
		return nil
	}
	pid, after, client := cur.projectID, m.cursorSection(), m.client
	order := m.projectSections(pid)
	cmd := m.simpleWrite("Added section “"+name+"”", func(ctx context.Context) error {
		s, err := client.AddSection(ctx, pid, name)
		if err != nil {
			return err
		}
		i := 0
		if after != "" {
			i = slices.Index(order, after) + 1
		}
		if i == len(order) { // Todoist already put it at the end
			return nil
		}
		return client.ReorderSections(ctx, slices.Insert(slices.Clone(order), i, s.ID))
	})
	return withRetry(cmd, inputAddSection, name)
}

// renameSection saves a new section name as typed.
func (m *Model) renameSection(name string) tea.Cmd {
	s := m.sections[m.targetSection]
	if s == nil || name == "" || name == s.Name {
		return nil
	}
	id, client := s.ID, m.client
	s.Name = name // show the new name at once. The sync confirms it.
	m.buildRows(false)
	cmd := m.simpleWrite("Renamed section to “"+name+"”", func(ctx context.Context) error {
		return client.RenameSection(ctx, id, name)
	})
	return withRetry(cmd, inputRenameSection, name)
}

// moveSection moves a section d places (-1 up, 1 down). The list changes at once and the
// cursor stays on the header. A sync confirms the new order.
func (m *Model) moveSection(s *todoist.Section, d int) tea.Cmd {
	order := m.projectSections(s.ProjectID)
	i := slices.Index(order, s.ID)
	j := i + d
	if i < 0 || j < 0 || j >= len(order) {
		m.setStatus("the section is already at the edge", false)
		return nil
	}
	order[i], order[j] = order[j], order[i]
	for k := range m.snap.Sections {
		if n := slices.Index(order, m.snap.Sections[k].ID); n >= 0 {
			m.snap.Sections[k].SectionOrder = n + 1
		}
	}
	id := s.ID
	m.buildRows(false)
	for k, r := range m.rows {
		if r.task == nil && r.sectionID == id {
			m.rowCur = k
		}
	}
	client, name := m.client, s.Name
	return m.simpleWrite("Moved section “"+name+"”", func(ctx context.Context) error {
		return client.ReorderSections(ctx, order)
	})
}
