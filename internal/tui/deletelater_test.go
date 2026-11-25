package tui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// deleteServer is a Todoist that records the requests and answers each with status.
func deleteServer(t *testing.T, status int) (*todoist.Client, func() []string) {
	t.Helper()
	var (
		mu  sync.Mutex
		got []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	c := todoist.New("tok")
	c.SetBaseURL(srv.URL)
	return c, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

// runCmd runs a command and gives its message, as Bubble Tea would.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command")
	}
	return cmd()
}

// A delete hides the task at once but sends nothing. u in the wait brings the task back,
// and the end of the wait then sends nothing.
func TestDeleteUndoInTime(t *testing.T) {
	m := send(t, onRow(t, 1), key("delete"))
	if m.confirm == nil || !strings.Contains(m.confirm.text, "u undoes it") {
		t.Fatalf("confirm = %+v, want the undo hint", m.confirm)
	}
	m = send(t, m, key("enter")) // Delete stays the default button
	if m.taskByID("t1") != nil || m.pending != 0 || !strings.Contains(m.status, "u to undo") {
		t.Fatalf("task shown = %v pending = %d status = %q, want it hidden and no write yet", m.taskByID("t1") != nil, m.pending, m.status)
	}
	n := m.laterN
	m = send(t, m, key("u"))
	if m.taskByID("t1") == nil || len(m.laterDeletes) != 0 || !strings.Contains(m.status, "Restored") {
		t.Fatalf("task shown = %v waiting = %d status = %q, want it back", m.taskByID("t1") != nil, len(m.laterDeletes), m.status)
	}
	m = send(t, m, deleteNowMsg{n: n})
	if m.pending != 0 {
		t.Errorf("pending = %d after an undone delete's wait, want no write", m.pending)
	}
}

// At the end of the wait the delete goes to Todoist. A sync that started before that
// does not bring the task back, and u then says that it is too late.
func TestDeleteSendsAfterWait(t *testing.T) {
	c, got := deleteServer(t, http.StatusNoContent)
	m := onRow(t, 1)
	m.client = c
	m = send(t, m, key("delete"), key("enter"))
	old := m.st // a sync that has not seen the delete yet
	m.syncSeq = 3
	m = send(t, m, syncMsg{st: old.Clone(), seq: 3})
	if m.taskByID("t1") != nil {
		t.Fatal("a sync in the wait brought the task back")
	}
	nm, cmd := m.Update(deleteNowMsg{n: m.laterN})
	m = nm.(Model)
	if m.pending != 1 {
		t.Fatalf("pending = %d, want the delete sent", m.pending)
	}
	m = send(t, m, runCmd(t, cmd))
	if r := got(); len(r) != 1 || r[0] != "DELETE /tasks/t1" {
		t.Fatalf("requests = %v, want one DELETE of t1", r)
	}
	if m.pending != 0 || m.taskByID("t1") != nil {
		t.Fatalf("pending = %d shown = %v, want the delete done and the task hidden", m.pending, m.taskByID("t1") != nil)
	}
	m = send(t, m, syncMsg{st: old.Clone(), seq: 3}) // late, from before the delete
	if m.taskByID("t1") != nil {
		t.Error("an old sync brought the deleted task back")
	}
	m = send(t, m, key("u")) // the delete went: u says so, and the step leaves the stack
	if !strings.HasPrefix(m.status, "too late") || len(m.undos) != 0 {
		t.Errorf("status = %q undos = %d, want too late and an empty stack", m.status, len(m.undos))
	}
	var fresh todoist.SyncState = old.Clone()
	fresh.Tasks = fresh.Tasks[1:] // Todoist no longer has t1
	m = send(t, m, syncMsg{st: fresh, seq: 4})
	if len(m.laterDeletes) != 0 {
		t.Errorf("waiting deletes = %d after a newer sync, want none", len(m.laterDeletes))
	}
}

// If Todoist refuses the delete, the task shows again with an error.
func TestDeleteFailsShowsAgain(t *testing.T) {
	c, _ := deleteServer(t, http.StatusForbidden)
	m := onRow(t, 1)
	m.client = c
	m = send(t, m, key("delete"), key("enter"))
	nm, cmd := m.Update(deleteNowMsg{n: m.laterN})
	m = send(t, nm.(Model), runCmd(t, cmd))
	if m.taskByID("t1") == nil || !m.statusErr || !strings.Contains(m.status, "could not delete") {
		t.Errorf("shown = %v status = %q, want the task back and an error", m.taskByID("t1") != nil, m.status)
	}
	if len(m.undos) != 0 { // the task is back, so u has nothing to undo for this delete
		t.Errorf("undos = %d after a failed delete, want none", len(m.undos))
	}
}

