package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

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

// A message on the status line after g, like the result of a write, does not cancel g:
// t still jumps to Today.
func TestGotoPrefixStatusChange(t *testing.T) {
	m := send(t, onRow(t, 1), key("g"))
	m.setStatus("Completed alpha", false) // like the result of a write that ends now
	m = send(t, m, key("t"))
	if cur := m.currentNav(); m.gPrefix || m.cal != nil || cur == nil || cur.kind != vkToday {
		t.Errorf("prefix %v, date dialog open %v, nav %+v, want a jump to Today", m.gPrefix, m.cal != nil, cur)
	}
}

// g shows "g…" at once and the box of the jump keys after gotoDelay. A quick second key
// never shows the box, and the message of an older g does nothing.
func TestGotoPopup(t *testing.T) {
	m := send(t, onRow(t, 1), key("g"))
	if m.status != gotoHint || m.gPopup {
		t.Fatalf("after g: status %q popup %v, want the hint and no box yet", m.status, m.gPopup)
	}
	m = send(t, m, gotoPopupMsg{m.gSeq})
	if !m.gPopup {
		t.Fatal("no box after the delay")
	}
	r := m.gotoRect()
	lines := strings.Split(stripANSI(m.View().Content), "\n")
	if r.x+r.w > m.width-1 || r.y+r.h > m.paneHeight()+1 {
		t.Errorf("box %+v is not inside the panes of a %dx%d screen", r, m.width, m.height)
	}
	if !strings.Contains(lines[r.y], "Go to") || !strings.Contains(lines[r.y+2], "t  Today") {
		t.Errorf("box not on the screen:\n%s", strings.Join(lines[r.y:r.y+r.h], "\n"))
	}
	m = send(t, m, key("u"))
	if cur := m.currentNav(); m.gPopup || m.gPrefix || m.status == gotoHint || cur == nil || cur.kind != vkUpcoming {
		t.Errorf("after u: popup %v prefix %v status %q nav %+v, want Upcoming and no box", m.gPopup, m.gPrefix, m.status, cur)
	}

	// A quick gt: the delayed message comes after the jump and shows nothing.
	m = send(t, onRow(t, 1), key("g"))
	seq := m.gSeq
	m = send(t, m, key("t"), gotoPopupMsg{seq})
	if m.gPopup {
		t.Error("the box shows after a quick gt")
	}
	// g, esc, g: the message of the first g does nothing.
	m = send(t, m, key("g"), key("esc"), key("g"))
	if m = send(t, m, gotoPopupMsg{m.gSeq - 1}); m.gPopup {
		t.Error("the message of an older g showed the box")
	}
	if m = send(t, m, key("esc")); m.gPrefix || m.status == gotoHint {
		t.Errorf("esc: prefix %v status %q, want g cancelled", m.gPrefix, m.status)
	}
}

// A dialog that opens while g waits, like the retry dialog after a failed write, gets the
// keys: g stops waiting, and the box does not show over the dialog.
func TestGotoPopupDialog(t *testing.T) {
	m := send(t, onRow(t, 1), key("g"))
	m.confirm = yesNo("Retry", "Try again?", "Retry", func(*Model) tea.Cmd { return nil })
	m = send(t, m, gotoPopupMsg{m.gSeq})
	if m.gPopup || m.gPrefix {
		t.Errorf("popup %v prefix %v with a dialog open, want g cancelled", m.gPopup, m.gPrefix)
	}
	m = send(t, onRow(t, 1), key("g"))
	m.confirm = yesNo("Retry", "Try again?", "Retry", func(*Model) tea.Cmd { return nil })
	m = send(t, m, key("n"))
	if m.gPrefix {
		t.Error("g still waits after a key that went to the dialog")
	}
}
