package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/biomassa/godoist/internal/state"
	"github.com/biomassa/godoist/internal/todoist"
)

// withSubs opens the work project with two sub-tasks under alpha: delta (open) and
// epsilon (completed). The cursor is on delta.
func withSubs(t *testing.T) Model {
	t.Helper()
	m := openWork(t)
	p := "t1"
	st := m.st
	st.Tasks = append(append([]todoist.Task(nil), st.Tasks...),
		todoist.Task{ID: "t4", ProjectID: "p1", ParentID: &p, Content: "delta", Priority: 1, ChildOrder: 1})
	st.Done = []todoist.Task{{ID: "t5", ProjectID: "p1", ParentID: &p, Content: "epsilon", ChildOrder: 2,
		CompletedAt: time.Now().UTC().Format(time.RFC3339)}}
	m.applyState(st)
	m.focus = paneTasks
	for i, r := range m.rows {
		if r.task != nil && r.task.ID == "t4" {
			m.rowCur = i
		}
	}
	return m
}

// rowByID returns the row of task id, or nil.
func rowByID(m Model, id string) *row {
	for i := range m.rows {
		if m.rows[i].task != nil && m.rows[i].task.ID == id {
			return &m.rows[i]
		}
	}
	return nil
}

// A completed sub-task shows struck through in its place under its open task, and the
// task shows the count of completed and all sub-tasks.
func TestDoneSubUnderTask(t *testing.T) {
	m := withSubs(t)
	if got := rowNames(m); !strings.Contains(got, "alpha delta^ epsilon^") {
		t.Fatalf("rows = %s", got)
	}
	if r := rowByID(m, "t5"); r == nil || !r.doneSub || !r.done {
		t.Errorf("epsilon row = %+v", r)
	}
	if r := rowByID(m, "t1"); r == nil || subCount(*r) != "1/2" {
		t.Errorf("alpha count = %q", subCount(*r))
	}
	if n := m.nav[m.navCur].count; n != 4 {
		t.Errorf("sidebar count = %d, want only the open tasks", n)
	}
}

// x on an open sub-task strikes it through in place. A sync that does not have the
// completion yet does not bring it back. x on the struck row reopens it in place.
func TestCompleteSubInPlace(t *testing.T) {
	m := withSubs(t)
	stale := m.st
	m = send(t, m, key("x"))
	if r := rowByID(m, "t4"); r == nil || !r.doneSub || m.pending != 1 || m.fx != nil {
		t.Fatalf("delta row = %+v pending = %d fx = %v", r, m.pending, m.fx != nil)
	}
	if m.status != "Completed “delta” · x to reopen" {
		t.Errorf("status = %q", m.status)
	}
	m.applyState(stale)
	if r := rowByID(m, "t4"); r == nil || !r.doneSub || m.taskByID("t4") != nil {
		t.Fatalf("after a stale sync: delta row = %+v", r)
	}
	m = send(t, m, key("x"))
	if r := rowByID(m, "t4"); r == nil || r.doneSub || m.taskByID("t4") == nil || m.pending != 2 {
		t.Errorf("after reopen: delta row = %+v pending = %d", r, m.pending)
	}
}

// ctrl+z after an in-place completion reopens the sub-task in place, without a flight.
func TestUndoSubInPlace(t *testing.T) {
	m := send(t, withSubs(t), key("x"), key("ctrl+z"))
	if r := rowByID(m, "t4"); r == nil || r.doneSub || m.fx != nil {
		t.Errorf("delta row = %+v fx = %v", r, m.fx != nil)
	}
}

// On a completed sub-task, the keys that change a task show a hint and do nothing.
func TestDoneSubKeysBlocked(t *testing.T) {
	m := withSubs(t)
	for i, r := range m.rows {
		if r.task != nil && r.task.ID == "t5" {
			m.rowCur = i
		}
	}
	for _, k := range []string{"e", "t", "p", "1", "m", "delete", "s", ">", "A"} {
		got := send(t, m, key(k))
		if got.status != "reopen the sub-task first (x)" || got.inputMode != inputNone || got.confirm != nil || got.cal != nil || got.pending != 0 {
			t.Errorf("%s: status = %q", k, got.status)
		}
	}
}

// Completing the top task takes its completed sub-tasks out of the tree.
func TestDoneSubLeavesWithTask(t *testing.T) {
	m := withSubs(t)
	st := m.st
	st.Tasks = st.Tasks[1:] // alpha is completed
	m.applyState(st)
	if rowByID(m, "t5") != nil {
		t.Errorf("rows = %s", rowNames(m))
	}
}

