package todoist

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTodoist is a small fake of the Todoist API. It records the requests, answers quick
// adds with real IDs, and accepts all Sync commands (or refuses the types in refuse).
type fakeTodoist struct {
	mu       sync.Mutex
	requests []string // "METHOD path"
	commands [][]map[string]any
	refuse   map[string]bool
	nextID   int
}

func (f *fakeTodoist) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	switch {
	case r.URL.Path == "/tasks/quick":
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.nextID++
		_ = json.NewEncoder(w).Encode(Task{ID: "real" + string(rune('0'+f.nextID)), Content: body["text"], ProjectID: "inbox"})
	case r.URL.Path == "/sync":
		b, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(b))
		resp := map[string]any{"sync_token": "t1", "full_sync": true, "sync_status": map[string]any{}, "temp_id_mapping": map[string]string{}}
		if c := form.Get("commands"); c != "" {
			var cmds []map[string]any
			_ = json.Unmarshal([]byte(c), &cmds)
			f.commands = append(f.commands, cmds)
			for _, cmd := range cmds {
				uuid, _ := cmd["uuid"].(string)
				if f.refuse[cmd["type"].(string)] {
					resp["sync_status"].(map[string]any)[uuid] = map[string]any{"error": "refused"}
					continue
				}
				resp["sync_status"].(map[string]any)[uuid] = "ok"
				if tmp, _ := cmd["temp_id"].(string); tmp != "" {
					f.nextID++
					resp["temp_id_mapping"].(map[string]string)[tmp] = "real" + string(rune('0'+f.nextID))
				}
			}
		}
		_ = json.NewEncoder(w).Encode(resp)
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "x"})
	}
}

// offlineURL is an address where no server listens, so that a request fails to dial.
func offlineURL(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return "http://" + addr
}

func queueClient(t *testing.T) (*Client, *Queue, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "queue.json")
	q, err := OpenQueue(path)
	if err != nil {
		t.Fatal(err)
	}
	c := New("test")
	c.SetQueue(q)
	c.base = offlineURL(t)
	return c, q, path
}

// Offline, task changes go into the queue in order, and the queue stays after a restart.
func TestQueueOffline(t *testing.T) {
	c, q, path := queueClient(t)
	ctx := WithQueuedFlag(context.Background())
	nt, err := c.QuickAdd(ctx, "milk tomorrow")
	if err != nil || !IsTempID(nt.ID) || nt.Content != "milk tomorrow" {
		t.Fatalf("QuickAdd offline: %+v %v", nt, err)
	}
	if err := c.Move(ctx, nt.ID, "p1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpdateTask(ctx, "t1", map[string]any{"due_string": "no date", "priority": 4}); err != nil {
		t.Fatal(err)
	}
	if !Queued(ctx) || q.Len() != 3 {
		t.Fatalf("queued = %v len = %d", Queued(ctx), q.Len())
	}
	q2, err := OpenQueue(path)
	if err != nil || q2.Len() != 3 {
		t.Fatalf("after a restart: len = %d err = %v", q2.Len(), err)
	}
	ops := q2.Ops()
	if ops[0].Kind != OpQuick || ops[1].Type != "item_move" || ops[2].Type != "item_update" {
		t.Errorf("order = %s %s %s", ops[0].Kind, ops[1].Type, ops[2].Type)
	}
	if due, ok := ops[2].Args["due"]; !ok || due != nil {
		t.Errorf("no date: due = %v, want null", ops[2].Args["due"])
	}
}

