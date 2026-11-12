package tui

import tea "charm.land/bubbletea/v2"

// The help is taller than most terminals, so it scrolls. j/k and the page keys move it,
// esc, ? and q close it. ctrl+c still quits.

// helpWidth is the width of the help box: it spans the task list and the details pane.
func (m Model) helpWidth() int {
	l := m.layout()
	return l.mid.w + l.detail.w
}

// helpMaxOff is the largest scroll offset of the help: the last line is at the bottom.
func (m Model) helpMaxOff() int {
	return max(0, len(helpLines(m.helpWidth()-2))-m.paneHeight())
}

// helpWindow returns the lines of the help that fit in h lines at the current offset.
func (m Model) helpWindow(w, h int) []string {
	lines := helpLines(w)
	off := min(max(0, m.helpOff), max(0, len(lines)-h))
	return lines[off:]
}

// helpKeys handles a key while the help is open.
func (m Model) helpKeys(key string) (tea.Model, tea.Cmd) {
	page := max(1, m.paneHeight()/2)
	m.helpOff = min(max(0, m.helpOff), m.helpMaxOff()) // the terminal may have grown
	switch key {
	case "ctrl+c":
		return m.quit()
	case "esc", "?", "q":
		m.showHelp = false
	case "j", "down":
		m.helpOff++
	case "k", "up":
		m.helpOff--
	case "ctrl+d", "pgdown", "space":
		m.helpOff += page
	case "ctrl+u", "pgup":
		m.helpOff -= page
	case "g", "home":
		m.helpOff = 0
	case "G", "end":
		m.helpOff = m.helpMaxOff()
	}
	m.helpOff = min(max(0, m.helpOff), m.helpMaxOff())
	return m, nil
}
