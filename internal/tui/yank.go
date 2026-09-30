package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// yank copies text of the task under the cursor to the system clipboard (y). In the task
// list, it copies the name. In the details pane, it copies the selected comment, or the
// description when no comment is selected. The text goes as it is in Todoist, with its
// markdown. The terminal writes the clipboard (OSC 52).
func (m Model) yank(details bool) (tea.Model, tea.Cmd) {
	t := m.currentTask()
	if t == nil {
		m.setStatus("no task under the cursor", false)
		return m, nil
	}
	what, text := "name", t.Content
	if details {
		what, text = "description", t.Description
		if cs := m.snap.Comments[t.ID]; m.comCur >= 0 && m.comCur < len(cs) {
			what, text = "comment", cs[m.comCur].Content
		}
	}
	if strings.TrimSpace(text) == "" {
		m.setStatus("the "+what+" is empty", false)
		return m, nil
	}
	m.setStatus("copied the "+what, false)
	return m, tea.SetClipboard(text)
}
