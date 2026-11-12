package tui

import (
	"strings"
	"testing"

	"github.com/biomassa/godoist/internal/todoist"
)

// checksModel opens the work project in task view, with a markdown description and a
// comment with a checkbox on alpha. The details pane has the focus.
func checksModel(t *testing.T) Model {
	t.Helper()
	m := onRow(t, 1)
	m.chkCur = -1
	m.taskByID("t1").Description = "## Steps\n\n- [ ] one"
	if m.snap.Comments == nil {
		m.snap.Comments = map[string][]todoist.Comment{}
	}
	m.snap.Comments["t1"] = []todoist.Comment{{ID: "c1", TaskID: "t1", Content: "- [ ] reply"}}
	m.buildRows(false)
	m.focusDetail()
	return m
}

// In task view, the description is markdown: the heading has no "##" and the checkbox is ☐.
func TestTaskDescriptionMarkdown(t *testing.T) {
	m := checksModel(t)
	d := stripANSI(strings.Join(m.buildDetail(60).lines, "\n"))
	if strings.Contains(d, "## Steps") || !strings.Contains(d, "Steps") || !strings.Contains(d, "☐ one") {
		t.Errorf("details:\n%s", d)
	}
}

// tab selects the checkboxes of the description and then of the comments. space toggles.
func TestTaskChecksToggle(t *testing.T) {
	m := checksModel(t)
	if n := m.noteChecks(); n != 2 {
		t.Fatalf("checkboxes = %d, want 2", n)
	}
	m = send(t, m, key("tab"), key("space"))
	if got := m.taskByID("t1").Description; !strings.Contains(got, "- [x] one") || m.pending != 1 {
		t.Errorf("description = %q pending = %d", got, m.pending)
	}
	m = send(t, m, key("tab"), key("space"))
	if got := m.snap.Comments["t1"][0].Content; got != "- [x] reply" || m.pending != 2 {
		t.Errorf("comment = %q pending = %d", got, m.pending)
	}
}

// With checkboxes and none selected, space in the task details selects the first
// checkbox and does not complete the task. x still completes it.
func TestTaskDetailSpaceSelectsCheckbox(t *testing.T) {
	m := send(t, checksModel(t), key("space"))
	if m.chkCur != 0 || m.pending != 0 || m.taskByID("t1") == nil {
		t.Fatalf("space: chkCur = %d pending = %d, want the first checkbox and no write", m.chkCur, m.pending)
	}
	m = send(t, checksModel(t), key("x"))
	if m.taskByID("t1") != nil && m.pending == 0 {
		t.Error("x did not complete the task")
	}
}

// A checkbox in fenced code is not a checkbox.
func TestCheckboxLinesSkipFences(t *testing.T) {
	if got := checkboxLines("- [ ] a\n```\n- [ ] code\n```\n- [ ] b"); len(got) != 2 {
		t.Errorf("checkbox lines = %v, want 2", got)
	}
}

// In the details pane, del deletes the selected comment, and with no comment selected
// it asks to delete the task. Both ask first.
func TestDetailDeleteKey(t *testing.T) {
	m := checksModel(t)
	m.comCur = 0
	m = send(t, m, key("delete"))
	if m.confirm == nil || m.confirm.title != "Delete comment" {
		t.Fatalf("del on a comment: confirm = %+v", m.confirm)
	}
	m = checksModel(t)
	m = send(t, m, key("backspace"))
	if m.confirm == nil || m.confirm.title == "Delete comment" {
		t.Fatalf("backspace with no comment: confirm = %+v, want the task delete dialog", m.confirm)
	}
	m = checksModel(t)
	if m = send(t, m, key("d")); m.confirm != nil {
		t.Error("d with no comment selected opened a dialog")
	}
}
