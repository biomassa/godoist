package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// notesModel opens the work project in notebook view. alpha has two checkboxes.
func notesModel(t *testing.T) Model {
	t.Helper()
	m := testModel(t)
	m.chkCur = -1
	m.ui.ProjectModes["p1"] = "notes"
	for i := range m.snap.Tasks {
		if m.snap.Tasks[i].ID == "t1" {
			m.snap.Tasks[i].Description = "## Head\n\n- [ ] one\n- [x] two"
		}
	}
	m = send(t, m, click(5, 8))
	m.buildRows(true)
	x := m.layout().mid.x + 10
	return send(t, m, click(x, 2), release(x, 2))
}

func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m = send(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func TestToggleCheckboxFunc(t *testing.T) {
	got, ok := toggleCheckbox("- [ ] a\ntext\n* [x] b\n1. [ ] c", 1)
	if !ok || got != "- [ ] a\ntext\n* [ ] b\n1. [ ] c" {
		t.Errorf("toggle = %q %v", got, ok)
	}
}

func TestReaderCheckboxes(t *testing.T) {
	m := notesModel(t)
	if m.currentTaskID() != "t1" {
		t.Fatalf("cursor = %q, want alpha", m.currentTaskID())
	}
	m = send(t, m, key("l")) // focus the reader
	if m.noteChecks() != 2 {
		t.Fatalf("noteChecks = %d, want 2", m.noteChecks())
	}
	m = send(t, m, key("tab"))
	if m.focus != paneDetail || m.chkCur != 0 {
		t.Fatalf("focus = %d chkCur = %d, want the first checkbox", m.focus, m.chkCur)
	}
	m = send(t, m, key("tab"), key("space"))
	if d := m.taskByID("t1").Description; !strings.HasSuffix(d, "- [ ] two") || m.pending != 1 {
		t.Errorf("description = %q pending = %d, want two unchecked", d, m.pending)
	}
	m = send(t, m, key("tab")) // wraps to the first
	if m.chkCur != 0 {
		t.Errorf("chkCur = %d after wrap, want 0", m.chkCur)
	}
}

func TestNoteEditorBasics(t *testing.T) {
	m := send(t, notesModel(t), key("E"))
	if m.note == nil {
		t.Fatal("E did not open the inline editor")
	}
	// The cursor starts at the end: "- [x] two". enter continues the list.
	m = send(t, m, key("enter"))
	if got := m.note.text(); !strings.HasSuffix(got, "- [x] two\n- [ ] ") {
		t.Fatalf("after enter = %q", got)
	}
	m = send(t, m, key("enter")) // an empty item ends the list
	if got := m.note.text(); !strings.HasSuffix(got, "- [x] two\n") {
		t.Fatalf("after second enter = %q", got)
	}
	m = typeText(t, m, "word")
	m = send(t, m, key("ctrl+b"))
	if got := m.note.text(); !strings.HasSuffix(got, "**word**") {
		t.Fatalf("after ctrl+b = %q", got)
	}
	m = send(t, m, key("ctrl+t"))
	if got := m.note.text(); !strings.HasSuffix(got, "- [ ] **word**") {
		t.Fatalf("after ctrl+t = %q", got)
	}
	m = send(t, m, key("ctrl+z"))
	if got := m.note.text(); !strings.HasSuffix(got, "\n**word**") {
		t.Fatalf("after undo = %q", got)
	}
	m = send(t, m, key("ctrl+y"))
	if got := m.note.text(); !strings.HasSuffix(got, "- [ ] **word**") {
		t.Fatalf("after redo = %q", got)
	}
	if !strings.Contains(m.noteTitle(), "unsaved") {
		t.Errorf("title = %q, want unsaved", m.noteTitle())
	}
}

func TestNoteAutosaveAndClose(t *testing.T) {
	m := send(t, notesModel(t), key("E"))
	m = typeText(t, m, "!")
	gen := m.note.gen
	m = send(t, m, noteTickMsg{gen: gen - 1}) // an old timer does nothing
	if m.pending != 0 {
		t.Fatalf("pending = %d after an old tick, want 0", m.pending)
	}
	m = send(t, m, noteTickMsg{gen: gen})
	if m.pending != 1 || !m.note.saving {
		t.Fatalf("pending = %d saving = %v, want an autosave", m.pending, m.note.saving)
	}
	m = send(t, m, noteSavedMsg{text: m.note.text()})
	if m.note.dirty() || !strings.Contains(m.noteTitle(), "saved") {
		t.Errorf("after save: dirty = %v title = %q", m.note.dirty(), m.noteTitle())
	}
	m = typeText(t, m, "?")
	m = send(t, m, key("esc"))
	if m.note != nil || m.pending != 1 {
		t.Errorf("note = %v pending = %d, want esc to close and save", m.note, m.pending)
	}
}

func TestLineKinds(t *testing.T) {
	k := lineKinds("- [ ] **b** `c`", false)
	if k[0] != skMarker || k[6] != skBold || k[12] != skCode {
		t.Errorf("kinds = %v", k)
	}
	if lineKinds("## x", false)[3] != skHeading {
		t.Error("heading not styled")
	}
}

func TestWordMovesAndDelete(t *testing.T) {
	e := &noteEditor{lines: splitRunes("alpha beta gamma"), want: -1}
	e.col = len(e.lines[0])
	e.wordLeft()
	if e.col != 11 {
		t.Errorf("wordLeft col = %d, want 11", e.col)
	}
	e.col = len(e.lines[0])
	e.deleteWordBack()
	if e.text() != "alpha beta " {
		t.Errorf("deleteWordBack = %q", e.text())
	}
}

func TestFindReplace(t *testing.T) {
	m := send(t, notesModel(t), key("E"), key("ctrl+f"))
	if m.note.search == nil {
		t.Fatal("ctrl+f did not open the find box")
	}
	m = typeText(t, m, "O") // matches "o" in "one" and "two" (case-insensitive)
	if n := len(m.note.search.matches); n != 2 {
		t.Fatalf("matches = %d, want 2", n)
	}
	if !strings.Contains(stripANSI(m.findBox(60)), "of 2") || !strings.Contains(stripANSI(m.findBox(60)), "with nothing") {
		t.Errorf("find box = %q", stripANSI(m.findBox(60)))
	}
	m = send(t, m, key("tab"), key("tab"), key("space")) // match case on
	if len(m.note.search.matches) != 0 {
		t.Fatalf("match case still finds %d", len(m.note.search.matches))
	}
	m = send(t, m, key("space"), key("shift+tab")) // case off, back to replace
	m = typeText(t, m, "0")
	m = send(t, m, key("enter")) // enter only goes to the next match
	if got := m.note.text(); strings.Contains(got, "0") {
		t.Fatalf("enter replaced: %q", got)
	}
	m = send(t, m, key("ctrl+r"))
	if got := m.note.text(); strings.Count(got, "0") != 1 {
		t.Fatalf("after ctrl+r = %q", got)
	}
	m = send(t, m, key("ctrl+a"))
	if got := m.note.text(); strings.Count(got, "0") != 2 || m.note.search != nil {
		t.Fatalf("after ctrl+a = %q (box open: %v)", got, m.note.search != nil)
	}
	m = send(t, m, key("ctrl+z"))
	if got := m.note.text(); strings.Count(got, "0") != 1 {
		t.Errorf("undo of replace all = %q, want one step back", got)
	}
}

func TestReplaceWithNothing(t *testing.T) {
	m := send(t, notesModel(t), key("E"), key("ctrl+f"))
	m = typeText(t, m, "[")
	m = send(t, m, key("ctrl+a")) // the replace field is empty: remove all "["
	if got := m.note.text(); strings.Contains(got, "[") {
		t.Errorf("after ctrl+a with an empty replace = %q", got)
	}
}
