package tui

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// Sub-notes are the sub-tasks of notes in notebook view. A is a new sub-note, > and <
// nest and unnest, [ ] move among the siblings, m moves a note under another note, to the
// top level, or to another project, and z folds. A mouse drag onto a note nests the note
// under it. A drop at the left edge of the list puts it at the top level. The sub-notes
// move with their note.

// isUnder reports whether task id is below task ancestor in the tree.
func (m Model) isUnder(id, ancestor string) bool {
	for t := m.taskByID(id); t != nil && t.Parent() != ""; t = m.taskByID(t.Parent()) {
		if t.Parent() == ancestor {
			return true
		}
	}
	return false
}

// subtreeIDs returns id and the IDs of all tasks below it.
func (m Model) subtreeIDs(id string) []string {
	out := []string{id}
	for i := 0; i < len(out); i++ {
		for _, t := range m.snap.Tasks {
			if t.Parent() == out[i] {
				out = append(out, t.ID)
			}
		}
	}
	return out
}

// setSubtreePlace puts a task and all tasks below it into a project and a section, so
// that the list shows them together before the next sync.
func (m *Model) setSubtreePlace(id, projectID, sectionID string) {
	for _, x := range m.subtreeIDs(id) {
		if t := m.taskByID(x); t != nil {
			t.ProjectID = projectID
			t.SectionID = nil
			if sectionID != "" {
				sid := sectionID
				t.SectionID = &sid
			}
		}
	}
}

// openSubnoteDialog opens the dialog for a new sub-note of note t.
func (m *Model) openSubnoteDialog(t *todoist.Task) tea.Cmd {
	m.targetID = t.ID
	return m.openDialog(inputAddSubnote, "")
}

// addSubnote adds a note with the title as typed under the note targetID. The parent
// shows its sub-notes, and the cursor goes to the new sub-note when the sync brings it.
func (m *Model) addSubnote(title string) tea.Cmd {
	parent := m.taskByID(m.targetID)
	if parent == nil {
		m.setStatus("the note is gone", true)
		return nil
	}
	pid, projectID, sectionID, client := parent.ID, parent.ProjectID, parent.Section(), m.client
	expand := parent.IsCollapsed
	parent.IsCollapsed = false
	m.buildRows(false)
	newID := new(string)
	m.jumpTo = newID
	return m.simpleWrite("Added sub-note “"+title+"” · E to write", func(ctx context.Context) error {
		t, err := client.AddTask(ctx, title, projectID, sectionID)
		if err != nil {
			return err
		}
		if err := client.MoveToParent(ctx, t.ID, pid); err != nil {
			return err
		}
		*newID = t.ID
		if expand {
			return client.SetCollapsed(ctx, "tasks", pid, false)
		}
		return nil
	})
}

// moveNoteUnder makes note t a sub-note of note parentID, at the end of its sub-notes.
func (m *Model) moveNoteUnder(t *todoist.Task, parentID string) tea.Cmd {
	pt := m.taskByID(parentID)
	switch {
	case pt == nil:
		return nil
	case parentID == t.ID || m.isUnder(parentID, t.ID):
		m.setStatus("a note cannot go under itself or its sub-notes", true)
		return nil
	case t.Parent() == parentID:
		m.setStatus("already a sub-note of “"+plain(pt.Content)+"”", false)
		return nil
	}
	pid := parentID
	t.ParentID = &pid
	t.ChildOrder = 1 << 20 // the end of the new parent's sub-notes
	m.setSubtreePlace(t.ID, pt.ProjectID, pt.Section())
	expand := pt.IsCollapsed
	pt.IsCollapsed = false // show the note in its new place
	m.buildRows(false)
	id, client := t.ID, m.client
	return m.simpleWrite("Moved “"+plain(t.Content)+"” under “"+plain(pt.Content)+"”", func(ctx context.Context) error {
		if err := client.MoveToParent(ctx, id, pid); err != nil {
			return err
		}
		if expand {
			return client.SetCollapsed(ctx, "tasks", pid, false)
		}
		return nil
	})
}

