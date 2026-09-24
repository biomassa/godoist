package tui

import (
	"strings"
	"testing"

	"github.com/biomassa/godoist/internal/todoist"
)

// onRow opens the work project and puts the cursor on list row r (y = r+1).
func onRow(t *testing.T, r int) Model {
	t.Helper()
	m := openWork(t)
	x := m.layout().mid.x + 10
	return send(t, m, click(x, r+1), release(x, r+1))
}

// rowNames lists the rows of the task list: [header], name, or name(depth).
func rowNames(m Model) string {
	var out []string
	for _, r := range m.rows {
		switch {
		case r.task != nil && r.preview == "" && r.depth > 0:
			out = append(out, r.task.Content+"^")
		case r.task != nil && r.preview == "":
			out = append(out, r.task.Content)
		case r.header != "":
			mark := ""
			if r.collapsed {
				mark = "+"
			}
			out = append(out, "["+r.header+mark+"]")
		}
	}
	return strings.Join(out, " ")
}

func TestAddSubtask(t *testing.T) {
	m := send(t, onRow(t, 1), key("A"))
	if m.inputMode != inputAddSubtask || m.targetID != "t1" {
		t.Fatalf("inputMode = %d target = %q, want the sub-task dialog for alpha", m.inputMode, m.targetID)
	}
	m = send(t, m, key("x"), key("enter"))
	if m.pending != 1 {
		t.Errorf("pending = %d, want one write", m.pending)
	}
}

func TestIndentOutdent(t *testing.T) {
	m := send(t, onRow(t, 2), key(">")) // beta under alpha
	if got := rowNames(m); got != "alpha beta^ [urgent] gamma [later]" {
		t.Fatalf("after > rows = %q", got)
	}
	m = send(t, m, key("<"))
	if got := rowNames(m); got != "alpha beta [urgent] gamma [later]" {
		t.Errorf("after < rows = %q", got)
	}
	if m.pending != 2 {
		t.Errorf("pending = %d, want two writes", m.pending)
	}
	m = send(t, onRow(t, 1), key(">"))
	if !strings.Contains(m.status, "no task above") {
		t.Errorf("status = %q, want no task above", m.status)
	}
}

func TestReorderAndCrossSection(t *testing.T) {
	m := send(t, onRow(t, 2), key("[")) // beta above alpha
	if got := rowNames(m); got != "beta alpha [urgent] gamma [later]" {
		t.Fatalf("after [ rows = %q", got)
	}
	m = send(t, m, key("]"), key("]")) // back down, then into urgent
	if got := rowNames(m); got != "alpha [urgent] beta gamma [later]" {
		t.Fatalf("after ] ] rows = %q", got)
	}
	if m.currentTaskID() != "t2" {
		t.Errorf("cursor = %q, want it to stay on beta", m.currentTaskID())
	}
	m = send(t, onRow(t, 1), key("["))
	if !strings.Contains(m.status, "edge") {
		t.Errorf("status = %q, want an edge message", m.status)
	}
}

func TestCollapseSectionAndTask(t *testing.T) {
	m := send(t, onRow(t, 4), key("z")) // the urgent header
	if got := rowNames(m); got != "alpha beta [urgent+] [later]" {
		t.Fatalf("after z rows = %q", got)
	}
	m = send(t, m, key("z"))
	if got := rowNames(m); got != "alpha beta [urgent] gamma [later]" {
		t.Fatalf("after z z rows = %q", got)
	}
	m = send(t, onRow(t, 2), key(">")) // beta under alpha
	m = send(t, m, key("k"), key("z")) // collapse alpha
	if got := rowNames(m); got != "alpha [urgent] gamma [later]" {
		t.Errorf("collapsed alpha rows = %q", got)
	}
}

func TestSelectionAndBulk(t *testing.T) {
	m := send(t, onRow(t, 1), key("s"), key("j"), key("s"))
	if n := len(m.selectedTasks()); n != 2 || !strings.Contains(m.taskTitle(), "2 selected") {
		t.Fatalf("selected = %d title = %q", n, m.taskTitle())
	}
	m = send(t, m, key("1"))
	for _, id := range []string{"t1", "t2"} {
		if m.taskByID(id).Priority != 4 {
			t.Errorf("%s priority = %d, want 4 (P1)", id, m.taskByID(id).Priority)
		}
	}
	if len(m.selectedTasks()) != 2 {
		t.Error("the selection did not stay after a bulk priority")
	}
	m = send(t, m, key("x")) // alpha is one-time, beta repeats
	if len(m.closed) != 1 || len(m.closed[0]) != 1 || m.closed[0][0] != "t1" {
		t.Errorf("undo stack = %v, want [[t1]]", m.closed)
	}
	if len(m.selectedTasks()) != 0 {
		t.Error("the selection did not clear after a bulk complete")
	}
}