// q does not wait for the end of the wait: it sends the waiting deletes and quits after.
func TestQuitSendsWaitingDeletes(t *testing.T) {
	c, got := deleteServer(t, http.StatusNoContent)
	m := onRow(t, 1)
	m.client = c
	m = send(t, m, key("delete"), key("enter"))
	nm, cmd := m.Update(key("q"))
	m = nm.(Model)
	if !m.quitting || m.pending != 1 {
		t.Fatalf("quitting = %v pending = %d, want the delete sent before quit", m.quitting, m.pending)
	}
	_, cmd = m.Update(runCmd(t, cmd))
	if r := got(); len(r) != 1 {
		t.Errorf("requests = %v, want the delete", r)
	}
	if _, ok := runCmd(t, cmd).(tea.QuitMsg); !ok {
		t.Error("godoist did not quit after the delete went")
	}
}

// A project delete hides its sub-projects, sections and tasks; a comment delete hides the
// comment.
func TestDeleteHidesWhatGoesWith(t *testing.T) {
	m := testModel(t)
	parent := "p1"
	m.st.Projects = append(m.st.Projects, todoist.Project{ID: "p2", Name: "sub", ParentID: &parent})
	m.st.Tasks = append(m.st.Tasks, todoist.Task{ID: "t4", ProjectID: "p2", Content: "delta"})
	m.applyState(m.st)
	noop := func(context.Context) error { return nil }
	m.deleteLater("project", []hiddenItem{{hideProject, "p1"}}, noop)
	if m.projects["p1"] != nil || m.projects["p2"] != nil || m.sections["s1"] != nil {
		t.Error("the project, its sub-project or its section is still shown")
	}
	for _, id := range []string{"t1", "t3", "t4"} {
		if m.taskByID(id) != nil {
			t.Errorf("task %s is still shown", id)
		}
	}
	m = send(t, m, key("u"))
	if m.projects["p2"] == nil || m.taskByID("t4") == nil {
		t.Error("u did not bring the sub-project back")
	}
	m.st.Comments = []todoist.Comment{{ID: "c1", TaskID: "t1", Content: "x"}, {ID: "c2", TaskID: "t1", Content: "y"}}
	m.applyState(m.st)
	m.deleteLater("the comment", []hiddenItem{{hideComment, "c1"}}, noop)
	if cs := m.snap.Comments["t1"]; len(cs) != 1 || cs[0].ID != "c2" {
		t.Errorf("comments = %+v, want only c2", cs)
	}
}

// u while the delete is on its way says that it is too late.
func TestDeleteUndoInFlight(t *testing.T) {
	m := send(t, onRow(t, 1), key("delete"), key("enter"))
	m = send(t, m, deleteNowMsg{n: m.laterN}, key("u"))
	if !strings.Contains(m.status, "too late") || m.taskByID("t1") != nil {
		t.Errorf("status = %q, want too late and the task still hidden", m.status)
	}
}

// When a delete went, u undoes the change before it, not the delete.
func TestDeleteStepLeavesStack(t *testing.T) {
	c, _ := deleteServer(t, http.StatusNoContent)
	m := onRow(t, 2)
	m.client = c
	m = send(t, m, key("1"), key("k"), key("delete"), key("enter"))
	if m.taskByID("t1") != nil {
		t.Fatal("alpha was not deleted")
	}
	nm, cmd := m.Update(deleteNowMsg{n: m.laterN})
	m = send(t, nm.(Model), runCmd(t, cmd))
	m = send(t, m, key("u")) // a late u is for the delete: it must not undo the priority
	if p := m.taskByID("t2").Priority; p == 1 || !strings.HasPrefix(m.status, "too late") {
		t.Fatalf("priority = %d status = %q, want too late and the priority kept", p, m.status)
	}
	m = send(t, m, key("u"))
	if p := m.taskByID("t2").Priority; p != 1 || len(m.undos) != 0 {
		t.Errorf("priority = %d undos = %d, want the priority change undone", p, len(m.undos))
	}
}

// If the delete that q sends fails, godoist stays open and shows the error.
func TestQuitDeleteFails(t *testing.T) {
	c, _ := deleteServer(t, http.StatusForbidden)
	m := onRow(t, 1)
	m.client = c
	m = send(t, m, key("delete"), key("enter"))
	nm, cmd := m.Update(key("q"))
	nm, cmd = nm.(Model).Update(runCmd(t, cmd))
	m = nm.(Model)
	if m.quitting || !strings.Contains(m.status, "could not delete") || m.taskByID("t1") == nil {
		t.Fatalf("quitting = %v status = %q, want godoist open with the error", m.quitting, m.status)
	}
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Error("godoist quit after a failed delete")
		}
	}
}

