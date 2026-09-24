package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/state"
	"github.com/biomassa/godoist/internal/todoist"
)

// testModel returns a 130x30 model with fake data. Writes make commands but the
// tests do not run them, so no request goes to the API.
//
// Sidebar rows: 0 space, 1 Inbox, 2 space, 3 Today, 4 Upcoming, 5 space, 6 "My Projects",
// 7 work, 8 space, 9 "Labels", 10 @home, 11 space, 12 All tasks, 13 Completed.
// Row r is at y = r+1.
// Rows of the work project: 0 spacer, 1 alpha, 2 beta (recurring), 3 spacer, 4 "urgent" header,
// 5 gamma, 6 spacer, 7 "later" header (empty). Row r is at y = r+1.
func testModel(t *testing.T) Model {
	t.Helper()
	// Keep the tests away from the user's state file.
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sec := "s1"
	st := todoist.SyncState{
		Token: "test",
		Projects: []todoist.Project{
			{ID: "inbox", Name: "Inbox", InboxProject: true, Color: "charcoal"},
			{ID: "p1", Name: "work", Color: "green", ChildOrder: 1},
		},
		Labels: []todoist.Label{{ID: "l1", Name: "home", Color: "blue", Order: 1}},
		Sections: []todoist.Section{
			{ID: "s1", ProjectID: "p1", Name: "urgent", SectionOrder: 1},
			{ID: "s2", ProjectID: "p1", Name: "later", SectionOrder: 2},
		},
		Tasks: []todoist.Task{
			{ID: "t1", ProjectID: "p1", Content: "alpha", Priority: 1, ChildOrder: 1, Labels: []string{"home"}},
			{ID: "t2", ProjectID: "p1", Content: "beta", Priority: 1, ChildOrder: 2,
				Due: &todoist.Due{Date: "2030-01-01", String: "every day", IsRecurring: true}},
			{ID: "t3", ProjectID: "p1", SectionID: &sec, Content: "gamma", Priority: 1, ChildOrder: 1},
		},
	}
	m := Model{client: todoist.New("test"), comCur: -1, md: newMDCache(),
		ui: state.State{ProjectModes: map[string]string{}}}
	m.width, m.height = 130, 30
	m.applyState(st)
	return m
}

func send(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		nm, _ := m.Update(msg)
		m = nm.(Model)
	}
	return m
}

func click(x, y int) tea.Msg      { return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft} }
func rightClick(x, y int) tea.Msg { return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseRight} }
func release(x, y int) tea.Msg    { return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft} }
func motion(x, y int) tea.Msg     { return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft} }

// openWork clicks the "work" project in the sidebar.
func openWork(t *testing.T) Model {
	m := send(t, testModel(t), click(5, 8))
	if cur := m.currentNav(); cur == nil || cur.projectID != "p1" {
		t.Fatalf("sidebar click did not open the work project: %+v", cur)
	}
	return m
}

func TestClickSidebarOpensProject(t *testing.T) {
	m := openWork(t)
	if m.focus != paneNav {
		t.Errorf("focus = %d, want sidebar", m.focus)
	}
	if len(m.rows) != 8 || !m.rows[0].spacer || !m.rows[3].spacer || m.rows[4].header != "urgent" || m.rows[7].header != "later" {
		t.Errorf("unexpected rows: %+v", m.rows)
	}
}

func TestClickRowSelectsTask(t *testing.T) {
	m := openWork(t)
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 3), release(x, 3))
	if m.currentTaskID() != "t2" || m.focus != paneTasks {
		t.Errorf("task = %q focus = %d, want t2 in the task list", m.currentTaskID(), m.focus)
	}
	// A click on the spacer does not move the cursor.
	m = send(t, m, click(x, 4), release(x, 4))
	if m.currentTaskID() != "t2" {
		t.Errorf("spacer click moved the cursor to %q", m.currentTaskID())
	}
}

func TestCircleClickCompletes(t *testing.T) {
	m := openWork(t)
	circle := m.layout().mid.x + 2
	m = send(t, m, click(circle, 2))
	if m.pending != 1 || m.confirm != nil {
		t.Fatalf("pending = %d confirm = %v, want one write and no prompt", m.pending, m.confirm)
	}
	for _, task := range m.snap.Tasks {
		if task.ID == "t1" {
			t.Error("completed task is still in the list")
		}
	}
}

