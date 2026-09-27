package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// selModel opens the inline editor on "one two\nthree" with the cursor at the start.
func selModel(t *testing.T) Model {
	t.Helper()
	m := notesModel(t)
	m.taskByID("t1").Description = "one two\nthree"
	m.buildRows(false)
	m = send(t, m, key("E"))
	m.note.row, m.note.col = 0, 0
	return m
}

func shiftKey(k rune) tea.Msg { return tea.KeyPressMsg{Code: k, Mod: tea.ModShift} }

// shift+arrows select, typed text replaces the selection, and ctrl+z brings it back.
func TestSelectAndReplace(t *testing.T) {
	m := selModel(t)
	m = send(t, m, shiftKey(tea.KeyRight), shiftKey(tea.KeyRight), shiftKey(tea.KeyRight))
	if got := m.note.selectedText(); got != "one" {
		t.Fatalf("selected = %q, want one", got)
	}
	m = send(t, m, key("X"))
	if got := m.note.text(); got != "X two\nthree" {
		t.Errorf("after typing = %q", got)
	}
	m = send(t, m, key("ctrl+z"))
	if got := m.note.text(); got != "one two\nthree" {
		t.Errorf("after undo = %q", got)
	}
}

// A selection over two lines: backspace removes it and joins the lines.
func TestSelectLinesAndDelete(t *testing.T) {
	m := selModel(t)
	m = send(t, m, shiftKey(tea.KeyDown), key("backspace"))
	if got := m.note.text(); got != "three" {
		t.Errorf("text = %q, want three", got)
	}
}

// ctrl+c copies the selection to the clipboard and ctrl+x cuts it. A move key without
// shift removes the selection.
func TestCopyCut(t *testing.T) {
	m := selModel(t)
	m = send(t, m, key("ctrl+a"))
	if m.note.selectedText() != "one two\nthree" {
		t.Fatalf("ctrl+a selected %q", m.note.selectedText())
	}
	nm, cmd := m.Update(key("ctrl+c"))
	m = nm.(Model)
	if cmd == nil || m.note.text() != "one two\nthree" || !strings.Contains(m.status, "copied") {
		t.Errorf("ctrl+c: cmd = %v status = %q", cmd != nil, m.status)
	}
	m = send(t, m, key("ctrl+x"))
	if m.note.text() != "" || !strings.Contains(m.status, "cut") {
		t.Errorf("ctrl+x: text = %q status = %q", m.note.text(), m.status)
	}
	m = send(t, m, key("ctrl+z"), key("ctrl+a"), key("left"))
	if _, _, _, _, ok := m.note.selRange(); ok {
		t.Error("left did not remove the selection")
	}
}

// The clipboard text from ctrl+v replaces the selection.
func TestClipboardPasteReplaces(t *testing.T) {
	m := selModel(t)
	m = send(t, m, shiftKey(tea.KeyRight), shiftKey(tea.KeyRight), shiftKey(tea.KeyRight))
	m = send(t, m, tea.ClipboardMsg{Content: "ONE"})
	if got := m.note.text(); got != "ONE two\nthree" {
		t.Errorf("text = %q", got)
	}
}

// A mouse drag selects text. Selected lines show as source.
func TestDragSelects(t *testing.T) {
	m := selModel(t)
	r := m.sideRect()
	y := r.y + 2 // the first text row, under the empty top line
	m = send(t, m, click(r.x+2, y), motion(r.x+2+3, y), release(r.x+2+3, y))
	if got := m.note.selectedText(); got != "one" {
		t.Errorf("drag selected %q, want one", got)
	}
}

// In the task dialog, ctrl+a selects the name, ctrl+x cuts it, and a clipboard paste
// puts text in.
func TestDialogClipboard(t *testing.T) {
	m := send(t, openWork(t), key("a"))
	m = typeText(t, m, "milk")
	m = send(t, m, key("ctrl+a"), key("ctrl+x"))
	if m.dlg.Value() != "" || !strings.Contains(m.status, "cut") {
		t.Fatalf("ctrl+x: value = %q status = %q", m.dlg.Value(), m.status)
	}
	m = send(t, m, tea.ClipboardMsg{Content: "bread"})
	if m.dlg.Value() != "bread" {
		t.Errorf("paste: value = %q", m.dlg.Value())
	}
}
