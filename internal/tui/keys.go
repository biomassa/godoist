package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// propertyKeys handles the keys that change task properties. They work in the
// task list and in the details pane. ok is false for other keys.
func (m Model) propertyKeys(key string) (tea.Model, tea.Cmd, bool) {
	var next tea.Cmd
	switch key {
	case "t":
		next = m.startDue()
	case "p":
		next = m.openPostpone()
	case "1", "2", "3", "4":
		next = m.setPriority(int(key[0] - '0'))
	case "@":
		next = m.openLabelPicker()
	case "m":
		if t := m.currentTask(); t != nil && m.notesMode() {
			next = m.openNoteParentPicker(t)
			break
		}
		next = m.openMovePicker()
	default:
		return m, nil, false
	}
	return m, next, true
}

func (m Model) updateKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.showHelp {
		return m.helpKeys(key)
	}
	switch key {
	case "ctrl+c", "q":
		return m.quit()
	case "?":
		m.showHelp, m.helpOff = true, 0
		return m, nil
	case "T":
		m.openThemePicker()
		return m, nil
	case "{", "}": // the border between the task list and the details pane
		d := paneStep
		if key == "{" {
			d = -paneStep
		}
		return m.moveBorder(d)
	case "r":
		m.setStatus("syncing…", false)
		next := m.startSync()
		return m, next
	case "tab", "shift+tab":
		// In the reader of a note with checkboxes, tab moves between the checkboxes.
		d := 1
		if key == "shift+tab" {
			d = -1
		}
		if m.noteChecks()+m.noteSubnotes() > 0 {
			m.moveCheck(d)
			return m, nil
		}
		m.cycleFocus(d)
		return m, nil
	case "a":
		mode := inputAdd
		if m.notesMode() {
			mode = inputAddNote
		}
		next := m.openAdd(mode, "")
		return m, next
	case "/":
		m.focus = paneTasks
		m.detailOpen = false
		next := m.openInput(inputFind, " / ", "find in this view · enter keep · esc clear", m.find)
		return m, next
	case "f":
		next := m.openDialog(inputQuery, m.filterQuery)
		return m, next
	case "v":
		return m.toggleNotes()
	case "ctrl+z":
		return m.undo()
	}

	switch m.focus {
	case paneNav:
		return m.navKeys(key)
	case paneDetail:
		return m.detailKeys(key)
	}
	return m.taskKeys(key)
}

func (m Model) navKeys(key string) (tea.Model, tea.Cmd) {
	if nm, cmd, ok := m.labelKeys(key); ok {
		return nm, cmd
	}
	if key == "z" {
		if p := m.navProject(); p != nil && m.nav[m.navCur].hasKids {
			next := m.toggleProjectCollapse(p)
			return m, next
		}
		m.setStatus("this item has no sub-projects", false)
		return m, nil
	}
	if nm, cmd, ok := m.projectKeys(key); ok {
		return nm, cmd
	}
	switch key {
	case "esc":
		if m.inFilterView() {
			m.clearFilter()
		}
	case "j", "down":
		m.moveNav(1)
	case "k", "up":
		m.moveNav(-1)
	case "g", "home", "pgup":
		m.moveNav(-len(m.nav))
	case "G", "end", "pgdown":
		m.moveNav(len(m.nav))
	case "ctrl+d":
		m.moveNav(max(1, m.paneHeight()/2))
	case "ctrl+u":
		m.moveNav(-max(1, m.paneHeight()/2))
	case "enter", "l", "right":
		m.focus = paneTasks
	}
	return m, nil
}

func (m Model) taskKeys(key string) (tea.Model, tea.Cmd) {
	if key == "esc" && m.clearSelection() {
		m.setStatus("selection cleared", false)
		return m, nil
	}
	if nm, cmd, ok := m.completedKeys(key); ok {
		return nm, cmd
	}
	if nm, cmd, ok := m.doneSubKeys(key, false); ok {
		return nm, cmd
	}
	if nm, cmd, ok := m.sectionKeys(key); ok {
		return nm, cmd
	}
	if nm, cmd, ok := m.selectionKeys(key); ok {
		return nm, cmd
	}
	if nm, cmd, ok := m.orderKeys(key); ok {
		return nm, cmd
	}
	if m.currentTask() == nil && taskKey(key) {
		m.setStatus("no task under the cursor · move to a task with j/k", true)
		return m, nil
	}
	if nm, cmd, ok := m.propertyKeys(key); ok {
		return nm, cmd
	}
	switch key {
	case "j", "down":
		m.moveRow(1)
	case "k", "up":
		m.moveRow(-1)
	case "g", "home":
		m.moveRow(-len(m.rows))
	case "G", "end":
		m.moveRow(len(m.rows))
	case "ctrl+d", "pgdown":
		m.moveRow(max(1, m.paneHeight()/2))
	case "ctrl+u", "pgup":
		m.moveRow(-max(1, m.paneHeight()/2))
	case "esc":
		switch {
		case m.find != "":
			m.find = ""
			m.buildRows(true)
		case m.inFilterView():
			m.clearFilter()
		default:
			m.focus = paneNav
		}
	case "h", "left":
		m.focus = paneNav
	case "enter", "l", "right":
		if m.currentTask() != nil {
			m.focusDetail()
		}
	case "x", "space":
		return m.completeTask()
	case "delete", "backspace":
		m.askDeleteTask()
		return m, nil
	case "o":
		return m.openLink()
	case "y":
		return m.yank(false)
	case "c":
		if t := m.currentTask(); t != nil {
			next := m.openEditor(editCommentNew, t.ID, "", "New comment · "+plain(t.Content), "")
			return m, next
		}
	case "e":
		next := m.startRename()
		return m, next
	case "E":
		if m.notesMode() { // notebook view: the inline markdown editor
			next := m.openNoteEditor()
			return m, next
		}
		if t := m.currentTask(); t != nil {
			next := m.openEditor(editDescription, t.ID, "", editTitleFor(t, m.notesMode()), t.Description)
			return m, next
		}
	}
	return m, nil
}

