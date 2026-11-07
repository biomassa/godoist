package todoist

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"time"
)

// syncResources are the resource types that Sync requests.
var syncResources = `["projects","sections","items","notes","labels","user"]`

// SyncState is a local replica of the account, kept current with the Sync API.
// A zero value (empty Token) triggers a full sync.
type SyncState struct {
	Token    string       `json:"sync_token"`
	SyncedAt time.Time    `json:"synced_at"`
	Projects []Project    `json:"projects"`
	Sections []Section    `json:"sections"`
	Tasks    []Task       `json:"tasks"`
	Comments []Comment    `json:"comments"`
	Labels   []Label      `json:"labels"`
	User     UserSettings `json:"user"`
	Done     []Task       `json:"done"` // the tasks completed in the last DoneDays days, newest first
}

// DoneDays is how far back SyncState.Done goes. Todoist gives a range of 3 months at most.
const DoneDays = 90

// Clone returns a copy of s. A sync can change the copy while s stays the same.
func (s SyncState) Clone() SyncState {
	s.Projects = append([]Project(nil), s.Projects...)
	s.Sections = append([]Section(nil), s.Sections...)
	s.Tasks = append([]Task(nil), s.Tasks...)
	s.Comments = append([]Comment(nil), s.Comments...)
	s.Labels = append([]Label(nil), s.Labels...)
	s.Done = append([]Task(nil), s.Done...)
	return s
}

// Sync pulls changes since the last sync (everything on first use) into s.
// With an offline queue, Sync first sends the changes that wait.
func (c *Client) Sync(ctx context.Context, s *SyncState) error {
	if c.queue.Len() > 0 {
		if err := c.flush(ctx, s); err != nil {
			return err
		}
	}
	token := s.Token
	if token == "" {
		token = "*"
	}
	var resp struct {
		SyncToken string        `json:"sync_token"`
		FullSync  bool          `json:"full_sync"`
		Projects  []Project     `json:"projects"`
		Sections  []Section     `json:"sections"`
		Items     []Task        `json:"items"`
		Notes     []Comment     `json:"notes"`
		Labels    []Label       `json:"labels"`
		User      *UserSettings `json:"user"`
	}
	form := url.Values{"sync_token": {token}, "resource_types": {syncResources}}
	if err := c.do(ctx, http.MethodPost, "/sync", nil, form, &resp); err != nil {
		return err
	}
	if resp.FullSync {
		s.Projects, s.Sections, s.Tasks, s.Comments, s.Labels = nil, nil, nil, nil, nil
	}
	s.Projects = merge(s.Projects, resp.Projects, func(p Project) string { return p.ID },
		func(p Project) bool { return p.IsDeleted || p.IsArchived })
	s.Sections = merge(s.Sections, resp.Sections, func(x Section) string { return x.ID },
		func(x Section) bool { return x.IsDeleted || x.IsArchived })
	s.Tasks = merge(s.Tasks, resp.Items, func(t Task) string { return t.ID },
		func(t Task) bool { return t.IsDeleted || t.Checked })
	c.syncDone(ctx, s, resp.Items)
	s.Comments = merge(s.Comments, resp.Notes, func(n Comment) string { return n.ID },
		func(n Comment) bool { return n.IsDeleted })
	// An incremental sync sends a changed label with "order": null. Keep the known order.
	oldOrder := map[string]int{}
	for _, l := range s.Labels {
		oldOrder[l.ID] = l.Order
	}
	for i := range resp.Labels {
		if resp.Labels[i].Order == 0 {
			resp.Labels[i].Order = oldOrder[resp.Labels[i].ID]
		}
	}
	s.Labels = merge(s.Labels, resp.Labels, func(l Label) string { return l.ID },
		func(l Label) bool { return l.IsDeleted })
	if resp.User != nil { // an incremental sync sends the user only after a change
		s.User = *resp.User
	}
	s.Token = resp.SyncToken
	s.SyncedAt = time.Now()
	return nil
}

// TrackDone makes each Sync also get the completed tasks of the last DoneDays days into
// SyncState.Done. This is one more request for each sync. The TUI uses it, and the CLI
// does not.
func (c *Client) TrackDone() { c.done = true }