func TestSelectRangeAndClear(t *testing.T) {
	m := send(t, onRow(t, 1), key("s"), key("j"), key("j"), key("j"), key("S")) // alpha … gamma
	if n := len(m.selectedTasks()); n != 3 {
		t.Fatalf("range selected %d tasks, want 3", n)
	}
	m = send(t, m, key("esc"))
	if len(m.selectedTasks()) != 0 {
		t.Error("esc did not clear the selection")
	}
	m = send(t, m, key("s"), click(5, 4)) // Today
	if len(m.selected) != 0 {
		t.Error("the selection did not clear on a view change")
	}
}

func TestBulkLabelsPartial(t *testing.T) {
	m := send(t, onRow(t, 1), key("s"), key("j"), key("s"), key("@"))
	var home *pickItem
	for i := range m.pick.items {
		if m.pick.items[i].text == "home" {
			home = &m.pick.items[i]
		}
	}
	if home == nil || home.state != stateSome {
		t.Fatalf("home = %+v, want [~]", home)
	}
	m = send(t, m, key("space"), key("enter")) // [~] → [x]
	for _, id := range []string{"t1", "t2"} {
		if tk := m.taskByID(id); len(tk.Labels) != 1 || tk.Labels[0] != "home" {
			t.Errorf("%s labels = %v, want [home]", id, tk.Labels)
		}
	}
}

func TestCompletedView(t *testing.T) {
	m := send(t, testModel(t), click(5, 14))
	if !m.inCompleted() || !m.completedLoading {
		t.Fatalf("Completed view not open or not loading")
	}
	done := todoist.Task{ID: "c1", ProjectID: "p1", Content: "old", CompletedAt: "2026-09-20T10:00:00Z"}
	m = send(t, m, completedMsg{tasks: []todoist.Task{done}})
	if got := rowNames(m); got != "[# work] old" || !m.rows[len(m.rows)-1].done {
		t.Fatalf("rows = %q", got)
	}
	m.focus = paneTasks
	m.rowCur = len(m.rows) - 1
	m = send(t, m, key("e"))
	if !strings.Contains(m.status, "reopen the task first") {
		t.Errorf("status = %q, want the reopen hint", m.status)
	}
	m = send(t, m, key("x"))
	if m.pending != 1 || len(m.completed) != 0 {
		t.Errorf("pending = %d completed = %d, want a reopen", m.pending, len(m.completed))
	}
}

func TestLabelViewAndManagement(t *testing.T) {
	m := send(t, testModel(t), click(5, 11)) // @home
	if l := m.navLabel(); l == nil || l.Name != "home" {
		t.Fatalf("cursor is not on @home")
	}
	if got := rowNames(m); got != "[# work] alpha" {
		t.Errorf("label view rows = %q", got)
	}
	m = send(t, m, key("e"))
	if m.inputMode != inputRenameLabel || m.dialogValue() != "home" {
		t.Fatalf("inputMode = %d value = %q", m.inputMode, m.dialogValue())
	}
	m = send(t, m, key("esc"), key("*"))
	if !m.navLabel().IsFavorite || m.pending != 1 {
		t.Errorf("favorite = %v pending = %d", m.navLabel().IsFavorite, m.pending)
	}
	m = send(t, m, key("delete"))
	if m.confirm == nil || !strings.Contains(m.confirm.text, "1 task") {
		t.Errorf("confirm = %+v, want the task count", m.confirm)
	}
	m = send(t, m, key("esc"), key("C"))
	if m.pick == nil || m.pick.labelID != "l1" {
		t.Errorf("pick = %+v, want the color picker for home", m.pick)
	}
}

func TestAllTasksOrderRules(t *testing.T) {
	m := send(t, testModel(t), click(5, 13))
	m.focus = paneTasks
	for i, r := range m.rows {
		if r.task != nil && r.task.ID == "t2" { // beta has a due date
			m.rowCur = i
		}
	}
	m = send(t, m, key("["))
	if !strings.Contains(m.status, "sorted by date") {
		t.Errorf("status = %q, want the sorted-by-date hint", m.status)
	}
}

func TestMoveLabelKeepsName(t *testing.T) {
	m := testModel(t)
	m.snap.Labels = append(m.snap.Labels, todoist.Label{ID: "l2", Name: "work", Order: 2})
	m.st.Labels = append(m.st.Labels, todoist.Label{ID: "l2", Name: "work", Order: 2})
	m.buildNav()
	m = send(t, m, click(5, 12)) // @work
	m = send(t, m, key("["))
	if m.pending != 1 {
		t.Errorf("pending = %d, want one write", m.pending)
	}
	if m.snap.Labels[0].Name != "work" || m.st.Labels[0].Name != "work" {
		t.Errorf("order = %v / %v, want work first", m.snap.Labels, m.st.Labels)
	}
}
