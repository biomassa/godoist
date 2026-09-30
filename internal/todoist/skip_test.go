package todoist

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// skipServer answers a quick add with the due date next (for the parser), and records the
// Sync commands.
func skipServer(t *testing.T, next string, cmds *[]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tasks/quick":
			_ = json.NewEncoder(w).Encode(Task{ID: "tmp1", Content: "x", Due: &Due{Date: next, String: "every mon", IsRecurring: true}})
		case r.URL.Path == "/sync":
			b, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(b))
			var list []map[string]any
			_ = json.Unmarshal([]byte(form.Get("commands")), &list)
			*cmds = append(*cmds, list...)
			st := map[string]any{}
			for _, c := range list {
				st[c["uuid"].(string)] = "ok"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"sync_status": st})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
}

func skipDate(t *testing.T, cmds []map[string]any) string {
	t.Helper()
	if len(cmds) != 1 || cmds[0]["type"] != "item_update" {
		t.Fatalf("commands = %v", cmds)
	}
	due := cmds[0]["args"].(map[string]any)["due"].(map[string]any)
	if due["is_recurring"] != true {
		t.Errorf("the repeat is gone: %v", due)
	}
	return due["date"].(string)
}

// An interval rule gets the current date plus the interval. The parser is not used.
func TestSkipInterval(t *testing.T) {
	for rule, want := range map[string]string{
		"every 2 weeks": "2026-10-14", "every other month": "2026-11-30", "every! 3 days": "2026-10-03", "every year": "2027-09-30",
		"every week on monday": "2026-10-07", "every day at 9am": "2026-10-01",
	} {
		var cmds []map[string]any
		srv := skipServer(t, "2099-01-01", &cmds)
		c := New("test")
		c.base = srv.URL
		due, err := c.SkipOccurrence(context.Background(), Task{ID: "t1", Due: &Due{Date: "2026-09-30", String: rule, IsRecurring: true}})
		srv.Close()
		if err != nil {
			t.Fatalf("%s: %v", rule, err)
		}
		if got := skipDate(t, cmds); got != want || due.Date != want {
			t.Errorf("%s: next = %s, want %s", rule, got, want)
		}
	}
}

// "every weekday" is not an interval: it goes to the parser.
func TestSkipWeekdayIsNotInterval(t *testing.T) {
	if intervalRule.MatchString("every weekday") || intervalRule.MatchString("every workday") {
		t.Error("every weekday matches the interval rule")
	}
}

// Another rule gets the date from the parser ("<rule> starting <the next day>").
func TestSkipParsed(t *testing.T) {
	var cmds []map[string]any
	srv := skipServer(t, "2026-10-12", &cmds)
	defer srv.Close()
	c := New("test")
	c.base = srv.URL
	if _, err := c.SkipOccurrence(context.Background(), Task{ID: "t1", Due: &Due{Date: "2026-10-05", String: "every mon", IsRecurring: true}}); err != nil {
		t.Fatal(err)
	}
	if got := skipDate(t, cmds); got != "2026-10-12" {
		t.Errorf("next = %s, want 2026-10-12", got)
	}
}
