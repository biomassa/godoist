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
	m = send(t, m, noteTickMsg{gen: m.note.gen})                 // autosave starts
	m = send(t, m, noteSavedMsg{e: m.note, text: m.note.text()}) // and ends
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
	e := m.note
	m = send(t, m, key("ctrl+q"))
	if m.note != nil || m.pending != 1 || m.discard == nil {
		t.Fatalf("note open %v pending %d, want closed and only the autosave running", m.note != nil, m.pending)
	}
	m = send(t, m, noteSavedMsg{e: e, text: old + "!"}) // the autosave ends
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
	m = send(t, m, noteTickMsg{gen: m.note.gen})                                         // autosave starts
	m = send(t, m, noteSavedMsg{e: m.note, text: old + "!", err: errors.New("offline")}) // and fails
	m = send(t, m, key("ctrl+q"))
	if m.note != nil || m.pending != 0 {
		t.Fatalf("note open %v pending %d, want closed and no write", m.note != nil, m.pending)
	}
	if got := m.taskByID(id).Description; got != old {
		t.Errorf("description after discard = %q, want %q", got, old)
	}
}

// The discard waits for the autosave of its own editor. The save of another editor that
// ends first does not start it, and that editor saves again later.
func TestEditorDiscardOtherSave(t *testing.T) {
	m := send(t, notesModel(t), key("E"))
	old := m.note.orig
	m = typeText(t, m, "!")
	m = send(t, m, noteTickMsg{gen: m.note.gen}) // the autosave of the first editor runs
	first := m.note
	m = send(t, m, key("ctrl+q"))
	m = send(t, m, key("E")) // a second editor, which saves too
	m = typeText(t, m, "?")
	m = send(t, m, noteTickMsg{gen: m.note.gen})
	second := m.note
	if !second.saving || m.pending != 2 {
		t.Fatalf("second saving %v pending %d, want both saves running", second.saving, m.pending)
	}
	m = send(t, m, noteSavedMsg{e: second, text: second.text()}) // the second save ends first
	if m.discard != first || m.pending != 1 {
		t.Fatalf("discard %v pending %d, want the discard still waiting for the first save", m.discard != nil, m.pending)
	}
	if second.saving || second.saved != second.text() {
		t.Errorf("second editor saving %v saved %q, want its save done", second.saving, second.saved)
	}
	m = send(t, m, noteSavedMsg{e: first, text: old + "!"}) // the first save ends
	if m.discard != nil || m.pending != 1 {
		t.Errorf("discard %v pending %d, want the old text sent now", m.discard != nil, m.pending)
	}
	if second.saved != second.text() {
		t.Errorf("the first save changed the second editor: saved %q", second.saved)
	}
}

// The save result of an editor that is closed now does not change the editor that is
// open: the open editor still waits for its own save.
func TestNoteSavedOtherEditor(t *testing.T) {
	m := send(t, notesModel(t), key("E"))
	m = typeText(t, m, "!")
	m = send(t, m, noteTickMsg{gen: m.note.gen}) // the autosave of the first editor runs
	first := m.note
	m = send(t, m, key("esc")) // closed: the editor saves nothing more while its save runs
	m = send(t, m, key("E"))
	m = typeText(t, m, "?")
	m = send(t, m, noteTickMsg{gen: m.note.gen})
	second := m.note
	saved := second.saved
	m = send(t, m, noteSavedMsg{e: first, text: first.text(), err: errors.New("offline")})
	if !second.saving || second.err != "" || second.saved != saved {
		t.Errorf("second editor saving %v err %q saved %q, want it unchanged", second.saving, second.err, second.saved)
	}
	if !m.statusErr {
		t.Error("the failed save of the closed editor shows no error")
	}
}
