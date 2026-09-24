package tui

import (
	"context"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// checkboxLine matches a markdown task list item: "- [ ] text", "* [x] text", "1. [ ] text".
var checkboxLine = regexp.MustCompile(`^(\s*(?:[-*+]|\d+[.)])\s+\[)([ xX])(\]\s)`)

// checkboxLines returns the numbers of the source lines that are checkbox items.
func checkboxLines(src string) []int {
	var out []int
	for i, l := range strings.Split(src, "\n") {
		if checkboxLine.MatchString(l) {
			out = append(out, i)
		}
	}
	return out
}

// toggleCheckbox changes checkbox k of the markdown src between [ ] and [x].
func toggleCheckbox(src string, k int) (string, bool) {
	lines := strings.Split(src, "\n")
	idx := checkboxLines(src)
	if k < 0 || k >= len(idx) {
		return src, false
	}
	i := idx[k]
	lines[i] = checkboxLine.ReplaceAllStringFunc(lines[i], func(s string) string {
		sub := checkboxLine.FindStringSubmatch(s)
		mark := "x"
		if sub[2] != " " {
			mark = " "
		}
		return sub[1] + mark + sub[3]
	})
	return strings.Join(lines, "\n"), true
}

// noteChecks reports whether the reader shows a note with checkboxes, so that tab moves
// between the checkboxes instead of between the panes.
func (m Model) noteChecks() int {
	if m.focus != paneDetail || !m.notesMode() {
		return 0
	}
	t := m.currentTask()
	if t == nil {
		return 0
	}
	return len(checkboxLines(t.Description))
}

// moveCheck moves the checkbox cursor by d, with wrap-around, and scrolls to it.
func (m *Model) moveCheck(d int) {
	n := m.noteChecks()
	if n == 0 {
		return
	}
	m.comCur = -1
	if m.chkCur < 0 && d < 0 {
		m.chkCur = n - 1
	} else {
		m.chkCur = (m.chkCur + d + n) % n
	}
	doc := m.buildDetail(m.detailInnerWidth() - 1)
	if m.chkCur < len(doc.checks) {
		line := doc.checks[m.chkCur]
		h := m.paneHeight()
		if line < m.detOff || line >= m.detOff+h {
			m.detOff = max(0, line-h/2)
		}
	}
}

// toggleNoteCheck toggles checkbox k of the note under the cursor and saves the note.
func (m *Model) toggleNoteCheck(k int) tea.Cmd {
	t := m.taskByID(m.currentTaskID())
	if t == nil {
		return nil
	}
	desc, ok := toggleCheckbox(t.Description, k)
	if !ok {
		return nil
	}
	t.Description = desc // show it at once. The sync confirms it.
	m.buildRows(false)
	id, client := t.ID, m.client
	return m.simpleWrite("", func(ctx context.Context) error {
		_, err := client.UpdateTask(ctx, id, map[string]any{"description": desc})
		return err
	})
}
