package tui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// dialogSpec is the title, the example text, and the hint of a dialog.
type dialogSpec struct {
	title       string
	placeholder string
	hint        string
	limit       int
}

// dialogSpecs gives the dialog for each input mode. inputFind is not here: it stays in
// the bottom bar, because it filters the list while the user types.
var dialogSpecs = map[inputMode]dialogSpec{
	inputAdd: {"Add task", "Pay rent tomorrow 9am p1 #private @label",
		"dates, #project, /section, @label and p1–p4 are parsed · enter add · esc cancel", 500},
	inputAddNote: {"New note", "Note title",
		"saved as typed · E writes the body later · enter add · esc cancel", 500},
	inputRename: {"Rename", "New name",
		"#project, /section, @label and p1–p4 change only those fields · dates: t · enter save · esc cancel", 500},
	inputAddSection: {"New section", "Section name",
		"saved as typed · goes after the section under the cursor · enter add · esc cancel", 120},
	inputRenameSection: {"Rename section", "Section name", "saved as typed · enter save · esc cancel", 120},
	inputAddProject: {"New project", "Project name",
		"saved as typed · next: color and sub-project · enter next · esc cancel", 120},
	inputRenameProject: {"Rename project", "Project name", "saved as typed · enter save · esc cancel", 120},
	inputAddSubtask: {"New sub-task", "Sub-task name",
		"dates, @label and p1–p4 are parsed · it goes under the task at the cursor · enter add · esc cancel", 500},
	inputAddLabel:    {"New label", "Label name", "saved as typed · enter add · esc cancel", 60},
	inputRenameLabel: {"Rename label", "Label name", "saved as typed · its tasks get the new name · enter save · esc cancel", 60},
	inputQuery: {"Todoist filter", "today | overdue · @read · #private & p1 · search: backup",
		"Todoist filter syntax. Plain words search the task names. An empty filter removes the view · enter run · esc cancel", 1024},
}

// descHints are the hints of the task dialog, for the name field and for the
// description field.
var descHints = map[inputMode][2]string{
	inputAdd: {"dates, #project, /section, @label and p1–p4 are parsed · tab description · enter add · esc cancel",
		"saved as typed · enter new line · ctrl+enter or ctrl+s add · tab name · esc cancel"},
	inputRename: {"#project, /section, @label and p1–p4 change only those fields · dates: t · tab description · enter save · esc cancel",
		"saved as typed · enter new line · ctrl+enter or ctrl+s save · tab name · esc cancel"},
	inputAddSubtask: {"dates, @label and p1–p4 are parsed · it goes under the task at the cursor · tab description · enter add · esc cancel",
		"saved as typed · enter new line · ctrl+enter or ctrl+s add · tab name · esc cancel"},
}

// hasDescField reports whether the dialog for mode has a description field under the
// name. A note in notebook view has its body in the reader, so its dialog has only a name.
func (m Model) hasDescField(mode inputMode) bool {
	switch mode {
	case inputAdd, inputAddSubtask:
		return true
	case inputRename:
		return !m.notesMode()
	}
	return false
}

// isDialog reports whether an input mode uses the dialog.
func isDialog(mode inputMode) bool {
	_, ok := dialogSpecs[mode]
	return ok
}

// dialogWidth is the outer width of the dialog.
func (m Model) dialogWidth() int { return max(30, min(72, m.width-8)) }

// openDialog opens the dialog for mode with value in it.
func (m *Model) openDialog(mode inputMode, value string) tea.Cmd {
	spec := dialogSpecs[mode]
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.Placeholder = spec.placeholder
	ta.CharLimit = spec.limit
	ta.MaxHeight = 0
	ta.KeyMap.InsertNewline.SetEnabled(false) // enter saves. A name has no line breaks.
	ta.SetStyles(areaStyles())
	clipboardKeys(&ta)
	ta.SetWidth(m.dialogWidth() - 4)
	ta.SetHeight(5)
	ta.SetValue(value)
	ta.MoveToEnd()
	m.dlg = &ta
	m.inputMode = mode
	m.dlgField = 0
	if m.hasDescField(mode) {
		m.dlg.SetHeight(dialogNameLines)
		m.dlgDesc = &noteEditor{lines: [][]rune{{}}, want: -1, noSave: true}
		m.sizeDialog()
	}
	return m.dlg.Focus()
}

