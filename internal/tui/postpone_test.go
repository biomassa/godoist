package tui

import (
	"strings"
	"testing"
	"time"
)

// The postpone date counts from the due date, or from today for an overdue task. 0 days
// is today, also for a later due date. The time of day stays.
func TestPostponeTo(t *testing.T) {
	now := time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local)
	for _, c := range []struct {
		due  time.Time
		days int
		want time.Time
	}{
		{time.Date(2026, 11, 6, 0, 0, 0, 0, time.Local), 1, time.Date(2026, 11, 7, 0, 0, 0, 0, time.Local)},
		{time.Date(2026, 11, 3, 9, 30, 0, 0, time.Local), 2, time.Date(2026, 11, 5, 9, 30, 0, 0, time.Local)},
		{time.Date(2026, 10, 20, 14, 0, 0, 0, time.Local), 7, time.Date(2026, 11, 10, 14, 0, 0, 0, time.Local)},
		{time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local), 3, time.Date(2027, 1, 3, 0, 0, 0, 0, time.Local)},
		{time.Date(2026, 11, 20, 9, 0, 0, 0, time.Local), 0, time.Date(2026, 11, 3, 9, 0, 0, 0, time.Local)},
		{time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), 0, time.Date(2026, 11, 3, 0, 0, 0, 0, time.Local)},
	} {
		if got := postponeTo(c.due, c.days, now); !got.Equal(c.want) {
			t.Errorf("postponeTo(%v, %d) = %v, want %v", c.due, c.days, got, c.want)
		}
	}
}

// p on a dated task opens the dialog: Today and four day buttons in one row, and
// Custom... and Cancel in a second row. A day button writes, and Custom... opens the date dialog.
func TestPostponeDialog(t *testing.T) {
	m := send(t, onRow(t, 1), key("p")) // alpha, due Mon 31 Dec 2029
	p := m.confirm
	if p == nil || p.title != "Postpone" || len(p.buttons) != 7 || p.head != "“alpha”" || !strings.Contains(p.text, "due Mon 31 Dec") {
		t.Fatalf("confirm = %+v", p)
	}
	var keys []string
	for _, b := range p.buttons {
		keys = append(keys, b.key)
	}
	if got := strings.Join(keys, " "); got != "t 1 2 3 7 c n" {
		t.Errorf("keys = %q", got)
	}
	_, hits := m.confirmLayout()
	if hits[0].y != hits[4].y || hits[5].y <= hits[4].y || hits[5].y != hits[6].y {
		t.Errorf("rows: %v", hits)
	}
	for i, h := range hits {
		if h.x+h.w > m.confirmWidth()-1 {
			t.Errorf("button %d goes past the border: %v", i, h)
		}
	}
	if p.sel != 1 {
		t.Errorf("selected button %d, want 1 (1 day): enter must not move the task to today", p.sel)
	}
	if w := send(t, m, key("1")); w.confirm != nil || w.pending != 1 {
		t.Errorf("1 day: confirm = %v pending = %d", w.confirm != nil, w.pending)
	}
	if c := send(t, m, key("c")); c.confirm != nil || c.cal == nil {
		t.Errorf("Custom...: confirm = %v cal = %v", c.confirm != nil, c.cal != nil)
	}
	if n := send(t, m, key("n")); n.confirm != nil || n.pending != 0 {
		t.Errorf("Cancel: confirm = %v pending = %d", n.confirm != nil, n.pending)
	}
}

// p on a task without a date opens the dialog too, and the days count from today.
func TestPostponeUndated(t *testing.T) {
	m := onRow(t, 1)
	m.currentTask().Due = nil
	m = send(t, m, key("p"))
	if m.confirm == nil || m.confirm.text != "no date" {
		t.Fatalf("confirm = %+v", m.confirm)
	}
	m = send(t, m, key("t"))
	if m.confirm != nil || m.pending != 1 {
		t.Errorf("Today: confirm = %v pending = %d", m.confirm != nil, m.pending)
	}
}

// With tasks selected, p postpones all of them in one write, and the selection stays.
func TestPostponeSelected(t *testing.T) {
	m := onRow(t, 1)
	m = send(t, m, key("s"), key("j"), key("s"))
	if n := len(m.selectedTasks()); n != 2 {
		t.Fatalf("%d tasks selected, want 2", n)
	}
	m = send(t, m, key("p"))
	if m.confirm == nil || m.confirm.text != "2 selected tasks" {
		t.Fatalf("confirm = %+v", m.confirm)
	}
	m = send(t, m, key("1"))
	if m.pending != 1 || len(m.selectedTasks()) != 2 {
		t.Errorf("pending = %d selected = %d, want one write and the selection", m.pending, len(m.selectedTasks()))
	}
}
