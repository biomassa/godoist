package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Text selection in the inline editor. The selection goes from an anchor to the cursor.
// shift with a move key, ctrl+a, or a mouse drag makes a selection. A move key without
// shift removes it. Typed text, enter, backspace, delete, and a paste replace it.
// ctrl+c and ctrl+x write it to the system clipboard through the terminal (OSC 52), and
// ctrl+v reads the clipboard from the terminal.

// selRange returns the selection in text order. ok is false if no text is selected.
func (e *noteEditor) selRange() (r1, c1, r2, c2 int, ok bool) {
	if !e.selOn {
		return 0, 0, 0, 0, false
	}
	r1, c1, r2, c2 = e.selRow, e.selCol, e.row, e.col
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	return r1, c1, r2, c2, r1 != r2 || c1 != c2
}

// inSel reports whether rune col of line row is selected.
func (e *noteEditor) inSel(row, col int) bool {
	r1, c1, r2, c2, ok := e.selRange()
	if !ok || row < r1 || row > r2 {
		return false
	}
	return (row > r1 || col >= c1) && (row < r2 || col < c2)
}

// selLine reports whether line row has selected text. Such lines show as source, so
// that the selection covers the true characters.
func (e *noteEditor) selLine(row int) bool {
	r1, _, r2, _, ok := e.selRange()
	return ok && row >= r1 && row <= r2
}

// selectedText returns the selected text, or "".
func (e *noteEditor) selectedText() string {
	r1, c1, r2, c2, ok := e.selRange()
	if !ok {
		return ""
	}
	if r1 == r2 {
		return string(e.lines[r1][c1:c2])
	}
	parts := []string{string(e.lines[r1][c1:])}
	for r := r1 + 1; r < r2; r++ {
		parts = append(parts, string(e.lines[r]))
	}
	parts = append(parts, string(e.lines[r2][:c2]))
	return strings.Join(parts, "\n")
}

// deleteSelection removes the selected text and puts the cursor where it started.
// It reports whether it removed text. The caller pushes the undo step.
func (e *noteEditor) deleteSelection() bool {
	r1, c1, r2, c2, ok := e.selRange()
	e.selOn = false
	if !ok {
		return false
	}
	tail := append([]rune(nil), e.lines[r2][c2:]...)
	e.lines[r1] = append(e.lines[r1][:c1:c1], tail...)
	e.lines = append(e.lines[:r1+1], e.lines[r2+1:]...)
	e.row, e.col = r1, c1
	return true
}

// selectKeys handles the selection and clipboard keys. handled is false for other keys.
// A move key without shift removes the selection and is not handled here.
func (m Model) selectKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	e := m.note
	key := msg.String()
	extend := func(move func()) {
		if !e.selOn {
			e.selRow, e.selCol, e.selOn = e.row, e.col, true
		}
		move()
		e.typing = false
	}
	w := m.noteWidth()
	switch key {
	case "shift+left":
		extend(e.moveLeft)
	case "shift+right":
		extend(e.moveRight)
	case "shift+up":
		extend(func() { e.moveVertical(-1, w) })
	case "shift+down":
		extend(func() { e.moveVertical(1, w) })
	case "shift+home":
		extend(func() { e.col = 0 })
	case "shift+end":
		extend(func() { e.col = len(e.lines[e.row]) })
	case "alt+shift+left", "ctrl+shift+left":
		extend(e.wordLeft)
	case "alt+shift+right", "ctrl+shift+right":
		extend(e.wordRight)
	case "ctrl+a":
		e.selRow, e.selCol, e.selOn = 0, 0, true
		e.row = len(e.lines) - 1
		e.col = len(e.lines[e.row])
	case "ctrl+c", "ctrl+x":
		text := e.selectedText()
		if text == "" {
			m.setStatus("select text first: shift+arrows, ctrl+a, or a mouse drag", false)
			return m, nil, true
		}
		copyCmd := tea.SetClipboard(text)
		if key == "ctrl+c" {
			m.setStatus("copied", false)
			return m, copyCmd, true
		}
		e.push(false)
		e.deleteSelection()
		m.setStatus("cut", false)
		return m, tea.Batch(copyCmd, e.changed()), true
	case "ctrl+v":
		return m, tea.ReadClipboard, true // the text comes as a ClipboardMsg
	case "left", "right", "up", "down", "home", "end", "pgup", "pgdown",
		"alt+left", "ctrl+left", "alt+right", "ctrl+right":
		e.selOn = false
		return m, nil, false
	default:
		return m, nil, false
	}
	return m, nil, true
}

// replaceSelection removes the selection before an edit that puts text in its place.
// It pushes the undo step, so the caller does not push one again.
func (e *noteEditor) replaceSelection() bool {
	if _, _, _, _, ok := e.selRange(); !ok {
		e.selOn = false
		return false
	}
	e.push(false)
	return e.deleteSelection()
}

// selectionStyle is the look of selected text.
func selectionStyle(s lipgloss.Style) lipgloss.Style {
	return s.Background(c(hexSelBg)).Foreground(c(hexText))
}

// notePosAt returns the source line and rune at screen cell x, y of the editor pane.
// ok is false outside the text.
func (m Model) notePosAt(x, y int) (row, col int, ok bool) {
	r := m.sideRect()
	e := m.note
	sr := r.row(y) - e.textTop(r.h-2)
	if !r.has(x, y) || sr < 0 {
		return 0, 0, false
	}
	rows, _ := m.noteLayout(r.w-4, r.h-2-e.textTop(r.h-2))
	if sr >= len(rows) {
		last := len(e.lines) - 1
		return last, len(e.lines[last]), true
	}
	er := rows[sr]
	if er.cells == nil { // a rendered block: the start of its line
		return er.line, 0, true
	}
	k := x - r.x - 2 // the border and the row margin
	col = len(e.lines[er.line])
	switch {
	case k >= 0 && k < len(er.cells):
		col = er.cells[k].src
	case k < 0 && len(er.cells) > 0:
		col = er.cells[0].src
	case er.source && len(er.cells) > 0 && k >= len(er.cells):
		col = er.cells[len(er.cells)-1].src + 1
	}
	return er.line, min(col, len(e.lines[er.line])), true
}

// noteDrag moves the cursor to the cell under the mouse while the button is down. The
// selection goes from the place of the click to the cursor.
func (m Model) noteDrag(x, y int) (tea.Model, tea.Cmd) {
	e := m.note
	row, col, ok := m.notePosAt(x, y)
	if !ok {
		return m, nil
	}
	e.row, e.col = row, col
	e.selOn = row != e.selRow || col != e.selCol
	e.typing, e.want = false, -1
	return m, nil
}
