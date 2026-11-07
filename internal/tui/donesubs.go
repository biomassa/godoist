package tui

import (
	"context"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// A completed sub-task of an open task stays under its task, dimmed and struck through,
// and does not go to Completed. x reopens it, and the other keys that change a task only
// show a hint. The tree views (project view, notebook view, All tasks, and labels) show
// these sub-tasks. They come from the tasks completed in the last todoist.DoneDays days.
// When the top task is completed too, its sub-tasks show under it in Completed.

// localTimeout is how long a completion or a reopen of this session stays on the screen
// when the synced lists do not show it yet.
const localTimeout = 2 * time.Minute

// doneHint is the status on a key that would change a completed sub-task.
func (m Model) doneHint() string {
	if m.notesMode() {
		return "reopen the sub-note first (x)"
	}
	return "reopen the sub-task first (x)"
}

// localDone is a task that this session completed or reopened. seq is the number of
// syncs that started before the change: only a later sync can show it.
type localDone struct {
	task todoist.Task
	at   time.Time
	seq  int
}

// mergeDone puts the completions and the reopens of this session into the snapshot until
// the synced replica shows them, and then makes the index of the completed sub-tasks. It
// can run more than one time on the same snapshot.
func (m *Model) mergeDone() {
	now := time.Now()
	active, done := map[string]bool{}, map[string]bool{}
	for _, t := range m.st.Tasks {
		active[t.ID] = true
	}
	for _, t := range m.st.Done {
		done[t.ID] = true
	}
	for id, l := range m.doneLocal {
		if m.stSeq > l.seq && done[id] && !active[id] || now.Sub(l.at) > localTimeout {
			delete(m.doneLocal, id)
		}
	}
	for id, l := range m.reopenLocal {
		if m.stSeq > l.seq && active[id] && !done[id] || now.Sub(l.at) > localTimeout {
			delete(m.reopenLocal, id)
		}
	}
	m.snap.Tasks = slices.DeleteFunc(m.snap.Tasks, func(t todoist.Task) bool { _, ok := m.doneLocal[t.ID]; return ok })
	m.snap.Done = slices.DeleteFunc(m.snap.Done, func(t todoist.Task) bool {
		_, ok := m.reopenLocal[t.ID]
		_, again := m.doneLocal[t.ID]
		return ok || again
	})
	inSnap := map[string]bool{}
	for _, t := range m.snap.Tasks {
		inSnap[t.ID] = true
	}
	for id, l := range m.reopenLocal {
		if !inSnap[id] {
			m.snap.Tasks = append(m.snap.Tasks, l.task)
		}
	}
	for _, l := range m.doneLocal {
		m.snap.Done = append(m.snap.Done, l.task)
	}
	m.indexDone()
}

// initLocal makes the maps of the completions and the reopens of this session.
func (m *Model) initLocal() {
	if m.doneLocal == nil {
		m.doneLocal, m.reopenLocal = map[string]localDone{}, map[string]localDone{}
	}
}

// indexDone finds the completed sub-tasks that show under an open task: their parent is
// open, or is such a completed sub-task.
func (m *Model) indexDone() {
	byID := map[string]*todoist.Task{}
	for i := range m.snap.Done {
		m.snap.Done[i].Checked = true
		byID[m.snap.Done[i].ID] = &m.snap.Done[i]
	}
	active := map[string]bool{}
	for _, t := range m.snap.Tasks {
		active[t.ID] = true
	}
	in := map[string]bool{}
	var under func(t *todoist.Task, depth int) bool
	under = func(t *todoist.Task, depth int) bool {
		p := t.Parent()
		switch {
		case p == "" || depth > 50:
			return false
		case active[p]:
			return true
		case byID[p] != nil:
			return under(byID[p], depth+1)
		}
		return false
	}
	m.doneKids = map[string][]*todoist.Task{}
	for _, t := range byID {
		if under(t, 0) {
			in[t.ID] = true
			m.doneKids[t.Parent()] = append(m.doneKids[t.Parent()], t)
		}
	}
	m.doneIn = in
}

// withDoneKids adds the completed sub-tasks to children: under each task in ids, and
// under each other.
func (m Model) withDoneKids(children map[string][]*todoist.Task, ids []string) {
	for queue := ids; len(queue) > 0; queue = queue[1:] {
		for _, k := range m.doneKids[queue[0]] {
			children[queue[0]] = append(children[queue[0]], k)
			queue = append(queue, k.ID)
		}
	}
}

// countDone counts the completed tasks in the subtree of id.
func countDone(children map[string][]*todoist.Task, id string) int {
	n := 0
	for _, t := range children[id] {
		if t.Checked {
			n++
		}
		n += countDone(children, t.ID)
	}
	return n
}

// subCount is the count after a task with sub-tasks: "1/3" (completed and all sub-tasks)
// if some are completed, or else the number of hidden sub-tasks of a collapsed task.
func subCount(r row) string {
	switch {
	case r.subDone > 0:
		return fmt.Sprintf("%d/%d", r.subDone, r.subAll)
	case r.collapsed && r.hidden > 0:
		return fmt.Sprintf("%d", r.hidden)
	}
	return ""
}

// openParent reports whether t is a sub-task of an open task.
func (m *Model) openParent(t *todoist.Task) bool {
	return t.Parent() != "" && m.taskByID(t.Parent()) != nil
}

// anyTask returns an open task or a completed sub-task.
func (m *Model) anyTask(id string) *todoist.Task {
	if t := m.taskByID(id); t != nil {
		return t
	}
	for i := range m.snap.Done {
		if m.snap.Done[i].ID == id {
			return &m.snap.Done[i]
		}
	}
	return nil
}

// strikeTasks completes the tasks ids in place: they leave the open tasks and show
// struck through under their open task.
func (m *Model) strikeTasks(ids []string) {
	m.initLocal()
	now := time.Now()
	for _, id := range ids {
		for _, x := range m.subtreeIDs(id) { // Todoist completes the sub-tasks too
			t := m.taskByID(x)
			if t == nil {
				continue
			}
			d := *t
			d.Checked, d.CompletedAt = true, now.UTC().Format(time.RFC3339)
			m.doneLocal[x] = localDone{d, now, m.syncSeq}
			delete(m.reopenLocal, x)
		}
	}
	m.mergeDone()
	m.buildNav()
	m.buildRows(false)
}

// unstrike makes the completed sub-task id open again in its place.
func (m *Model) unstrike(id string) {
	t := m.anyTask(id)
	if t == nil || !t.Checked {
		return
	}
	m.initLocal()
	r := *t
	r.Checked, r.CompletedAt = false, ""
	delete(m.doneLocal, id)
	m.reopenLocal[id] = localDone{r, time.Now(), m.syncSeq}
	m.mergeDone()
	m.buildNav()
	m.buildRows(false)
}

// completeSub completes t, a one-time sub-task of an open task, in place.
func (m *Model) completeSub(t *todoist.Task) tea.Cmd {
	id, content := t.ID, t.Content
	m.pushClosed([]*todoist.Task{t}, map[string]bool{id: true})
	m.strikeTasks([]string{id})
	text := "Completed “" + content + "” · x to reopen"
	m.setStatus(text, false)
	client := m.client
	return m.simpleWrite(text, func(ctx context.Context) error { return client.Close(ctx, id) })
}

// reopenSub reopens t, a completed sub-task under its open task, in place.
func (m *Model) reopenSub(t *todoist.Task) tea.Cmd {
	id, name := t.ID, plain(t.Content)
	m.unstrike(id)
	client := m.client
	return m.simpleWrite("Reopened “"+name+"”", func(ctx context.Context) error { return client.Reopen(ctx, id) })
}

// onDoneSub reports whether the cursor is on a completed sub-task under its open task.
func (m *Model) onDoneSub() bool {
	r := m.currentRow()
	return r != nil && r.doneSub && !m.inCompleted()
}

// doneSubKeys handles the keys on a completed sub-task under its open task: x and space
// reopen it, and the keys that change a task show a hint. ok is false for other keys.
func (m Model) doneSubKeys(key string, detail bool) (tea.Model, tea.Cmd, bool) {
	if !m.onDoneSub() {
		return m, nil, false
	}
	switch key {
	case "x", "space":
		next := m.complete()
		return m, next, true
	case "s", "S", "[", "]", ">", "<", "A", "z":
	case "d", "enter":
		k := m.chkCur - m.noteChecks()
		subnote := m.chkCur >= 0 && k >= 0 && k < m.noteSubnotes() // enter opens a sub-note
		if !detail || key == "enter" && (!m.notesMode() || subnote) {
			return m, nil, false
		}
	default:
		if !taskKey(key) {
			return m, nil, false
		}
	}
	m.setStatus(m.doneHint(), true)
	return m, nil, true
}

// completedTree arranges the Completed view tasks: a sub-task of an open task does not
// show, and a sub-task of a completed task goes under it. It returns the top tasks and
// the sub-tasks of each task.
func (m *Model) completedTree() ([]todoist.Task, map[string][]*todoist.Task) {
	in := map[string]bool{}
	for _, t := range m.completed {
		in[t.ID] = true
	}
	var top []todoist.Task
	kids := map[string][]*todoist.Task{}
	for i := range m.completed {
		t := &m.completed[i]
		switch p := t.Parent(); {
		case p != "" && (m.taskByID(p) != nil || m.doneIn[t.ID]): // under its open task
		case p != "" && in[p]:
			kids[p] = append(kids[p], t)
		default:
			top = append(top, *t)
		}
	}
	for _, ks := range kids {
		slices.SortStableFunc(ks, func(a, b *todoist.Task) int { return a.ChildOrder - b.ChildOrder })
	}
	return top, kids
}

// parentNamesMsg brings the names of tasks that the Completed view shows after their
// completed sub-tasks.
type parentNamesMsg map[string]string

// parentName is the name of the task above a completed top row of the Completed view, or
// "". It looks in the open tasks, the completed tasks of the last todoist.DoneDays days,
// and the names that godoist got from Todoist.
func (m *Model) parentName(id string) string {
	if t := m.anyTask(id); t != nil {
		return t.Content
	}
	return m.parentNames[id]
}

// fetchParentNames gets the names of the tasks above the top rows of the Completed view
// that godoist does not know yet. Each name is one request.
func (m *Model) fetchParentNames() tea.Cmd {
	if m.parentAsked == nil {
		m.parentAsked = map[string]bool{}
	}
	top, _ := m.completedTree()
	var ids []string
	for _, t := range top {
		if p := t.Parent(); p != "" && m.parentName(p) == "" && !m.parentAsked[p] {
			m.parentAsked[p] = true
			ids = append(ids, p)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		names := parentNamesMsg{}
		for _, id := range ids {
			if t, err := client.Task(ctx, id); err == nil {
				names[id] = t.Content
			}
		}
		return names
	}
}
