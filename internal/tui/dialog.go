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
		"dates, #project, /section, @label and p1–p4 are parsed and change only those fields · enter save · esc cancel", 500},
	inputAddSection: {"New section", "Section name",
		"saved as typed · goes after the section under the cursor · enter add · esc cancel", 120},
	inputRenameSection: {"Rename section", "Section name", "saved as typed · enter save · esc cancel", 120},
	inputQuery: {"Todoist filter", "today | overdue · @read · #private & p1 · search: backup",
		"Todoist filter syntax. Plain words search the task names. An empty filter removes the view · enter run · esc cancel", 1024},
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
	ta.SetStyles(textarea.DefaultStyles(darkTheme))
	ta.SetWidth(m.dialogWidth() - 4)
	ta.SetHeight(5)
	ta.SetValue(value)
	ta.MoveToEnd()
	m.dlg = ta
	m.inputMode = mode
	return m.dlg.Focus()
}

// dialogValue is the dialog text on one line. Pasted line breaks become spaces.
func (m Model) dialogValue() string {
	return strings.Join(strings.Fields(strings.ReplaceAll(m.dlg.Value(), "\n", " ")), " ")
}

// dialogBox draws the dialog: the wrapped text and the hint.
func (m Model) dialogBox() string {
	spec := dialogSpecs[m.inputMode]
	if m.inputMode == inputRename && m.notesMode() {
		spec.hint = "note titles are saved as typed · enter save · esc cancel"
	}
	w := m.dialogWidth()
	inner := w - 2
	lines := []string{""}
	for _, l := range strings.Split(m.dlg.View(), "\n") {
		lines = append(lines, " "+l)
	}
	lines = append(lines, "")
	hint := lipgloss.NewStyle().Width(inner - 2).Foreground(c(hexDim)).Render(spec.hint)
	for _, l := range strings.Split(hint, "\n") {
		lines = append(lines, " "+l)
	}
	return box(st(fg(hexAccent)).Bold(true).Render(spec.title), padLines(lines, inner, len(lines)), w, fg(hexAccent))
}
