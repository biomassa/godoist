package todoist

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// request is one request that the fake server got. path is escaped.
type request struct {
	method, path, query, contentType, auth string
	body                                   string
}

// fakeAPI starts a server that records each request and answers with reply. A nil reply
// answers with an empty JSON object.
func fakeAPI(t *testing.T, reply func(w http.ResponseWriter, r *http.Request, body string)) (*Client, *[]request) {
	t.Helper()
	var (
		mu  sync.Mutex
		got []request
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		got = append(got, request{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), string(b)})
		if reply != nil {
			reply(w, r, string(b))
			return
		}
		io.WriteString(w, "{}")
	}))
	t.Cleanup(srv.Close)
	c := New("tok")
	c.SetBaseURL(srv.URL)
	return c, &got
}

// do sends the token, JSON bodies as JSON, and url.Values as a form.
func TestDoBodies(t *testing.T) {
	c, got := fakeAPI(t, nil)
	ctx := context.Background()
	if err := c.do(ctx, http.MethodPost, "/a", nil, map[string]string{"k": "v"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.do(ctx, http.MethodPost, "/b", url.Values{"q": {"1"}}, url.Values{"k": {"v w"}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.do(ctx, http.MethodGet, "/c", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	r := *got
	if r[0].auth != "Bearer tok" {
		t.Errorf("Authorization = %q", r[0].auth)
	}
	if r[0].contentType != "application/json" || r[0].body != `{"k":"v"}` {
		t.Errorf("JSON request = %+v", r[0])
	}
	if r[1].contentType != "application/x-www-form-urlencoded" || r[1].body != "k=v+w" || r[1].query != "q=1" {
		t.Errorf("form request = %+v", r[1])
	}
	if r[2].contentType != "" || r[2].body != "" {
		t.Errorf("request without a body = %+v", r[2])
	}
}

// A status of 300 or more is an *APIError. Its message uses the "error" field of the
// body when there is one.
func TestAPIError(t *testing.T) {
	for _, tc := range []struct {
		body, want string
	}{
		{`{"error":"Task not found"}`, "Todoist: Task not found (HTTP 404)"},
		{"not json", "Todoist: HTTP 404 not json"},
	} {
		c, _ := fakeAPI(t, func(w http.ResponseWriter, _ *http.Request, _ string) {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, tc.body+"\n")
		})
		_, err := c.Task(context.Background(), "x")
		apiErr, ok := err.(*APIError)
		if !ok || apiErr.Status != http.StatusNotFound {
			t.Fatalf("err = %#v, want *APIError with 404", err)
		}
		if err.Error() != tc.want {
			t.Errorf("Error() = %q, want %q", err.Error(), tc.want)
		}
	}
}

// list follows next_cursor until it is empty and asks for 200 results per page.
func TestListPages(t *testing.T) {
	n := 0
	c, got := fakeAPI(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		if n++; n > 2 { // a broken cursor must not loop forever
			w.WriteHeader(http.StatusTeapot)
			return
		}
		switch r.URL.Query().Get("cursor") {
		case "":
			io.WriteString(w, `{"results":[{"id":"p1"},{"id":"p2"}],"next_cursor":"c2"}`)
		case "c2":
			io.WriteString(w, `{"results":[{"id":"p3"}],"next_cursor":null}`)
		}
	})
	ps, err := c.Projects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 3 || ps[2].ID != "p3" {
		t.Fatalf("projects = %+v", ps)
	}
	for _, r := range *got {
		if q, _ := url.ParseQuery(r.query); q.Get("limit") != "200" || r.path != "/projects" {
			t.Errorf("request = %+v", r)
		}
	}
	if len(*got) != 2 {
		t.Errorf("%d requests, want 2", len(*got))
	}
}

// Filter sends the query as it is.
func TestFilter(t *testing.T) {
	c, got := fakeAPI(t, nil)
	if _, err := c.Filter(context.Background(), "today | overdue"); err != nil {
		t.Fatal(err)
	}
	r := (*got)[0]
	if q, _ := url.ParseQuery(r.query); r.path != "/tasks/filter" || q.Get("query") != "today | overdue" {
		t.Errorf("request = %+v", r)
	}
}

// Each REST call goes to its method and path with its body. The client has no offline
// queue here, so the task calls also go to the REST API.
func TestRESTCalls(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name               string
		call               func(c *Client) error
		method, path, body string
	}{
		{"AddProject", func(c *Client) error { _, err := c.AddProject(ctx, "Home", "red", ""); return err },
			"POST", "/projects", `{"color":"red","name":"Home"}`},
		{"AddProject sub", func(c *Client) error { _, err := c.AddProject(ctx, "Home", "red", "p0"); return err },
			"POST", "/projects", `{"color":"red","name":"Home","parent_id":"p0"}`},
		{"UpdateProject", func(c *Client) error { return c.UpdateProject(ctx, "p1", map[string]any{"is_favorite": true}) },
			"POST", "/projects/p1", `{"is_favorite":true}`},
		{"DeleteProject", func(c *Client) error { return c.DeleteProject(ctx, "p1") }, "DELETE", "/projects/p1", ""},
		{"ArchiveProject", func(c *Client) error { return c.ArchiveProject(ctx, "p1") }, "POST", "/projects/p1/archive", ""},
		{"AddSection", func(c *Client) error { _, err := c.AddSection(ctx, "p1", "Next"); return err },
			"POST", "/sections", `{"name":"Next","project_id":"p1"}`},
		{"RenameSection", func(c *Client) error { return c.RenameSection(ctx, "s1", "Later") },
			"POST", "/sections/s1", `{"name":"Later"}`},
		{"DeleteSection", func(c *Client) error { return c.DeleteSection(ctx, "s1") }, "DELETE", "/sections/s1", ""},
		{"CreateLabel", func(c *Client) error { _, err := c.CreateLabel(ctx, "work"); return err },
			"POST", "/labels", `{"name":"work"}`},
		{"UpdateLabel", func(c *Client) error { return c.UpdateLabel(ctx, "l1", map[string]any{"name": "job"}) },
			"POST", "/labels/l1", `{"name":"job"}`},
		{"DeleteLabel", func(c *Client) error { return c.DeleteLabel(ctx, "l1") }, "DELETE", "/labels/l1", ""},
		{"AddTask", func(c *Client) error { _, err := c.AddTask(ctx, "milk", "p1", "s1"); return err },
			"POST", "/tasks", `{"content":"milk","project_id":"p1","section_id":"s1"}`},
		{"UpdateTask", func(c *Client) error {
			_, err := c.UpdateTask(ctx, "t1", map[string]any{"due_string": "tomorrow"})
			return err
		}, "POST", "/tasks/t1", `{"due_string":"tomorrow"}`},
		{"Move to project", func(c *Client) error { return c.Move(ctx, "t1", "p2", "") },
			"POST", "/tasks/t1/move", `{"project_id":"p2"}`},
		{"Move to section", func(c *Client) error { return c.Move(ctx, "t1", "p2", "s2") },
			"POST", "/tasks/t1/move", `{"section_id":"s2"}`},
		{"MoveToParent", func(c *Client) error { return c.MoveToParent(ctx, "t1", "t0") },
			"POST", "/tasks/t1/move", `{"parent_id":"t0"}`},
		{"SetCollapsed section", func(c *Client) error { return c.SetCollapsed(ctx, "sections", "s1", true) },
			"POST", "/sections/s1", `{"is_collapsed":true}`},
		{"Close", func(c *Client) error { return c.Close(ctx, "t1") }, "POST", "/tasks/t1/close", ""},
		{"Reopen", func(c *Client) error { return c.Reopen(ctx, "t1") }, "POST", "/tasks/t1/reopen", ""},
		{"Delete", func(c *Client) error { return c.Delete(ctx, "t1") }, "DELETE", "/tasks/t1", ""},
		{"AddComment", func(c *Client) error { _, err := c.AddComment(ctx, "t1", "hi"); return err },
			"POST", "/comments", `{"content":"hi","task_id":"t1"}`},
		{"UpdateComment", func(c *Client) error { _, err := c.UpdateComment(ctx, "c1", "hey"); return err },
			"POST", "/comments/c1", `{"content":"hey"}`},
		{"DeleteComment", func(c *Client) error { return c.DeleteComment(ctx, "c1") }, "DELETE", "/comments/c1", ""},
		{"path escape", func(c *Client) error { return c.Delete(ctx, "a/b") }, "DELETE", "/tasks/a%2Fb", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, got := fakeAPI(t, nil)
			if err := tc.call(c); err != nil {
				t.Fatal(err)
			}
			if len(*got) != 1 {
				t.Fatalf("%d requests, want 1", len(*got))
			}
			r := (*got)[0]
			if r.method != tc.method || r.path != tc.path || r.body != tc.body {
				t.Errorf("got %s %s %s, want %s %s %s", r.method, r.path, r.body, tc.method, tc.path, tc.body)
			}
		})
	}
}

// syncReply answers a /sync request with status for each command of the request.
func syncReply(status string) func(w http.ResponseWriter, r *http.Request, body string) {
	return func(w http.ResponseWriter, _ *http.Request, body string) {
		form, _ := url.ParseQuery(body)
		var cmds []struct {
			UUID string `json:"uuid"`
		}
		json.Unmarshal([]byte(form.Get("commands")), &cmds)
		res := map[string]json.RawMessage{}
		for _, c := range cmds {
			res[c.UUID] = json.RawMessage(status)
		}
		json.NewEncoder(w).Encode(map[string]any{"sync_status": res})
	}
}

// syncCommand sends one command with a new UUID. A status that is not "ok" is an error.
func TestSyncCommand(t *testing.T) {
	c, got := fakeAPI(t, syncReply(`"ok"`))
	if err := c.MoveProject(context.Background(), "p1", ""); err != nil {
		t.Fatal(err)
	}
	form, _ := url.ParseQuery((*got)[0].body)
	var cmds []struct {
		Type string         `json:"type"`
		UUID string         `json:"uuid"`
		Args map[string]any `json:"args"`
	}
	if err := json.Unmarshal([]byte(form.Get("commands")), &cmds); err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 || cmds[0].Type != "project_move" || len(cmds[0].UUID) != 36 {
		t.Fatalf("commands = %+v", cmds)
	}
	// A top-level project has a null parent_id, not an empty string.
	if v, ok := cmds[0].Args["parent_id"]; !ok || v != nil {
		t.Errorf("parent_id = %#v, want null", v)
	}

	c, _ = fakeAPI(t, syncReply(`{"error":"Invalid argument"}`))
	err := c.ReorderProjects(context.Background(), []string{"a", "b"})
	if err == nil || !strings.Contains(err.Error(), "project_reorder") || !strings.Contains(err.Error(), "Invalid argument") {
		t.Errorf("err = %v", err)
	}
}

// The reorder calls number the items from 1 in the given order.
func TestReorderArgs(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		call func(c *Client) error
		want string
	}{
		{func(c *Client) error { return c.ReorderProjects(ctx, []string{"a", "b"}) },
			`{"projects":[{"child_order":1,"id":"a"},{"child_order":2,"id":"b"}]}`},
		{func(c *Client) error { return c.ReorderSections(ctx, []string{"a", "b"}) },
			`{"sections":[{"id":"a","section_order":1},{"id":"b","section_order":2}]}`},
		{func(c *Client) error { return c.ReorderLabels(ctx, []string{"a", "b"}) },
			`{"id_order_mapping":{"a":1,"b":2}}`},
		{func(c *Client) error { return c.ReorderTasks(ctx, []string{"a", "b"}) },
			`{"items":[{"child_order":1,"id":"a"},{"child_order":2,"id":"b"}]}`},
	} {
		c, got := fakeAPI(t, syncReply(`"ok"`))
		if err := tc.call(c); err != nil {
			t.Fatal(err)
		}
		form, _ := url.ParseQuery((*got)[0].body)
		var cmds []struct {
			Args json.RawMessage `json:"args"`
		}
		json.Unmarshal([]byte(form.Get("commands")), &cmds)
		if string(cmds[0].Args) != tc.want {
			t.Errorf("args = %s, want %s", cmds[0].Args, tc.want)
		}
	}
}

// syncFields turns the REST due fields into the one "due" object of the Sync API.
func TestSyncFields(t *testing.T) {
	for _, tc := range []struct {
		in   map[string]any
		want map[string]any
	}{
		{map[string]any{"content": "x"}, map[string]any{"id": "t", "content": "x"}},
		{map[string]any{"due_string": "every day"}, map[string]any{"id": "t", "due": map[string]any{"string": "every day"}}},
		{map[string]any{"due_string": " No Date "}, map[string]any{"id": "t", "due": nil}},
		{map[string]any{"due_date": "2026-11-12"}, map[string]any{"id": "t", "due": map[string]any{"date": "2026-11-12"}}},
		{map[string]any{"due_datetime": "2026-11-12T10:00:00Z"}, map[string]any{"id": "t", "due": map[string]any{"date": "2026-11-12T10:00:00Z"}}},
	} {
		if got := syncFields("t", tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("syncFields(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
