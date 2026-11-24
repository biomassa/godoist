package tui

import (
	"errors"
	"testing"
)

// ctrl+q closes the editor without saving. If autosave already saved the new text, the
// old text goes back to Todoist and shows at once.
func TestEditorDiscard(t *testing.T) {
	m := send(t, notesModel(t), key("E"))
	id, old := m.note.taskID, m.note.orig
	m = typeText(t, m, "!")
	m = send(t, m, key("ctrl+q"))
	if m.note != nil || m.pending != 0 {
		t.Fatalf("note open %v pending %d, want closed and no write", m.note != nil, m.pending)
	}

	m = send(t, notesModel(t), key("E"))
	m = typeText(t, m, "!")
	m = send(t, m, noteTickMsg{gen: m.note.gen})      // autosave starts
	m = send(t, m, noteSavedMsg{text: m.note.text()}) // and ends
	if got := m.taskByID(id).Description; got != old+"!" {
		t.Fatalf("description after autosave = %q", got)
	}
	m = send(t, m, key("ctrl+q"))
	if m.note != nil || m.pending != 1 {
		t.Fatalf("note open %v pending %d, want closed and one write", m.note != nil, m.pending)
	}
	if got := m.taskByID(id).Description; got != old {
		t.Errorf("description after discard = %q, want %q", got, old)
	}
}

// ctrl+q during an autosave waits for it, and only then sends the old text. Two writes
// at once could land in any order and leave the new text in Todoist.
func TestEditorDiscardDuringAutosave(t *testing.T) {
	m := send(t, notesModel(t), key("E"))
	id, old := m.note.taskID, m.note.orig
	m = typeText(t, m, "!")
	m = send(t, m, noteTickMsg{gen: m.note.gen}) // autosave starts and is still running
	m = send(t, m, key("ctrl+q"))
	if m.note != nil || m.pending != 1 || m.discard == nil {
		t.Fatalf("note open %v pending %d, want closed and only the autosave running", m.note != nil, m.pending)
	}
	m = send(t, m, noteSavedMsg{text: old + "!"}) // the autosave ends
	if m.pending != 1 || m.discard != nil || m.taskByID(id).Description != old {
		t.Errorf("pending %d description %q, want the old text sent after the autosave", m.pending, m.taskByID(id).Description)
	}
}

// After a failed autosave, ctrl+q sends nothing, because Todoist still has the old text.
// The details pane showed the new text since the autosave started: it shows the old one again.
func TestEditorDiscardAfterFailedAutosave(t *testing.T) {
	m := send(t, notesModel(t), key("E"))
	id, old := m.note.taskID, m.note.orig
	m = typeText(t, m, "!")
	m = send(t, m, noteTickMsg{gen: m.note.gen})                              // autosave starts
	m = send(t, m, noteSavedMsg{text: old + "!", err: errors.New("offline")}) // and fails
	m = send(t, m, key("ctrl+q"))
	if m.note != nil || m.pending != 0 {
		t.Fatalf("note open %v pending %d, want closed and no write", m.note != nil, m.pending)
	}
	if got := m.taskByID(id).Description; got != old {
		t.Errorf("description after discard = %q, want %q", got, old)
	}
}