// openTaskDialog opens the task dialog for mode with a name and a description.
func (m *Model) openTaskDialog(mode inputMode, name, desc string) tea.Cmd {
	cmd := m.openDialog(mode, name)
	if m.hasDescField(mode) {
		m.dlgDesc.lines = splitRunes(desc)
	}
	return cmd
}

// clipboardKeys sets the selection keys of a dialog field: ctrl+a selects all (home goes
// to the line start). The
// textarea copy key (ctrl+shift+c) is off: it needs a clipboard program, and kitty uses
// that key. ctrl+c, ctrl+x, and ctrl+v go through the terminal (see dialogClipboard).
func clipboardKeys(ta *textarea.Model) {
	ta.KeyMap.LineStart.SetKeys("home") // not ctrl+a: that selects all
	ta.KeyMap.SelectAll.SetKeys("ctrl+a", "ctrl+g")
	ta.KeyMap.CopySelection.SetEnabled(false)
}

// dialogClipboard copies, cuts, or pastes in the name field of the dialog. The system
// clipboard goes through the terminal (OSC 52). The description field is a markdown
// editor with its own clipboard keys (see selectKeys).
func (m Model) dialogClipboard(key string) (tea.Model, tea.Cmd) {
	f := m.dlg
	if key == "ctrl+v" {
		return m, tea.ReadClipboard // the text comes as a ClipboardMsg and goes to paste
	}
	if !f.HasSelection() {
		m.setStatus("select text first: shift+arrows or ctrl+a", false)
		return m, nil
	}
	copyCmd := tea.SetClipboard(f.SelectedText())
	if key == "ctrl+x" {
		f.DeleteSelection()
		m.setStatus("cut", false)
	} else {
		m.setStatus("copied", false)
	}
	return m, copyCmd
}

// dialogNameLines is the height of the name field in the task dialog.
const dialogNameLines = 2

// sizeDialog makes the task dialog 2/3 of the terminal height. The description field
// gets the lines that the name, the labels, and the hint do not use.
func (m *Model) sizeDialog() {
	if !m.hasDescField(m.inputMode) {
		return
	}
	fixed := 7 + dialogNameLines + len(m.dialogHintLines()) // borders, spaces, labels, name, hint
	m.dlgDescH = max(3, m.height*2/3-fixed)
}

// dialogDescWidth is the text width of the description field: the dialog without its
// borders and the row margin.
func (m Model) dialogDescWidth() int { return m.dialogWidth() - 4 }

// setDialogField moves the keys to field f of the task dialog (0 name, 1 description).
func (m *Model) setDialogField(f int) tea.Cmd {
	m.dlgField = f
	m.comp = nil
	m.dlgDesc.selOn = false
	m.dlg.Blur()
	if f == 1 {
		return nil
	}
	return m.dlg.Focus()
}

// dialogDesc is the description in the task dialog as typed, without space at the ends.
func (m Model) dialogDesc() string { return strings.TrimSpace(m.dlgDesc.text()) }

// dialogHint is the hint of the open dialog.
func (m Model) dialogHint() string {
	if m.comp != nil {
		return "↑↓ choose · tab or enter insert · esc close the list"
	}
	if m.hasDescField(m.inputMode) {
		return descHints[m.inputMode][m.dlgField]
	}
	spec := dialogSpecs[m.inputMode]
	if m.inputMode == inputRename && m.notesMode() {
		return "note titles are saved as typed · enter save · esc cancel"
	}
	return spec.hint
}

// dialogHintLines is the hint cut into lines of the dialog width.
func (m Model) dialogHintLines() []string {
	inner := m.dialogWidth() - 2
	hint := lipgloss.NewStyle().Width(inner - 2).Foreground(c(hexDim)).Render(m.dialogHint())
	return strings.Split(hint, "\n")
}

