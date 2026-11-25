package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/biomassa/godoist/internal/todoist"
)

// withKeys draws the marked keys in the accent color and the rest in the base style. The
// marks do not reach the screen.
func TestWithKeys(t *testing.T) {
	base := st(hexDim)
	got := withKeys(keyed("{enter} save · {esc} cancel"), base)
	want := st(fg(hexAccent)).Render("enter") + base.Render(" save · ") + st(fg(hexAccent)).Render("esc") + base.Render(" cancel")
	if got != want {
		t.Errorf("withKeys = %q, want %q", got, want)
	}
	if strings.ContainsAny(got, keyOpen+keyClose) || ansi.Strip(got) != "enter save · esc cancel" {
		t.Errorf("withKeys left marks or lost text: %q", ansi.Strip(got))
	}
	if got := withKeys("plain {text}", base); got != base.Render("plain {text}") {
		t.Errorf("a text without marks: %q", got) // braces of a task name stay as they are
	}
	if got := ansi.Strip(withKeys("cut "+keyOpen+"ent", base)); got != "cut ent" {
		t.Errorf("a cut text: %q", got)
	}
}

// The delete dialog has the question in the text color, and the undo hint on its own line,
// paler, with u in the accent color.
func TestDeleteDialogNote(t *testing.T) {
	m := send(t, onRow(t, 1), key("delete"))
	d, _ := m.confirmLayout()
	lines := strings.Split(d, "\n")
	q, n := -1, -1
	for i, l := range lines {
		switch plain := ansi.Strip(l); {
		case strings.Contains(plain, "Delete “alpha”?"):
			q = i
			if strings.Contains(plain, "undoes") {
				t.Errorf("the undo hint is on the line of the question: %q", plain)
			}
		case strings.Contains(plain, "u undoes it for 10 seconds."):
			n = i
			if !strings.Contains(l, withKeys(undoHint, st(hexMuted))) {
				t.Errorf("the undo hint is not muted with an accent u: %q", l)
			}
		}
	}
	if q < 0 || n != q+1 {
		t.Errorf("question on line %d, hint on line %d, want the hint on the next line", q, n)
	}
}

// After a delete, the status counts the seconds down. Once another status replaced it, the
// count leaves that status alone, and after u the count stops.
func TestDeleteCountdown(t *testing.T) {
	m := send(t, onRow(t, 1), key("delete"), key("enter"))
	n := m.laterN
	if got := plainKeys(m.status); got != "Deleted “alpha” · u to undo (10 s)" {
		t.Fatalf("status = %q", got)
	}
	m.laterDeletes[n].at = time.Now().Add(-3*time.Second - 100*time.Millisecond)
	nm, cmd := m.Update(deleteTickMsg{n: n})
	m = nm.(Model)
	if got := plainKeys(m.status); got != "Deleted “alpha” · u to undo (7 s)" || cmd == nil {
		t.Fatalf("status = %q, next tick %v, want 7 s and one more tick", got, cmd != nil)
	}
	m.setStatus("Priority → P1", false)
	m = send(t, m, deleteTickMsg{n: n})
	if m.status != "Priority → P1" {
		t.Errorf("status = %q, want the newer status kept", m.status)
	}
	m = send(t, m, key("u"))
	if _, cmd := m.Update(deleteTickMsg{n: n}); cmd != nil {
		t.Error("the count goes on after u")
	}
}

// deleteTick wakes when the shown seconds change.
func TestDeleteTickTime(t *testing.T) {
	for _, tc := range []struct{ left, want time.Duration }{
		{10 * time.Second, time.Second},
		{9300 * time.Millisecond, 300 * time.Millisecond},
		{400 * time.Millisecond, 400 * time.Millisecond},
	} {
		if got := tc.left - (tc.left - time.Nanosecond).Truncate(time.Second); got != tc.want {
			t.Errorf("left %v: next tick in %v, want %v", tc.left, got, tc.want)
		}
	}
}

// Each marked key on these screens is drawn by withKeys: the frame has no mark before the
// last safety pass of view. A mark that reaches the terminal switches the character set of
// some terminals.
func TestNoKeyMarksOnScreen(t *testing.T) {
	withInput := func(m Model) Model { ti := newTextInput(); m.input = &ti; return m } // as New makes it
	screens := map[string]Model{
		"help":       send(t, onRow(t, 1), key("?")),
		"delete":     send(t, onRow(t, 1), key("delete")),
		"deleted":    send(t, onRow(t, 1), key("delete"), key("enter")),
		"add":        send(t, onRow(t, 1), key("a")),
		"date":       send(t, onRow(t, 1), key("t")),
		"move":       send(t, onRow(t, 1), key("m")),
		"goto":       send(t, onRow(t, 1), key("ctrl+k")),
		"find":       send(t, withInput(onRow(t, 1)), key("/")),
		"theme":      send(t, onRow(t, 1), key("T")),
		"details":    send(t, onRow(t, 1), key("l")),
		"no match":   send(t, withInput(onRow(t, 1)), key("/"), key("z"), key("z"), key("z"), key("enter")),
		"notes":      send(t, notesModel(t), key("E")),
		"checkboxes": send(t, onRow(t, 1), key("l"), key("space")),
	}
	for name, m := range screens {
		screen := m.render()
		if m.width > 0 {
			screen = m.overlays(screen)
		}
		if i := strings.IndexAny(screen, keyOpen+keyClose); i >= 0 {
			t.Errorf("%s: a key mark on the screen near %q", name, ansi.Strip(screen[max(0, i-40):min(len(screen), i+20)]))
		}
	}
}

