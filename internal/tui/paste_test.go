package tui

import (
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func pasteMsg(s string) tea.Msg { return tea.PasteMsg{Content: s} }

// A paste goes to the name field without line breaks, and to the description with them.
func TestPasteTaskDialog(t *testing.T) {
	m := send(t, openWork(t), key("a"), pasteMsg("buy\nmilk"))
	if got := m.dlg.Value(); got != "buy milk" {
		t.Errorf("name = %q, want %q", got, "buy milk")
	}
	m = send(t, m, key("tab"), pasteMsg("one\ntwo"))
	if got := m.dlgDesc.Value(); got != "one\ntwo" {
		t.Errorf("description = %q, want %q", got, "one\ntwo")
	}
}

// A paste into find filters the list at once.
func TestPasteFind(t *testing.T) {
	m := openWork(t)
	ti := textinput.New() // New makes it in the program
	m.input = &ti
	m = send(t, m, key("/"), pasteMsg("gam"))
	if m.find != "gam" {
		t.Errorf("find = %q, want gam", m.find)
	}
}

// A paste into the date text replaces "(mixed)" and makes the text the input to save.
func TestPasteCalendarText(t *testing.T) {
	m := send(t, onRow(t, 1), key("s"), key("j"), key("s"), key("t"), pasteMsg("fri"))
	if m.cal.text.Value() != "fri" || m.cal.source != srcText {
		t.Errorf("text = %q source = %d", m.cal.text.Value(), m.cal.source)
	}
}

// A paste into a picker goes to its filter.
func TestPastePickerFilter(t *testing.T) {
	m := send(t, onRow(t, 1), key("m"), pasteMsg("wor"))
	if m.pick == nil || m.pick.filter.Value() != "wor" {
		t.Fatalf("picker filter = %v", m.pick)
	}
}

// A paste into the E editor keeps the line breaks.
func TestPasteEditor(t *testing.T) {
	m := send(t, onRow(t, 1), key("E"), pasteMsg("a\nb"))
	if m.note == nil || m.note.text() != "a\nb" {
		t.Fatalf("editor = %v", m.note)
	}
}