// A filter result in the wait does not bring the task back: Todoist still has it.
func TestDeleteHiddenInFilter(t *testing.T) {
	m := send(t, onRow(t, 1), key("delete"), key("enter"))
	m = send(t, m, filterMsg{query: "all", tasks: m.st.Tasks})
	for _, task := range m.filterTasks {
		if task.ID == "t1" {
			t.Fatal("the filter result shows the deleted task")
		}
	}
}

// A sync in the wait runs the filter again: the sub-tasks of the deleted task stay out of
// its result too, because Todoist deletes them with the task.
func TestDeleteHiddenInFilterSubtask(t *testing.T) {
	m := onRow(t, 1)
	parent := "t1"
	m.st.Tasks = append(m.st.Tasks, todoist.Task{ID: "t9", ProjectID: "p1", ParentID: &parent, Content: "child"})
	m.applyState(m.st)
	m = send(t, m, key("delete"), key("enter"))
	m = send(t, m, filterMsg{query: "all", tasks: m.st.Tasks})
	for _, task := range m.filterTasks {
		if task.ID == "t1" || task.ID == "t9" {
			t.Fatalf("the filter result shows %s of the deleted task", task.ID)
		}
	}
}

// u after a comment delete selects the comment again, not the one above it.
func TestDeleteCommentUndoCursor(t *testing.T) {
	m := onRow(t, 1)
	m.st.Comments = []todoist.Comment{{ID: "c1", TaskID: "t1", Content: "x"}, {ID: "c2", TaskID: "t1", Content: "y"}}
	m.applyState(m.st)
	m.focus, m.comCur = paneDetail, 0
	m = send(t, m, key("d"), key("enter"))
	if m.comCur != -1 {
		t.Fatalf("comCur = %d after the delete, want -1", m.comCur)
	}
	m = send(t, m, key("u"))
	if m.comCur != 0 {
		t.Errorf("comCur = %d after u, want 0 (the restored comment)", m.comCur)
	}
}

// u after the delete of the open project opens the project again.
func TestDeleteProjectUndoReturns(t *testing.T) {
	m := openWork(t)
	m = send(t, m, key("esc")) // the sidebar
	m.focus = paneNav
	m.askDeleteProject(m.projects["p1"])
	m = send(t, m, key("enter"))
	if cur := m.currentNav(); cur != nil && cur.projectID == "p1" {
		t.Fatal("the deleted project is still open")
	}
	m = send(t, m, key("u"))
	if cur := m.currentNav(); cur == nil || cur.projectID != "p1" {
		t.Errorf("nav = %+v after u, want the work project", cur)
	}
}

// A quick add that names a #project or a /section with a waiting delete is refused, and
// the dialog stays open with the text: Todoist still has them and would put the task in
// them. After u brings the project back, the same text goes.
func TestQuickAddWaitingDelete(t *testing.T) {
	m := openWork(t)
	m.focus = paneNav
	m.askDeleteProject(m.projects["p1"])
	m = send(t, m, key("enter"))
	m = send(t, m, key("g"), key("i"), key("a"))
	m = typeText(t, m, "milk #work")
	m = send(t, m, key("enter"))
	if m.inputMode != inputAdd || m.pending != 0 || !m.statusErr || m.status != "#work is being deleted: pick another project" {
		t.Fatalf("dialog %v pending %d status %q, want the add refused and the dialog open", m.inputMode, m.pending, m.status)
	}
	if !strings.Contains(m.dialogValue(), "milk #work") {
		t.Errorf("dialog text %q, want the typed text kept", m.dialogValue())
	}
	m = send(t, m, key("esc"), key("u"), key("g"), key("i"), key("a"))
	m = typeText(t, m, "#work milk") // the tag first: the autocomplete of #work is closed at enter
	m = send(t, m, key("enter"))
	if m.inputMode == inputAdd || m.pending != 1 {
		t.Errorf("dialog %v pending %d after u, want the add sent", m.inputMode, m.pending)
	}

	// A /section of the open project with a waiting delete.
	m = openWork(t)
	m.deleteLater("section “urgent”", []hiddenItem{{hideSection, "s1"}}, nil)
	if got := m.waitingDeleteIn("milk /urgent"); got != "/urgent" {
		t.Errorf("waitingDeleteIn(/urgent) = %q", got)
	}
	m = send(t, m, key("a"))
	m = typeText(t, m, "milk /urgent")
	if m = send(t, m, key("enter")); m.inputMode != inputAdd || m.status != "/urgent is being deleted: pick another section" {
		t.Errorf("dialog %v status %q, want the add refused for the section", m.inputMode, m.status)
	}
	if got := m.waitingDeleteIn("milk /later"); got != "" {
		t.Errorf("waitingDeleteIn(/later) = %q, want nothing: the section stays", got)
	}
}
