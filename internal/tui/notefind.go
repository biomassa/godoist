package tui

import (
	"fmt"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Fields of the find box, in tab order.
const (
	findField = iota
	replaceField
	caseField
	findFields
)

// noteFind is the find and replace box of the inline editor. It sits at the top of the
// editor pane, so that the text and the highlighted matches stay visible. enter goes to the
// next match. ctrl+r and ctrl+a replace with the replace field as it is, so that an empty
// field removes the matches.
type noteFind struct {
	find, repl textinput.Model
	focus      int
	matchCase  bool
	matches    []noteMatch
	cur        int // current match, -1 if there is no match
}

// noteMatch is a match in the editor text: line row, runes [a, b).
type noteMatch struct{ row, a, b int }

func newFindInput(prompt string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.CharLimit = 200
	return ti
}

// openFind opens the find box, or moves the focus to its find field if it is open.
func (e *noteEditor) openFind(w int) tea.Cmd {
	if e.search == nil {
		f := &noteFind{find: newFindInput(" find    › "), repl: newFindInput(" replace › "), cur: -1}
		e.search = f
	}
	f := e.search
	f.find.SetWidth(max(8, w-16))
	f.repl.SetWidth(max(8, w-16))
	f.focus = findField
	f.repl.Blur()
	return f.find.Focus()
}

// runesEqual compares two runes, with or without case.
func runesEqual(a, b rune, matchCase bool) bool {
	if matchCase {
		return a == b
	}
	return unicode.ToLower(a) == unicode.ToLower(b)
}

// findMatches returns all matches of q in the editor text.
func (e *noteEditor) findMatches(q []rune, matchCase bool) []noteMatch {
	if len(q) == 0 {
		return nil
	}
	var out []noteMatch
	for row, l := range e.lines {
		for i := 0; i+len(q) <= len(l); i++ {
			ok := true
			for j := range q {
				if !runesEqual(l[i+j], q[j], matchCase) {
					ok = false
					break
				}
			}
			if ok {
				out = append(out, noteMatch{row, i, i + len(q)})
				i += len(q) - 1
			}
		}
	}
	return out
}

// refind updates the matches and selects the first match at or after the cursor.
func (e *noteEditor) refind() {
	f := e.search
	f.matches = e.findMatches([]rune(f.find.Value()), f.matchCase)
	f.cur = -1
	for i, mt := range f.matches {
		if mt.row > e.row || (mt.row == e.row && mt.a >= e.col) {
			f.cur = i
			break
		}
	}
	if f.cur < 0 && len(f.matches) > 0 {
		f.cur = 0
	}
	e.gotoMatch()
}

// gotoMatch puts the editor cursor at the start of the current match.
func (e *noteEditor) gotoMatch() {
	f := e.search
	if f.cur >= 0 && f.cur < len(f.matches) {
		mt := f.matches[f.cur]
		e.row, e.col, e.want = mt.row, mt.a, -1
	}
}

// stepMatch moves to the next (d = 1) or previous (d = -1) match, with wrap-around.
func (e *noteEditor) stepMatch(d int) {
	f := e.search
	if n := len(f.matches); n > 0 {
		f.cur = (f.cur + d + n) % n
		e.gotoMatch()
	}
}

// replaceCurrent replaces the current match and moves to the next match.
func (e *noteEditor) replaceCurrent() {
	f := e.search
	if f.cur < 0 || f.cur >= len(f.matches) {
		return
	}
	e.push(false)
	mt := f.matches[f.cur]
	r := []rune(f.repl.Value())
	l := e.lines[mt.row]
	e.lines[mt.row] = append(append(append([]rune{}, l[:mt.a]...), r...), l[mt.b:]...)
	e.row, e.col = mt.row, mt.a+len(r) // search again after the new text
	e.refind()
}

// replaceAll replaces every match in one undo step and returns the number of changes.
func (e *noteEditor) replaceAll() int {
	f := e.search
	ms := e.findMatches([]rune(f.find.Value()), f.matchCase)
	if len(ms) == 0 {
		return 0
	}
	e.push(false)
	r := []rune(f.repl.Value())
	for i := len(ms) - 1; i >= 0; i-- { // from the end, so that earlier positions stay correct
		mt := ms[i]
		l := e.lines[mt.row]
		e.lines[mt.row] = append(append(append([]rune{}, l[:mt.a]...), r...), l[mt.b:]...)
	}
	return len(ms)
}

// updateFind handles keys while the find box is open. ok is false for keys that the
// editor itself must handle (none at this time: the box takes all keys).
func (m Model) updateFind(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	e := m.note
	f := e.search
	key := msg.String()
	switch key {
	case "esc":
		e.search = nil // the cursor stays at the current match
		return m, nil
	case "tab", "shift+tab":
		d := 1
		if key == "shift+tab" {
			d = -1
		}
		f.focus = (f.focus + d + findFields) % findFields
		f.find.Blur()
		f.repl.Blur()
		switch f.focus {
		case findField:
			return m, f.find.Focus()
		case replaceField:
			return m, f.repl.Focus()
		}
		return m, nil
	case "shift+enter":
		e.stepMatch(-1)
		return m, nil
	case "enter":
		e.stepMatch(1)
		return m, nil
	case "ctrl+r":
		if f.cur < 0 {
			return m, nil
		}
		e.replaceCurrent()
		return m, e.changed()
	case "ctrl+a":
		n := e.replaceAll()
		e.search = nil
		if n == 0 {
			return m, nil
		}
		return m, e.changed()
	case "space":
		if f.focus == caseField {
			f.matchCase = !f.matchCase
			e.refind()
			return m, nil
		}
	}
	var cmd tea.Cmd
	switch f.focus {
	case findField:
		before := f.find.Value()
		f.find, cmd = f.find.Update(msg)
		if f.find.Value() != before {
			e.refind()
		}
	case replaceField:
		f.repl, cmd = f.repl.Update(msg)
	}
	return m, cmd
}

// findBox draws the find box for an editor pane of inner width w.
func (m Model) findBox(w int) string {
	f := m.note.search
	check := func(on bool, label string, field int) string {
		mark := "[ ]"
		if on {
			mark = "[x]"
		}
		s := st(hexText)
		if f.focus == field {
			s = s.Reverse(true)
		}
		return s.Render(mark + " " + label)
	}
	inner := w - 2
	lines := []string{
		f.find.View(),
		f.repl.View(),
		" " + check(f.matchCase, "match case", caseField),
	}
	with := "“" + f.repl.Value() + "”"
	if f.repl.Value() == "" {
		with = "nothing"
	}
	hint := "enter next · ^r replace with " + with + " · ^a replace all · tab field · esc close"
	lines = append(lines, wrapHint(hint, hexDim, inner)...)
	title := "Find"
	switch {
	case f.find.Value() == "":
	case len(f.matches) == 0:
		title += " · no match"
	default:
		title += fmt.Sprintf(" · %d of %d", f.cur+1, len(f.matches))
	}
	return box(st(fg(hexAccent)).Bold(true).Render(title), padLines(lines, inner, len(lines)), w, fg(hexAccent))
}

// matchStyle returns the style of a rune at row, col if it is in a match: a yellow
// background, stronger for the current match. ok is false for other runes.
func (e *noteEditor) matchStyle(row, col int) (lipgloss.Style, bool) {
	f := e.search
	if f == nil {
		return lipgloss.Style{}, false
	}
	for i, mt := range f.matches {
		if mt.row == row && col >= mt.a && col < mt.b {
			bg := mix("#E0B000", baseBg, 0.6)
			if i == f.cur {
				bg = "#E0B000"
			}
			return lipgloss.NewStyle().Background(c(bg)).Foreground(c(baseBg)).Bold(i == f.cur), true
		}
	}
	return lipgloss.Style{}, false
}
