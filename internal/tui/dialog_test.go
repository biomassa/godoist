package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var ctrlEnter = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}

// The add dialog has a name and a description. tab moves between them, enter in the
// description adds a line break, and ctrl+enter saves.
func TestTaskDialogNameAndDescription(t *testing.T) {
	m := send(t, openWork(t), key("a"))
	if !m.hasDescField(m.inputMode) {
		t.Fatal("the add dialog has no description field")
	}
	m = typeText(t, m, "buy milk")
	m = send(t, m, key("tab"))
	if m.dlgField != 1 {
		t.Fatalf("tab: field = %d, want the description", m.dlgField)
	}
	m = typeText(t, m, "one")
	m = send(t, m, key("enter"))
	m = typeText(t, m, "two")
	if m.inputMode != inputAdd || m.dlgDesc.text() != "one\ntwo" {
		t.Fatalf("enter in the description: mode = %d desc = %q", m.inputMode, m.dlgDesc.text())
	}
	if m.dialogValue() != "buy milk" {
		t.Errorf("name = %q", m.dialogValue())
	}
	view := stripANSI(m.dialogBox())
	for _, want := range []string{"Name", "Description", "ctrl+enter or ctrl+s add"} {
		if !strings.Contains(view, want) {
			t.Errorf("dialog has no %q:\n%s", want, view)
		}
	}
	if h := m.dialogRect().h; h != m.height*2/3 {
		t.Errorf("dialog height = %d, want %d (2/3 of %d)", h, m.height*2/3, m.height)
	}
	m = send(t, m, ctrlEnter)
	if m.inputMode != inputNone || m.pending != 1 {
		t.Errorf("ctrl+enter: mode = %d pending = %d, want one save", m.inputMode, m.pending)
	}
}

// enter in the name saves at once. shift+tab goes back to the name.
func TestTaskDialogEnterInNameSaves(t *testing.T) {
	m := send(t, openWork(t), key("a"))
	m = typeText(t, m, "x")
	m = send(t, m, key("tab"), tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.dlgField != 0 {
		t.Fatalf("shift+tab: field = %d, want the name", m.dlgField)
	}
	m = send(t, m, key("enter"))
	if m.inputMode != inputNone || m.pending != 1 {
		t.Errorf("enter in the name: mode = %d pending = %d, want one save", m.inputMode, m.pending)
	}
}

// e opens the dialog with the name and the description of the task.
func TestEditDialogHasDescription(t *testing.T) {
	m := onRow(t, 1)
	m.taskByID("t1").Description = "old notes"
	m.buildRows(false)
	m = send(t, m, key("e"))
	if m.inputMode != inputRename || m.dialogValue() != "alpha" || m.dlgDesc.text() != "old notes" {
		t.Fatalf("mode = %d name = %q desc = %q", m.inputMode, m.dialogValue(), m.dlgDesc.text())
	}
	if !strings.Contains(stripANSI(m.dialogBox()), "Edit task") {
		t.Error("the edit dialog title is not \"Edit task\"")
	}
	m = send(t, m, key("enter")) // nothing changed
	if m.pending != 0 {
		t.Errorf("pending = %d, want no save without changes", m.pending)
	}
}

// A click on the description field moves the keys there.
func TestTaskDialogClickField(t *testing.T) {
	m := send(t, openWork(t), key("a"))
	r := m.dialogRect()
	m = send(t, m, click(r.x+5, r.y+1+4+dialogNameLines))
	if m.dlgField != 1 {
		t.Errorf("click on the description: field = %d", m.dlgField)
	}
}

// ctrl+s saves the task dialog too, for terminals that send ctrl+enter as enter.
func TestTaskDialogCtrlS(t *testing.T) {
	m := send(t, openWork(t), key("a"))
	m = typeText(t, m, "x")
	m = send(t, m, key("tab"), key("ctrl+s"))
	if m.inputMode != inputNone || m.pending != 1 {
		t.Errorf("ctrl+s in the description: mode = %d pending = %d", m.inputMode, m.pending)
	}
}

// ctrl+enter saves the description editor (E) and closes it.
func TestEditorCtrlEnterSaves(t *testing.T) {
	m := send(t, onRow(t, 1), key("E"), pasteMsg("new text"), key("ctrl+enter"))
	if m.note != nil || m.pending != 1 || m.taskByID("t1").Description != "new text" {
		t.Errorf("note = %v pending = %d description = %q", m.note != nil, m.pending, m.taskByID("t1").Description)
	}
}

// The description of the task dialog is a markdown editor. With the keys in the name, all
// lines show rendered. In the description, the cursor line shows its source, enter
// continues a list, and a click puts the cursor at the clicked character.
func TestTaskDialogDescriptionPreview(t *testing.T) {
	m := onRow(t, 1)
	m.taskByID("t1").Description = "## Head\n- one **bold**"
	m.buildRows(false)
	m = send(t, m, key("e"))
	view := stripANSI(m.dialogBox())
	if strings.Contains(view, "##") || !strings.Contains(view, "• one bold") {
		t.Fatalf("name focused, the description is not rendered:\n%s", view)
	}
	m = send(t, m, key("tab")) // the cursor is at the start of the heading
	view = stripANSI(m.dialogBox())
	if !strings.Contains(view, "## Head") || !strings.Contains(view, "• one bold") {
		t.Fatalf("description focused:\n%s", view)
	}
	m = send(t, m, key("down"), key("end"), key("enter"))
	m = typeText(t, m, "two")
	if got := m.dlgDesc.text(); got != "## Head\n- one **bold**\n- two" {
		t.Fatalf("list continuation: %q", got)
	}
	// "o" of "one" is the 3rd cell of the rendered row "• one bold"; in the source it is rune 2.
	r := m.dialogRect()
	m = send(t, m, click(r.x+2+1+2, m.descTop()+1))
	if e := m.dlgDesc; e.row != 1 || e.col != 2 {
		t.Errorf("click: row = %d col = %d, want 1, 2", e.row, e.col)
	}
}