// syncDone updates s.Done. The tasks that the sync gives as checked go in at once, and the
// tasks that are active or deleted go out. With TrackDone, syncDone then gets the
// completed tasks of the last DoneDays days from Todoist. If that fails, the old list
// stays.
func (c *Client) syncDone(ctx context.Context, s *SyncState, items []Task) {
	now := time.Now()
	out := map[string]bool{}
	for _, t := range s.Tasks {
		out[t.ID] = true
	}
	var checked []Task
	for _, t := range items {
		switch {
		case t.IsDeleted || !t.Checked:
			out[t.ID] = true
		case t.CompletedAt == "":
			t.CompletedAt = now.UTC().Format(time.RFC3339)
			fallthrough
		default:
			checked = append(checked, t)
		}
	}
	done := merge(s.Done, checked, func(t Task) string { return t.ID }, func(Task) bool { return false })
	if c.done {
		done = c.withCompleted(ctx, done, now)
	}
	since := now.AddDate(0, 0, -DoneDays)
	s.Done = s.Done[:0:0]
	for _, t := range done {
		if at, ok := t.CompletedTime(); !out[t.ID] && (!ok || at.After(since)) {
			s.Done = append(s.Done, t)
		}
	}
	sort.SliceStable(s.Done, func(i, j int) bool { return s.Done[i].CompletedAt > s.Done[j].CompletedAt })
}

// withCompleted returns the completed tasks of the last DoneDays days from Todoist. The
// list can come some seconds late, so the tasks of done from the last minutes that it
// does not have yet stay. If the request fails, withCompleted returns done.
func (c *Client) withCompleted(ctx context.Context, done []Task, now time.Time) []Task {
	got, err := c.CompletedTasks(ctx, now.AddDate(0, 0, -DoneDays), now.Add(time.Minute))
	if err != nil {
		return done
	}
	in := map[string]bool{}
	for _, t := range got {
		in[t.ID] = true
	}
	for _, t := range done {
		if at, ok := t.CompletedTime(); !in[t.ID] && ok && now.Sub(at) < 10*time.Minute {
			got = append(got, t)
		}
	}
	return got
}

// merge adds or replaces entries by ID. It removes the entries for which gone returns true.
func merge[T any](cur, changes []T, id func(T) string, gone func(T) bool) []T {
	if len(changes) == 0 {
		return cur
	}
	idx := make(map[string]int, len(cur))
	for i, x := range cur {
		idx[id(x)] = i
	}
	drop := map[string]bool{}
	for _, x := range changes {
		k := id(x)
		if gone(x) {
			drop[k] = true
			continue
		}
		delete(drop, k)
		if i, ok := idx[k]; ok {
			cur[i] = x
		} else {
			idx[k] = len(cur)
			cur = append(cur, x)
		}
	}
	if len(drop) == 0 {
		return cur
	}
	out := cur[:0]
	for _, x := range cur {
		if !drop[id(x)] {
			out = append(out, x)
		}
	}
	return out
}

// Snapshot is a read-only view of a SyncState for rendering.
type Snapshot struct {
	Projects []Project
	Sections []Section
	Tasks    []Task
	Comments map[string][]Comment // by task ID, oldest first
	Labels   []Label              // in the user's label order
	User     UserSettings
	Done     []Task // the tasks completed in the last DoneDays days, newest first
}

// Snapshot groups the comments by task and returns copies of all lists.
func (s SyncState) Snapshot() Snapshot {
	c := s.Clone()
	byTask := map[string][]Comment{}
	for _, n := range c.Comments {
		byTask[n.TaskID] = append(byTask[n.TaskID], n)
	}
	for _, cs := range byTask {
		sort.Slice(cs, func(i, j int) bool { return cs[i].PostedAt < cs[j].PostedAt })
	}
	sort.SliceStable(c.Labels, func(i, j int) bool { return c.Labels[i].Order < c.Labels[j].Order })
	return Snapshot{Projects: c.Projects, Sections: c.Sections, Tasks: c.Tasks, Comments: byTask, Labels: c.Labels, User: c.User, Done: c.Done}
}
