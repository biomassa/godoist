package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// completedDays is how far back the Completed view goes.
const completedDays = 30

// loadCompleted fetches the tasks completed in the last completedDays days.
func (m *Model) loadCompleted() tea.Cmd {
	if m.completedLoading {
		return nil
	}
	m.completedLoading = true
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		now := time.Now()
		ts, err := client.CompletedTasks(ctx, now.AddDate(0, 0, -completedDays), now.Add(time.Minute))
		return completedMsg{tasks: ts, err: err}
	}
}

// inCompleted reports whether the Completed view is open.
func (m Model) inCompleted() bool {
	cur := m.currentNav()
	return cur != nil && cur.kind == vkCompleted
}

// completedKeys handles keys in the Completed view: x reopens the task, other task keys
// are blocked. ok is false for keys that the normal handler must process.
func (m Model) completedKeys(key string) (tea.Model, tea.Cmd, bool) {
	if !m.inCompleted() || !taskKey(key) && key != "s" && key != "S" && key != "A" && key != "[" && key != "]" && key != ">" && key != "<" && key != "z" {
		return m, nil, false
	}
	t := m.currentTask()
	if t == nil {
		return m, nil, false
	}
	if key != "x" && key != "space" {
		m.setStatus("reopen the task first (x)", true)
		return m, nil, true
	}
	id, name, client := t.ID, plain(t.Content), m.client
	m.completed = removeByID(m.completed, id)
	m.buildRows(false)
	next := m.simpleWrite("Reopened “"+name+"”", func(ctx context.Context) error { return client.Reopen(ctx, id) })
	return m, next, true
}

// removeByID returns ts without the task id.
func removeByID(ts []todoist.Task, id string) []todoist.Task {
	out := ts[:0:0]
	for _, t := range ts {
		if t.ID != id {
			out = append(out, t)
		}
	}
	return out
}

// completedLabel formats a completion time: "Today 14:05", "Yesterday 09:10", "23 Sep".
func completedLabel(s string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	t = t.Local()
	d, today := dayOf(t), dayOf(now)
	switch {
	case d.Equal(today):
		return "Today " + t.Format("15:04")
	case d.Equal(today.AddDate(0, 0, -1)):
		return "Yesterday " + t.Format("15:04")
	case t.Year() == now.Year():
		return t.Format("2 Jan")
	}
	return t.Format("2 Jan 2006")
}
