package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/biomassa/godoist/internal/todoist"
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

// withProjects adds the top-level project "home" after work, for sub-project tests.
func withProjects(t *testing.T) Model {
	t.Helper()
	m := testModel(t)
	m.st.Projects = append(m.st.Projects, todoist.Project{ID: "p2", Name: "home", Color: "teal", ChildOrder: 2})
	m.applyState(m.st)
	return send(t, m, click(5, 9)) // home is row 8, under work
}

func TestIndentOutdentProject(t *testing.T) {
	m := withProjects(t)
	if p := m.navProject(); p == nil || p.ID != "p2" {
		t.Fatalf("cursor is not on home: %+v", m.currentNav())
	}
	m = send(t, m, key(">"))
	if p := m.projects["p2"]; p.ParentID == nil || *p.ParentID != "p1" || m.pending != 1 {
		t.Fatalf("home parent = %v pending = %d, want under work", p.ParentID, m.pending)
	}
	if n := m.currentNav(); n == nil || n.projectID != "p2" || n.depth != 1 {
		t.Errorf("cursor = %+v, want home at depth 1", n)
	}
	m = send(t, m, key("<"))
	if p := m.projects["p2"]; p.ParentID != nil {
		t.Errorf("home parent = %v after <, want top level", *p.ParentID)
	}
	if got := m.childProjects(""); len(got) != 2 || got[0] != "p1" || got[1] != "p2" {
		t.Errorf("top-level order = %v, want [p1 p2]", got)
	}
}

func TestParentPicker(t *testing.T) {
	m := send(t, withProjects(t), key("m"))
	if m.pick == nil || m.pick.kind != pickParent {
		t.Fatalf("pick = %+v, want the parent picker", m.pick)
	}
	for _, it := range m.pick.items {
		if it.projectID == "p2" {
			t.Error("the picker lists the project itself")
		}
	}
	m = send(t, m, key("down"), key("enter")) // top level, then work
	if p := m.projects["p2"]; p.ParentID == nil || *p.ParentID != "p1" {
		t.Errorf("home parent = %v, want work", p.ParentID)
	}
	m = send(t, m, click(5, 8), key("m")) // work: home is inside it now
	for _, it := range m.pick.items {
		if it.projectID == "p2" || it.projectID == "p1" {
			t.Errorf("the picker lists %q, which is work or inside work", it.projectID)
		}
	}
}

func TestDragProject(t *testing.T) {
	m := withProjects(t)
	m = send(t, m, motion(6, 9), motion(6, 8), release(6, 8)) // home onto work
	if p := m.projects["p2"]; p.ParentID == nil || *p.ParentID != "p1" {
		t.Fatalf("home parent = %v, want work after the drag", p.ParentID)
	}
	m = send(t, m, click(5, 9), motion(6, 8), motion(6, 7), release(6, 7)) // onto "My Projects"
	if p := m.projects["p2"]; p.ParentID != nil {
		t.Errorf("home parent = %v, want top level", *p.ParentID)
	}
}

func TestSidebarTreeLines(t *testing.T) {
	m := testModel(t)
	w, h := "p1", "p2"
	m.st.Projects = append(m.st.Projects,
		todoist.Project{ID: "p2", Name: "home", ParentID: &w, ChildOrder: 1},
		todoist.Project{ID: "p3", Name: "garden", ParentID: &h, ChildOrder: 1},
		todoist.Project{ID: "p4", Name: "clients", ParentID: &w, ChildOrder: 2},
		todoist.Project{ID: "p5", Name: "misc", ChildOrder: 3},
	)
	m.applyState(m.st)
	got := map[string]string{}
	for _, n := range m.nav {
		if n.inTree {
			got[n.name] = n.tree
		}
	}
	want := map[string]string{"work": "", "home": "  ├ ", "garden": "  │ └ ", "clients": "  └ ", "misc": ""}
	for name, tr := range want {
		if got[name] != tr {
			t.Errorf("%s tree = %q, want %q", name, got[name], tr)
		}
	}
}

// Top-level projects start at the same column as Inbox. The ▾/▸ marker goes after the
// name, and a click on it hides the sub-projects.
func TestSidebarMarkerAfterName(t *testing.T) {
	m := testModel(t)
	w := "p1"
	m.st.Projects = append(m.st.Projects, todoist.Project{ID: "p2", Name: "home", ParentID: &w, ChildOrder: 1})
	m.applyState(m.st)
	l := m.layout()
	lines := m.navLines(l.nav.w-2, 12)
	for _, want := range []string{" ⌂ Inbox", " # work ▾", "   └ # home"} {
		found := false
		for _, ln := range lines {
			found = found || strings.HasPrefix(stripANSI(ln), want)
		}
		if !found {
			t.Errorf("no sidebar line starts with %q", want)
		}
	}
	var work navItem
	for _, n := range m.nav {
		if n.projectID == "p1" {
			work = n
		}
	}
	col := navMarkCol(work, l.nav.w-2)
	m = send(t, m, click(l.nav.x+1+col, 8))
	collapsed := false
	for _, n := range m.nav {
		if n.projectID == "p1" {
			collapsed = n.collapsed
		}
	}
	if !collapsed {
		t.Error("a click on the marker did not collapse work")
	}
}
