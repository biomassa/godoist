package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/biomassa/godoist/internal/todoist"
)

// overviewModel adds a parent "trip" (not due) with sub-tasks "flights" (due today) and
// "hotel" (no date), and a parent "report" (due today) with a sub-task "draft".
func overviewModel(t *testing.T) Model {
	t.Helper()
	m := testModel(t)
	today := time.Now().Format("2006-01-02")
	p1, p2 := "trip", "report"
	m.st.Tasks = append(m.st.Tasks,
		todoist.Task{ID: "trip", ProjectID: "p1", Content: "trip", Priority: 1, ChildOrder: 5},
		todoist.Task{ID: "flights", ProjectID: "p1", ParentID: &p1, Content: "flights", Priority: 1, ChildOrder: 1, Due: &todoist.Due{Date: today}},
		todoist.Task{ID: "hotel", ProjectID: "p1", ParentID: &p1, Content: "hotel", Priority: 1, ChildOrder: 2},
		todoist.Task{ID: "report", ProjectID: "p1", Content: "report", Priority: 1, ChildOrder: 6, Due: &todoist.Due{Date: today}},
		todoist.Task{ID: "draft", ProjectID: "p1", ParentID: &p2, Content: "draft", Priority: 1, ChildOrder: 1},
	)
	m.applyState(m.st)
	return send(t, m, click(5, 4)) // Today
}

func TestTodayPullsInParentExpanded(t *testing.T) {
	m := overviewModel(t)
	got := rowNames(m)
	// trip is pulled in (not due) and open; report is due and collapsed.
	if !strings.Contains(got, "trip flights^ hotel^") || !strings.Contains(got, "report") || strings.Contains(got, "draft") {
		t.Fatalf("today rows = %q", got)
	}
	for _, r := range m.rows {
		if r.task != nil && r.task.ID == "trip" && !r.pulled {
			t.Error("trip is not dimmed as a pulled-in parent")
		}
		if r.task != nil && r.task.ID == "report" && (!r.collapsed || r.hidden != 1) {
			t.Errorf("report row = %+v, want collapsed with 1 hidden", r)
		}
	}
}

func TestOverviewToggleIsPerView(t *testing.T) {
	m := overviewModel(t)
	m.focus = paneTasks
	for i, r := range m.rows {
		if r.task != nil && r.task.ID == "report" {
			m.rowCur = i
		}
	}
	m = send(t, m, key("z"))
	if got := rowNames(m); !strings.Contains(got, "report draft^") {
		t.Fatalf("after z rows = %q, want report expanded", got)
	}
	if !m.ui.OverviewOpen["today"]["report"] {
		t.Error("the state was not kept for the Today view")
	}
	if m.taskByID("report").IsCollapsed || m.pending != 0 {
		t.Error("the overview toggle changed the Todoist state")
	}
	m = send(t, m, click(5, 13)) // All tasks: its own state, report collapsed there
	if got := rowNames(m); strings.Contains(got, "draft") {
		t.Errorf("All tasks rows = %q, want report collapsed", got)
	}
}

func TestHeadingBars(t *testing.T) {
	c := newMDCache()
	lines := c.render("# One\n\ntext\n\n### Three", 30)
	if len(lines) < 5 || !strings.Contains(lines[0], "One") || !strings.Contains(lines[len(lines)-1], "Three") {
		t.Fatalf("lines = %q", lines)
	}
	if lines[0] == lines[len(lines)-1] || !strings.Contains(lines[0], "\x1b[") {
		t.Error("heading bars have no style")
	}
	if n, text := headingLevel("## Setup ##"); n != 2 || text != "Setup" {
		t.Errorf("headingLevel = %d %q", n, text)
	}
}

func TestEnterAndDoubleClickOpenEditor(t *testing.T) {
	m := send(t, notesModel(t), key("l"), key("enter"))
	if m.note == nil {
		t.Fatal("enter in the reader did not open the editor")
	}
	if n := sourceLineFor("## Head\n\n- [ ] one\n- [x] two", "  ☑ two"); n != 3 {
		t.Errorf("sourceLineFor = %d, want 3", n)
	}
	if d := m.buildDetail(40); d.editStart == 0 {
		t.Error("the reader has no editStart")
	}
}
