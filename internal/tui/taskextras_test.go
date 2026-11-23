package tui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/biomassa/godoist/internal/todoist"
)

// The web address of a task is the Todoist app address with the task ID.
func TestTaskURL(t *testing.T) {
	if got := taskURL("6X7rM8997g3RQmvh"); got != "https://app.todoist.com/app/task/6X7rM8997g3RQmvh" {
		t.Errorf("taskURL = %q", got)
	}
}

// Y copies the link of the task under the cursor.
func TestYankURL(t *testing.T) {
	m := onRow(t, 1)
	nm, cmd := m.yankURL()
	if cmd == nil || nm.(Model).status != "copied the Todoist link" {
		t.Errorf("cmd = %v status = %q", cmd != nil, nm.(Model).status)
	}
}

// A task added offline has a temporary ID and no web address yet: Y and O say so and
// copy or open nothing.
func TestYankURLOffline(t *testing.T) {
	m := onRow(t, 1)
	m.currentTask().ID = "tmp-1"
	nm, cmd := m.yankURL()
	if cmd != nil || !strings.Contains(nm.(Model).status, "not in Todoist yet") {
		t.Errorf("cmd = %v status = %q, want no copy and the hint", cmd != nil, nm.(Model).status)
	}
}

// ctrl+a selects all open tasks of the list, and a second ctrl+a clears the selection.
func TestSelectAll(t *testing.T) {
	m := send(t, onRow(t, 1), key("ctrl+a"))
	n := 0
	for _, r := range m.rows {
		if r.task != nil && !r.done && !r.doneSub {
			n++
		}
	}
	if n == 0 || len(m.selectedTasks()) != n {
		t.Fatalf("%d selected, want all %d tasks", len(m.selectedTasks()), n)
	}
	if m = send(t, m, key("ctrl+a")); len(m.selectedTasks()) != 0 {
		t.Errorf("%d selected after the second ctrl+a, want 0", len(m.selectedTasks()))
	}
}

// D adds a copy of the task under the cursor: an add with the name, project and section,
// then one update with the other fields. The date goes as a date, not as its text, so
// that a text like "tomorrow" does not move the copy.
func TestDuplicate(t *testing.T) {
	var (
		mu  sync.Mutex
		got []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, r.Method+" "+r.URL.Path+" "+string(b))
		mu.Unlock()
		io.WriteString(w, `{"id":"new"}`)
	}))
	defer srv.Close()
	m := onRow(t, 1)
	src := m.currentTask()
	src.Description, src.Priority, src.Labels = "notes", 3, []string{"home"}
	m.client = todoist.New("tok")
	m.client.SetBaseURL(srv.URL)
	nm, cmd := m.Update(key("D"))
	m = nm.(Model)
	if m.pending != 1 || cmd == nil {
		t.Fatalf("pending = %d, want one write", m.pending)
	}
	if msg, _ := cmd().(actionMsg); msg.err != nil || !strings.HasPrefix(msg.text, "Copied") {
		t.Fatalf("result = %+v", msg)
	}
	if len(got) != 2 {
		t.Fatalf("requests = %q, want an add and an update", got)
	}
	if !strings.HasPrefix(got[0], "POST /tasks ") || !strings.Contains(got[0], `"content":"`+src.Content+`"`) {
		t.Errorf("add = %q", got[0])
	}
	for _, want := range []string{"POST /tasks/new ", `"priority":3`, `"description":"notes"`, `"labels":["home"]`, `"due_date":"2029-12-31"`} {
		if !strings.Contains(got[1], want) {
			t.Errorf("update %q has no %q", got[1], want)
		}
	}
	if strings.Contains(got[1], "due_string") {
		t.Errorf("update %q sends the date as text", got[1])
	}
}

// In the notebook view, ctrl+a counts each note once, not its preview lines too.
func TestSelectAllNotebook(t *testing.T) {
	m := send(t, notesModel(t), key("ctrl+a"))
	if n := len(m.selectedTasks()); n == 0 || m.status != plural(n, "task")+" selected" {
		t.Errorf("%d selected, status %q", n, m.status)
	}
}

// D on a recurring sub-task keeps the rule and the current date (one more command after the
// update), and puts the copy under the same parent task.
func TestDuplicateRecurringSubtask(t *testing.T) {
	var (
		mu  sync.Mutex
		got []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, r.Method+" "+r.URL.Path+" "+string(b))
		mu.Unlock()
		if r.URL.Path == "/sync" { // accept the command: its uuid is in the form value
			var cmds []struct{ UUID string }
			form, _ := url.ParseQuery(string(b))
			json.Unmarshal([]byte(form.Get("commands")), &cmds)
			io.WriteString(w, `{"sync_status":{"`+cmds[0].UUID+`":"ok"}}`)
			return
		}
		io.WriteString(w, `{"id":"new"}`)
	}))
	defer srv.Close()
	m := onRow(t, 1)
	src := m.currentTask()
	parent := "p-task"
	src.ParentID = &parent
	src.Due = &todoist.Due{Date: "2029-12-31", String: "every day", Lang: "en", IsRecurring: true}
	m.client = todoist.New("tok")
	m.client.SetBaseURL(srv.URL)
	nm, cmd := m.Update(key("D"))
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("no write")
	}
	if msg, _ := cmd().(actionMsg); msg.err != nil || !strings.HasPrefix(msg.text, "Copied") {
		t.Fatalf("result = %+v %v requests %q", msg, msg.err, got)
	}
	all, _ := url.QueryUnescape(strings.Join(got, "\n"))
	if strings.Contains(got[1], "due_date") || strings.Contains(got[1], "due_string") {
		t.Errorf("update %q sends the date, want it in its own command", got[1])
	}
	for _, want := range []string{"every day", "2029-12-31", parent} {
		if !strings.Contains(all, want) {
			t.Errorf("requests %q have no %q", got, want)
		}
	}
}
