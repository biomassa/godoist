package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func key(k string) tea.Msg { return keyMsg(k) }

// openCal opens the date dialog on task i of the work project (0 alpha, 1 beta recurring).
func openCal(t *testing.T, row int) Model {
	m := openWork(t)
	m = send(t, m, click(m.layout().mid.x+10, row+2), release(m.layout().mid.x+10, row+2), key("t"))
	if m.cal == nil {
		t.Fatal("t did not open the date dialog")
	}
	return m
}

func TestCalendarArrowSelectsDay(t *testing.T) {
	m := openCal(t, 0)
	start := m.cal.day
	m = send(t, m, key("tab"), key("right"))
	if m.cal.focus != calGrid || !m.cal.day.Equal(start.AddDate(0, 0, 1)) || m.cal.source != srcCal {
		t.Fatalf("focus = %d day = %v source = %d", m.cal.focus, m.cal.day, m.cal.source)
	}
	m = send(t, m, key("enter"))
	if m.pending != 1 || !m.cal.saving {
		t.Errorf("pending = %d saving = %v, want one save", m.pending, m.cal.saving)
	}
}

func TestCalendarTabDoesNotParseUnchangedText(t *testing.T) {
	m := openCal(t, 0)
	m = send(t, m, key("tab"))
	if m.cal.parsing || m.pending != 0 {
		t.Error("tab parsed a text that did not change")
	}
}

func TestCalendarTabParsesChangedText(t *testing.T) {
	m := openCal(t, 0)
	m = send(t, m, key("f"), key("r"), key("i"), key("tab"))
	if !m.cal.parsing || m.pending != 1 || m.cal.source != srcText {
		t.Errorf("parsing = %v pending = %d source = %d, want one parse and the text as source",
			m.cal.parsing, m.pending, m.cal.source)
	}
}

func TestCalendarRecurringAsks(t *testing.T) {
	m := openCal(t, 1)
	m = send(t, m, key("tab"), key("down"), key("enter"))
	if !m.cal.askRecur || m.pending != 0 {
		t.Fatalf("askRecur = %v pending = %d, want the o/r prompt", m.cal.askRecur, m.pending)
	}
	m = send(t, m, key("o"))
	if m.pending != 1 || !strings.Contains(m.cal.note, "saving") {
		t.Errorf("pending = %d note = %q, want a save", m.pending, m.cal.note)
	}
}

func TestCalendarQuickPickAndNoDate(t *testing.T) {
	m := openCal(t, 0)
	m = send(t, m, key("tab"), key("2"))
	if want := dayOf(time.Now()).AddDate(0, 0, 1); !m.cal.day.Equal(want) {
		t.Errorf("pick 2 = %v, want tomorrow %v", m.cal.day, want)
	}
	m = send(t, m, key("5"))
	if !m.cal.cleared {
		t.Error("pick 5 did not select No date")
	}
}

func TestCalendarBadTime(t *testing.T) {
	m := openCal(t, 0)
	m = send(t, m, key("tab"), key("tab"), key("9"), key("9"), key("enter"))
	if !m.cal.noteErr || m.pending != 0 {
		t.Errorf("noteErr = %v pending = %d, want an error for 99", m.cal.noteErr, m.pending)
	}
}

func TestCalendarWeekStart(t *testing.T) {
	m := openCal(t, 0)
	m.snap.User.StartDay = 7 // Sunday
	if wd := m.calGridStart().Weekday(); wd != time.Sunday {
		t.Errorf("grid starts on %v, want Sunday", wd)
	}
	m.snap.User.StartDay = 1
	if wd := m.calGridStart().Weekday(); wd != time.Monday {
		t.Errorf("grid starts on %v, want Monday", wd)
	}
}

