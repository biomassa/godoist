package tui

import tea "charm.land/bubbletea/v2"

// discardNoteEditor closes the editor and drops the changes (ctrl+q). esc saves, so this
// is the only way out without the new text. If autosave already sent a changed text,
// the text from before the editor opened goes back to Todoist.
func (m Model) discardNoteEditor() (tea.Model, tea.Cmd) {
	e := m.note
	m.note = nil
	if e.kind == editCommentNew || e.saved == e.orig && !e.saving {
		if e.err != "" {
			// An autosave failed. Todoist has the old text, but saveNote already put the
			// new text into the details pane: put the old one back there too.
			m.showLocalText(e, e.orig)
		}
		m.setStatus("changes discarded", false)
		return m, nil
	}
	e.lines = splitRunes(e.orig)
	if e.saving { // two writes at once could land in any order: wait for the autosave
		m.discard = e
		m.setStatus("discarding the changes…", false)
		return m, nil
	}
	next := m.saveNote(e, discardDone)
	return m, next
}

// discardDone is the status after ctrl+q wrote the old text back.
const discardDone = "Changes discarded · the old text is back"

// discardAfterSave sends the old text of a discarded editor once its autosave ended. ok
// is false if no discard waits.
func (m *Model) discardAfterSave() (next tea.Cmd, ok bool) {
	e := m.discard
	if e == nil {
		return nil, false
	}
	m.discard = nil
	return m.saveNote(e, discardDone), true
}

// showLocalText puts text into the local copy of the task or comment that e edits, as
// saveNote does, so the details pane shows it before the next sync.
func (m *Model) showLocalText(e *noteEditor, text string) {
	switch e.kind {
	case editDescription:
		if t := m.taskByID(e.taskID); t != nil {
			t.Description = text
		}
	case editCommentEdit:
		for i, cm := range m.snap.Comments[e.taskID] {
			if cm.ID == e.commentID {
				m.snap.Comments[e.taskID][i].Content = text
			}
		}
	}
}
