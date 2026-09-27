package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/state"
)

// notesMarker is the line in a project description that turns on notebook view. Todoist
// syncs the description, so the view is the same on all computers.
const notesMarker = "godoist:notes"

// hasNotesMarker reports whether a project description has the notebook marker line.
func hasNotesMarker(desc string) bool {
	for _, l := range strings.Split(desc, "\n") {
		if strings.TrimSpace(l) == notesMarker {
			return true
		}
	}
	return false
}

// withNotesMarker adds the marker as the last line of desc, or removes it.
func withNotesMarker(desc string, on bool) string {
	var keep []string
	for _, l := range strings.Split(desc, "\n") {
		if strings.TrimSpace(l) != notesMarker {
			keep = append(keep, l)
		}
	}
	out := strings.TrimRight(strings.Join(keep, "\n"), "\n ")
	if on {
		if out != "" {
			out += "\n\n"
		}
		out += notesMarker
	}
	return out
}

// projectNotes reports whether project id is in notebook view: its description has the
// marker, or the local state has the old setting that was not moved to Todoist yet.
func (m Model) projectNotes(id string) bool {
	if p := m.projects[id]; p != nil && hasNotesMarker(p.Description) {
		return true
	}
	return m.ui.ProjectModes[id] == state.ModeNotes
}

// setProjectNotes turns notebook view on or off for project id and saves the marker in
// the project description.
func (m *Model) setProjectNotes(id string, on bool) tea.Cmd {
	p := m.projects[id]
	if p == nil {
		return nil
	}
	desc := withNotesMarker(p.Description, on)
	p.Description = desc // show it at once. The sync confirms it.
	if _, ok := m.ui.ProjectModes[id]; ok {
		delete(m.ui.ProjectModes, id) // Todoist keeps the setting now
		if err := state.Save(m.ui); err != nil {
			m.setStatus("could not save the view state: "+err.Error(), true)
		}
	}
	client := m.client
	return m.simpleWrite("", func(ctx context.Context) error {
		return client.UpdateProject(ctx, id, map[string]any{"description": desc})
	})
}

// moveNotesModes moves the notebook settings of the local state to Todoist. A setting
// that Todoist has already goes out of the local state. The first sync of each run
// writes the other settings to the project descriptions.
func (m *Model) moveNotesModes() tea.Cmd {
	var cmds []tea.Cmd
	changed := false
	for id, mode := range m.ui.ProjectModes {
		p := m.projects[id]
		switch {
		case mode != state.ModeNotes || p == nil:
		case hasNotesMarker(p.Description):
			delete(m.ui.ProjectModes, id)
			changed = true
		case !m.notesMoved:
			desc := withNotesMarker(p.Description, true)
			p.Description = desc
			client, pid := m.client, id
			cmds = append(cmds, m.simpleWrite("", func(ctx context.Context) error {
				return client.UpdateProject(ctx, pid, map[string]any{"description": desc})
			}))
		}
	}
	m.notesMoved = true
	if changed {
		if err := state.Save(m.ui); err != nil {
			m.setStatus("could not save the view state: "+err.Error(), true)
		}
	}
	return tea.Batch(cmds...)
}
