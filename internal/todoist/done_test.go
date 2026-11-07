package todoist

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A sync keeps the completed tasks of the last DoneDays days. A checked task of the sync
// goes in at once, also when the list of completed tasks does not have it yet. A task
// that is active again goes out. If the list fails, the old list stays.
func TestSyncDone(t *testing.T) {
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	p := "t1"
	listOK := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sync":
			json.NewEncoder(w).Encode(map[string]any{"sync_token": "n", "items": []Task{
				{ID: "t1", Content: "top"},
				{ID: "new", ParentID: &p, Content: "just done", Checked: true},
				{ID: "back", Content: "reopened"},
			}})
		case "/tasks/completed/by_completion_date":
			if !listOK {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"items": []Task{
				{ID: "old", ParentID: &p, Content: "done", CompletedAt: at(48 * time.Hour)},
				{ID: "back", Content: "reopened", CompletedAt: at(time.Hour)},
			}})
		}
	}))
	defer srv.Close()
	c := New("x")
	c.base = srv.URL
	c.TrackDone()
	var st SyncState
	if err := c.Sync(context.Background(), &st); err != nil {
		t.Fatal(err)
	}
	ids := func() []string {
		var out []string
		for _, d := range st.Done {
			out = append(out, d.ID)
		}
		return out
	}
	if got := ids(); len(got) != 2 || got[0] != "new" || got[1] != "old" {
		t.Errorf("done = %v, want [new old]", got)
	}
	listOK = false
	if err := c.Sync(context.Background(), &st); err != nil {
		t.Fatalf("a failed list must not fail the sync: %v", err)
	}
	if got := ids(); len(got) != 2 {
		t.Errorf("done = %v, want the old list", got)
	}
}
