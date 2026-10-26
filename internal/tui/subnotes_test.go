package tui

import (
	"strings"
	"testing"

	"github.com/biomassa/godoist/internal/todoist"
)

// subnoteModel is the notebook of notesModel with sub-notes under alpha: one and two,
// and deep under one. The cursor is on alpha.
func subnoteModel(t *testing.T) Model {
	t.Helper()
	m := notesModel(t)
	p := func(s string) *string { return &s }
	m.snap.Tasks = append(m.snap.Tasks,
		todoist.Task{ID: "n1", ProjectID: "p1", ParentID: p("t1"), Content: "one", ChildOrder: 1},
		todoist.Task{ID: "n2", ProjectID: "p1", ParentID: p("t1"), Content: "two", ChildOrder: 2},
		todoist.Task{ID: "n3", ProjectID: "p1", ParentID: p("n1"), Content: "deep", ChildOrder: 1})
	m.buildRows(false)
	m.focus = paneTasks
	return m
}

// cursorTo puts the list cursor on the note with this title.
func cursorTo(t *testing.T, m Model, name string) Model {
	t.Helper()
	for i, r := range m.rows {
		if r.task != nil && r.preview == "" && r.task.Content == name {
			m.rowCur = i
			return m
		}
	}
	t.Fatalf("no row %q in %q", name, rowNames(m))
	return m
}

// The list shows sub-notes with tree lines, and a parent with ▾. z folds it: ▸ and the
// number of hidden sub-notes.
func TestSubnoteTree(t *testing.T) {
	m := subnoteModel(t)
	if got := rowNames(m); !strings.HasPrefix(got, "alpha one^ deep^ two^ beta") {
		t.Fatalf("rows = %q", got)
	}
	view := stripANSI(strings.Join(m.taskLines(60, 20), "\n"))
	for _, want := range []string{"alpha ▾", "├ one ▾", "│ └ deep", "└ two"} {
		if !strings.Contains(view, want) {
			t.Errorf("no %q in\n%s", want, view)
		}
	}
	m = send(t, cursorTo(t, m, "alpha"), key("z"))
	if got := rowNames(m); !strings.HasPrefix(got, "alpha beta") {
		t.Errorf("after z rows = %q", got)
	}
	if view := stripANSI(strings.Join(m.taskLines(60, 20), "\n")); !strings.Contains(view, "alpha ▸ 3") {
		t.Errorf("folded:\n%s", view)
	}
}

// A opens "New sub-note" for the note at the cursor. The title is saved as typed.
func TestAddSubnote(t *testing.T) {
	m := send(t, cursorTo(t, subnoteModel(t), "two"), key("A"))
	if m.inputMode != inputAddSubnote || m.targetID != "n2" {
		t.Fatalf("mode = %d target = %q", m.inputMode, m.targetID)
	}
	m = typeText(t, m, "tomorrow #x")
	m = send(t, m, key("enter"))
	if m.inputMode != inputNone || m.pending != 1 || m.jumpTo == nil {
		t.Errorf("mode = %d pending = %d", m.inputMode, m.pending)
	}
}

// m opens "Move under": the top level and the notes, without the note and its sub-notes.
// A note nests the note under it; the top level un-nests it.
func TestMoveNoteUnder(t *testing.T) {
	m := send(t, cursorTo(t, subnoteModel(t), "one"), key("m"))
	if m.pick == nil || m.pick.kind != pickNoteParent {
		t.Fatalf("pick = %+v", m.pick)
	}
	var names []string
	for _, it := range m.pick.items {
		if it.taskID != "" || it.top {
			names = append(names, it.text)
		}
	}
	if got := strings.Join(names, ","); got != "⌂ top level,○ alpha,○ two,○ beta,○ gamma" {
		t.Fatalf("items = %q", got)
	}
	nt := func(n string) pickItem {
		for _, it := range m.pick.items {
			if it.text == n {
				return it
			}
		}
		t.Fatalf("no item %q", n)
		return pickItem{}
	}
	next := m.pickNoteParentEnter(nt("○ beta"))
	if next == nil || m.taskByID("n1").Parent() != "t2" || !strings.Contains(rowNames(m), "beta one^ deep^") {
		t.Fatalf("under beta: rows = %q", rowNames(m))
	}
	m = send(t, cursorTo(t, m, "one"), key("m"))
	m.pickNoteParentEnter(nt("⌂ top level"))
	if p := m.taskByID("n1").Parent(); p != "" || !strings.Contains(rowNames(m), "one deep^") {
		t.Errorf("top level: parent = %q rows = %q", p, rowNames(m))
	}
	if m.moveNoteUnder(m.taskByID("t1"), "n2") != nil || !strings.Contains(m.status, "itself") {
		t.Errorf("a note went under its own sub-note: status = %q", m.status)
	}
}

// The reader of a sub-note shows its path; a parent lists its sub-notes. tab selects a
// sub-note and enter opens it.
func TestReaderSubnotes(t *testing.T) {
	m := subnoteModel(t)
	m.focus = paneDetail
	d := m.buildDetail(50)
	if len(d.subnotes) != 2 || !strings.Contains(stripANSI(strings.Join(d.lines, "\n")), "Sub-notes 2") {
		t.Fatalf("subnotes = %+v", d.subnotes)
	}
	n := m.noteChecks()
	for range n + 1 { // the checkboxes, then the first sub-note
		m = send(t, m, key("tab"))
	}
	m = send(t, m, key("enter"))
	if m.currentTaskID() != "n1" || m.focus != paneDetail {
		t.Fatalf("enter: current = %q focus = %d", m.currentTaskID(), m.focus)
	}
	m = cursorTo(t, m, "deep")
	if d := m.buildDetail(50); len(d.crumbs) != 2 || !strings.Contains(stripANSI(d.lines[d.crumbs[0].line]), "alpha › one ›") {
		t.Errorf("crumbs = %+v", d.crumbs)
	}
}

// Delete on a note with sub-notes names their number. A drop at the left edge of a row
// puts a sub-note at the top level.
func TestSubnoteDeleteAndDrop(t *testing.T) {
	m := send(t, cursorTo(t, subnoteModel(t), "alpha"), key("delete"))
	if m.confirm == nil || !strings.Contains(m.confirm.text, "3 sub-notes") {
		t.Fatalf("confirm = %+v", m.confirm)
	}
	m = subnoteModel(t)
	if m.moveNoteTop(m.taskByID("n3"), "t1") == nil || m.taskByID("n3").Parent() != "" {
		t.Errorf("top: rows = %q", rowNames(m))
	}
	if got := rowNames(m); !strings.Contains(got, " deep ") { // undated: after the dated notes
		t.Errorf("rows = %q, want deep at the top level", got)
	}
}
