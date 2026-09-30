package todoist

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// intervalRule matches a repeat that counts from the last date: "every week",
// "every 2 weeks", "every other month", "every! 3 days". Such a rule restarts its count at
// a "starting" date, so SkipOccurrence adds the interval itself. "every weekday" does not
// match: it is not an interval.
var intervalRule = regexp.MustCompile(`(?i)^every!?\s+(?:(\d+|other)\s+)?(day|week|month|year)s?\b`)

// SkipOccurrence moves a recurring task to its next date and keeps the repeat. Todoist
// records no completion. The API has no command for this: for an interval rule, the next
// date is the current date plus the interval. For other rules, the Todoist parser gives the
// first date of the rule after the current date. It returns the new due date.
func (c *Client) SkipOccurrence(ctx context.Context, t Task) (*Due, error) {
	if t.Due == nil || !t.Due.IsRecurring {
		return nil, fmt.Errorf("the task does not repeat")
	}
	cur, hasTime, ok := t.Due.Time()
	if !ok {
		return nil, fmt.Errorf("the task has no date")
	}
	rule := t.Due.String
	var next time.Time
	if m := intervalRule.FindStringSubmatch(rule); m != nil {
		n := 1 // "every week"
		if m[1] == "other" {
			n = 2
		} else if v, err := strconv.Atoi(m[1]); err == nil {
			n = v
		}
		switch strings.ToLower(m[2]) {
		case "day":
			next = cur.AddDate(0, 0, n)
		case "week":
			next = cur.AddDate(0, 0, 7*n)
		case "month":
			next = cur.AddDate(0, n, 0)
		default:
			next = cur.AddDate(n, 0, 0)
		}
	} else {
		due, err := c.ParseDue(ctx, rule+" starting "+cur.AddDate(0, 0, 1).Format("Jan 2 2006"))
		if err != nil {
			return nil, err
		}
		if due == nil {
			return nil, fmt.Errorf("Todoist found no next date for %q", rule) //lint:ignore ST1005 Todoist is a name
		}
		n, _, ok := due.Time()
		if !ok || !n.After(cur) {
			return nil, fmt.Errorf("Todoist found no next date for %q", rule) //lint:ignore ST1005 Todoist is a name
		}
		next = n
	}
	date := next.Format("2006-01-02")
	if hasTime {
		date = next.Local().Format("2006-01-02T15:04:05")
	}
	due := &Due{Date: date, String: rule, Lang: t.Due.Lang, IsRecurring: true}
	return due, c.MoveOccurrence(ctx, t.ID, date, rule, t.Due.Lang)
}