func TestCircleClickOnRecurringAsks(t *testing.T) {
	m := openWork(t)
	m = send(t, m, click(m.layout().mid.x+2, 3))
	if m.confirm == nil || m.pending != 0 {
		t.Fatalf("confirm = %v pending = %d, want a prompt and no write", m.confirm, m.pending)
	}
	m = send(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if m.pending != 1 {
		t.Errorf("pending = %d after y, want 1", m.pending)
	}
}

func TestDoubleClickRenames(t *testing.T) {
	m := openWork(t)
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 2), release(x, 2), click(x, 2))
	if m.inputMode != inputRename || m.targetID != "t1" {
		t.Errorf("inputMode = %d target = %q, want the rename dialog for t1", m.inputMode, m.targetID)
	}
	// A click outside the dialog cancels it.
	m = send(t, m, click(1, 1))
	if m.inputMode != inputNone {
		t.Error("click outside the dialog did not close it")
	}
}

func TestRightClickMenu(t *testing.T) {
	m := openWork(t)
	x := m.layout().mid.x + 10
	m = send(t, m, rightClick(x, 2))
	if m.menu == nil {
		t.Fatal("right-click did not open the menu")
	}
	r := m.menuRect()
	m = send(t, m, click(r.x+3, r.y+2)) // row 1 is "Due date"
	if m.menu != nil || m.cal == nil {
		t.Errorf("menu = %v cal = %v, want the date dialog", m.menu, m.cal)
	}
}

func TestLegendClick(t *testing.T) {
	m := openWork(t)
	_, hits := m.bottomBarLayout()
	var add *legendHit
	for i := range hits {
		if hits[i].key == "a" {
			add = &hits[i]
		}
	}
	if add == nil {
		t.Fatal("no legend item for a")
	}
	m = send(t, m, click(add.x0, m.layout().bar))
	if m.inputMode != inputAdd {
		t.Errorf("inputMode = %d, want the add dialog", m.inputMode)
	}
}

func TestDragToSection(t *testing.T) {
	m := openWork(t)
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 2), motion(x, 4), motion(x, 5), release(x, 5))
	if m.pending != 1 || !strings.Contains(m.status, "urgent") {
		t.Errorf("pending = %d status = %q, want a move to urgent", m.pending, m.status)
	}
}

func TestDragToSidebarProject(t *testing.T) {
	m := openWork(t)
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 2), motion(x-20, 3), motion(5, 2), release(5, 2))
	if m.pending != 1 || !strings.Contains(m.status, "#Inbox") {
		t.Errorf("pending = %d status = %q, want a move to Inbox", m.pending, m.status)
	}
}

func TestDragCancelled(t *testing.T) {
	m := openWork(t)
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 2), motion(x, 3), release(x, 3))
	if m.pending != 0 || m.drag != nil {
		t.Errorf("pending = %d drag = %v, want no move", m.pending, m.drag)
	}
}

func TestWheelScrollsList(t *testing.T) {
	m := openWork(t)
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 2), release(x, 2), tea.MouseWheelMsg{X: x, Y: 5, Button: tea.MouseWheelDown})
	if m.currentTaskID() == "t1" {
		t.Error("wheel down did not move the cursor")
	}
}

func TestMenuBorderIsSolid(t *testing.T) {
	m := openWork(t)
	m = send(t, m, rightClick(m.layout().mid.x+10, 2))
	top := strings.Split(m.menuBox(), "\n")[0]
	if strings.Contains(top, " ") {
		t.Errorf("menu top border has a gap: %q", top)
	}
}

func TestEscClearsFilter(t *testing.T) {
	m := openWork(t)
	m.focusFilterNav = true
	m = send(t, m, filterMsg{query: "search: alpha", tasks: []todoist.Task{m.snap.Tasks[0]}})
	if !m.inFilterView() {
		t.Fatal("the filter view is not selected")
	}
	if bar, _ := m.bottomBarLayout(); !strings.Contains(bar, "clear filter") {
		t.Errorf("legend has no clear filter hint: %q", bar)
	}
	m = send(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.filterQuery != "" || m.currentNav().projectID != "p1" {
		t.Errorf("filter = %q view = %+v, want no filter and the work project", m.filterQuery, m.currentNav())
	}
}

func TestLegendOverflowGoesToStatusLine(t *testing.T) {
	m := openWork(t)
	m.width = 80
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 2), release(x, 2)) // focus the task list: its legend is long
	bar, hits := m.bottomBarLayout()
	lines := strings.Split(bar, "\n")
	if len(lines) != 2 {
		t.Fatalf("bottom bar has %d lines, want 2", len(lines))
	}
	var onLine2 *legendHit
	for i := range hits {
		if hits[i].y == 1 {
			onLine2 = &hits[i]
			break
		}
	}
	if onLine2 == nil {
		t.Fatalf("no legend item moved to the status line: %q", lines)
	}
	if onLine2.key == "f" { // f opens the filter dialog
		m = send(t, m, click(onLine2.x0, m.layout().bar+1))
		if m.inputMode != inputQuery {
			t.Errorf("click on %q did not run it", onLine2.key)
		}
	}
}
