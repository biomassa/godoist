package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func stripANSI(s string) string { return ansi.Strip(s) }

// onWorkNav puts the sidebar cursor on the work project (row 7).
func onWorkNav(t *testing.T) Model {
	t.Helper()
	m := openWork(t)
	if m.navProject() == nil || m.navProject().ID != "p1" || m.focus != paneNav {
		t.Fatalf("sidebar cursor is not on work")
	}
	return m
}

func TestNewProjectFlow(t *testing.T) {
	m := send(t, onWorkNav(t), key("A"))
	if m.inputMode != inputAddProject {
		t.Fatalf("inputMode = %d, want the new project dialog", m.inputMode)
	}
	m = send(t, m, key("R"), key("d"), key("enter"))
	if m.pick == nil || m.pick.kind != pickColor || m.pick.newName != "Rd" {
		t.Fatalf("pick = %+v, want the color picker for Rd", m.pick)
	}
	if !m.pick.items[0].toggleRow || m.pick.items[0].projectID != "p1" {
		t.Fatalf("first row = %+v, want the sub-project row for work", m.pick.items[0])
	}
	m = send(t, m, key("space"))
	if !m.pick.items[0].checked {
		t.Error("space did not check the sub-project row")
	}
	if vis := m.pick.visible(); vis[m.pick.cur].colorName != "charcoal" {
		t.Errorf("cursor color = %q, want charcoal", vis[m.pick.cur].colorName)
	}
	m = send(t, m, key("enter"))
	if m.pick != nil || m.pending != 1 {
		t.Errorf("pick = %v pending = %d, want one write", m.pick, m.pending)
	}
}

func TestFailedCreateReopensDialog(t *testing.T) {
	m := onWorkNav(t)
	m.pending = 1
	m = send(t, m, actionMsg{err: errors.New("Todoist: Maximum number of projects reached (HTTP 403)"),
		retryMode: inputAddProject, retryText: "Rd"})
	if m.inputMode != inputAddProject || m.dialogValue() != "Rd" || !m.statusErr {
		t.Errorf("inputMode = %d value = %q err = %v, want the dialog again with the name", m.inputMode, m.dialogValue(), m.statusErr)
	}
}

func TestRenameAndFavoriteProject(t *testing.T) {
	m := send(t, onWorkNav(t), key("e"))
	if m.inputMode != inputRenameProject || m.dialogValue() != "work" {
		t.Fatalf("inputMode = %d value = %q", m.inputMode, m.dialogValue())
	}
	m = send(t, m, key("esc"), key("*"))
	if !m.projects["p1"].IsFavorite || m.pending != 1 {
		t.Fatalf("favorite = %v pending = %d", m.projects["p1"].IsFavorite, m.pending)
	}
	found := false
	for _, n := range m.nav {
		if n.header == "Favorites" {
			found = true
		}
	}
	if !found {
		t.Error("no Favorites group after *")
	}
	if m.navProject() == nil || m.navProject().ID != "p1" {
		t.Error("the cursor left the project after *")
	}
}

func TestMoveProjectEdge(t *testing.T) {
	m := send(t, onWorkNav(t), key("["))
	if m.pending != 0 || !strings.Contains(m.status, "edge") {
		t.Errorf("pending = %d status = %q, want an edge message", m.pending, m.status)
	}
}

func TestDeleteOrArchiveProject(t *testing.T) {
	m := send(t, onWorkNav(t), key("delete"))
	if m.confirm == nil || len(m.confirm.buttons) != 3 || !strings.Contains(m.confirm.text, "3 task(s)") {
		t.Fatalf("confirm = %+v", m.confirm)
	}
	if m.confirm.sel != 0 {
		t.Fatalf("sel = %d, want the first button (Delete) selected", m.confirm.sel)
	}
	r := m.confirmRect()
	_, hits := m.confirmLayout()
	archive := hits[1]
	m = send(t, m, click(r.x+archive.x+1, r.y+archive.y))
	if m.confirm != nil || m.pending != 1 {
		t.Errorf("confirm = %v pending = %d, want archive to run", m.confirm, m.pending)
	}
}

