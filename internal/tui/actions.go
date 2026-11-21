package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// namesProject reports whether text contains "#<name>" for an existing project, which
// Todoist's quick add would parse. Other "#" uses (e.g. "issue #42") stay in the content.
func (m Model) namesProject(text string) bool {
	lower := strings.ToLower(text)
	for _, p := range m.snap.Projects {
		tag := "#" + strings.ToLower(p.Name)
		for i := strings.Index(lower, tag); i >= 0; {
			end := i + len(tag)
			if end == len(lower) || strings.ContainsRune(" \t,.;:!?", rune(lower[end])) {
				return true
			}
			next := strings.Index(lower[end:], tag)
			if next < 0 {
				break
			}
			i = end + next
		}
	}
	return false
}

// quickAdd creates a task with natural-language parsing.
// In a project view, the task goes to that project and to the section under the cursor,
// unless the text names a different #project.
// In Today or Upcoming, the task gets the date of that day, unless the text sets a date.
// A description that is not empty is saved as typed.
func (m *Model) quickAdd(text, desc string) tea.Cmd {
	var projectID, sectionID, date string
	if cur := m.currentNav(); cur != nil {
		switch {
		case cur.kind == vkProject && !m.namesProject(text):
			projectID = cur.projectID
			sectionID = m.cursorSection()
		case cur.kind == vkToday:
			date = time.Now().Format("2006-01-02")
		case cur.kind == vkUpcoming:
			date = m.cursorDate()
		}
	}
	client, projects := m.client, m.projects
	return m.write("", func(ctx context.Context) (string, error) {
		t, err := client.QuickAddOn(ctx, text, date) // date only if the text has no date
		if err != nil {
			return "", err
		}
		if projectID != "" && (t.ProjectID != projectID || sectionID != "") {
			if err := client.Move(ctx, t.ID, projectID, sectionID); err != nil {
				return "", fmt.Errorf("added, but move failed: %w", err)
			}
			t.ProjectID = projectID
		}
		if desc != "" {
			if _, err := client.UpdateTask(ctx, t.ID, map[string]any{"description": desc}); err != nil {
				return "", fmt.Errorf("added, but the description was not saved: %w", err)
			}
		}
		where := ""
		if p := projects[t.ProjectID]; p != nil {
			where = " → #" + p.Name
		}
		return "Added “" + t.Content + "”" + where, nil
	})
}

// addNote creates a note with a literal title (no date/#project parsing).
func (m *Model) addNote(title string) tea.Cmd {
	cur := m.currentNav()
	if cur == nil || cur.kind != vkProject {
		return nil
	}
	client, pid, sid := m.client, cur.projectID, m.cursorSection()
	return m.simpleWrite("Added note “"+title+"” · E to write", func(ctx context.Context) error {
		_, err := client.AddTask(ctx, title, pid, sid)
		return err
	})
}

// rename saves a new name. In task view, Todoist parses the text as in quick add:
// a date, #project, /section, @label or p1–p4 in the text changes that field, and the
// other fields stay the same. In notebook view, the name is saved as typed.
// desc is the new description, or nil if the dialog had no description field.
func (m *Model) rename(text string, desc *string) tea.Cmd {
	id := m.targetID
	var old *todoist.Task
	for i := range m.snap.Tasks {
		if m.snap.Tasks[i].ID == id {
			old = &m.snap.Tasks[i]
		}
	}
	if text == "" || old == nil {
		return nil
	}
	descChanged := desc != nil && *desc != strings.TrimSpace(old.Description)
	if text == old.Content && !descChanged {
		return nil
	}
	retryDesc := old.Description
	if desc != nil {
		retryDesc = *desc
	}
	client, cur := m.client, *old
	if text == old.Content { // only the description changed: no parse
		cmd := m.simpleWrite("Description saved", func(ctx context.Context) error {
			_, err := client.UpdateTask(ctx, id, map[string]any{"description": *desc})
			return err
		})
		return withRetryDesc(cmd, inputRename, text, retryDesc)
	}
	if m.notesMode() {
		cmd := m.simpleWrite("Renamed to “"+text+"”", func(ctx context.Context) error {
			_, err := client.UpdateTask(ctx, id, map[string]any{"content": text})
			return err
		})
		return withRetry(cmd, inputRename, text)
	}
	projects, sections := m.projects, m.sections
	var newDesc *string
	if descChanged {
		newDesc = desc
	}
	st := m.st // ApplyName only reads the projects and the sections
	cmd := m.write("", func(ctx context.Context) (string, error) {
		p, fields, err := client.ApplyName(ctx, &st, cur, text, newDesc)
		if err != nil {
			return "", err
		}
		changes := []string{"Renamed to “" + p.Content + "”"}
		if newDesc != nil {
			changes = append(changes, "description saved")
		}
		if _, ok := fields["priority"]; ok {
			changes = append(changes, fmt.Sprintf("P%d", 5-p.Priority))
		}
		if len(p.Labels) > 0 {
			changes = append(changes, "@"+strings.Join(p.Labels, " @"))
		}
		moved := (p.ProjectID != "" && p.ProjectID != cur.ProjectID) || (p.SectionID != "" && p.SectionID != cur.Section())
		if pr := projects[p.ProjectID]; pr != nil && moved {
			where := "#" + pr.Name
			if s := sections[p.SectionID]; s != nil {
				where += " / " + s.Name
			}
			changes = append(changes, "→ "+where)
		}
		return strings.Join(changes, " · "), nil
	})
	return withRetryDesc(cmd, inputRename, text, retryDesc)
}

