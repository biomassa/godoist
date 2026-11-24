package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// u (and ctrl+z) undoes the last change. A completion reopens the tasks. A change of the
// priority, the date, the project or section, or the labels writes the old values back.

// maxUndo is the length of the undo stack. Older steps drop off.
const maxUndo = 50

// undoKind is the field that an undo step writes back.
type undoKind int

const (
	undoClose undoKind = iota // a completion: undo reopens the tasks
	undoPriority
	undoDue
	undoMove
	undoLabels
	undoDelete // a waiting delete (deletelater.go): undo cancels it
)

// taskWas holds the fields of a task before a change.
type taskWas struct {
	id, projectID       string
	sectionID, parentID string
	priority            int
	due                 *todoist.Due
	labels              []string
}

// undoStep is one entry of the undo stack.
type undoStep struct {
	kind undoKind
	ids  []string  // undoClose: the tasks that undo reopens
	was  []taskWas // other kinds: the tasks before the change

	deleteN int // undoDelete: the number of the waiting delete
}

// pushUndo puts a step on the undo stack.
func (m *Model) pushUndo(s undoStep) {
	m.undos = append(m.undos, s)
	if len(m.undos) > maxUndo {
		m.undos = m.undos[len(m.undos)-maxUndo:]
	}
}

// rememberTasks puts a step on the undo stack that writes back field kind of ts. Call it
// before the change.
func (m *Model) rememberTasks(kind undoKind, ts []*todoist.Task) {
	if s, ok := undoStepOf(kind, ts); ok {
		m.pushUndo(s)
	}
}

// undoStepOf returns the step that writes back field kind of ts. ok is false if ts has no
// task.
func undoStepOf(kind undoKind, ts []*todoist.Task) (s undoStep, ok bool) {
	var was []taskWas
	for _, t := range ts {
		if t == nil {
			continue
		}
		w := taskWas{id: t.ID, projectID: t.ProjectID, sectionID: t.Section(), parentID: t.Parent(),
			priority: t.Priority, labels: slices.Clone(t.Labels)}
		if t.Due != nil {
			d := *t.Due
			w.due = &d
		}
		was = append(was, w)
	}
	return undoStep{kind: kind, was: was}, len(was) > 0
}

// rememberIDs is rememberTasks for task IDs.
func (m *Model) rememberIDs(kind undoKind, ids []string) {
	m.rememberTasks(kind, m.tasksByID(ids))
}

// tasksByID returns the tasks with these IDs, nil for a task that is gone.
func (m *Model) tasksByID(ids []string) []*todoist.Task {
	ts := make([]*todoist.Task, 0, len(ids))
	for _, id := range ids {
		ts = append(ts, m.taskByID(id))
	}
	return ts
}

// undoChange writes back the old values of a step that is not a completion. The task list
// shows them at once, and the sync confirms them.
// A task that is gone (deleted, or not in the replica any more) is left out.
func (m *Model) undoChange(s undoStep) tea.Cmd {
	var was []taskWas
	for _, w := range s.was {
		t := m.taskByID(w.id)
		if t == nil {
			continue
		}
		was = append(was, w)
		switch s.kind {
		case undoPriority:
			t.Priority = w.priority
		case undoDue:
			t.Due = w.due
		case undoLabels:
			t.Labels = slices.Clone(w.labels)
		}
	}
	if len(was) == 0 {
		m.setStatus("nothing to undo: the task is gone", false)
		return nil
	}
	m.buildRows(false)
	what := map[undoKind]string{undoPriority: "priority", undoDue: "date", undoMove: "move", undoLabels: "labels"}[s.kind]
	done := "Undid the " + what + " change"
	if len(was) > 1 {
		done += fmt.Sprintf(" of %d tasks", len(was))
	}
	client, kind := m.client, s.kind
	return m.simpleWrite(done, func(ctx context.Context) error {
		// One failed task does not stop the others: the step is gone from the stack.
		var errs []error
		for _, w := range was {
			errs = append(errs, writeBack(ctx, client, kind, w))
		}
		return errors.Join(errs...)
	})
}

// writeBack sends the old value of one field of one task to Todoist.
func writeBack(ctx context.Context, client *todoist.Client, kind undoKind, w taskWas) error {
	var err error
	switch kind {
	case undoPriority:
		_, err = client.UpdateTask(ctx, w.id, map[string]any{"priority": w.priority})
	case undoLabels:
		ls := w.labels
		if ls == nil {
			ls = []string{}
		}
		_, err = client.UpdateTask(ctx, w.id, map[string]any{"labels": ls})
	case undoDue:
		err = writeDue(ctx, client, w.id, w.due)
	case undoMove:
		if w.parentID != "" {
			return client.MoveToParent(ctx, w.id, w.parentID)
		}
		err = client.Move(ctx, w.id, w.projectID, w.sectionID)
	}
	return err
}

// writeDue sets the due date of a task to due. A recurring due date gets its date and its
// rule back in one change, so that the repeat does not start again from today.
func writeDue(ctx context.Context, client *todoist.Client, id string, due *todoist.Due) error {
	var err error
	switch {
	case due == nil:
		_, err = client.UpdateTask(ctx, id, map[string]any{"due_string": "no date"})
	case due.IsRecurring:
		err = client.MoveOccurrence(ctx, id, due.Date, due.String, due.Lang)
	case len(due.Date) > len("2006-01-02"):
		_, err = client.UpdateTask(ctx, id, map[string]any{"due_datetime": due.Date})
	default:
		_, err = client.UpdateTask(ctx, id, map[string]any{"due_date": due.Date})
	}
	return err
}
