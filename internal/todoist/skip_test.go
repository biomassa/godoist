package todoist

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
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

// The other rules get their next date locally: weekdays, Monday to Friday, and a day of
// the month. No request goes to the parser.
func TestNextDateLocal(t *testing.T) {
	wed := time.Date(2026, 9, 30, 9, 0, 0, 0, time.Local) // a Wednesday
	fri := time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local)
	jan31 := time.Date(2026, 1, 31, 0, 0, 0, 0, time.Local)
	for _, c := range []struct {
		rule string
		cur  time.Time
		want string
	}{
		{"every mon", wed, "2026-10-05"},
		{"every monday, thursday", wed, "2026-10-01"},
		{"every tue and fri", fri, "2026-10-06"},
		{"every weekday", fri, "2026-10-05"},
		{"every workday at 9am", wed, "2026-10-01"},
		{"every 15th", wed, "2026-10-15"},
		{"every 31st", jan31, "2026-03-31"}, // February has no 31st
		{"daily", wed, "2026-10-01"},
		{"every mon at 9am", wed, "2026-10-05"},
	} {
		got, ok := nextDate(c.rule, c.cur)
		if !ok || got.Format("2006-01-02") != c.want {
			t.Errorf("%s from %s: %v %v, want %s", c.rule, c.cur.Format("Mon 2006-01-02"), got.Format("2006-01-02"), ok, c.want)
		}
	}
	if got, _ := nextDate("every mon at 9am", wed); got.Hour() != 9 {
		t.Errorf("the time changed: %v", got)
	}
	for _, rule := range []string{"every other monday", "every 3rd friday", "every last day", "every mon except holidays"} {
		if CanSkip(rule) {
			t.Errorf("%q: godoist cannot know its next date, but CanSkip is true", rule)
		}
	}
}

func TestShiftRule(t *testing.T) {
	wed := time.Date(2026, 10, 14, 0, 0, 0, 0, time.Local)
	for rule, want := range map[string]string{
		"every Thursday":        "every Wednesday",
		"every thu at 9am":      "every Wednesday at 9am",
		"every! fri":            "every! Wednesday",
		"every 15th":            "every 14th",
		"every 1st at 10:00":    "every 14th at 10:00",
		"every mon until dec 1": "every Wednesday until dec 1",
	} {
		if got, ok := ShiftRule(rule, wed); !ok || got != want {
			t.Errorf("%q: %q %v, want %q", rule, got, ok, want)
		}
	}
	for _, rule := range []string{"every mon, thu", "every 2 weeks", "every weekday", "every day", "every other friday"} {
		if got, ok := ShiftRule(rule, wed); ok {
			t.Errorf("%q: shifted to %q, but the new rule is not clear", rule, got)
		}
	}
	if ordinal(22) != "22nd" || ordinal(11) != "11th" || ordinal(31) != "31st" {
		t.Error("ordinal is wrong")
	}
}
