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
}

// Clone returns a copy of s. A sync can change the copy while s stays the same.
func (s SyncState) Clone() SyncState {
	s.Projects = append([]Project(nil), s.Projects...)
	s.Sections = append([]Section(nil), s.Sections...)
	s.Tasks = append([]Task(nil), s.Tasks...)
	s.Comments = append([]Comment(nil), s.Comments...)
	s.Labels = append([]Label(nil), s.Labels...)
	return s
}

// Sync pulls changes since the last sync (everything on first use) into s.
func (c *Client) Sync(ctx context.Context, s *SyncState) error {
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
	return Snapshot{Projects: c.Projects, Sections: c.Sections, Tasks: c.Tasks, Comments: byTask, Labels: c.Labels, User: c.User}
}