// The next sync sends the queue in order: the quick add by REST, then the commands with
// the real ID in place of the temporary ID.
func TestQueueFlush(t *testing.T) {
	c, q, _ := queueClient(t)
	ctx := context.Background()
	nt, _ := c.QuickAdd(ctx, "milk")
	_ = c.Move(ctx, nt.ID, "p1", "")
	_ = c.Close(ctx, "t9")
	f := &fakeTodoist{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c.base = srv.URL
	var st SyncState
	if err := c.Sync(ctx, &st); err != nil {
		t.Fatal(err)
	}
	if q.Len() != 0 {
		t.Fatalf("queue len = %d after the sync", q.Len())
	}
	if len(f.commands) < 1 || len(f.commands[0]) != 2 {
		t.Fatalf("commands = %v", f.commands)
	}
	mv := f.commands[0][0]
	if mv["type"] != "item_move" || mv["args"].(map[string]any)["id"] != "real1" {
		t.Errorf("move = %v, want the real ID", mv)
	}
	if f.requests[0] != "POST /tasks/quick" {
		t.Errorf("first request = %s", f.requests[0])
	}
	// A change after the flush that uses the old temporary ID gets the real ID too.
	c.base = offlineURL(t)
	_, _ = c.UpdateTask(ctx, "t2", map[string]any{"content": "x"}) // the queue is empty: REST fails, then queued
	_ = c.Delete(ctx, nt.ID)
	if ops := q.Ops(); len(ops) != 2 || ops[1].Args["id"] != "real1" {
		t.Errorf("ops = %+v", ops)
	}
}

// A refused command goes out of the queue, the error is kept, and the next read is full.
func TestQueueRefused(t *testing.T) {
	c, q, _ := queueClient(t)
	ctx := context.Background()
	_ = c.Delete(ctx, "gone")
	_ = c.Close(ctx, "t1")
	f := &fakeTodoist{refuse: map[string]bool{"item_delete": true}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c.base = srv.URL
	st := SyncState{Token: "old"}
	if err := c.Sync(ctx, &st); err != nil {
		t.Fatal(err)
	}
	errs := q.TakeErrors()
	if q.Len() != 0 || len(errs) != 1 || !strings.Contains(errs[0], "item delete") {
		t.Errorf("len = %d errs = %v", q.Len(), errs)
	}
}

// Offline, a sync keeps the queue and returns an offline error.
func TestQueueSyncOffline(t *testing.T) {
	c, q, _ := queueClient(t)
	ctx := context.Background()
	_ = c.Close(ctx, "t1")
	var st SyncState
	err := c.Sync(ctx, &st)
	if !IsOffline(err) || q.Len() != 1 {
		t.Errorf("err = %v len = %d", err, q.Len())
	}
}

// After a time-out, an add is not queued, because Todoist can have added it.
func TestQueueAddAfterTimeout(t *testing.T) {
	c, q, _ := queueClient(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()
	c.base = srv.URL
	c.http.Timeout = 50 * time.Millisecond
	if _, err := c.QuickAdd(context.Background(), "milk"); err == nil || q.Len() != 0 {
		t.Errorf("err = %v len = %d, want an error and no queued add", err, q.Len())
	}
	if _, err := c.UpdateTask(context.Background(), "t1", map[string]any{"content": "x"}); err != nil || q.Len() != 1 {
		t.Errorf("update after a time-out: err = %v len = %d, want it queued", err, q.Len())
	}
}

// Overlay shows the queued changes on a copy of the data.
func TestQueueOverlay(t *testing.T) {
	c, q, _ := queueClient(t)
	ctx := context.Background()
	nt, _ := c.QuickAdd(ctx, "milk")
	_ = c.Close(ctx, "t1")
	_, _ = c.UpdateTask(ctx, "t2", map[string]any{"due_string": "fri", "priority": 4, "labels": []string{"home"}})
	_, _ = c.AddComment(ctx, "t2", "hello")
	st := SyncState{
		Projects: []Project{{ID: "inbox", InboxProject: true}},
		Tasks:    []Task{{ID: "t1", Content: "one"}, {ID: "t2", Content: "two", Priority: 1}},
	}
	pending := q.Overlay(&st)
	names := map[string]Task{}
	for _, x := range st.Tasks {
		names[x.ID] = x
	}
	if _, ok := names["t1"]; ok {
		t.Error("the completed task is still there")
	}
	if x, ok := names[nt.ID]; !ok || x.ProjectID != "inbox" || !pending[nt.ID] {
		t.Errorf("the queued add: %+v pending = %v", x, pending[nt.ID])
	}
	if x := names["t2"]; x.Priority != 4 || len(x.Labels) != 1 || x.Due == nil || x.Due.String != "fri" || !pending["t2"] {
		t.Errorf("t2 = %+v pending = %v", x, pending["t2"])
	}
	if len(st.Comments) != 1 || st.Comments[0].Content != "hello" {
		t.Errorf("comments = %+v", st.Comments)
	}
}

// A queued add from Today gets the date of the view only if Todoist finds no date.
func TestQueueQuickAddDate(t *testing.T) {
	c, _, _ := queueClient(t)
	ctx := context.Background()
	if _, err := c.QuickAddOn(ctx, "milk", "2026-09-27"); err != nil {
		t.Fatal(err)
	}
	f := &fakeTodoist{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c.base = srv.URL
	var st SyncState
	if err := c.Sync(ctx, &st); err != nil {
		t.Fatal(err)
	}
	if len(f.requests) < 2 || f.requests[0] != "POST /tasks/quick" || f.requests[1] != "POST /tasks/real1" {
		t.Errorf("requests = %v, want the quick add and then the date", f.requests)
	}
}

// If the add works and the date request fails, only the date waits in the queue. The add
// does not go into the queue, so it cannot happen two times.
func TestQueueQuickAddDateFailsAfterAdd(t *testing.T) {
	c, q, _ := queueClient(t)
	f := &fakeTodoist{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tasks/real1" { // the connection breaks: no answer
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		f.ServeHTTP(w, r)
	}))
	defer srv.Close()
	c.base = srv.URL
	nt, err := c.QuickAddOn(context.Background(), "milk", "2026-09-27")
	if err != nil || nt.ID != "real1" {
		t.Fatalf("task = %+v err = %v", nt, err)
	}
	ops := q.Ops()
	if len(ops) != 1 || ops[0].Type != "item_update" || ops[0].Args["id"] != "real1" {
		t.Errorf("ops = %+v, want only the date with the real ID", ops)
	}
}

// A server error (HTTP 503) keeps the queue for the next sync.
func TestQueueKeepsOnServerError(t *testing.T) {
	c, q, _ := queueClient(t)
	ctx := context.Background()
	_ = c.Close(ctx, "t1")
	_, _ = c.QuickAdd(ctx, "milk")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c.base = srv.URL
	var st SyncState
	if err := c.Sync(ctx, &st); err == nil || q.Len() != 2 || len(q.TakeErrors()) != 0 {
		t.Errorf("err = %v len = %d, want an error and the queue kept", err, q.Len())
	}
}

// A queued add with the date of the view shows on that date. Complete, reopen, and
// complete again leave the task completed.
func TestQueueOverlayDateAndOrder(t *testing.T) {
	c, q, _ := queueClient(t)
	ctx := context.Background()
	nt, _ := c.QuickAddOn(ctx, "milk", "2026-09-27")
	_ = c.Close(ctx, "t1")
	_ = c.Reopen(ctx, "t1")
	_ = c.Close(ctx, "t1")
	_ = c.Close(ctx, "t2")
	_ = c.Reopen(ctx, "t2")
	st := SyncState{Projects: []Project{{ID: "inbox", InboxProject: true}},
		Tasks: []Task{{ID: "t1", Content: "one"}, {ID: "t2", Content: "two"}}}
	q.Overlay(&st)
	got := map[string]Task{}
	for _, x := range st.Tasks {
		got[x.ID] = x
	}
	if x := got[nt.ID]; x.Due == nil || x.Due.Date != "2026-09-27" {
		t.Errorf("queued add = %+v, want the date of the view", x)
	}
	if _, ok := got["t1"]; ok {
		t.Error("t1 shows, but the last queued change completes it")
	}
	if _, ok := got["t2"]; !ok {
		t.Error("t2 is gone, but the last queued change reopens it")
	}
}
