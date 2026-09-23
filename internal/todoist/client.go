// Package todoist is a small client for the Todoist API v1.
package todoist

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://api.todoist.com/api/v1"

// Client sends requests to the Todoist API with a personal API token.
type Client struct {
	token string
	http  *http.Client
}

// New returns a client for the token.
func New(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 20 * time.Second}}
}

// APIError is a response with a status code of 300 or more.
type APIError struct {
	Status int
	Body   string
}

// Error returns the message from the "error" field of the JSON body if there is one,
// because the full body is too long for the status line.
func (e *APIError) Error() string {
	var body struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(e.Body), &body) == nil && body.Error != "" {
		return fmt.Sprintf("Todoist: %s (HTTP %d)", body.Error, e.Status)
	}
	return fmt.Sprintf("Todoist: HTTP %d %s", e.Status, e.Body)
}

// do sends a request and decodes the JSON response into out. It sends a url.Values body
// as a form and other bodies as JSON.
func (c *Client) do(ctx context.Context, method, path string, q url.Values, body, out any) error {
	u := baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	contentType := ""
	switch b := body.(type) {
	case nil:
	case url.Values:
		rd, contentType = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
	default:
		j, err := json.Marshal(b)
		if err != nil {
			return err
		}
		rd, contentType = bytes.NewReader(j), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &APIError{Status: resp.StatusCode, Body: string(bytes.TrimSpace(b))}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type page[T any] struct {
	Results    []T     `json:"results"`
	NextCursor *string `json:"next_cursor"`
}

// list fetches every page of a cursor-paginated endpoint.
func list[T any](ctx context.Context, c *Client, path string, q url.Values) ([]T, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("limit", "200")
	var all []T
	for {
		var p page[T]
		if err := c.do(ctx, http.MethodGet, path, q, nil, &p); err != nil {
			return nil, err
		}
		all = append(all, p.Results...)
		if p.NextCursor == nil || *p.NextCursor == "" {
			return all, nil
		}
		q.Set("cursor", *p.NextCursor)
	}
}

// Projects returns all projects.
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	return list[Project](ctx, c, "/projects", nil)
}

// Sections returns all sections.
func (c *Client) Sections(ctx context.Context) ([]Section, error) {
	return list[Section](ctx, c, "/sections", nil)
}

// Labels returns all personal labels.
func (c *Client) Labels(ctx context.Context) ([]Label, error) {
	return list[Label](ctx, c, "/labels", nil)
}

// Tasks returns all active tasks.
func (c *Client) Tasks(ctx context.Context) ([]Task, error) {
	return list[Task](ctx, c, "/tasks", nil)
}

// Filter returns active tasks matching a Todoist filter query (e.g. "today | overdue").
func (c *Client) Filter(ctx context.Context, query string) ([]Task, error) {
	return list[Task](ctx, c, "/tasks/filter", url.Values{"query": {query}})
}

// QuickAdd creates a task from natural-language text ("Buy milk tomorrow #home p2").
func (c *Client) QuickAdd(ctx context.Context, text string) (Task, error) {
	var t Task
	err := c.do(ctx, http.MethodPost, "/tasks/quick", nil, map[string]string{"text": text}, &t)
	return t, err
}

// Parse returns the result of Todoist's natural-language parser for text. The API has no
// parse-only call, so Parse quick-adds a temporary task and deletes it at once.
func (c *Client) Parse(ctx context.Context, text string) (Task, error) {
	t, err := c.QuickAdd(ctx, text)
	if err != nil {
		return t, err
	}
	if t.Content == "" {
		return t, fmt.Errorf("the text has no name after parsing")
	}
	if err := c.Delete(ctx, t.ID); err != nil {
		return t, fmt.Errorf("parsed, but the temporary task %s was not deleted: %w", t.ID, err)
	}
	return t, nil
}

// ParseDue returns the due date that Todoist reads in text, or nil if the text has none.
// A fixed task name goes before the text, so that a text with only a date also parses.
func (c *Client) ParseDue(ctx context.Context, text string) (*Due, error) {
	t, err := c.Parse(ctx, "x "+text)
	if err != nil {
		return nil, err
	}
	return t.Due, nil
}

// MoveOccurrence moves only the current occurrence of a recurring task to date
// ("2006-01-02" or "2006-01-02T15:04:05"). The repeat rule stays. The REST update
// cannot do this: a new due_date removes the rule. The Sync API item_update can.
func (c *Client) MoveOccurrence(ctx context.Context, id, date, rule, lang string) error {
	if lang == "" {
		lang = "en"
	}
	return c.syncCommand(ctx, "item_update", map[string]any{"id": id, "due": map[string]any{
		"date": date, "string": rule, "lang": lang, "is_recurring": true}})
}

// syncCommand sends one Sync API command and returns an error if Todoist does not accept it.
func (c *Client) syncCommand(ctx context.Context, typ string, args map[string]any) error {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80 // UUID version 4
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	j, err := json.Marshal([]map[string]any{{"type": typ, "uuid": uuid, "args": args}})
	if err != nil {
		return err
	}
	var resp struct {
		SyncStatus map[string]json.RawMessage `json:"sync_status"`
	}
	if err := c.do(ctx, http.MethodPost, "/sync", nil, url.Values{"commands": {string(j)}}, &resp); err != nil {
		return err
	}
	if st := string(resp.SyncStatus[uuid]); st != `"ok"` {
		return fmt.Errorf("Todoist did not accept %s: %s", typ, st)
	}
	return nil
}

// AddSection creates a section at the end of a project.
func (c *Client) AddSection(ctx context.Context, projectID, name string) (Section, error) {
	var s Section
	err := c.do(ctx, http.MethodPost, "/sections", nil, map[string]string{"project_id": projectID, "name": name}, &s)
	return s, err
}

// RenameSection changes the name of a section.
func (c *Client) RenameSection(ctx context.Context, id, name string) error {
	return c.do(ctx, http.MethodPost, "/sections/"+url.PathEscape(id), nil, map[string]string{"name": name}, nil)
}

// DeleteSection deletes a section and all of its tasks.
func (c *Client) DeleteSection(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/sections/"+url.PathEscape(id), nil, nil, nil)
}

// ReorderSections gives the sections of a project the order of ids (the first gets 1).
// The REST API cannot do this: its "order" field does not move the other sections.
func (c *Client) ReorderSections(ctx context.Context, ids []string) error {
	list := make([]map[string]any, len(ids))
	for i, id := range ids {
		list[i] = map[string]any{"id": id, "section_order": i + 1}
	}
	return c.syncCommand(ctx, "section_reorder", map[string]any{"sections": list})
}

// AddTask creates a task with literal content (no natural-language parsing).
func (c *Client) AddTask(ctx context.Context, content, projectID, sectionID string) (Task, error) {
	body := map[string]string{"content": content, "project_id": projectID}
	if sectionID != "" {
		body["section_id"] = sectionID
	}
	var t Task
	err := c.do(ctx, http.MethodPost, "/tasks", nil, body, &t)
	return t, err
}

// Move moves a task to a project. If sectionID is not empty, it moves the task to that section.
func (c *Client) Move(ctx context.Context, id, projectID, sectionID string) error {
	body := map[string]string{"project_id": projectID}
	if sectionID != "" {
		body = map[string]string{"section_id": sectionID}
	}
	return c.do(ctx, http.MethodPost, "/tasks/"+url.PathEscape(id)+"/move", nil, body, nil)
}

// UpdateTask sets task fields, e.g. {"due_date": "2026-09-26"} or {"content": "…"}.
func (c *Client) UpdateTask(ctx context.Context, id string, fields map[string]any) (Task, error) {
	var t Task
	err := c.do(ctx, http.MethodPost, "/tasks/"+url.PathEscape(id), nil, fields, &t)
	return t, err
}

// CreateLabel creates a personal label. A task can hold an unknown label name,
// but Todoist does not add that name to the label list.
func (c *Client) CreateLabel(ctx context.Context, name string) (Label, error) {
	var l Label
	err := c.do(ctx, http.MethodPost, "/labels", nil, map[string]string{"name": name}, &l)
	return l, err
}

// AddComment adds a comment to a task.
func (c *Client) AddComment(ctx context.Context, taskID, content string) (Comment, error) {
	var cm Comment
	err := c.do(ctx, http.MethodPost, "/comments", nil, map[string]string{"task_id": taskID, "content": content}, &cm)
	return cm, err
}

// UpdateComment replaces the text of a comment.
func (c *Client) UpdateComment(ctx context.Context, id, content string) (Comment, error) {
	var cm Comment
	err := c.do(ctx, http.MethodPost, "/comments/"+url.PathEscape(id), nil, map[string]string{"content": content}, &cm)
	return cm, err
}

// DeleteComment deletes a comment.
func (c *Client) DeleteComment(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/comments/"+url.PathEscape(id), nil, nil, nil)
}

// Close completes a task. For a recurring task, it moves the due date to the next occurrence.
func (c *Client) Close(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/tasks/"+url.PathEscape(id)+"/close", nil, nil, nil)
}

// Reopen makes a completed task active again.
func (c *Client) Reopen(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/tasks/"+url.PathEscape(id)+"/reopen", nil, nil, nil)
}

// Delete deletes a task permanently.
func (c *Client) Delete(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/tasks/"+url.PathEscape(id), nil, nil, nil)
}