// startDue opens the date dialog with the due string of the task under the cursor.
// The due string holds the recurrence too, for example "every Thursday at 12:00".
func (m *Model) startDue() tea.Cmd {
	t := m.currentTask()
	if t == nil {
		return nil
	}
	text := ""
	if t.Due != nil {
		text = t.Due.String
	}
	return m.openCalendar(t, text)
}

// setPriority sets p1 to p4 on the task under the cursor.
func (m *Model) setPriority(p int) tea.Cmd {
	t := m.taskByID(m.currentTaskID())
	if t == nil {
		return nil
	}
	if t.UIPriority() == p {
		m.setStatus(fmt.Sprintf("already P%d", p), false)
		return nil
	}
	id, client := t.ID, m.client
	t.Priority = 5 - p // show the new color at once. The sync confirms it.
	return m.simpleWrite(fmt.Sprintf("Priority → P%d", p), func(ctx context.Context) error {
		_, err := client.UpdateTask(ctx, id, map[string]any{"priority": 5 - p})
		return err
	})
}

func editTitleFor(t *todoist.Task, notes bool) string {
	if notes {
		return "Note · " + plain(t.Content)
	}
	return "Description · " + plain(t.Content)
}

// completeTask completes the task under the cursor. It removes a one-time task from the list at once.
func (m Model) completeTask() (tea.Model, tea.Cmd) {
	next := m.complete()
	return m, next
}

// askDeleteTask asks y/n and then deletes the task under the cursor. The API has no undo
// for a deletion, so the prompt says so.
func (m *Model) askDeleteTask() {
	t := m.currentTask()
	if t == nil {
		return
	}
	id, name, client := t.ID, plain(t.Content), m.client
	text := "Delete “" + name + "”? This cannot be undone."
	if n := len(m.snap.Comments[id]); n > 0 {
		text = fmt.Sprintf("Delete “%s” and its %d comment(s)? This cannot be undone.", name, n)
	}
	subtree := m.subtreeIDs(id)
	if n := len(subtree) - 1; n > 0 && m.notesMode() { // Todoist deletes the sub-notes too
		text = fmt.Sprintf("Delete “%s” and its %s? This cannot be undone.", name, plural(n, "sub-note"))
	}
	del := func(m *Model) tea.Cmd {
		for _, x := range subtree[1:] { // Todoist deletes the sub-tasks with the task
			m.removeTask(x)
		}
		m.removeTask(id)
		return m.simpleWrite("Deleted “"+name+"”", func(ctx context.Context) error {
			return client.Delete(ctx, id)
		})
	}
	if t.Due == nil || !t.Due.IsRecurring {
		m.confirm = yesNo("Delete task", text, "Delete", del)
		return
	}
	// A recurring task: skip only this date, or delete the task with all its dates.
	task := *t
	date := todoist.FormatDue(t.Due, time.Now())
	if !todoist.CanSkip(t.Due.String) { // godoist cannot find the next date of this rule
		m.confirm = &confirmPrompt{title: "Delete recurring task",
			text: fmt.Sprintf("“%s” repeats %s. godoist cannot find the next date of this repeat, so it cannot skip only this date. Delete the task with all its dates? A deletion cannot be undone.",
				name, t.Due.String),
			buttons: []confirmButton{
				{key: "d", label: "Delete all", danger: true, run: del},
				{key: "n", label: "Cancel"},
			}}
		return
	}
	m.confirm = &confirmPrompt{title: "Delete recurring task",
		text: fmt.Sprintf("“%s” repeats %s. Skip only this date (%s), or delete the task with all its dates? A deletion cannot be undone.",
			name, t.Due.String, date),
		buttons: []confirmButton{
			{key: "s", label: "Skip this date", run: func(m *Model) tea.Cmd { return m.skipDates([]todoist.Task{task}) }},
			{key: "d", label: "Delete all", danger: true, run: del},
			{key: "n", label: "Cancel"},
		}}
}