// moveNoteTop makes note t a top-level note of its notebook. It goes after the top-level
// note afterID and into its section, or, if afterID is "", to the end of the section of
// its top-level note.
func (m *Model) moveNoteTop(t *todoist.Task, afterID string) tea.Cmd {
	place := m.rootOf(t)
	if a := m.taskByID(afterID); a != nil {
		place = a
	}
	if t.Parent() == "" && (afterID == "" || afterID == t.ID) {
		m.setStatus("already at the top level", false)
		return nil
	}
	projectID, sectionID := place.ProjectID, place.Section()
	dest := slices.DeleteFunc(m.groupIDs(projectID, sectionID, ""), func(x string) bool { return x == t.ID })
	if i := slices.Index(dest, afterID); i >= 0 {
		dest = slices.Insert(dest, i+1, t.ID)
	} else {
		dest = append(dest, t.ID)
	}
	t.ParentID = nil
	m.setSubtreePlace(t.ID, projectID, sectionID)
	m.setOrder(dest)
	m.buildRows(false)
	id, client := t.ID, m.client
	return m.simpleWrite("Moved “"+plain(t.Content)+"” to the top level", func(ctx context.Context) error {
		if err := client.Move(ctx, id, projectID, sectionID); err != nil {
			return err
		}
		return client.ReorderTasks(ctx, dest)
	})
}

// rootOf returns the top-level note above t, or t itself.
func (m Model) rootOf(t *todoist.Task) *todoist.Task {
	for t.Parent() != "" {
		p := m.taskByID(t.Parent())
		if p == nil {
			break
		}
		t = p
	}
	return t
}

// openNoteParentPicker opens "Move under" for note t: the top level, the other notes of
// the notebook (not t and its sub-notes), and then all projects and sections.
func (m *Model) openNoteParentPicker(t *todoist.Task) tea.Cmd {
	items := []pickItem{{text: "⌂ top level", match: "top level", color: hexMuted, top: true}}
	children := map[string][]*todoist.Task{}
	for i := range m.snap.Tasks {
		x := &m.snap.Tasks[i]
		if x.ProjectID == t.ProjectID {
			children[x.Parent()] = append(children[x.Parent()], x)
		}
	}
	rank := map[string]int{} // the tasks without a section come first
	for i, sid := range m.projectSections(t.ProjectID) {
		rank[sid] = i + 1
	}
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		kids := children[parent]
		slices.SortStableFunc(kids, func(a, b *todoist.Task) int {
			if ra, rb := rank[a.Section()], rank[b.Section()]; ra != rb { // the sections in their order
				return ra - rb
			}
			return a.ChildOrder - b.ChildOrder
		})
		for _, x := range kids {
			if x.ID == t.ID {
				continue // the note and its sub-notes cannot be the new parent
			}
			items = append(items, pickItem{text: "○ " + plain(x.Content), match: strings.ToLower(plain(x.Content)),
				color: hexText, depth: depth, taskID: x.ID})
			walk(x.ID, depth+1)
		}
	}
	walk("", 0)
	moves, _ := m.moveItems(t)
	items = append(items, moves...)
	m.pick = &picker{kind: pickNoteParent, taskIDs: []string{t.ID}, title: "Move under · " + plain(t.Content),
		items: items, filter: newPickerFilter()}
	m.pick.filter.Placeholder = "type to filter notes, projects, and sections"
	m.pick.filter.SetWidth(pickerFilterWidth(m.pickerWidth()))
	return m.pick.filter.Focus()
}

// pickNoteParentEnter applies the choice of "Move under".
func (m *Model) pickNoteParentEnter(it pickItem) tea.Cmd {
	t := m.taskByID(m.pick.taskIDs[0])
	m.pick = nil
	switch {
	case t == nil:
		return nil
	case it.top:
		return m.moveNoteTop(t, "")
	case it.taskID != "":
		return m.moveNoteUnder(t, it.taskID)
	}
	return m.moveTask(t.ID, it.projectID, it.sectionID)
}

// openSubnoteAt opens sub-note k of the reader.
func (m *Model) openSubnoteAt(k int) tea.Cmd {
	d := m.buildDetail(m.detailInnerWidth() - 1)
	if k < 0 || k >= len(d.subnotes) {
		return nil
	}
	return m.openNote(d.subnotes[k].id)
}
