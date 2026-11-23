package tui

import "testing"

// gg goes to the top, gt opens Today, and a key that is not a jump key works as usual.
func TestGotoPrefix(t *testing.T) {
	m := onRow(t, 2)
	m = send(t, m, key("g"))
	if !m.gPrefix || m.status != gotoHint {
		t.Fatalf("after g: prefix %v status %q", m.gPrefix, m.status)
	}
	m = send(t, m, key("g"))
	if m.gPrefix || m.currentTask() == nil || m.rowCur > 1 {
		t.Errorf("after gg: prefix %v rowCur %d, want the top", m.gPrefix, m.rowCur)
	}
	m = send(t, m, key("g"), key("t"))
	if cur := m.currentNav(); cur == nil || cur.kind != vkToday || m.focus != paneTasks {
		t.Errorf("after gt: nav %+v focus %d, want Today and the task list", cur, m.focus)
	}
	m = send(t, onRow(t, 1), key("g"), key("j"))
	if m.gPrefix || m.rowCur != 2 || m.status == gotoHint {
		t.Errorf("after g j: prefix %v rowCur %d status %q, want j to move down", m.gPrefix, m.rowCur, m.status)
	}
}

// ctrl+k opens the go-to picker. Typing filters it, and enter opens the item.
func TestGotoPicker(t *testing.T) {
	m := send(t, testModel(t), key("ctrl+k"))
	if m.pick == nil || m.pick.kind != pickGoto {
		t.Fatal("ctrl+k did not open the go-to picker")
	}
	m = send(t, m, key("w"), key("o"), key("r"), key("k"), key("enter"))
	if cur := m.currentNav(); m.pick != nil || cur == nil || cur.projectID != "p1" || m.focus != paneTasks {
		t.Errorf("picker open %v nav %+v focus %d, want the work project", m.pick != nil, cur, m.focus)
	}
}

// A click between g and the next key cancels g: t after the click opens the date dialog,
// it does not jump to Today.
func TestGotoPrefixMouse(t *testing.T) {
	m := send(t, onRow(t, 1), key("g"))
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 3), release(x, 3)) // another row: a second click on a row is a double click
	if m.gPrefix || m.status == gotoHint {
		t.Fatalf("prefix %v status %q after a click, want g cancelled", m.gPrefix, m.status)
	}
	m = send(t, m, key("t"))
	if m.cal == nil {
		t.Error("t after the click did not open the date dialog")
	}
}

// A jump clears the find text of the old view, like a move in the sidebar.
func TestGotoClearsFind(t *testing.T) {
	m := onRow(t, 1)
	m.find = "alpha"
	m = send(t, m, key("g"), key("i"))
	if m.find != "" {
		t.Errorf("find = %q after gi, want it cleared", m.find)
	}
}

// If another message takes the status line after g, the hint is gone, so g is dropped:
// t then opens the date dialog and does not jump to Today.
func TestGotoPrefixStatusChange(t *testing.T) {
	m := send(t, onRow(t, 1), key("g"))
	m.setStatus("Completed alpha", false) // like the result of a write that ends now
	m = send(t, m, key("t"))
	if m.gPrefix || m.cal == nil {
		t.Errorf("prefix %v, date dialog open %v, want g dropped and the date dialog", m.gPrefix, m.cal != nil)
	}
}
