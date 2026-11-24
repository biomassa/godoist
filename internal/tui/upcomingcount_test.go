package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/biomassa/godoist/internal/todoist"
)

// The Upcoming row of the sidebar shows the tasks of tomorrow / all tasks of the Upcoming
// view. A recurring task counts once, by its next date. Tasks of today, overdue tasks, and
// tasks without a date do not count.
func TestUpcomingCount(t *testing.T) {
	m := testModel(t)
	date := func(d int) *todoist.Due {
		return &todoist.Due{Date: time.Now().AddDate(0, 0, d).Format("2006-01-02")}
	}
	daily := date(1)
	daily.IsRecurring, daily.String = true, "every day"
	m.st.Tasks = nil
	for i, d := range []*todoist.Due{date(1), daily, date(5), date(40), date(0), date(-2), nil} {
		m.st.Tasks = append(m.st.Tasks, todoist.Task{ID: fmt.Sprint("u", i), ProjectID: "p1", Content: fmt.Sprint("u", i), Priority: 1, Due: d})
	}
	m.applyState(m.st)
	var up navItem
	for _, n := range m.nav {
		if n.kind == vkUpcoming {
			up = n
		}
	}
	if up.countPre != 2 || up.count != 4 || up.countText() != "2/4" {
		t.Fatalf("Upcoming count %q (%d/%d), want 2/4", up.countText(), up.countPre, up.count)
	}
	m = send(t, m, key("g"), key("u"))
	shown := 0
	for _, r := range m.rows {
		if r.task != nil && r.preview == "" {
			shown++
		}
	}
	if shown != up.count {
		t.Errorf("the Upcoming view shows %d tasks, the count is %d", shown, up.count)
	}
	row := strings.TrimRight(ansi.Strip(navRow(navLine{n: up, w: 26})), " ")
	if !strings.Contains(row, "Upcoming") || !strings.HasSuffix(row, "2/4") {
		t.Errorf("sidebar row %q, want Upcoming with 2/4 at the end", row)
	}

	// No upcoming task: no count, like the other rows.
	m.st.Tasks = m.st.Tasks[4:]
	m.applyState(m.st)
	for _, n := range m.nav {
		if n.kind == vkUpcoming && strings.Contains(ansi.Strip(navRow(navLine{n: n, w: 26})), "/") {
			t.Errorf("a count without upcoming tasks: %q", ansi.Strip(navRow(navLine{n: n, w: 26})))
		}
	}
}
