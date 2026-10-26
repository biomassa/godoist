package tui

import (
	"context"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// checkboxLine matches a markdown task list item: "- [ ] text", "* [x] text", "1. [ ] text".
var checkboxLine = regexp.MustCompile(`^(\s*(?:[-*+]|\d+[.)])\s+\[)([ xX])(\]\s)`)

// checkboxLines returns the numbers of the source lines that are checkbox items. Lines
// in fenced code are not items, because the reader shows them as code.
func checkboxLines(src string) []int {
	var out []int
	inFence := false
	for i, l := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inFence = !inFence
			continue
		}
		if !inFence && checkboxLine.MatchString(l) {
			out = append(out, i)
		}
	}
	return out
}

// checkRef is a checkbox in the details pane: checkbox k of the description (commentID
// is empty) or of a comment.
type checkRef struct {
	commentID string
	k         int
}

// checkRefs returns the checkboxes of task t in the order of the details pane: the
// description first, then each comment.
func (m Model) checkRefs(t *todoist.Task) []checkRef {
	var refs []checkRef
	for k := range checkboxLines(t.Description) {
		refs = append(refs, checkRef{k: k})
	}
	for _, cm := range m.snap.Comments[t.ID] {
		for k := range checkboxLines(cm.Content) {
			refs = append(refs, checkRef{commentID: cm.ID, k: k})
		}
	}
	return refs
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

// noteChecks is the number of checkboxes in the focused details pane (the description and
// the comments). If it is not 0, tab moves between the checkboxes instead of between the panes.
func (m Model) noteChecks() int {
	if m.focus != paneDetail {
		return 0
	}
	t := m.currentTask()
	if t == nil {
		return 0
	}
	return len(m.checkRefs(t))
}

// moveCheck moves the checkbox cursor by d, with wrap-around, and scrolls to it.
func (m *Model) moveCheck(d int) {
	n := m.noteChecks() + m.noteSubnotes()
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
	line := -1
	if m.chkCur < len(doc.checks) {
		line = doc.checks[m.chkCur]
	} else if k := m.chkCur - len(doc.checks); k < len(doc.subnotes) {
		line = doc.subnotes[k].line
	}
	if h := m.paneHeight(); line >= 0 && (line < m.detOff || line >= m.detOff+h) {
		m.detOff = max(0, line-h/2)
	}
}

// noteSubnotes is the number of sub-notes in the focused reader of notebook view. tab
// selects them after the checkboxes.
func (m Model) noteSubnotes() int {
	t := m.currentTask()
	if m.focus != paneDetail || !m.notesMode() || t == nil {
		return 0
	}
	n := 0
	for _, x := range m.snap.Tasks {
		if x.Parent() == t.ID {
			n++
		}
	}
	return n
}

// openNote moves the cursor of the list to note id and shows it in the reader. A folded
// note above it unfolds, in Todoist too.
func (m *Model) openNote(id string) tea.Cmd {
	var cmds []tea.Cmd
	client := m.client
	for p := m.taskByID(m.taskByID(id).Parent()); p != nil; p = m.taskByID(p.Parent()) {
		if p.IsCollapsed {
			p.IsCollapsed = false
			pid := p.ID
			cmds = append(cmds, m.simpleWrite("", func(ctx context.Context) error { return client.SetCollapsed(ctx, "tasks", pid, false) }))
		}
	}
	jump := id
	m.jumpTo = &jump
	m.buildRows(false)
	return tea.Batch(cmds...)
}

// toggleNoteCheck toggles checkbox k of the details pane and saves the description or
// the comment that has it.
func (m *Model) toggleNoteCheck(k int) tea.Cmd {
	t := m.taskByID(m.currentTaskID())
	if t == nil {
		return nil
	}
	refs := m.checkRefs(t)
	if k < 0 || k >= len(refs) {
		return nil
	}
	ref, client := refs[k], m.client
	if ref.commentID == "" {
		desc, ok := toggleCheckbox(t.Description, ref.k)
		if !ok {
			return nil
		}
		t.Description = desc // show it at once. The sync confirms it.
		m.buildRows(false)
		id := t.ID
		return m.simpleWrite("", func(ctx context.Context) error {
			_, err := client.UpdateTask(ctx, id, map[string]any{"description": desc})
			return err
		})
	}
	comments := m.snap.Comments[t.ID]
	for i := range comments {
		if comments[i].ID != ref.commentID {
			continue
		}
		content, ok := toggleCheckbox(comments[i].Content, ref.k)
		if !ok {
			return nil
		}
		comments[i].Content = content // show it at once. The sync confirms it.
		id := ref.commentID
		return m.simpleWrite("", func(ctx context.Context) error {
			_, err := client.UpdateComment(ctx, id, content)
			return err
		})
	}
	return nil
}
