package todoist

import (
	"strings"
	"time"
)

// Project is a Todoist project.
type Project struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Color        string  `json:"color"`
	ParentID     *string `json:"parent_id"`
	ChildOrder   int     `json:"child_order"`
	IsFavorite   bool    `json:"is_favorite"`
	IsArchived   bool    `json:"is_archived"`
	IsShared     bool    `json:"is_shared"`
	InboxProject bool    `json:"inbox_project"`
	IsDeleted    bool    `json:"is_deleted"`
	ViewStyle    string  `json:"view_style"`
	Description  string  `json:"description"`
}

// Section is a group of tasks inside a project.
type Section struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Name         string `json:"name"`
	SectionOrder int    `json:"section_order"`
	IsCollapsed  bool   `json:"is_collapsed"`
	IsArchived   bool   `json:"is_archived"`
	IsDeleted    bool   `json:"is_deleted"`
}

// Label is a personal label.
type Label struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	Order      int    `json:"order"`
	IsFavorite bool   `json:"is_favorite"`
	IsDeleted  bool   `json:"is_deleted"`
}

// Due is the due date of a task. Date has one of the formats that Time reads.
type Due struct {
	Date        string  `json:"date"`
	String      string  `json:"string"`
	Timezone    *string `json:"timezone"`
	Lang        string  `json:"lang"`
	IsRecurring bool    `json:"is_recurring"`
}

// UserSettings are the Todoist settings that the date picker uses.
// Days are 1 (Monday) to 7 (Sunday).
type UserSettings struct {
	StartDay        int `json:"start_day"`         // first day of the week
	NextWeek        int `json:"next_week"`         // the day that "next week" means
	WeekendStartDay int `json:"weekend_start_day"` // the day that "weekend" means
}

// Weekday converts a Todoist day number (1 = Monday … 7 = Sunday) to a time.Weekday.
// 0 gives def.
func Weekday(n int, def time.Weekday) time.Weekday {
	if n < 1 || n > 7 {
		return def
	}
	return time.Weekday(n % 7)
}

// Time parses the due date into local time. hasTime is false for all-day dates.
// Todoist sends "2006-01-02", floating "2006-01-02T15:04:05", or UTC "...Z".
func (d *Due) Time() (t time.Time, hasTime bool, ok bool) {
	if d == nil || d.Date == "" {
		return time.Time{}, false, false
	}
	if t, err := time.ParseInLocation("2006-01-02", d.Date, time.Local); err == nil {
		return t, false, true
	}
	if strings.HasSuffix(d.Date, "Z") {
		if t, err := time.Parse(time.RFC3339, d.Date); err == nil {
			return t.Local(), true, true
		}
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", d.Date, time.Local); err == nil {
		return t, true, true
	}
	return time.Time{}, false, false
}

// Task is an active task. The Sync API calls it an "item".
type Task struct {
	ID          string   `json:"id"`
	ProjectID   string   `json:"project_id"`
	SectionID   *string  `json:"section_id"`
	ParentID    *string  `json:"parent_id"`
	Content     string   `json:"content"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	Priority    int      `json:"priority"` // 4 = p1 (urgent) … 1 = p4 (none)
	Due         *Due     `json:"due"`
	ChildOrder  int      `json:"child_order"`
	NoteCount   int      `json:"note_count"`
	AddedAt     string   `json:"added_at"`
	Checked     bool     `json:"checked"`
	IsDeleted   bool     `json:"is_deleted"`
}

// Section returns the section ID, or "" if the task has no section.
func (t Task) Section() string {
	if t.SectionID == nil {
		return ""
	}
	return *t.SectionID
}

// Parent returns the parent task ID, or "" for a top-level task.
func (t Task) Parent() string {
	if t.ParentID == nil {
		return ""
	}
	return *t.ParentID
}

// UIPriority converts the API priority to the p1 to p4 scale that the Todoist app shows.
func (t Task) UIPriority() int { return 5 - t.Priority }

// DueDay classifies a due date relative to today (local time).
type DueDay int

const (
	NoDue DueDay = iota
	Overdue
	DueToday
	DueTomorrow
	DueThisWeek
	DueLater
)

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// Classify puts a due date in a DueDay group. A time earlier today stays in DueToday,
// as in the Todoist app.
func Classify(d *Due, now time.Time) DueDay {
	t, _, ok := d.Time()
	if !ok {
		return NoDue
	}
	today := startOfDay(now)
	day := startOfDay(t)
	switch {
	case day.Before(today):
		return Overdue
	case day.Equal(today):
		return DueToday
	case day.Equal(today.AddDate(0, 0, 1)):
		return DueTomorrow
	case day.Before(today.AddDate(0, 0, 7)):
		return DueThisWeek
	default:
		return DueLater
	}
}

// PastDue reports whether a due date or time is in the past. A time earlier today counts.
func PastDue(d *Due, now time.Time) bool {
	t, hasTime, ok := d.Time()
	if !ok {
		return false
	}
	if hasTime {
		return t.Before(now)
	}
	return startOfDay(t).Before(startOfDay(now))
}

// FormatDue renders a short, Todoist-style due label ("Today 11:00", "Fri", "19 Jul").
func FormatDue(d *Due, now time.Time) string {
	t, hasTime, ok := d.Time()
	if !ok {
		return ""
	}
	today := startOfDay(now)
	day := startOfDay(t)
	var s string
	switch {
	case day.Equal(today):
		s = "Today"
	case day.Equal(today.AddDate(0, 0, 1)):
		s = "Tomorrow"
	case day.Equal(today.AddDate(0, 0, -1)):
		s = "Yesterday"
	case day.After(today) && day.Before(today.AddDate(0, 0, 7)):
		s = t.Format("Mon")
	case t.Year() == now.Year():
		s = t.Format("2 Jan")
	default:
		s = t.Format("2 Jan 2006")
	}
	if hasTime {
		s += " " + t.Format("15:04")
	}
	return s
}

// Attachment is a file or a link preview on a comment.
type Attachment struct {
	ResourceType string `json:"resource_type"` // file, image, video, website
	FileName     string `json:"file_name"`
	URL          string `json:"url"`
	FileURL      string `json:"file_url"`
	Title        string `json:"title"`
}

// Comment is a task comment ("note" in the Sync API).
type Comment struct {
	ID             string      `json:"id"`
	TaskID         string      `json:"item_id"`
	Content        string      `json:"content"`
	PostedAt       string      `json:"posted_at"`
	IsDeleted      bool        `json:"is_deleted"`
	FileAttachment *Attachment `json:"file_attachment"`
}

// Posted returns the time of the comment in local time.
func (c Comment) Posted() time.Time {
	t, _ := time.Parse(time.RFC3339, c.PostedAt)
	return t.Local()
}
