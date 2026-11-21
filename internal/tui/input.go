package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// inputMode is the use of the one-line input in the bottom bar.
type inputMode int

const (
	inputNone inputMode = iota
	inputAdd
	inputAddNote
	inputFind
	inputQuery
	inputRename
	inputAddSection
	inputRenameSection
	inputAddProject
	inputRenameProject
	inputAddSubtask
	inputAddSubnote
	inputAddLabel
	inputRenameLabel
)

func (m Model) updateInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.comp != nil && m.dlgField == 0 {
		if nm, cmd, ok := m.compKeys(msg); ok {
			return nm, cmd
		}
	}
	if m.hasDescField(m.inputMode) {
		switch msg.String() {
		case "tab", "shift+tab":
			next := m.setDialogField(1 - m.dlgField)
			return m, next
		case "ctrl+enter", "ctrl+s": // ctrl+s for terminals that send ctrl+enter as enter
			return m.submitInput()
		case "esc":
		default:
			if m.dlgField == 1 { // the description is a markdown editor
				return m.editKeys(m.dlgDesc, msg, bodyWidth(m.dialogDescWidth()), m.dlgDescH/2)
			}
			switch msg.String() {
			case "ctrl+c", "ctrl+x", "ctrl+v":
				return m.dialogClipboard(msg.String())
			}
		}
	}
	switch msg.String() {
	case "esc":
		if m.inputMode == inputFind {
			m.find = ""
			m.buildRows(true)
		}
		m.closeInput()
		return m, nil
	case "enter":
		return m.submitInput()
	}
	var cmd tea.Cmd
	if isDialog(m.inputMode) {
		*m.dlg, cmd = m.dlg.Update(msg)
		m.updateCompletion()
		return m, cmd
	}
	*m.input, cmd = m.input.Update(msg)
	if m.inputMode == inputFind {
		m.find = m.input.Value()
		m.buildRows(true)
	}
	return m, cmd
}

// submitInput closes the input and runs the action of its mode with the typed text.
func (m Model) submitInput() (tea.Model, tea.Cmd) {
	var val string
	if isDialog(m.inputMode) {
		val = m.dialogValue()
	} else {
		val = strings.TrimSpace(m.input.Value())
	}
	desc, hasDesc := "", m.hasDescField(m.inputMode)
	if hasDesc {
		desc = m.dialogDesc()
	}
	mode := m.inputMode
	m.closeInput()
	switch mode {
	case inputAdd:
		if val != "" {
			next := withRetryDesc(m.quickAdd(val, desc), inputAdd, val, desc)
			return m, next
		}
	case inputAddNote:
		if val != "" {
			next := m.addNote(val)
			if next != nil {
				next = withRetry(next, inputAddNote, val)
			}
			return m, next
		}
	case inputRename:
		var d *string
		if hasDesc {
			d = &desc
		}
		next := m.rename(val, d)
		return m, next
	case inputAddSection:
		next := m.addSection(val)
		return m, next
	case inputAddProject:
		next := m.startNewProject(val)
		return m, next
	case inputAddSubtask:
		next := m.addSubtask(val, desc)
		return m, next
	case inputAddSubnote:
		if val != "" {
			next := m.addSubnote(val)
			if next != nil {
				next = withRetry(next, inputAddSubnote, val)
			}
			return m, next
		}
	case inputAddLabel:
		next := m.addLabel(val)
		return m, next
	case inputRenameLabel:
		next := m.renameLabel(val)
		return m, next
	case inputRenameProject:
		next := m.renameProject(val)
		return m, next
	case inputRenameSection:
		next := m.renameSection(val)
		return m, next
	case inputQuery:
		if val == "" && m.filterQuery != "" {
			// An empty query removes the filter view from the sidebar.
			m.clearFilter()
			return m, nil
		}
		if val != "" {
			m.focusFilterNav = true
			m.setStatus("filtering…", false)
			return m, m.runFilter(val)
		}
	case inputFind:
		m.find = val
		m.buildRows(true)
	}
	return m, nil
}

// openInput opens the find input in the bottom bar, or the dialog for the other modes.
func (m *Model) openInput(mode inputMode, prompt, placeholder, value string) tea.Cmd {
	if isDialog(mode) {
		return m.openDialog(mode, value)
	}
	m.inputMode = mode
	m.input.Prompt = prompt
	m.input.Placeholder = placeholder
	m.input.SetValue(value)
	m.input.CursorEnd()
	return m.input.Focus()
}

// openAdd opens the dialog for a new task, a new note, a new name, or a due date.
func (m *Model) openAdd(mode inputMode, text string) tea.Cmd {
	return m.openDialog(mode, text)
}

// withRetry makes a failed add reopen the input with the typed text.
func withRetry(cmd tea.Cmd, mode inputMode, text string) tea.Cmd {
	return withRetryDesc(cmd, mode, text, "")
}

// withRetryDesc makes a failed save reopen the task dialog with the typed name and description.
func withRetryDesc(cmd tea.Cmd, mode inputMode, text, desc string) tea.Cmd {
	return func() tea.Msg {
		msg := cmd()
		if a, ok := msg.(actionMsg); ok && a.err != nil {
			a.retryMode, a.retryText, a.retryDesc = mode, text, desc
			return a
		}
		return msg
	}
}

func (m *Model) closeInput() {
	m.inputMode = inputNone
	if m.input != nil {
		m.input.Blur()
		m.input.SetValue("")
	}
	m.dlg, m.dlgDesc, m.comp, m.compHide = nil, nil, nil, 0
	m.dlgField = 0
}

// notesMode reports whether the current project is in notebook view.
// startRename opens the input with the name of the task under the cursor.
func (m *Model) startRename() tea.Cmd {
	t := m.currentTask()
	if t == nil {
		return nil
	}
	m.targetID = t.ID
	return m.openTaskDialog(inputRename, t.Content, t.Description)
}
