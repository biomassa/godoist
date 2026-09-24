package tui

import (
	"strings"
	"testing"
)

// onHeader opens the work project and puts the cursor on the "urgent" header (row 4).
func onHeader(t *testing.T) Model {
	t.Helper()
	m := openWork(t)
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 5), release(x, 5))
	if m.headerSection() == nil || m.headerSection().ID != "s1" {
		t.Fatalf("cursor is not on the urgent header: row %d", m.rowCur)
	}
	return m
}

func TestAddSection(t *testing.T) {
	m := onHeader(t)
	m = send(t, m, key("A"))
	if m.inputMode != inputAddSection {
		t.Fatalf("inputMode = %d, want the new section dialog", m.inputMode)
	}
	m = send(t, m, key("n"), key("e"), key("w"), key("enter"))
	if m.pending != 1 {
		t.Errorf("pending = %d, want one write", m.pending)
	}
}

func TestRenameSection(t *testing.T) {
	m := send(t, onHeader(t), key("e"))
	if m.inputMode != inputRenameSection || m.dialogValue() != "urgent" || m.targetSection != "s1" {
		t.Errorf("inputMode = %d value = %q target = %q", m.inputMode, m.dialogValue(), m.targetSection)
	}
}

func TestDeleteSectionAsks(t *testing.T) {
	m := send(t, onHeader(t), key("delete"))
	if m.confirm == nil || !strings.Contains(m.confirm.text, "1 task") {
		t.Fatalf("confirm = %+v, want a prompt with the task count", m.confirm)
	}
	m = send(t, m, key("y"))
	if m.pending != 1 {
		t.Errorf("pending = %d after y, want 1", m.pending)
	}
}

func TestMoveSectionDown(t *testing.T) {
	m := send(t, onHeader(t), key("]"))
	if got := m.projectSections("p1"); got[0] != "s2" || got[1] != "s1" {
		t.Errorf("order = %v, want [s2 s1]", got)
	}
	if s := m.headerSection(); s == nil || s.ID != "s1" || m.pending != 1 {
		t.Errorf("cursor section = %+v pending = %d, want the cursor on urgent and one write", s, m.pending)
	}
	m = send(t, m, key("]"))
	if m.pending != 1 || !strings.Contains(m.status, "edge") {
		t.Errorf("status = %q, want an edge message and no write", m.status)
	}
}

func TestSectionMenu(t *testing.T) {
	m := onHeader(t)
	m = send(t, m, rightClick(m.layout().mid.x+10, 5))
	if m.menu == nil || m.menu.items[0].key != "A" {
		t.Fatalf("menu = %+v, want the section menu", m.menu)
	}
	r := m.menuRect()
	m = send(t, m, click(r.x+3, r.y+2)) // row 1 is "Rename"
	if m.inputMode != inputRenameSection {
		t.Errorf("inputMode = %d, want the rename section dialog", m.inputMode)
	}
}

func TestDeleteTaskAsks(t *testing.T) {
	m := openWork(t)
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 2), release(x, 2), key("delete"))
	if m.confirm == nil || !strings.Contains(m.confirm.text, "alpha") {
		t.Fatalf("confirm = %+v, want a prompt for alpha", m.confirm)
	}
	m = send(t, m, key("n"))
	if m.pending != 0 || m.confirm != nil {
		t.Fatalf("n did not cancel: pending = %d", m.pending)
	}
	m = send(t, m, key("backspace"), key("y"))
	if m.pending != 1 {
		t.Errorf("pending = %d after y, want 1", m.pending)
	}
	for _, task := range m.snap.Tasks {
		if task.ID == "t1" {
			t.Error("deleted task is still in the list")
		}
	}
}

func TestMenuDelete(t *testing.T) {
	m := openWork(t)
	m = send(t, m, rightClick(m.layout().mid.x+10, 2))
	last := len(m.menu.items) - 1
	if m.menu.items[last].key != "delete" {
		t.Fatalf("last menu item = %+v, want Delete", m.menu.items[last])
	}
	r := m.menuRect()
	m = send(t, m, click(r.x+3, r.y+1+last))
	if m.confirm == nil {
		t.Error("menu Delete did not ask")
	}
}