// A long name in the status of a waiting delete gets shorter, so that the closing quote,
// the key, and the count stay on the screen.
func TestDeleteStatusCut(t *testing.T) {
	m := send(t, onRow(t, 1), key("delete"), key("enter"))
	if got := ansi.Strip(m.cutStatus(st(hexToday).Render("✓ ")+withKeys(m.status, st(hexText)), 30)); !strings.HasSuffix(got, "” · u to undo (10 s)") || lipgloss.Width(got) > 30 {
		t.Errorf("cut status = %q, want the end kept within 30 cells", got)
	}
	m.setStatus("Priority → P1 for a very long name of a task", false)
	if got := ansi.Strip(m.cutStatus(st(hexToday).Render("✓ ")+withKeys(m.status, st(hexText)), 20)); !strings.HasSuffix(got, "…") {
		t.Errorf("another status = %q, want it cut at the end as before", got)
	}
}

// In the details pane, a blank line and a dim rule come before the "Description" label,
// and a blank line between the label and the text.
func TestDescriptionBlankLine(t *testing.T) {
	m := onRow(t, 1)
	m.currentTask().Description = "first line of the description"
	lines := m.detailLines(60, 40)
	for i, l := range lines {
		if strings.TrimSpace(ansi.Strip(l)) == "Description" {
			if rule := strings.TrimSpace(ansi.Strip(lines[i-1])); rule == "" || strings.Trim(rule, "─") != "" || strings.TrimSpace(ansi.Strip(lines[i-2])) != "" {
				t.Errorf("before Description: %q, %q, want a blank line, then a rule", ansi.Strip(lines[i-2]), ansi.Strip(lines[i-1]))
			}
			if i+2 >= len(lines) || strings.TrimSpace(ansi.Strip(lines[i+1])) != "" ||
				!strings.Contains(ansi.Strip(lines[i+2]), "first line of the description") {
				t.Errorf("after Description: %q, %q, want a blank line, then the text", ansi.Strip(lines[i+1]), ansi.Strip(lines[min(i+2, len(lines)-1)]))
			}
			return
		}
	}
	t.Fatal("no Description label")
}

// Comments come after a dim rule, the label, and a blank line. Each comment starts with its
// date and " - ", and its text wraps under its first letter. A blank line divides comments.
func TestCommentLayout(t *testing.T) {
	m := onRow(t, 1)
	id := m.currentTask().ID
	posted := time.Date(2026, 11, 25, 10, 53, 0, 0, time.Local).Format(time.RFC3339)
	m.snap.Comments[id] = []todoist.Comment{
		{ID: "c1", TaskID: id, PostedAt: posted, Content: "testing a comment that is long enough to wrap onto more lines of the pane"},
		{ID: "c2", TaskID: id, PostedAt: posted, Content: "short one"},
	}
	lines := m.detailLines(50, 60)
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = strings.TrimRight(strings.TrimPrefix(ansi.Strip(l), " "), " ") // the pane has one cell of margin
	}
	at := -1
	for i, l := range plain {
		if strings.TrimSpace(l) == "Comments 2" {
			at = i
		}
	}
	if at < 2 || strings.Trim(strings.TrimSpace(plain[at-1]), "─") != "" || plain[at-1] == "" || plain[at-2] != "" || plain[at+1] != "" {
		t.Fatalf("around the label: %q", plain[max(0, at-2):min(len(plain), at+2)])
	}
	first := plain[at+2]
	const head = " 25 Nov 10:53 - testing"
	if !strings.HasPrefix(first, head) {
		t.Fatalf("first comment line = %q, want it to start with %q", first, head)
	}
	col := len(" 25 Nov 10:53 - ")
	next := plain[at+3]
	if strings.TrimSpace(next[:min(col, len(next))]) != "" || len(next) <= col || next[col] == ' ' {
		t.Errorf("second line = %q, want its text to start under %q at column %d", next, "testing", col)
	}
	var blank, short = -1, -1
	for i := at + 3; i < len(plain); i++ {
		if strings.HasPrefix(plain[i], " 25 Nov 10:53 - short one") {
			short = i
			break
		}
		if plain[i] == "" && blank < 0 {
			blank = i
		}
	}
	if short < 0 || blank != short-1 {
		t.Errorf("second comment on line %d, blank line on %d, want one blank line before it", short, blank)
	}
	if strings.Contains(strings.Join(plain, "\n"), "── 25 Nov") {
		t.Error("the old date rule is still there")
	}
}