// The Completed view leaves out the sub-tasks of open tasks and puts the other sub-tasks
// under their completed task. A sub-task whose task is not in the view shows its name.
func TestCompletedTree(t *testing.T) {
	m := withSubs(t)
	top, alpha, gone := "c1", "t1", "zz"
	m.completed = []todoist.Task{
		{ID: "c1", ProjectID: "p1", Content: "parent", CompletedAt: "2026-11-02T10:00:00Z"},
		{ID: "c2", ProjectID: "p1", ParentID: &top, Content: "child", CompletedAt: "2026-11-01T10:00:00Z"},
		{ID: "t5", ProjectID: "p1", ParentID: &alpha, Content: "epsilon", CompletedAt: "2026-11-01T09:00:00Z"},
		{ID: "c3", ProjectID: "p1", ParentID: &gone, Content: "orphan", CompletedAt: "2026-11-01T08:00:00Z"},
	}
	m.parentNames = map[string]string{"zz": "old task"}
	rows := m.completedRows()
	var names []string
	for _, r := range rows {
		switch {
		case r.task != nil:
			names = append(names, strings.Repeat(">", r.depth)+r.task.Content+"|"+r.upName)
		default:
			names = append(names, "["+r.header+"]")
		}
	}
	if got := strings.Join(names, " "); got != "[# work] parent| >child| orphan|old task" {
		t.Errorf("rows = %s", got)
	}
	if rows[0].count != 3 {
		t.Errorf("header count = %d", rows[0].count)
	}
}

// All tasks shows the completed sub-tasks under their open task too.
func TestDoneSubInAllTasks(t *testing.T) {
	m := withSubs(t)
	m.selectNav(func(n navItem) bool { return n.kind == vkAll })
	m.ui.OverviewOpen = map[string]map[string]bool{m.overviewKey(): {"t1": true}}
	m.buildRows(true)
	if r := rowByID(m, "t5"); r == nil || !r.doneSub {
		t.Errorf("rows = %s", rowNames(m))
	}
}

// Bulk x: a selected sub-task of an open task stays in place, and the others go.
func TestBulkCompleteSub(t *testing.T) {
	m := withSubs(t)
	m.toggleSelect("t4")
	m.toggleSelect("t3")
	m = send(t, m, key("x"))
	if r := rowByID(m, "t4"); r == nil || !r.doneSub {
		t.Errorf("delta row = %+v", r)
	}
	if rowByID(m, "t3") != nil || m.taskByID("t3") != nil {
		t.Errorf("gamma is still there: %s", rowNames(m))
	}
}

// Two completions, then a sync that has neither: both stay struck through. A reopen and
// then another completion: the reopened task stays open.
func TestLocalChangesSurviveStaleSync(t *testing.T) {
	m := withSubs(t)
	p := "t1"
	st := m.st
	st.Tasks = append(st.Tasks, todoist.Task{ID: "t6", ProjectID: "p1", ParentID: &p, Content: "zeta", Priority: 1, ChildOrder: 3})
	m.applyState(st)
	stale := m.st
	cursorOn := func(id string) {
		for i, r := range m.rows {
			if r.task != nil && r.task.ID == id {
				m.rowCur = i
			}
		}
	}
	cursorOn("t4")
	m = send(t, m, key("x"))
	cursorOn("t6")
	m = send(t, m, key("x"))
	m.applyState(stale)
	for _, id := range []string{"t4", "t6"} {
		if r := rowByID(m, id); r == nil || !r.doneSub {
			t.Errorf("%s after a stale sync: %+v", id, r)
		}
	}
	cursorOn("t5")
	m = send(t, m, key("x")) // reopen epsilon
	cursorOn("t4")
	m = send(t, m, key("x")) // reopen delta
	m.applyState(stale)
	if r := rowByID(m, "t5"); r == nil || r.doneSub {
		t.Errorf("epsilon after a stale sync: %+v", r)
	}
}

// A double-click in the reader of a completed note does not open the editor.
func TestDoneNoteReaderClick(t *testing.T) {
	m := withSubs(t)
	m.ui.ProjectModes["p1"] = state.ModeNotes
	m.buildRows(false)
	for i, r := range m.rows {
		if r.task != nil && r.task.ID == "t5" && r.preview == "" {
			m.rowCur = i
		}
	}
	if !m.notesMode() || !m.onDoneSub() {
		t.Fatalf("notes = %v on done = %v", m.notesMode(), m.onDoneSub())
	}
	s := m.sideRect()
	m = send(t, m, click(s.x+5, s.y+6), click(s.x+5, s.y+6)) // a double-click under the path line
	if m.note != nil || m.status != "reopen the sub-note first (x)" {
		t.Errorf("editor = %v status = %q", m.note != nil, m.status)
	}
}

// A task with sub-tasks is bold, also in the Completed view. A task without is not.
func TestParentBold(t *testing.T) {
	m := withSubs(t)
	bold := func(r *row) bool {
		return strings.Contains(m.taskLine(*r, 80, lipgloss.NewStyle(), false, time.Now()), "\x1b[1;")
	}
	if !bold(rowByID(m, "t1")) || bold(rowByID(m, "t4")) {
		t.Errorf("alpha bold = %v, delta bold = %v", bold(rowByID(m, "t1")), bold(rowByID(m, "t4")))
	}
	top := "c1"
	m.completed = []todoist.Task{
		{ID: "c1", ProjectID: "p1", Content: "parent", CompletedAt: "2026-11-02T10:00:00Z"},
		{ID: "c2", ProjectID: "p1", ParentID: &top, Content: "child", CompletedAt: "2026-11-01T10:00:00Z"},
	}
	rows := m.completedRows()
	if !bold(&rows[1]) || bold(&rows[2]) {
		t.Errorf("Completed: parent bold = %v, child bold = %v", bold(&rows[1]), bold(&rows[2]))
	}
}
