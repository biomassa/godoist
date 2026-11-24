package tui

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/biomassa/godoist/internal/todoist"
)

// u after a priority change shows the old priority at once and writes it back.
func TestUndoPriority(t *testing.T) {
	m := onRow(t, 1)
	id := m.currentTaskID()
	old := m.taskByID(id).Priority
	m = send(t, m, key("1"))
	if m.taskByID(id).Priority != 4 {
		t.Fatalf("priority = %d after 1, want 4", m.taskByID(id).Priority)
	}
	pending := m.pending
	m = send(t, m, key("u"))
	if got := m.taskByID(id).Priority; got != old {
		t.Errorf("priority = %d after u, want %d", got, old)
	}
	if m.pending != pending+1 || len(m.undos) != 0 {
		t.Errorf("pending = %d undos = %d, want one write and an empty stack", m.pending, len(m.undos))
	}
	if m = send(t, m, key("u")); m.status != "nothing to undo" {
		t.Errorf("status = %q", m.status)
	}
}

// u after a label change brings the old labels back, also for several tasks at once.
func TestUndoLabels(t *testing.T) {
	m := onRow(t, 1)
	a, b := m.taskByID("t1"), m.taskByID("t2")
	oldA, oldB := slices.Clone(a.Labels), slices.Clone(b.Labels)
	m.rememberTasks(undoLabels, []*todoist.Task{a, b})
	a.Labels, b.Labels = []string{"x"}, []string{"x"}
	m = send(t, m, key("u"))
	if !slices.Equal(m.taskByID("t1").Labels, oldA) || !slices.Equal(m.taskByID("t2").Labels, oldB) {
		t.Errorf("labels = %v %v, want %v %v", m.taskByID("t1").Labels, m.taskByID("t2").Labels, oldA, oldB)
	}
}

// The undo stack keeps the newest maxUndo steps.
func TestUndoStackLimit(t *testing.T) {
	m := onRow(t, 1)
	for range maxUndo + 5 {
		m.rememberIDs(undoPriority, []string{"t1"})
	}
	if len(m.undos) != maxUndo {
		t.Errorf("%d steps, want %d", len(m.undos), maxUndo)
	}
}

// writeBack sends each kind of old value with the right call. A recurring date gets its
// date and its rule back in one Sync command.
func TestWriteBack(t *testing.T) {
	var (
		mu  sync.Mutex
		got []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body := string(b)
		if r.URL.Path == "/sync" {
			form, _ := url.ParseQuery(body)
			body = form.Get("commands")
			// Accept every command: give back "ok" for the uuid in the body.
			i := strings.Index(body, `"uuid":"`) + len(`"uuid":"`)
			uuid := body[i : i+36]
			io.WriteString(w, `{"sync_status":{"`+uuid+`":"ok"}}`)
		} else {
			io.WriteString(w, "{}")
		}
		mu.Lock()
		got = append(got, r.URL.Path+" "+body)
		mu.Unlock()
	}))
	defer srv.Close()
	c := todoist.New("tok")
	c.SetBaseURL(srv.URL)
	ctx := context.Background()
	for _, tc := range []struct {
		kind undoKind
		w    taskWas
		want []string
	}{
		{undoPriority, taskWas{id: "t1", priority: 2}, []string{"/tasks/t1", `"priority":2`}},
		{undoLabels, taskWas{id: "t1"}, []string{"/tasks/t1", `"labels":[]`}},
		{undoDue, taskWas{id: "t1"}, []string{"/tasks/t1", `"due_string":"no date"`}},
		{undoDue, taskWas{id: "t1", due: &todoist.Due{Date: "2026-11-12"}}, []string{"/tasks/t1", `"due_date":"2026-11-12"`}},
		{undoDue, taskWas{id: "t1", due: &todoist.Due{Date: "2026-11-12T09:00:00"}}, []string{"/tasks/t1", `"due_datetime":"2026-11-12T09:00:00"`}},
		{undoDue, taskWas{id: "t1", due: &todoist.Due{Date: "2026-11-12", String: "every day", IsRecurring: true}},
			[]string{"/sync", `"type":"item_update"`, `"date":"2026-11-12"`, `"string":"every day"`}},
		{undoMove, taskWas{id: "t1", projectID: "p2", sectionID: "s2"}, []string{"/tasks/t1/move", `"section_id":"s2"`}},
		{undoMove, taskWas{id: "t1", projectID: "p2", parentID: "t0"}, []string{"/tasks/t1/move", `"parent_id":"t0"`}},
	} {
		got = nil
		if err := writeBack(ctx, c, tc.kind, tc.w); err != nil {
			t.Fatalf("%+v: %v", tc.w, err)
		}
		if len(got) != 1 {
			t.Fatalf("%+v: %d requests, want 1", tc.w, len(got))
		}
		for _, w := range tc.want {
			if !strings.Contains(got[0], w) {
				t.Errorf("kind %d %+v: request %q has no %q", tc.kind, tc.w, got[0], w)
			}
		}
	}
}

// The date dialog puts an undo step on the stack only when the save worked.
func TestUndoDateOnlyAfterSave(t *testing.T) {
	m := openCal(t, 0)
	nm, cmd := m.calSave(func(context.Context) (string, error) { return "", errors.New("no") })
	m = send(t, nm.(Model), cmd())
	if len(m.undos) != 0 || m.cal == nil {
		t.Fatalf("undos = %d after a failed save, want none and the dialog open", len(m.undos))
	}
	nm, cmd = m.calSave(func(context.Context) (string, error) { return "Due → today", nil })
	m = send(t, nm.(Model), cmd())
	if len(m.undos) != 1 || m.undos[0].kind != undoDue {
		t.Errorf("undos = %+v after a save, want one date step", m.undos)
	}
}

// u for a task that is gone sends nothing.
func TestUndoGoneTask(t *testing.T) {
	m := send(t, onRow(t, 1), key("1"))
	m.removeTask(m.undos[0].was[0].id)
	pending := m.pending
	m = send(t, m, key("u"))
	if m.pending != pending || !strings.Contains(m.status, "gone") {
		t.Errorf("pending = %d status = %q, want no write", m.pending, m.status)
	}
}
