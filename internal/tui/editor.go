package tui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

// editKind is the text that the editor changes.
type editKind int

const (
	editCommentNew editKind = iota
	editCommentEdit
	editDescription
)

// editSession is an open editor. The editor stays open until the save succeeds,
// so a failed request does not lose the text.
type editSession struct {
	kind         editKind
	taskID       string
	commentID    string
	title        string
	original     string
	saving       bool
	discardArmed bool // the user pushed esc one time with unsaved changes
}

type (
	editSavedMsg  struct{ err error }
	editorDoneMsg struct {
		path string
		err  error
	}
)

// editorSize is the inner size of the pane that shows the editor.
func (m Model) editorSize() (w, h int) {
	w = m.width - m.navWidth() - 2
	if m.wide() {
		w = m.detailWidth() - 2
	}
	return max(10, w), max(3, m.paneHeight()-2)
}

// sizeEditor fits the editor to its pane. It does nothing while no edit is open,
// because the zero textarea.Model panics on SetWidth.
func (m *Model) sizeEditor() {
	if m.edit == nil {
		return
	}
	w, h := m.editorSize()
	m.editor.SetWidth(w)
	m.editor.SetHeight(h)
}

// openEditor opens the built-in editor with text.
func (m *Model) openEditor(kind editKind, taskID, commentID, title, text string) tea.Cmd {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 16000 // the Todoist limit for descriptions and comments is 16384
	ta.MaxHeight = 0
	ta.SetStyles(textarea.DefaultStyles(darkTheme))
	ta.SetValue(text)
	m.editor = ta
	m.edit = &editSession{kind: kind, taskID: taskID, commentID: commentID, title: title, original: text}
	m.sizeEditor()
	return m.editor.Focus()
}

// updateEditor handles keys while the editor is open.
func (m Model) updateEditor(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	e := m.edit
	if e.saving {
		return m, nil
	}
	switch msg.String() {
	case "ctrl+s":
		return m.saveEdit()
	case "esc":
		if m.editor.Value() != e.original && !e.discardArmed {
			e.discardArmed = true
			m.setStatus("unsaved changes · esc again to discard, ctrl+s to save", true)
			return m, nil
		}
		m.edit = nil
		m.setStatus("edit cancelled", false)
		return m, nil
	case "ctrl+e":
		return m, m.externalEditor()
	}
	e.discardArmed = false
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}

// saveEdit sends the editor text to the API. The editor stays open until editSaved.
func (m Model) saveEdit() (tea.Model, tea.Cmd) {
	e := m.edit
	text := strings.TrimRight(m.editor.Value(), " \n")
	if text == strings.TrimRight(e.original, " \n") {
		m.edit = nil
		m.setStatus("no changes", false)
		return m, nil
	}
	if text == "" && e.kind != editDescription {
		m.setStatus("comment is empty · use d in the details pane to delete a comment", true)
		return m, nil
	}
	e.saving = true
	m.pending++
	m.setStatus("saving…", false)
	client, kind, taskID, commentID := m.client, e.kind, e.taskID, e.commentID
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var err error
		switch kind {
		case editCommentNew:
			_, err = client.AddComment(ctx, taskID, text)
		case editCommentEdit:
			_, err = client.UpdateComment(ctx, commentID, text)
		case editDescription:
			_, err = client.UpdateTask(ctx, taskID, map[string]any{"description": text})
		}
		return editSavedMsg{err: err}
	}
}

// editSaved closes the editor after a successful save and starts a sync.
// After a failed save, the editor stays open with the text.
func (m Model) editSaved(msg editSavedMsg) (tea.Model, tea.Cmd) {
	m.pending = max(0, m.pending-1)
	if msg.err != nil {
		if m.edit != nil {
			m.edit.saving = false
		}
		m.setStatus("save failed, the text is still in the editor: "+msg.err.Error(), true)
		return m, nil
	}
	m.edit = nil
	m.setStatus("saved", false)
	if m.quitting && m.pending == 0 {
		return m, tea.Quit
	}
	next := m.startSync()
	return m, next
}

// externalEditor writes the editor text to a temporary file and opens it in
// $VISUAL or $EDITOR. The result goes back into the built-in editor.
func (m Model) externalEditor() tea.Cmd {
	f, err := os.CreateTemp("", "godoist-*.md")
	if err != nil {
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}
	_, werr := f.WriteString(m.editor.Value() + "\n") // editors expect a final newline
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

// externalEditorDone loads the file from $EDITOR into the built-in editor and deletes the file.
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
	if m.edit != nil {
		m.editor.SetValue(strings.TrimRight(string(b), "\n"))
		m.setStatus("text loaded from $EDITOR · ctrl+s to save", false)
	}
	return m, nil
}
