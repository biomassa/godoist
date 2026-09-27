package tui

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// editKind is the text that the inline editor changes.
type editKind int

const (
	editCommentNew editKind = iota
	editCommentEdit
	editDescription
)

type editorDoneMsg struct {
	path string
	err  error
}

// externalEditor writes text to a temporary file and opens it in $VISUAL or $EDITOR.
// The result goes back into the inline editor.
func (m Model) externalEditor(text string) tea.Cmd {
	f, err := os.CreateTemp("", "godoist-*.md")
	if err != nil {
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}
	_, werr := f.WriteString(text + "\n") // editors expect a final newline
	f.Close()
	if werr != nil {
		return func() tea.Msg { return editorDoneMsg{path: f.Name(), err: werr} }
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	args := append(strings.Fields(editor), f.Name())
	c := exec.Command(args[0], args[1:]...)
	path := f.Name()
	return tea.ExecProcess(c, func(err error) tea.Msg { return editorDoneMsg{path: path, err: err} })
}

// externalEditorDone loads the file from $EDITOR into the inline editor and deletes the
// file. The change can be undone with ctrl+z, and autosave saves it.
func (m Model) externalEditorDone(msg editorDoneMsg) (tea.Model, tea.Cmd) {
	if msg.path != "" {
		defer os.Remove(msg.path)
	}
	if msg.err != nil {
		m.setStatus("external editor: "+msg.err.Error(), true)
		return m, nil
	}
	b, err := os.ReadFile(msg.path)
	if err != nil {
		m.setStatus("external editor: "+err.Error(), true)
		return m, nil
	}
	e := m.note
	if e == nil {
		return m, nil
	}
	text := strings.TrimRight(string(b), "\n")
	if text == e.text() {
		return m, nil
	}
	e.push(false)
	e.lines = splitRunes(text)
	e.row = min(e.row, len(e.lines)-1)
	e.col = min(e.col, len(e.lines[e.row]))
	m.setStatus("text loaded from $EDITOR", false)
	return m, e.changed()
}
