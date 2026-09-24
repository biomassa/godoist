package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// isSelected reports whether a task is in the multi-selection.
func (m Model) isSelected(id string) bool { return m.selected[id] }

// selectedTasks returns the selected tasks that still exist, in the order of the list.
func (m Model) selectedTasks() []*todoist.Task {
	if len(m.selected) == 0 {
		return nil
	}
	var out []*todoist.Task
	seen := map[string]bool{}
	for _, r := range m.rows {
		if r.task != nil && r.preview == "" && m.selected[r.task.ID] && !seen[r.task.ID] {
			seen[r.task.ID] = true
			if t := m.taskByID(r.task.ID); t != nil {
				out = append(out, t)
			}
		}
	}
	return out
}

// toggleSelect adds the task to the selection or removes it.
func (m *Model) toggleSelect(id string) {
	if m.selected == nil {
		m.selected = map[string]bool{}
	}
	if m.selected[id] {
		delete(m.selected, id)
	} else {
		m.selected[id] = true
	}
	m.lastMark = id
}

// selectRange selects the tasks from the last marked task to row i.
func (m *Model) selectRange(i int) {
	from := -1
	for k, r := range m.rows {
		if r.task != nil && r.task.ID == m.lastMark {
			from = k
		}
	}
	if from < 0 {
		from = i
	}
	lo, hi := min(from, i), max(from, i)
	if m.selected == nil {
		m.selected = map[string]bool{}
	}
	for k := lo; k <= hi; k++ {
		if r := m.rows[k]; r.task != nil && r.preview == "" && !r.done {
			m.selected[r.task.ID] = true
		}
	}
}

// clearSelection empties the selection and reports whether it had tasks.
func (m *Model) clearSelection() bool {
	had := len(m.selected) > 0
	m.selected, m.lastMark = nil, ""
	return had
}

// pruneSelection drops selected tasks that no longer exist (completed or deleted).
func (m *Model) pruneSelection() {
	for id := range m.selected {
		if m.taskByID(id) == nil {
			delete(m.selected, id)
		}
	}
}

// selectionKeys handles s, S, and the bulk actions when tasks are selected.
// ok is false for other keys, which act on the cursor task as usual.
func (m Model) selectionKeys(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "s":
		if t := m.currentTask(); t != nil {
			m.toggleSelect(t.ID)
		}
		return m, nil, true
	case "S":
		if m.currentTask() != nil {
			m.selectRange(m.rowCur)
		}
		return m, nil, true
	}
	sel := m.selectedTasks()
	if len(sel) == 0 {
		return m, nil, false
	}
	var next tea.Cmd
	switch key {
	case "x", "space":
		next = m.bulkComplete(sel)
	case "1", "2", "3", "4":
		next = m.bulkPriority(sel, int(key[0]-'0'))
	case "t":
		next = m.openCalendarFor(sel)
	case "m":
		next = m.openMovePickerFor(sel)
	case "@":
		next = m.openLabelPickerFor(sel)
	case "delete", "backspace":
		m.askBulkDelete(sel)
	default:
		return m, nil, false
	}
	return m, next, true
}

// bulkComplete completes the selected tasks. One ctrl+z reopens all one-time tasks of it.
func (m *Model) bulkComplete(sel []*todoist.Task) tea.Cmd {
	var ids, once []string
	for _, t := range sel {
		ids = append(ids, t.ID)
		if t.Due == nil || !t.Due.IsRecurring {
			once = append(once, t.ID)
		}
	}
	if len(once) > 0 {
		m.closed = append(m.closed, once)
	}
	for _, id := range once {
		m.removeTask(id)
	}
	m.clearSelection()
	client := m.client
	done := fmt.Sprintf("Completed %d task(s) · ctrl+z to undo", len(ids))
	if len(once) == 0 {
		done = fmt.Sprintf("Completed %d occurrence(s)", len(ids))
	}
	return m.simpleWrite(done, func(ctx context.Context) error {
		for _, id := range ids {
			if err := client.Close(ctx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// bulkPriority sets one priority on the selected tasks.
func (m *Model) bulkPriority(sel []*todoist.Task, p int) tea.Cmd {
	var ids []string
	for _, t := range sel {
		t.Priority = 5 - p
		ids = append(ids, t.ID)
	}
	client := m.client
	return m.simpleWrite(fmt.Sprintf("Priority → P%d for %d task(s)", p, len(ids)), func(ctx context.Context) error {
		for _, id := range ids {
			if _, err := client.UpdateTask(ctx, id, map[string]any{"priority": 5 - p}); err != nil {
				return err
			}
		}
		return nil
	})
}

// askBulkDelete asks once and then deletes the selected tasks.
func (m *Model) askBulkDelete(sel []*todoist.Task) {
	var ids []string
	for _, t := range sel {
		ids = append(ids, t.ID)
	}
	client := m.client
	text := fmt.Sprintf("Delete %d tasks? This cannot be undone.", len(ids))
	m.confirm = yesNo("Delete tasks", text, "Delete", func(m *Model) tea.Cmd {
		for _, id := range ids {
			m.removeTask(id)
		}
		m.clearSelection()
		return m.simpleWrite(fmt.Sprintf("Deleted %d task(s)", len(ids)), func(ctx context.Context) error {
			for _, id := range ids {
				if err := client.Delete(ctx, id); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

// selectionTitle is the "· N selected" part of the task list title.
func (m Model) selectionTitle() string {
	if n := len(m.selectedTasks()); n > 0 {
		return st(fg(hexAccent)).Bold(true).Render(fmt.Sprintf(" · %d selected", n))
	}
	return ""
}

// labelStates counts, for each label name, how many of the tasks have it.
func labelStates(ts []*todoist.Task) map[string]int {
	n := map[string]int{}
	for _, t := range ts {
		for _, l := range t.Labels {
			n[l]++
		}
	}
	return n
}

// joinIDs is for messages that name several tasks.
func joinIDs(ts []*todoist.Task) []string {
	ids := make([]string, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
	}
	return ids
}