func TestCalendarClickDay(t *testing.T) {
	m := openCal(t, 0)
	r := m.calRect()
	// Click the first column of the third week row.
	x, y := r.x+1+calGridLeft(), r.y+1+calLineWeek0+2
	want := m.calGridStart().AddDate(0, 0, 14)
	m = send(t, m, click(x, y))
	if !m.cal.day.Equal(want) || m.cal.source != srcCal {
		t.Errorf("day = %v, want %v", m.cal.day, want)
	}
	m = send(t, m, click(x, y)) // double-click saves
	if m.pending != 1 {
		t.Errorf("pending = %d after double-click, want 1", m.pending)
	}
}

func TestParseClock(t *testing.T) {
	for _, tc := range []struct {
		in      string
		h, m    int
		all, ok bool
	}{{"", 0, 0, true, true}, {"9", 9, 0, false, true}, {"9:30", 9, 30, false, true},
		{"0930", 9, 30, false, true}, {"21:05", 21, 5, false, true}, {"25:00", 0, 0, false, false}} {
		h, mi, all, ok := parseClock(tc.in)
		if h != tc.h || mi != tc.m || all != tc.all || ok != tc.ok {
			t.Errorf("parseClock(%q) = %d %d %v %v", tc.in, h, mi, all, ok)
		}
	}
}

func TestHintsAreNotTruncated(t *testing.T) {
	m := openCal(t, 1)
	check := func(name, s string) {
		t.Helper()
		for _, l := range strings.Split(s, "\n") {
			if strings.Contains(l, "…") && !strings.Contains(l, "Due date ·") {
				t.Errorf("%s: truncated line %q", name, l)
			}
		}
	}
	for _, f := range []calFocus{calText, calGrid, calTime} {
		m.cal.setCalFocus(f)
		check(fmt.Sprintf("focus %d", f), m.calBox())
	}
	m.cal.askRecur = true
	check("recurring prompt", m.calBox())
	m.cal = nil
	m.openLabelPicker()
	check("label picker", m.pickerBox(m.pickerWidth(), m.paneHeight()))
	m.pick = nil
	m.openMovePicker()
	check("move picker", m.pickerBox(m.pickerWidth(), m.paneHeight()))
}

func TestWrapHintKeepsItemsTogether(t *testing.T) {
	lines := wrapHint("arrows day · PgUp/PgDn month · Home today · 1–5 picks · enter save", hexDim, 50)
	if len(lines) != 2 || !strings.Contains(lines[1], "1–5 picks") {
		t.Errorf("wrapHint split an item: %q", lines)
	}
}

// Tasks with different dates show mixedDue. Enter with it does nothing. One backspace
// removes it and shows mixedHint, and enter then clears all dates.
func TestCalendarBulkMixedClears(t *testing.T) {
	m := send(t, onRow(t, 1), key("s"), key("j"), key("s"), key("t")) // alpha (no date), beta
	if m.cal == nil || m.cal.text.Value() != mixedDue {
		t.Fatalf("text = %q, want %q", m.cal.text.Value(), mixedDue)
	}
	m = send(t, m, key("enter"))
	if m.cal != nil || m.pending != 0 {
		t.Fatalf("enter with %q saved: pending = %d", mixedDue, m.pending)
	}
	m = send(t, m, key("t"), key("backspace"))
	if m.cal.text.Value() != "" || m.cal.text.Placeholder != mixedHint {
		t.Fatalf("after one backspace text = %q placeholder = %q, want the hint", m.cal.text.Value(), m.cal.text.Placeholder)
	}
	m = send(t, m, key("enter"))
	if m.pending != 1 || !m.cal.saving {
		t.Errorf("pending = %d, want one save that removes the dates", m.pending)
	}
}

// Typed text replaces mixedDue.
func TestCalendarBulkMixedTypingReplaces(t *testing.T) {
	m := send(t, onRow(t, 1), key("s"), key("j"), key("s"), key("t"), key("f"))
	if got := m.cal.text.Value(); got != "f" {
		t.Errorf("text = %q, want typing to replace %q", got, mixedDue)
	}
}
