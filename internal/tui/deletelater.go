package tui

import (
	"context"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// A delete waits deleteDelay before it goes to Todoist, because Todoist cannot bring
// deleted items back. Until then the items are only hidden, and u brings them back. When
// godoist quits with q or ctrl+c, the waiting deletes go at once. A kill or a closed
// terminal in the wait drops them, and the items stay in Todoist.

// deleteDelay is how long u can undo a delete.
const deleteDelay = 10 * time.Second

// undoHint ends the text of each delete dialog.
var undoHint = fmt.Sprintf("u undoes it for %d seconds.", int(deleteDelay.Seconds()))

// hiddenKind is the type of a hidden item.
type hiddenKind int

const (
	hideTask hiddenKind = iota
	hideSection
	hideProject
	hideComment
)

// hiddenItem is an item that a waiting delete hides.
type hiddenItem struct {
	kind hiddenKind
	id   string
}

// laterDelete is a delete that waits for its time.
type laterDelete struct {
	label string // for the status, for example “milk” or "3 tasks"
	hide  []hiddenItem
	send  func(context.Context) error // nil once the delete went

	done      bool // Todoist took the delete
	syncAfter int  // the last sync that may have started before Todoist took it

	// Where the cursor was, so that u puts it back.
	navWas string
	comCur int
}

type (
	deleteNowMsg  struct{ n int } // the wait of delete n ended
	deleteSentMsg struct {
		n   int
		act actionMsg // the result of the write
	}
)

// deleteLater hides the items now and sends the delete after deleteDelay. u in that time
// brings the items back and sends nothing.
func (m *Model) deleteLater(label string, hide []hiddenItem, send func(context.Context) error) tea.Cmd {
	if m.laterDeletes == nil {
		m.laterDeletes = map[int]*laterDelete{}
	}
	m.laterN++
	n := m.laterN
	d := &laterDelete{label: label, hide: hide, send: send, comCur: m.comCur}
	if cur := m.currentNav(); cur != nil {
		d.navWas = navKey(*cur)
	}
	m.laterDeletes[n] = d
	m.pushUndo(undoStep{kind: undoDelete, deleteN: n})
	m.hideDeleted()
	m.buildNav()
	m.buildRows(false)
	m.setStatus(fmt.Sprintf("Deleted %s · u to undo (%d s)", label, int(deleteDelay.Seconds())), false)
	return tea.Tick(deleteDelay, func(time.Time) tea.Msg { return deleteNowMsg{n: n} })
}

// hiddenSets returns the IDs that the waiting deletes hide, by kind. The sub-tasks of a
// hidden task and the sub-projects of a hidden project count too, because Todoist
// deletes them with it. It walks the replica, not m.snap: m.snap has lost the children
// already, after the first hideDeleted.
func (m Model) hiddenSets() map[hiddenKind]map[string]bool {
	sets := map[hiddenKind]map[string]bool{hideTask: {}, hideSection: {}, hideProject: {}, hideComment: {}}
	for _, d := range m.laterDeletes {
		for _, h := range d.hide {
			sets[h.kind][h.id] = true
		}
	}
	for grew := true; grew; { // walk down the trees until nothing new shows up
		grew = false
		for _, p := range m.st.Projects {
			if !sets[hideProject][p.ID] && p.ParentID != nil && sets[hideProject][*p.ParentID] {
				sets[hideProject][p.ID], grew = true, true
			}
		}
		for _, t := range m.st.Tasks {
			if !sets[hideTask][t.ID] && (sets[hideTask][t.Parent()] || sets[hideSection][t.Section()] || sets[hideProject][t.ProjectID]) {
				sets[hideTask][t.ID], grew = true, true
			}
		}
	}
	return sets
}

// hideDeleted removes the items of the waiting deletes from the screen data. The
// replica (m.st) keeps them until the delete goes to Todoist and a sync drops them.
func (m *Model) hideDeleted() {
	if len(m.laterDeletes) == 0 {
		return
	}
	hidden := m.hiddenSets()
	keepTasks := func(ts []todoist.Task) []todoist.Task {
		out := ts[:0:0]
		for _, t := range ts {
			if !hidden[hideTask][t.ID] && !hidden[hideSection][t.Section()] && !hidden[hideProject][t.ProjectID] {
				out = append(out, t)
			}
		}
		return out
	}
	m.snap.Tasks = keepTasks(m.snap.Tasks)
	m.filterTasks = keepTasks(m.filterTasks)
	secs := m.snap.Sections[:0:0]
	for _, s := range m.snap.Sections {
		if !hidden[hideSection][s.ID] && !hidden[hideProject][s.ProjectID] {
			secs = append(secs, s)
		}
	}
	m.snap.Sections = secs
	projects := m.snap.Projects[:0:0]
	for _, p := range m.snap.Projects {
		if !hidden[hideProject][p.ID] {
			projects = append(projects, p)
		}
	}
	m.snap.Projects = projects
	m.projects = map[string]*todoist.Project{}
	for i := range m.snap.Projects {
		m.projects[m.snap.Projects[i].ID] = &m.snap.Projects[i]
	}
	m.sections = map[string]*todoist.Section{}
	for i := range m.snap.Sections {
		m.sections[m.snap.Sections[i].ID] = &m.snap.Sections[i]
	}
	comments := map[string][]todoist.Comment{}
	for taskID, cs := range m.snap.Comments {
		keep := cs[:0:0]
		for _, c := range cs {
			if !hidden[hideComment][c.ID] {
				keep = append(keep, c)
			}
		}
		comments[taskID] = keep
	}
	m.snap.Comments = comments
}

// deleteNow sends delete n when its wait ends.
func (m Model) deleteNow(msg deleteNowMsg) (tea.Model, tea.Cmd) {
	next := m.sendDelete(msg.n)
	return m, next
}

// sendDelete sends delete n as a normal write. It does nothing if u brought the items
// back or the delete went already.
func (m *Model) sendDelete(n int) tea.Cmd {
	d := m.laterDeletes[n]
	if d == nil || d.send == nil {
		return nil
	}
	write := m.simpleWrite("Deleted "+d.label, d.send)
	d.send = nil
	return func() tea.Msg { return deleteSentMsg{n: n, act: write().(actionMsg)} }
}

// deleteSent ends delete n and then handles its write like any other. If the delete
// failed, the items show again. If it worked, they stay hidden until a sync that started
// after it, so that an older sync does not bring them back for a moment.
// The undo step of a sent delete stays on the stack: a late u then says that it is too
// late, and does not undo an older change that the user did not mean.
func (m Model) deleteSent(msg deleteSentMsg) (tea.Model, tea.Cmd) {
	if d := m.laterDeletes[msg.n]; d != nil {
		switch {
		case msg.act.err != nil:
			m.dropDeleteStep(msg.n) // the items are back: nothing to undo
			delete(m.laterDeletes, msg.n)
			msg.act.err = fmt.Errorf("could not delete %s · %w", d.label, msg.act.err)
			m.applyState(m.st)
			m.quitting = false // stay open, so that the user sees that the delete failed
		case msg.act.queued: // the offline queue hides the items from now on
			delete(m.laterDeletes, msg.n)
		default:
			d.done, d.syncAfter = true, m.syncSeq
		}
	}
	return m.Update(msg.act)
}

// dropDeleteStep takes the undo step of delete n off the undo stack.
func (m *Model) dropDeleteStep(n int) {
	m.undos = slices.DeleteFunc(m.undos, func(s undoStep) bool { return s.kind == undoDelete && s.deleteN == n })
}

// withoutHidden returns st without the projects and sections of the waiting deletes, for
// code that reads the replica, such as #project in a task name.
func (m Model) withoutHidden(st todoist.SyncState) todoist.SyncState {
	if len(m.laterDeletes) == 0 {
		return st
	}
	hidden := m.hiddenSets()
	st.Projects = slices.DeleteFunc(slices.Clone(st.Projects), func(p todoist.Project) bool { return hidden[hideProject][p.ID] })
	st.Sections = slices.DeleteFunc(slices.Clone(st.Sections), func(s todoist.Section) bool {
		return hidden[hideSection][s.ID] || hidden[hideProject][s.ProjectID]
	})
	return st
}

// dropSentDeletes forgets the sent deletes that sync seq already shows.
func (m *Model) dropSentDeletes(seq int) {
	for n, d := range m.laterDeletes {
		if d.done && seq > d.syncAfter {
			delete(m.laterDeletes, n)
		}
	}
}

// undoDelete brings the items of delete n back, if the delete did not go yet. Else it
// says that it is too late.
func (m *Model) undoDelete(n int) tea.Cmd {
	d := m.laterDeletes[n]
	if d == nil || d.send == nil {
		m.setStatus("too late: the delete already went to Todoist", true)
		return nil
	}
	delete(m.laterDeletes, n)
	m.applyState(m.st) // the replica still has the items
	if cur := m.currentNav(); d.navWas != "" && (cur == nil || navKey(*cur) != d.navWas) {
		m.selectNav(func(n navItem) bool { return navKey(n) == d.navWas })
		m.buildRows(true)
	}
	m.comCur = d.comCur
	m.setStatus("Restored "+d.label, false)
	return nil
}

// flushDeletes sends all waiting deletes at once, for quit.
func (m *Model) flushDeletes() tea.Cmd {
	var cmds []tea.Cmd
	for n := range m.laterDeletes {
		cmds = append(cmds, m.sendDelete(n))
	}
	return tea.Batch(cmds...)
}

// waitingDeleteIn returns the name of a #project or /section in a quick add text that a
// waiting delete hides, or "". Todoist reads the text and still has them, so the new task
// would go into them and be deleted with them.
func (m Model) waitingDeleteIn(text string) string {
	if len(m.laterDeletes) == 0 {
		return ""
	}
	cur := ""
	if n := m.currentNav(); n != nil && n.kind == vkProject {
		cur = n.projectID
	}
	p := todoist.ParseName(text, m.st.Projects, m.st.Sections, cur)
	hidden := m.hiddenSets()
	if p.SectionID != "" && hidden[hideSection][p.SectionID] {
		for _, s := range m.st.Sections {
			if s.ID == p.SectionID {
				return "/" + s.Name
			}
		}
	}
	if p.ProjectID != "" && p.ProjectID != cur && hidden[hideProject][p.ProjectID] {
		for _, pr := range m.st.Projects {
			if pr.ID == p.ProjectID {
				return "#" + pr.Name
			}
		}
	}
	return ""
}
