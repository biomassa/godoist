package tui

import (
	"context"
	"fmt"
	"slices"
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// taskByID returns a pointer into the snapshot, or nil.
func (m *Model) taskByID(id string) *todoist.Task {
	for i := range m.snap.Tasks {
		if m.snap.Tasks[i].ID == id {
			return &m.snap.Tasks[i]
		}
	}
	return nil
}

// siblingIDs returns the tasks with the same parent as t (and, at the top level, the same
// project and section), in their order.
func (m Model) siblingIDs(t *todoist.Task) []string {
	return m.groupIDs(t.ProjectID, t.Section(), t.Parent())
}

// groupIDs returns the tasks of one project, section, and parent, in their order.
func (m Model) groupIDs(projectID, sectionID, parentID string) []string {
	var ts []todoist.Task
	for _, x := range m.snap.Tasks {
		if x.ProjectID != projectID || x.Parent() != parentID {
			continue
		}
		if parentID == "" && x.Section() != sectionID {
			continue
		}
		ts = append(ts, x)
	}
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].ChildOrder < ts[j].ChildOrder })
	ids := make([]string, len(ts))
	for i, x := range ts {
		ids[i] = x.ID
	}
	return ids
}

// hasChildren reports whether a task has sub-tasks.
func (m Model) hasChildren(id string) bool {
	for _, t := range m.snap.Tasks {
		if t.Parent() == id {
			return true
		}
	}
	return false
}

// setOrder gives the tasks the order of ids in the snapshot, so that the list changes at once.
func (m *Model) setOrder(ids []string) {
	for i, id := range ids {
		if t := m.taskByID(id); t != nil {
			t.ChildOrder = i + 1
		}
	}
}

// orderAllowed reports whether [ ] > < work in the current view, and sets the status if not.
// In All tasks, [ ] work only on tasks without a due date, because the view sorts dated tasks.
func (m *Model) orderAllowed(t *todoist.Task, reorder bool) bool {
	cur := m.currentNav()
	switch {
	case cur == nil:
		return false
	case cur.kind == vkProject:
		return true
	case cur.kind == vkAll && reorder && t.Due != nil:
		m.setStatus("sorted by date · open the project to reorder", true)
		return false
	case cur.kind == vkAll:
		return true
	}
	m.setStatus("open the project to change the order", true)
	return false
}

// orderKeys handles A (sub-task), [ ] (order), > < (indent), and z (collapse) on a task.
// ok is false for other keys.
func (m Model) orderKeys(key string) (tea.Model, tea.Cmd, bool) {
	// The rows of a project view hold copies of the tasks. Changes go to the snapshot.
	t := m.taskByID(m.currentTaskID())
	if t == nil {
		return m, nil, false
	}
	var next tea.Cmd
	switch key {
	case "A":
		m.targetID = t.ID
		next = m.openDialog(inputAddSubtask, "")
	case "[", "]":
		if !m.orderAllowed(t, true) {
			return m, nil, true
		}
		d := -1
		if key == "]" {
			d = 1
		}
		next = m.moveTaskOrder(t, d)
	case ">":
		if !m.orderAllowed(t, false) {
			return m, nil, true
		}
		next = m.indent(t)
	case "<":
		if !m.orderAllowed(t, false) {
			return m, nil, true
		}
		next = m.outdent(t)
	case "z":
		if !m.hasChildren(t.ID) {
			m.setStatus("this task has no sub-tasks", false)
			return m, nil, true
		}
		if cur := m.currentNav(); cur != nil && isOverview(cur.kind) {
			next = m.toggleOverview(*m.currentRow()) // overviews keep their own state
		} else {
			next = m.toggleTaskCollapse(t)
		}
	default:
		return m, nil, false
	}
	return m, next, true
}

// addSubtask adds a task with Todoist parsing and makes it a sub-task of the target task.
// A #project in the text has no effect: the sub-task goes to the parent's project.
// A description that is not empty is saved as typed.
func (m *Model) addSubtask(text, desc string) tea.Cmd {
	parent := m.taskByID(m.targetID)
	if parent == nil || text == "" {
		return nil
	}
	pid, pname, client := parent.ID, plain(parent.Content), m.client
	cmd := m.write("", func(ctx context.Context) (string, error) {
		t, err := client.QuickAdd(ctx, text)
		if err != nil {
			return "", err
		}
		if desc != "" {
			if _, err := client.UpdateTask(ctx, t.ID, map[string]any{"description": desc}); err != nil {
				return "", fmt.Errorf("added, but the description was not saved: %w", err)
			}
		}
		if err := client.MoveToParent(ctx, t.ID, pid); err != nil {
			return "", fmt.Errorf("added, but it is not a sub-task: %w", err)
		}
		return "Added sub-task “" + t.Content + "” to “" + pname + "”", nil
	})
	return withRetryDesc(cmd, inputAddSubtask, text, desc)
}

