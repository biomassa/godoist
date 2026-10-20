package tui

import (
	"strings"
	"testing"
)

// # lists the projects that contain the typed text, the names that start with it first.
// tab puts the selected name and a space in the text.
func TestCompleteProject(t *testing.T) {
	m := send(t, openWork(t), key("a"))
	m = typeText(t, m, "buy #or")
	if m.comp == nil || len(m.comp.items) != 1 || m.comp.items[0].name != "work" {
		t.Fatalf("comp = %+v", m.comp)
	}
	if !strings.Contains(stripANSI(m.compBox()), "● work") {
		t.Errorf("box:\n%s", stripANSI(m.compBox()))
	}
	m = send(t, m, key("tab"))
	if got := m.dlg.Value(); got != "buy #work " || m.comp != nil || m.dlgField != 0 {
		t.Fatalf("after tab: %q comp = %v field = %d", got, m.comp != nil, m.dlgField)
	}
	m = typeText(t, m, "milk")
	if got := m.dlg.Value(); got != "buy #work milk" {
		t.Errorf("text = %q", got)
	}
}

// @ lists the labels and a "+ create" row for a new name. esc closes only the list, and
// enter with the list open does not save.
func TestCompleteLabel(t *testing.T) {
	m := send(t, openWork(t), key("a"))
	m = typeText(t, m, "x @ho")
	if m.comp == nil || len(m.comp.items) != 2 || m.comp.items[0].name != "home" || !m.comp.items[1].create {
		t.Fatalf("comp = %+v", m.comp)
	}
	m = send(t, m, key("esc"))
	if m.comp != nil || m.inputMode != inputAdd {
		t.Fatalf("esc: comp = %v mode = %d", m.comp != nil, m.inputMode)
	}
	m = typeText(t, m, "m")
	if m.comp != nil {
		t.Error("the list came back in the word that esc closed")
	}
	m = typeText(t, m, " @new")
	if m.comp == nil || len(m.comp.items) != 1 || !m.comp.items[0].create {
		t.Fatalf("comp = %+v", m.comp)
	}
	m = send(t, m, key("enter"))
	if got := m.dlg.Value(); got != "x @hom @new " || m.inputMode != inputAdd || m.pending != 1 {
		t.Errorf("create: %q mode = %d pending = %d", got, m.inputMode, m.pending)
	}
}

// / lists all sections with their projects. A section brings its #project, unless the
// text names it already. down moves the selection.
func TestCompleteSection(t *testing.T) {
	m := send(t, testModel(t), key("a"))
	m = typeText(t, m, "#Inbox y /")
	if m.comp == nil || len(m.comp.items) != 2 || m.comp.items[0].project != "work" {
		t.Fatalf("comp = %+v", m.comp)
	}
	m = send(t, m, key("down"), key("tab"))
	if got := m.dlg.Value(); got != "y /later #work " {
		t.Errorf("text = %q", got)
	}
	m = send(t, onRow(t, 1), key("A")) // a sub-task: only labels
	m = typeText(t, m, "#")
	if m.comp != nil {
		t.Error("the sub-task dialog lists projects")
	}
}
