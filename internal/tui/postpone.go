package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// p postpones the task under the cursor: a dialog moves its date by 1, 2, or 3 days or a
// week, or opens the date dialog. The time of the task stays. An overdue task counts from
// today. A recurring task moves only this date and keeps its repeat.

// postponeDays are the buttons of the postpone dialog that move the date.
var postponeDays = []struct {
	key, label string
	days       int
}{{"1", "1 day", 1}, {"2", "2 days", 2}, {"3", "3 days", 3}, {"7", "A week", 7}}

// postponeTo returns the due time moved by days. An overdue due time counts from today,
// at the same time of day.
func postponeTo(due time.Time, days int, now time.Time) time.Time {
	if dayOf(due).Before(dayOf(now)) {
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

// openPostpone opens the postpone dialog for the task under the cursor. A task without a
// date gets a status message only.
func (m *Model) openPostpone() tea.Cmd {
	t := m.currentTask()
	if t == nil {
		return nil
	}
	due, hasTime, ok := t.Due.Time()
	if !ok {
		m.setStatus("this task has no date", false)
		return nil
	}
	id := t.ID
	var buttons []confirmButton
	for _, d := range postponeDays {
		days := d.days
		buttons = append(buttons, confirmButton{key: d.key, label: d.label,
			run: func(m *Model) tea.Cmd { return m.postpone(id, days) }})
	}
	buttons = append(buttons,
		confirmButton{key: "c", label: "Custom...", newRow: true, run: func(m *Model) tea.Cmd {
			if t := m.taskByID(id); t != nil && t.Due != nil {
				return m.openCalendar(t, t.Due.String)
			}
			return nil
		}},
		confirmButton{key: "n", label: "Cancel"})
	m.confirm = &confirmPrompt{title: "Postpone",
		head:    "“" + plain(t.Content) + "”",
		text:    "due " + dueLabel(due, hasTime),
		buttons: buttons}
	return nil
}

// postpone moves the date of task id by days. The time stays. A recurring task moves only
// this date.
func (m *Model) postpone(id string, days int) tea.Cmd {
	t := m.taskByID(id)
	if t == nil {
		return nil
	}
	due, hasTime, ok := t.Due.Time()
	if !ok {
		m.setStatus("this task has no date", false)
		return nil
	}
	to := postponeTo(due, days, time.Now())
	date, field := to.Format("2006-01-02"), "due_date"
	if hasTime {
		date, field = to.Format("2006-01-02T15:04:05"), "due_datetime"
	}
	text := "Postponed to " + dueLabel(to, hasTime)
	recurring, rule, lang, client := t.Due.IsRecurring, t.Due.String, t.Due.Lang, m.client
	if recurring {
		text += " · repeats " + rule
	}
	return m.write(text, func(ctx context.Context) (string, error) {
		if recurring {
			return "", client.MoveOccurrence(ctx, id, date, rule, lang)
		}
		_, err := client.UpdateTask(ctx, id, map[string]any{field: date})
		return "", err
	})
}
