package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// oneLine makes pasted text fit a one-line field: line breaks and tabs become spaces.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, s)
}

// paste sends pasted text to the field that gets the keys. The terminal sends a paste
// as one message (bracketed paste), not as key presses. Fields with one line get the text
// without line breaks. The description field and the editors keep the line breaks.
func (m Model) paste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	line := tea.PasteMsg{Content: oneLine(msg.Content)}
	var cmd tea.Cmd
	switch {
	case m.quitting, m.menu != nil, m.confirm != nil:
	case m.cal != nil:
		c := m.cal
		switch c.focus {
		case calText:
			before := c.text.Value()
			if before == mixedDue { // pasted text replaces "(mixed)", as typed text does
				c.text.SetValue("")
			}
			c.text, cmd = c.text.Update(line)
			if c.text.Value() != before {
				c.source = srcText
			}
		case calTime:
			before := c.timeIn.Value()
			c.timeIn, cmd = c.timeIn.Update(line)
			if c.timeIn.Value() != before {
				c.source, c.cleared = srcCal, false
			}
		}
	case m.pick != nil:
		p := m.pick
		before := p.filter.Value()
		p.filter, cmd = p.filter.Update(line)
		if p.filter.Value() != before {
			p.cur, p.off = 0, 0
		}
	case m.note != nil:
		e := m.note
		if f := e.search; f != nil {
			switch f.focus {
			case findField:
				f.find, cmd = f.find.Update(line)
				e.refind()
			case replaceField:
				f.repl, cmd = f.repl.Update(line)
			}
			return m, cmd
		}
		if !e.replaceSelection() { // a paste replaces the selected text
			e.push(false)
		}
		e.insertText(msg.Content)
		return m, e.changed()
	case m.hasDescField(m.inputMode) && m.dlgField == 1:
		*m.dlgDesc, cmd = m.dlgDesc.Update(msg)
	case isDialog(m.inputMode):
		*m.dlg, cmd = m.dlg.Update(line)
	case m.inputMode != inputNone:
		*m.input, cmd = m.input.Update(line)
		if m.inputMode == inputFind {
			m.find = m.input.Value()
			m.buildRows(true)
		}
	}
	return m, cmd
}