func TestInboxIsProtected(t *testing.T) {
	m := send(t, testModel(t), click(5, 2)) // Inbox
	m = send(t, m, key("e"))
	if m.inputMode != inputNone || !m.statusErr {
		t.Errorf("inputMode = %d, want an error for the Inbox", m.inputMode)
	}
}

func TestProjectMenu(t *testing.T) {
	m := send(t, testModel(t), rightClick(5, 8))
	if m.menu == nil || m.menu.items[0].key != "A" || m.menu.focus != paneNav {
		t.Fatalf("menu = %+v, want the project menu", m.menu)
	}
	r := m.menuRect()
	m = send(t, m, click(r.x+3, r.y+1+2)) // Color
	if m.pick == nil || m.pick.kind != pickColor || m.pick.projectID != "p1" {
		t.Errorf("pick = %+v, want the color picker for work", m.pick)
	}
}

func TestAllTasksView(t *testing.T) {
	m := send(t, testModel(t), click(5, 13))
	if cur := m.currentNav(); cur == nil || cur.kind != vkAll {
		t.Fatalf("click did not open All tasks: %+v", cur)
	}
	var order []string
	for _, r := range m.rows {
		switch {
		case r.header != "":
			order = append(order, "["+r.header+"]")
		case r.task != nil:
			order = append(order, r.task.Content)
		}
	}
	if got := strings.Join(order, " "); got != "[# work] beta alpha gamma" {
		t.Errorf("rows = %q, want dated first, then undated", got)
	}
}

func TestSidebarOrder(t *testing.T) {
	m := testModel(t)
	var got []string
	for _, n := range m.nav {
		if n.header == "" {
			got = append(got, n.name)
		}
	}
	if s := strings.Join(got, ","); s != "Inbox,Today,Upcoming,work,home,All tasks,Completed" {
		t.Errorf("sidebar = %q", s)
	}
	if m.nav[0].header != " " {
		t.Error("no empty line at the top of the sidebar")
	}
	if d := m.buildDetail(40); len(d.lines) == 0 || d.lines[0] != "" {
		t.Error("no empty line at the top of the details pane")
	}
}

func TestOverflowKeysStartAtColumnOne(t *testing.T) {
	m := openWork(t)
	m.width = 80
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 2), release(x, 2))
	m.setStatus("Renamed a task", false)
	bar, _ := m.bottomBarLayout()
	line2 := strings.Split(bar, "\n")[1]
	plainLine := stripANSI(line2)
	if !strings.HasPrefix(plainLine, " ") || strings.HasPrefix(plainLine, "  ") {
		t.Errorf("line 2 = %q, want the keys at column 1", plainLine)
	}
	if strings.Index(plainLine, "Renamed") < strings.Index(plainLine, "·") {
		t.Errorf("line 2 = %q, want the keys before the status", plainLine)
	}
}

func TestConfirmKeys(t *testing.T) {
	m := send(t, onWorkNav(t), key("delete"))
	m = send(t, m, key("right"), key("tab"), key("l"))
	if m.confirm.sel != 0 { // three moves right wrap to the first button
		t.Fatalf("sel = %d after three moves, want 0", m.confirm.sel)
	}
	m = send(t, m, key("left"))
	if m.confirm.sel != 2 {
		t.Fatalf("sel = %d after left, want 2 (Cancel)", m.confirm.sel)
	}
	m = send(t, m, key("enter")) // Cancel
	if m.confirm != nil || m.pending != 0 {
		t.Fatalf("confirm = %v pending = %d, want Cancel to close without a write", m.confirm, m.pending)
	}
	m = send(t, m, key("delete"), key("h"), key("h"), key("enter")) // Archive
	if m.pending != 1 || !strings.Contains(m.status, "") {
		t.Errorf("pending = %d, want Archive to run", m.pending)
	}
	m = send(t, m, key("delete"), key("esc"))
	if m.confirm != nil {
		t.Error("esc did not close the dialog")
	}
	m = send(t, m, key("delete"), key("enter")) // the default is Delete
	if m.pending != 2 {
		t.Errorf("pending = %d, want enter to push Delete", m.pending)
	}
}