// skipDates moves recurring tasks to their next dates and keeps their repeats.
func (m *Model) skipDates(ts []todoist.Task) tea.Cmd {
	client := m.client
	return m.write("", func(ctx context.Context) (string, error) {
		var last *todoist.Due
		for _, t := range ts {
			next, err := client.SkipOccurrence(ctx, t)
			if err != nil {
				return "", fmt.Errorf("skip “%s”: %w", plain(t.Content), err)
			}
			last = next
		}
		if len(ts) == 1 {
			return "Skipped to " + todoist.FormatDue(last, time.Now()) + " · repeats " + ts[0].Due.String, nil
		}
		return fmt.Sprintf("Skipped the dates of %d recurring task(s)", len(ts)), nil
	})
}

// closedTask is the name and the project of a task on the undo stack, for the flight of
// ctrl+z to the project. inPlace is true for a sub-task that stays under its open task.
type closedTask struct {
	name, projectID string
	inPlace         bool
}

// pushClosed puts the one-time tasks of one completion on the undo stack. inPlace has the
// sub-tasks that stay under their open tasks.
func (m *Model) pushClosed(ts []*todoist.Task, inPlace map[string]bool) {
	if len(ts) == 0 {
		return
	}
	if m.closedTask == nil {
		m.closedTask = map[string]closedTask{}
	}
	ids := make([]string, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
		m.closedTask[t.ID] = closedTask{t.Content, t.ProjectID, inPlace[t.ID]}
	}
	m.closed = append(m.closed, ids)
}

// complete is completeTask for callers that hold a *Model, such as a confirm prompt.
func (m *Model) complete() tea.Cmd {
	t := m.currentTask()
	if t == nil {
		return nil
	}
	id, content, recurring := t.ID, t.Content, t.Due != nil && t.Due.IsRecurring
	switch {
	case m.onDoneSub():
		return m.reopenSub(t)
	case !recurring && m.openParent(t): // the sub-task stays under its task
		return m.completeSub(t)
	}
	// Reopen does not move the date of a recurring task back. Thus undo is only for one-time tasks.
	text := "Completed occurrence of “" + content + "” · next one scheduled"
	var fx tea.Cmd
	if recurring {
		fx = m.startFx(nil, []string{id}, content)
	} else {
		m.pushClosed([]*todoist.Task{t}, nil)
		fx = m.startFx([]string{id}, nil, content)
		text = "Completed “" + content + "” · ctrl+z to undo"
	}
	m.setStatus(text, false)
	client := m.client
	return tea.Batch(fx, m.simpleWrite(text, func(ctx context.Context) error { return client.Close(ctx, id) }))
}

func (m Model) undo() (tea.Model, tea.Cmd) {
	if len(m.closed) == 0 {
		m.setStatus("nothing to undo", false)
		return m, nil
	}
	ids := m.closed[len(m.closed)-1]
	m.closed = m.closed[:len(m.closed)-1]
	var fly []string // the tasks that went to Completed fly back. The others stay in place.
	for _, id := range ids {
		if m.closedTask[id].inPlace {
			m.unstrike(id)
		} else {
			fly = append(fly, id)
		}
	}
	var fx tea.Cmd
	if len(fly) > 0 {
		first := m.closedTask[fly[0]]
		label := first.name
		if len(fly) > 1 {
			label = plural(len(fly), "task")
		}
		fx = m.startReopenFx(fly[0], first.projectID, label, false)
	}
	for _, id := range ids {
		delete(m.closedTask, id)
	}
	client := m.client
	done := "Reopened task"
	if len(ids) > 1 {
		done = fmt.Sprintf("Reopened %d tasks", len(ids))
	}
	next := m.simpleWrite(done, func(ctx context.Context) error {
		for _, id := range ids {
			if err := client.Reopen(ctx, id); err != nil {
				return err
			}
		}
		return nil
	})
	return m, tea.Batch(fx, next)
}
