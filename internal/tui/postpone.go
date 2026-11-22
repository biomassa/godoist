package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// p postpones the task under the cursor, or the selected tasks: a dialog moves the date to
// today, or by 1, 2, or 3 days or a week, or opens the date dialog. The time of the task
// stays. An overdue task and a task without a date count from today. A recurring task
// moves only this date and keeps its repeat.

// postponeDays are the buttons of the postpone dialog that move the date.
var postponeDays = []struct {
	key, label string
	days       int
}{{"t", "Today", 0}, {"1", "1 day", 1}, {"2", "2 days", 2}, {"3", "3 days", 3}, {"7", "A week", 7}}

// postponeTo returns the due time moved by days. An overdue due time counts from today,
// at the same time of day. 0 days is today, also for a due time after today.
func postponeTo(due time.Time, days int, now time.Time) time.Time {
	if days == 0 || dayOf(due).Before(dayOf(now)) {
		due = time.Date(now.Year(), now.Month(), now.Day(), due.Hour(), due.Minute(), due.Second(), 0, time.Local)
	}
	return due.AddDate(0, 0, days)
}

// dueLabel is a due time for the postpone dialog and its status, for example "Fri 6 Nov
// 14:00".
func dueLabel(t time.Time, hasTime bool) string {
	if hasTime {
		return t.Format("Mon 2 Jan 15:04")
	}
	return t.Format("Mon 2 Jan")
}

// baseDue is the due time that postpone counts from: the due time of t, or today without
// a time for a task without a date.
func baseDue(t *todoist.Task, now time.Time) (due time.Time, hasTime, dated bool) {
	if due, hasTime, ok := t.Due.Time(); ok {
		return due, hasTime, true
	}
	return dayOf(now), false, false
}

// openPostpone opens the postpone dialog for the task under the cursor.
func (m *Model) openPostpone() tea.Cmd {
	t := m.currentTask()
	if t == nil {
		return nil
	}
	due, hasTime, dated := baseDue(t, time.Now())
	text := "no date"
	if dated {
		text = "due " + dueLabel(due, hasTime)
	}
	id := t.ID
	m.confirm = &confirmPrompt{title: "Postpone", sel: 1, // enter is 1 day, as before Today came
		head: "“" + plain(t.Content) + "”",
		text: text,
		buttons: postponeButtons(func(m *Model, days int) tea.Cmd { return m.postpone(id, days) },
			func(m *Model) tea.Cmd {
				if t := m.taskByID(id); t != nil {
					s := ""
					if t.Due != nil {
						s = t.Due.String
					}
					return m.openCalendar(t, s)
				}
				return nil
			})}
	return nil
}

// postponeButtons returns the buttons of the postpone dialog. move runs for a day button,
// custom for "Custom...".
func postponeButtons(move func(m *Model, days int) tea.Cmd, custom func(m *Model) tea.Cmd) []confirmButton {
	var buttons []confirmButton
	for _, d := range postponeDays {
		days := d.days
		buttons = append(buttons, confirmButton{key: d.key, label: d.label,
			run: func(m *Model) tea.Cmd { return move(m, days) }})
	}
	return append(buttons,
		confirmButton{key: "c", label: "Custom...", newRow: true, run: custom},
		confirmButton{key: "n", label: "Cancel"})
}

// openBulkPostpone opens the postpone dialog for the selected tasks. Each task moves from
// its own date. The selection stays, as after the other bulk changes.
func (m *Model) openBulkPostpone(ts []*todoist.Task) tea.Cmd {
	ids := make([]string, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
	}
	m.confirm = &confirmPrompt{title: "Postpone", sel: 1,
		text: fmt.Sprintf("%d selected tasks", len(ts)),
		buttons: postponeButtons(func(m *Model, days int) tea.Cmd {
			var sends []func(context.Context) error
			for _, id := range ids {
				if _, send := m.postponeWrite(id, days); send != nil {
					sends = append(sends, send)
				}
			}
			// One write that sends the tasks one by one, like the other bulk changes.
			return m.simpleWrite(fmt.Sprintf("Postponed %d tasks", len(sends)), func(ctx context.Context) error {
				for _, send := range sends {
					if err := send(ctx); err != nil {
						return err
					}
				}
				return nil
			})
		}, func(m *Model) tea.Cmd {
			return m.openCalendarFor(m.selectedTasks())
		})}
	return nil
}

// postpone moves the date of task id by days. The time stays. A recurring task moves only
// this date.
func (m *Model) postpone(id string, days int) tea.Cmd {
	text, send := m.postponeWrite(id, days)
	if send == nil {
		return nil
	}
	return m.simpleWrite(text, send)
}

// postponeWrite returns the status text and the write that move the date of task id by
// days. send is nil if the task is gone.
func (m *Model) postponeWrite(id string, days int) (text string, send func(context.Context) error) {
	t := m.taskByID(id)
	if t == nil {
		return "", nil
	}
	due, hasTime, _ := baseDue(t, time.Now())
	to := postponeTo(due, days, time.Now())
	date, field := to.Format("2006-01-02"), "due_date"
	if hasTime {
		date, field = to.Format("2006-01-02T15:04:05"), "due_datetime"
	}
	text = "Postponed to " + dueLabel(to, hasTime)
	recurring, client := t.Due != nil && t.Due.IsRecurring, m.client
	var rule, lang string
	if recurring {
		rule, lang = t.Due.String, t.Due.Lang
		text += " · repeats " + rule
	}
	return text, func(ctx context.Context) error {
		if recurring {
			return client.MoveOccurrence(ctx, id, date, rule, lang)
		}
		_, err := client.UpdateTask(ctx, id, map[string]any{field: date})
		return err
	}
}