// taskKey reports whether key acts on the task under the cursor.
func taskKey(key string) bool {
	switch key {
	case "x", "space", "e", "E", "c", "t", "p", "1", "2", "3", "4", "@", "m", "delete", "backspace":
		return true
	}
	return false
}

func (m Model) detailKeys(key string) (tea.Model, tea.Cmd) {
	if nm, cmd, ok := m.doneSubKeys(key, true); ok {
		return nm, cmd
	}
	if k := m.chkCur - m.noteChecks(); m.chkCur >= 0 && k >= 0 && k < m.noteSubnotes() && key == "enter" {
		next := m.openSubnoteAt(k) // enter on a selected sub-note opens it
		return m, next
	}
	if key == "A" && m.notesMode() { // a sub-note of the note in the reader
		if t := m.currentTask(); t != nil {
			next := m.openSubnoteDialog(t)
			return m, next
		}
	}
	// With checkboxes on screen, space never completes the task: with no checkbox selected,
	// it selects the first one. x still completes.
	if key == "space" && m.noteChecks() > 0 && m.chkCur < m.noteChecks() {
		if m.chkCur < 0 {
			m.moveCheck(1)
			m.setStatus("space toggles the checkbox · tab next · x completes the task", false)
			return m, nil
		}
		next := m.toggleNoteCheck(m.chkCur)
		return m, next
	}
	if (key == "j" || key == "k") && m.chkCur >= 0 {
		m.chkCur = -1 // j/k go back to the comments
	}
	if key == "enter" && m.notesMode() && m.comCur < 0 {
		next := m.openNoteEditor()
		return m, next
	}
	if nm, cmd, ok := m.completedKeys(key); ok { // a completed task: x reopens, other keys wait
		return nm, cmd
	}
	if nm, cmd, ok := m.propertyKeys(key); ok {
		return nm, cmd
	}
	t := m.currentTask()
	var comments []todoist.Comment
	if t != nil {
		comments = m.snap.Comments[t.ID]
	}
	switch key {
	case "h", "left", "esc":
		m.leaveDetail()
	case "j", "down":
		if len(comments) == 0 {
			m.detOff++
		} else if m.comCur < len(comments)-1 {
			m.comCur++
			m.scrollToComment()
		}
	case "k", "up":
		if m.comCur >= 0 {
			m.comCur--
			m.scrollToComment()
		} else {
			m.detOff--
		}
	case "ctrl+d", "pgdown":
		m.detOff += max(1, m.paneHeight()/2)
	case "ctrl+u", "pgup":
		m.detOff -= max(1, m.paneHeight()/2)
	case "g", "home":
		m.detOff, m.comCur = 0, -1
	case "G", "end":
		m.detOff = 1 << 20
		if len(comments) > 0 {
			m.comCur = len(comments) - 1
		}
	case "x", "space":
		return m.completeTask()
	case "o":
		return m.openLink()
	case "y":
		return m.yank(true)
	case "c":
		if t != nil {
			next := m.openEditor(editCommentNew, t.ID, "", "New comment · "+plain(t.Content), "")
			return m, next
		}
	case "e":
		if t == nil {
			return m, nil
		}
		if m.comCur >= 0 && m.comCur < len(comments) {
			cm := comments[m.comCur]
			next := m.openEditor(editCommentEdit, t.ID, cm.ID, "Edit comment · "+plain(t.Content), cm.Content)
			return m, next
		}
		next := m.startRename()
		return m, next
	case "E":
		if m.notesMode() {
			next := m.openNoteEditor()
			return m, next
		}
		if t != nil {
			next := m.openEditor(editDescription, t.ID, "", editTitleFor(t, m.notesMode()), t.Description)
			return m, next
		}
	case "d", "delete", "backspace":
		// del and backspace delete like in the task list: the selected comment, or else
		// the task. d deletes only a comment.
		if t != nil && key != "d" && (m.comCur < 0 || m.comCur >= len(comments)) {
			m.askDeleteTask()
			return m, nil
		}
		if t != nil && m.comCur >= 0 && m.comCur < len(comments) {
			id, client := comments[m.comCur].ID, m.client
			m.confirm = yesNo("Delete comment", "Delete this comment? This cannot be undone.", "Delete", func(m *Model) tea.Cmd {
				m.comCur--
				return m.simpleWrite("Comment deleted", func(ctx context.Context) error { return client.DeleteComment(ctx, id) })
			})
		}
	}
	return m, nil
}
