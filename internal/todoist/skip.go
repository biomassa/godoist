package todoist

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The API has no command that skips one date of a recurring task. SkipOccurrence finds the
// next date itself and moves only this occurrence there (see MoveOccurrence). It does not
// ask the Todoist parser: a parse needs a temporary task, and the Google Calendar sync of
// Todoist can keep an event of that task.

var (
	// intervalRule is a repeat that counts from the last date: "every week", "every 2
	// weeks", "every other month", "every! 3 days". "every weekday" does not match.
	intervalRule = regexp.MustCompile(`(?i)^every!?\s+(?:(\d+|other)\s+)?(day|week|month|year)s?\b`)
	// weekdayRule is a repeat on weekdays: "every mon", "every monday, thursday",
	// "every tue and fri".
	weekdayRule = regexp.MustCompile(`(?i)^every!?\s+((?:mon|tue|wed|thu|fri|sat|sun)[a-z]*(?:\s*(?:,|and|&)\s*(?:mon|tue|wed|thu|fri|sat|sun)[a-z]*)*)\b`)
	// workdayRule is a repeat on Monday to Friday.
	workdayRule = regexp.MustCompile(`(?i)^every!?\s+(?:weekday|workday)s?\b`)
	// monthDayRule is a repeat on a day of the month: "every 15th".
	monthDayRule = regexp.MustCompile(`(?i)^every!?\s+(\d{1,2})(?:st|nd|rd|th)\b`)
	// ruleTail is the part after the rule that does not change the next date: a time, or
	// the start or the end of the repeat.
	ruleTail     = regexp.MustCompile(`(?i)^\s*(?:$|at\s|from\s|starting\s|until\s|ending\s|for\s)`)
	weekdayWord  = regexp.MustCompile(`(?i)mon|tue|wed|thu|fri|sat|sun`)
	weekdayNames = map[string]time.Weekday{"mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
		"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday, "sun": time.Sunday}
)

// ruleAliases are the one-word repeats.
var ruleAliases = map[string]string{"daily": "every day", "weekly": "every week", "monthly": "every month", "yearly": "every year"}

// nextDate returns the date after cur for the repeat rule. ok is false if godoist cannot
// compute it for this rule.
func nextDate(rule string, cur time.Time) (time.Time, bool) {
	rule = strings.TrimSpace(rule)
	if a, ok := ruleAliases[strings.ToLower(rule)]; ok {
		rule = a
	}
	tailOK := func(m []int) bool { return ruleTail.MatchString(rule[m[1]:]) }
	if m := intervalRule.FindStringSubmatchIndex(rule); m != nil {
		n := 1 // "every week"
		if m[2] >= 0 {
			if v, err := strconv.Atoi(rule[m[2]:m[3]]); err == nil {
				n = v
			} else {
				n = 2 // "other"
			}
		}
		switch strings.ToLower(rule[m[4]:m[5]]) {
		case "day":
			return cur.AddDate(0, 0, n), true
		case "week":
			return cur.AddDate(0, 0, 7*n), true
		case "month":
			return cur.AddDate(0, n, 0), true
		}
		return cur.AddDate(n, 0, 0), true
	}
	if m := workdayRule.FindStringIndex(rule); m != nil && tailOK(m) {
		next := cur.AddDate(0, 0, 1)
		for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
			next = next.AddDate(0, 0, 1)
		}
		return next, true
	}
	if m := weekdayRule.FindStringSubmatchIndex(rule); m != nil && tailOK(m) {
		days := map[time.Weekday]bool{}
		for _, w := range weekdayWord.FindAllString(rule[m[2]:m[3]], -1) {
			days[weekdayNames[strings.ToLower(w)]] = true
		}
		for i := 1; i <= 7; i++ {
			if next := cur.AddDate(0, 0, i); days[next.Weekday()] {
				return next, true
			}
		}
	}
	if m := monthDayRule.FindStringSubmatchIndex(rule); m != nil && tailOK(m) {
		day, _ := strconv.Atoi(rule[m[2]:m[3]])
		if day < 1 || day > 31 {
			return time.Time{}, false
		}
		// The first later month that has the day. A month without the day is skipped.
		for i := 1; i <= 12; i++ {
			y, mo, _ := cur.Date()
			next := time.Date(y, mo+time.Month(i), day, cur.Hour(), cur.Minute(), 0, 0, cur.Location())
			if next.Day() == day {
				return next, true
			}
		}
	}
	return time.Time{}, false
}

// CanSkip reports whether SkipOccurrence can find the next date of the repeat rule.
func CanSkip(rule string) bool {
	_, ok := nextDate(rule, time.Now())
	return ok
}

// SkipOccurrence moves a recurring task to its next date and keeps the repeat. Todoist
// records no completion. It returns the new due date.
func (c *Client) SkipOccurrence(ctx context.Context, t Task) (*Due, error) {
	if t.Due == nil || !t.Due.IsRecurring {
		return nil, fmt.Errorf("the task does not repeat")
	}
	cur, hasTime, ok := t.Due.Time()
	if !ok {
		return nil, fmt.Errorf("the task has no date")
	}
	next, ok := nextDate(t.Due.String, cur)
	if !ok {
		return nil, fmt.Errorf("godoist cannot find the next date of %q", t.Due.String)
	}
	date := next.Format("2006-01-02")
	if hasTime {
		date = next.Local().Format("2006-01-02T15:04:05")
	}
	due := &Due{Date: date, String: t.Due.String, Lang: t.Due.Lang, IsRecurring: true}
	return due, c.MoveOccurrence(ctx, t.ID, date, t.Due.String, t.Due.Lang)
}