// moveTaskOrder moves a task d places among its siblings. A top-level task at the first
// or last place goes into the previous or next section. A sub-task stops at the edge.
func (m *Model) moveTaskOrder(t *todoist.Task, d int) tea.Cmd {
	order := m.siblingIDs(t)
	i := slices.Index(order, t.ID)
	j := i + d
	id, name, client := t.ID, plain(t.Content), m.client
	if j >= 0 && j < len(order) {
		order[i], order[j] = order[j], order[i]
		m.setOrder(order)
		m.buildRows(false)
		return m.simpleWrite("Moved “"+name+"”", func(ctx context.Context) error { return client.ReorderTasks(ctx, order) })
	}
	if t.Parent() != "" {
		m.setStatus("already the first or last sub-task", false)
		return nil
	}
	groups := append([]string{""}, m.projectSections(t.ProjectID)...) // "" is the part without a section
	g := slices.Index(groups, t.Section()) + d
	if g < 0 || g >= len(groups) {
		m.setStatus("already at the edge of the project", false)
		return nil
	}
	target := groups[g]
	dest := m.groupIDs(t.ProjectID, target, "")
	if d < 0 {
		dest = append(dest, id) // the end of the previous section
	} else {
		dest = append([]string{id}, dest...) // the start of the next section
	}
	if target == "" {
		t.SectionID = nil
	} else {
		sid := target
		t.SectionID = &sid
	}
	m.setOrder(dest)
	m.buildRows(false)
	where := "the top of the project"
	if s := m.sections[target]; s != nil {
		where = "section “" + s.Name + "”"
	}
	pid := t.ProjectID
	return m.simpleWrite("Moved “"+name+"” to "+where, func(ctx context.Context) error {
		if err := client.Move(ctx, id, pid, target); err != nil {
			return err
		}
		return client.ReorderTasks(ctx, dest)
	})
}

// indent makes a task a sub-task of the task right above it at the same level.
func (m *Model) indent(t *todoist.Task) tea.Cmd {
	order := m.siblingIDs(t)
	i := slices.Index(order, t.ID)
	if i <= 0 {
		m.setStatus("no task above at the same level", false)
		return nil
	}
	parent := order[i-1]
	pt := m.taskByID(parent)
	pid := parent
	t.ParentID = &pid
	t.ChildOrder = 1 << 20 // the end of the new parent's sub-tasks
	if pt != nil && pt.IsCollapsed {
		pt.IsCollapsed = false // show the task in its new place
	}
	m.buildRows(false)
	id, name, client := t.ID, plain(t.Content), m.client
	return m.simpleWrite("Indented “"+name+"”", func(ctx context.Context) error {
		return client.MoveToParent(ctx, id, parent)
	})
}

// outdent moves a sub-task one level up, right after its old parent.
func (m *Model) outdent(t *todoist.Task) tea.Cmd {
	parent := m.taskByID(t.Parent())
	if parent == nil {
		m.setStatus("already at the top level", false)
		return nil
	}
	newParent := parent.Parent()
	dest := m.groupIDs(parent.ProjectID, parent.Section(), newParent)
	dest = slices.Insert(dest, slices.Index(dest, parent.ID)+1, t.ID)
	if newParent == "" {
		t.ParentID = nil
	} else {
		np := newParent
		t.ParentID = &np
	}
	t.SectionID = parent.SectionID
	m.setOrder(dest)
	m.buildRows(false)
	id, name, client := t.ID, plain(t.Content), m.client
	projectID, sectionID := parent.ProjectID, parent.Section()
	return m.simpleWrite("Outdented “"+name+"”", func(ctx context.Context) error {
		var err error
		if newParent != "" {
			err = client.MoveToParent(ctx, id, newParent)
		} else {
			err = client.Move(ctx, id, projectID, sectionID)
		}
		if err != nil {
			return err
		}
		return client.ReorderTasks(ctx, dest)
	})
}

// toggleTaskCollapse hides or shows the sub-tasks of a task and saves the state in Todoist.
func (m *Model) toggleTaskCollapse(t *todoist.Task) tea.Cmd {
	t.IsCollapsed = !t.IsCollapsed
	m.buildRows(false)
	id, state, client := t.ID, t.IsCollapsed, m.client
	return m.simpleWrite("", func(ctx context.Context) error { return client.SetCollapsed(ctx, "tasks", id, state) })
}

// toggleSectionCollapse hides or shows the tasks of a section.
func (m *Model) toggleSectionCollapse(s *todoist.Section) tea.Cmd {
	s.IsCollapsed = !s.IsCollapsed
	m.buildRows(false)
	id, state, client := s.ID, s.IsCollapsed, m.client
	return m.simpleWrite("", func(ctx context.Context) error { return client.SetCollapsed(ctx, "sections", id, state) })
}

// toggleProjectCollapse hides or shows the sub-projects of a project in the sidebar.
func (m *Model) toggleProjectCollapse(p *todoist.Project) tea.Cmd {
	p.IsCollapsed = !p.IsCollapsed
	m.buildNav()
	id, state, client := p.ID, p.IsCollapsed, m.client
	return m.simpleWrite("", func(ctx context.Context) error { return client.SetCollapsed(ctx, "projects", id, state) })
}