// dialogValue is the dialog text on one line. Pasted line breaks become spaces.
func (m Model) dialogValue() string {
	return strings.Join(strings.Fields(strings.ReplaceAll(m.dlg.Value(), "\n", " ")), " ")
}

// dialogBox draws the dialog: the wrapped text and the hint. The task dialog has a
// name field and a description field, each with a label.
func (m Model) dialogBox() string {
	spec := dialogSpecs[m.inputMode]
	w := m.dialogWidth()
	inner := w - 2
	two := m.hasDescField(m.inputMode)
	label := func(name string, f int) string {
		if f == m.dlgField {
			return " " + st(fg(hexAccent)).Bold(true).Render(name)
		}
		return " " + st(hexMuted).Render(name)
	}
	lines := []string{""}
	if two {
		lines = append(lines, label("Name", 0))
	}
	for _, l := range strings.Split(m.dlg.View(), "\n") {
		lines = append(lines, " "+l)
	}
	if two {
		lines = append(lines, "", label("Description", 1))
		lines = append(lines, m.descLines()...)
	}
	lines = append(lines, "")
	for _, l := range m.dialogHintLines() {
		lines = append(lines, " "+l)
	}
	title := spec.title
	if two && m.inputMode == inputRename { // the dialog edits the name and the description
		title = "Edit task"
	}
	return box(st(fg(hexAccent)).Bold(true).Render(title), padLines(lines, inner, len(lines)), w, fg(hexAccent))
}

// descLines draws the description field of the task dialog: a markdown editor with a
// live preview. With the keys in the name field, all lines show rendered.
func (m Model) descLines() []string {
	e, tw, h := m.dlgDesc, m.dialogDescWidth(), m.dlgDescH
	focused := m.dlgField == 1
	var out []string
	if len(e.lines) == 1 && len(e.lines[0]) == 0 { // empty: the placeholder
		cursor := ""
		if focused {
			cursor = lipgloss.NewStyle().Reverse(true).Render(" ")
		}
		out = append(out, "  "+cursor+st(hexDim).Render("Description (optional)"))
	} else {
		rows, cy := m.md.editorLayout(e, tw, h, focused)
		for i, r := range rows {
			cx := -1
			if focused && i == cy {
				cx = e.col - r.start
			}
			out = append(out, renderRow(r, tw, cx))
		}
	}
	for len(out) < h {
		out = append(out, "")
	}
	return out
}

// descTop is the screen line of the first description row of the task dialog.
func (m Model) descTop() int { return m.dialogRect().y + 1 + 4 + dialogNameLines }

// descPosAt returns the source line and rune of the description at screen cell x, y.
// ok is false outside the description rows.
func (m Model) descPosAt(x, y int) (row, col int, ok bool) {
	e := m.dlgDesc
	sr := y - m.descTop()
	if sr < 0 || sr >= m.dlgDescH {
		return 0, 0, false
	}
	rows, _ := m.md.editorLayout(e, m.dialogDescWidth(), m.dlgDescH, true)
	row, col = editorPos(e, rows, sr, x-m.dialogRect().x-2) // the border and the row margin
	return row, col, true
}

// descClick puts the description cursor at a click, or moves it there while the mouse
// button is down (drag). A drag selects text.
func (m Model) descClick(x, y int, drag bool) (tea.Model, tea.Cmd) {
	e := m.dlgDesc
	row, col, ok := m.descPosAt(x, y)
	if !ok {
		return m, nil
	}
	e.row, e.col = row, col
	if drag {
		e.selOn = row != e.selRow || col != e.selCol
	} else {
		e.selOn, e.selRow, e.selCol, e.dragging = false, row, col, true
	}
	e.typing, e.want = false, -1
	return m, nil
}

// dialogFieldAt is the task dialog field (0 name, 1 description) at screen line y, or -1.
func (m Model) dialogFieldAt(y int) int {
	if !m.hasDescField(m.inputMode) {
		return -1
	}
	i := y - m.dialogRect().y - 1 // the line inside the border
	switch {
	case i >= 1 && i <= 1+dialogNameLines:
		return 0
	case i >= 3+dialogNameLines && i <= 3+dialogNameLines+m.dlgDescH:
		return 1
	}
	return -1
}
