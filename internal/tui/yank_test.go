package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// y copies the name in the task list, the description in the details pane, and the
// selected comment when a comment is selected.
func TestYank(t *testing.T) {
	m := onRow(t, 1)
	m.taskByID("t1").Description = "the body"
	m.snap.Comments = map[string][]todoist.Comment{"t1": {{ID: "c1", TaskID: "t1", Content: "a comment"}}}
	m.buildRows(false)
	nm, cmd := m.Update(key("y"))
	m = nm.(Model)
	if cmd == nil || !strings.Contains(m.status, "copied the name") {
		t.Fatalf("task list: cmd = %v status = %q", cmd != nil, m.status)
	}
	m.focusDetail()
	m.comCur = -1
	nm, cmd = m.Update(key("y"))
	m = nm.(Model)
	if cmd == nil || !strings.Contains(m.status, "copied the description") {
		t.Errorf("details: status = %q", m.status)
	}
	m.comCur = 0
	nm, cmd = m.Update(key("y"))
	m = nm.(Model)
	if cmd == nil || !strings.Contains(m.status, "copied the comment") {
		t.Errorf("comment: status = %q", m.status)
	}
	m.comCur = -1
	m.taskByID("t1").Description = ""
	m.buildRows(false)
	m = send(t, m, key("y"))
	if !strings.Contains(m.status, "empty") {
		t.Errorf("empty description: status = %q", m.status)
	}
}

// enter on "+ create" in the label picker makes the label and puts it on the task, also
// in notebook view. Note rows show their labels.
func TestCreateLabelWithEnter(t *testing.T) {
	m := notesModel(t)
	m = send(t, m, key("@"))
	m = typeText(t, m, "newlab")
	m = send(t, m, key("enter"))
	if m.pending != 1 || !strings.Contains(strings.Join(m.taskByID("t1").Labels, " "), "newlab") {
		t.Fatalf("pending = %d labels = %v status = %q", m.pending, m.taskByID("t1").Labels, m.status)
	}
	var line string
	for i, r := range m.rows {
		if r.task != nil && r.task.ID == "t1" && r.preview == "" {
			_ = i
			line = stripANSI(m.noteLine(r, 80, lipgloss.NewStyle(), false))
		}
	}
	if !strings.Contains(line, "@home") || !strings.Contains(line, "@newlab") {
		t.Errorf("note row = %q, want its labels", line)
	}
	if d := stripANSI(strings.Join(m.buildDetail(60).lines, "\n")); !strings.Contains(d, "@home @newlab") {
		t.Errorf("reader meta has no labels:\n%s", d)
	}
}

// pgup and pgdown in the sidebar go to the top and the bottom.
func TestSidebarPaging(t *testing.T) {
	m := testModel(t)
	m.focus = paneNav
	m = send(t, m, key("pgdown"))
	last := m.navCur
	m = send(t, m, key("pgup"))
	if m.navCur >= last || m.nav[last].name != "Completed" {
		t.Errorf("pgdown → %d (%s), pgup → %d", last, m.nav[last].name, m.navCur)
	}
}
