package tui

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// noteAutosave is the pause after the last change before the editor saves.
const noteAutosave = time.Second

// noteEditor is the inline markdown editor of notebook view. It shows the source with
// live styling and saves by itself one second after the last change and when it closes.
type noteEditor struct {
	taskID string
	title  string
	lines  [][]rune
	row    int // cursor line
	col    int // cursor rune in the line
	want   int // column that up/down try to keep
	off    int // first visible screen line
	saved  string
	gen    int  // change counter, for the autosave timer
	saving bool // a save is in flight
	err    string
	undo   []noteState
	redo   []noteState
	typing bool      // the last change was typing, so the next letter joins its undo step
	search *noteFind // the open find box, or nil
}

// noteState is one undo step.
type noteState struct {
	lines    [][]rune
	row, col int
}

type (
	noteTickMsg  struct{ gen int }
	noteSavedMsg struct {
		text string
		err  error
	}
)

func splitRunes(s string) [][]rune {
	var out [][]rune
	for _, l := range strings.Split(s, "\n") {
		out = append(out, []rune(l))
	}
	return out
}

func (e *noteEditor) text() string {
	parts := make([]string, len(e.lines))
	for i, l := range e.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

func (e *noteEditor) dirty() bool { return e.text() != e.saved }

func (e *noteEditor) snapshot() noteState {
	c := make([][]rune, len(e.lines))
	for i, l := range e.lines {
		c[i] = append([]rune(nil), l...)
	}
	return noteState{c, e.row, e.col}
}

// push saves an undo step. Consecutive letters of typing share one step.
func (e *noteEditor) push(typing bool) {
	if typing && e.typing {
		return
	}
	e.undo = append(e.undo, e.snapshot())
	if len(e.undo) > 200 {
		e.undo = e.undo[1:]
	}
	e.redo = nil
	e.typing = typing
}

// openNoteEditor turns the reader into the editor for the note under the cursor.
func (m *Model) openNoteEditor() tea.Cmd {
	t := m.currentTask()
	if t == nil {
		return nil
	}
	e := &noteEditor{taskID: t.ID, title: plain(t.Content), lines: splitRunes(t.Description), saved: t.Description, want: -1}
	e.row = len(e.lines) - 1
	e.col = len(e.lines[e.row])
	m.note = e
	m.focusDetail()
	return nil
}

// openNoteEditorAt opens the editor with the cursor at the start of source line n.
func (m *Model) openNoteEditorAt(n int) tea.Cmd {
	cmd := m.openNoteEditor()
	if e := m.note; e != nil && n >= 0 && n < len(e.lines) {
		e.row, e.col = n, 0
	}
	return cmd
}

// sourceLineFor finds the source line of the note that a rendered reader line shows.
// It compares the first words of the text. It returns -1 if it finds no line.
func sourceLineFor(src, rendered string) int {
	text := strings.TrimSpace(ansi.Strip(rendered))
	text = strings.TrimLeft(text, "☐☑•-*0123456789. ")
	if r := []rune(text); len(r) > 16 {
		text = string(r[:16])
	}
	if strings.TrimSpace(text) == "" {
		return -1
	}
	for i, l := range strings.Split(src, "\n") {
		if strings.Contains(plain(strings.NewReplacer("*", "", "_", "", "`", "").Replace(l)), text) {
			return i
		}
	}
	return -1
}

// closeNoteEditor saves unsaved text and returns to the reader.
func (m Model) closeNoteEditor() (tea.Model, tea.Cmd) {
	e := m.note
	m.note = nil
	if !e.dirty() {
		return m, nil
	}
	next := m.saveNote(e, "Saved “"+e.title+"”")
	return m, next
}

// saveNote sends the editor text. The result arrives as noteSavedMsg.
func (m *Model) saveNote(e *noteEditor, done string) tea.Cmd {
	text := e.text()
	e.saving = true
	m.pending++
	client, id := m.client, e.taskID
	if t := m.taskByID(id); t != nil {
		t.Description = text // the reader shows the new text at once
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := client.UpdateTask(ctx, id, map[string]any{"description": text})
		if err == nil && done != "" {
			return actionMsg{text: done} // the editor is closed: report like other writes
		}
		return noteSavedMsg{text: text, err: err}
	}
}

func (m Model) noteSaved(msg noteSavedMsg) (tea.Model, tea.Cmd) {
	m.pending = max(0, m.pending-1)
	e := m.note
	if msg.err != nil {
		if e != nil {
			e.saving, e.err = false, msg.err.Error()
		} else {
			m.setStatus("note not saved: "+msg.err.Error(), true)
		}
		return m, nil
	}
	if e != nil {
		e.saving, e.err, e.saved = false, "", msg.text
	}
	if m.quitting && m.pending == 0 {
		return m, tea.Quit
	}
	next := m.startSync()
	return m, next
}

// noteTick saves if no change came during the autosave pause.
func (m Model) noteTick(msg noteTickMsg) (tea.Model, tea.Cmd) {
	e := m.note
	if e == nil || msg.gen != e.gen || e.saving || !e.dirty() {
		return m, nil
	}
	next := m.saveNote(e, "")
	return m, next
}

// changed restarts the autosave timer after a change.
func (e *noteEditor) changed() tea.Cmd {
	e.gen++
	g := e.gen
	return tea.Tick(noteAutosave, func(time.Time) tea.Msg { return noteTickMsg{gen: g} })
}

var (
	listPrefix = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])(\s+)(\[[ xX]\]\s)?`)
	wordRune   = func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
)

// updateNoteEditor handles keys in the editor.
func (m Model) updateNoteEditor(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	e := m.note
	key := msg.String()
	if e.search != nil {
		return m.updateFind(msg)
	}
	line := e.lines[e.row]
	switch key {
	case "ctrl+f":
		return m, e.openFind(m.detailInnerWidth())
	case "esc", "ctrl+c":
		return m.closeNoteEditor()
	case "ctrl+z":
		if n := len(e.undo); n > 0 {
			e.redo = append(e.redo, e.snapshot())
			st := e.undo[n-1]
			e.undo = e.undo[:n-1]
			e.lines, e.row, e.col, e.typing = st.lines, st.row, st.col, false
			return m, e.changed()
		}
		return m, nil
	case "ctrl+y":
		if n := len(e.redo); n > 0 {
			e.undo = append(e.undo, e.snapshot())
			st := e.redo[n-1]
			e.redo = e.redo[:n-1]
			e.lines, e.row, e.col, e.typing = st.lines, st.row, st.col, false
			return m, e.changed()
		}
		return m, nil
	case "left":
		e.moveLeft()
	case "right":
		e.moveRight()
	case "up":
		e.moveVertical(-1, m.noteWidth())
	case "down":
		e.moveVertical(1, m.noteWidth())
	case "pgup":
		for i := 0; i < m.paneHeight()/2; i++ {
			e.moveVertical(-1, m.noteWidth())
		}
	case "pgdown":
		for i := 0; i < m.paneHeight()/2; i++ {
			e.moveVertical(1, m.noteWidth())
		}
	case "home":
		e.col = 0
	case "end":
		e.col = len(line)
	case "alt+left", "ctrl+left":
		e.wordLeft()
	case "alt+right", "ctrl+right":
		e.wordRight()
	case "enter":
		e.push(false)
		e.newline()
		return m, e.changed()
	case "backspace":
		e.push(false)
		e.backspace()
		return m, e.changed()
	case "alt+backspace", "ctrl+w":
		e.push(false)
		e.deleteWordBack()
		return m, e.changed()
	case "delete":
		e.push(false)
		e.deleteForward()
		return m, e.changed()
	case "ctrl+b":
		e.push(false)
		e.wrapWord("**", "**")
		return m, e.changed()
	case "ctrl+i":
		e.push(false)
		e.wrapWord("*", "*")
		return m, e.changed()
	case "ctrl+k":
		e.push(false)
		e.wrapWord("[", "]()")
		return m, e.changed()
	case "ctrl+t":
		e.push(false)
		e.toggleLineCheckbox()
		return m, e.changed()
	case "tab":
		e.push(false)
		e.insert([]rune("  "))
		return m, e.changed()
	default:
		if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			e.push(true)
			e.insert([]rune(msg.Text))
			return m, e.changed()
		}
	}
	e.typing = false
	e.want = -1
	return m, nil
}

// deleteWordBack deletes the word before the cursor. At the start of a line, it joins
// the line with the line above.
func (e *noteEditor) deleteWordBack() {
	if e.col == 0 {
		e.backspace()
		return
	}
	end := e.col
	e.wordLeft()
	l := e.lines[e.row]
	e.lines[e.row] = append(append([]rune{}, l[:e.col]...), l[end:]...)
}

func (e *noteEditor) insert(rs []rune) {
	l := e.lines[e.row]
	nl := append(append(append([]rune{}, l[:e.col]...), rs...), l[e.col:]...)
	e.lines[e.row] = nl
	e.col += len(rs)
	e.want = -1
}

// insertText inserts pasted text, which can have line breaks.
func (e *noteEditor) insertText(s string) {
	parts := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, p := range parts {
		if i > 0 {
			e.splitLine()
		}
		e.insert([]rune(p))
	}
}

func (e *noteEditor) splitLine() {
	l := e.lines[e.row]
	head, tail := append([]rune{}, l[:e.col]...), append([]rune{}, l[e.col:]...)
	e.lines[e.row] = head
	e.lines = append(e.lines[:e.row+1], append([][]rune{tail}, e.lines[e.row+1:]...)...)
	e.row++
	e.col = 0
}

// newline splits the line. On a list item, the new line starts the next item. On an
// empty item, enter ends the list instead.
func (e *noteEditor) newline() {
	l := string(e.lines[e.row])
	p := listPrefix.FindStringSubmatch(l)
	if p == nil {
		e.splitLine()
		return
	}
	if strings.TrimSpace(l[len(p[0]):]) == "" { // an empty item ends the list
		e.lines[e.row] = []rune{}
		e.col = 0
		return
	}
	marker := p[2]
	if n, err := strconv.Atoi(strings.TrimRight(marker, ".)")); err == nil {
		marker = strconv.Itoa(n+1) + marker[len(marker)-1:]
	}
	next := p[1] + marker + p[3]
	if p[4] != "" {
		next += "[ ] "
	}
	e.splitLine()
	e.insert([]rune(next))
}

func (e *noteEditor) backspace() {
	if e.col > 0 {
		l := e.lines[e.row]
		e.lines[e.row] = append(append([]rune{}, l[:e.col-1]...), l[e.col:]...)
		e.col--
		return
	}
	if e.row > 0 {
		prev := e.lines[e.row-1]
		e.col = len(prev)
		e.lines[e.row-1] = append(append([]rune{}, prev...), e.lines[e.row]...)
		e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
		e.row--
	}
}

func (e *noteEditor) deleteForward() {
	l := e.lines[e.row]
	if e.col < len(l) {
		e.lines[e.row] = append(append([]rune{}, l[:e.col]...), l[e.col+1:]...)
		return
	}
	if e.row < len(e.lines)-1 {
		e.lines[e.row] = append(append([]rune{}, l...), e.lines[e.row+1]...)
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
	}
}

func (e *noteEditor) moveLeft() {
	switch {
	case e.col > 0:
		e.col--
	case e.row > 0:
		e.row--
		e.col = len(e.lines[e.row])
	}
}

func (e *noteEditor) moveRight() {
	switch {
	case e.col < len(e.lines[e.row]):
		e.col++
	case e.row < len(e.lines)-1:
		e.row++
		e.col = 0
	}
}

func (e *noteEditor) wordLeft() {
	l := e.lines[e.row]
	if e.col == 0 {
		e.moveLeft()
		return
	}
	i := e.col
	for i > 0 && !wordRune(l[i-1]) {
		i--
	}
	for i > 0 && wordRune(l[i-1]) {
		i--
	}
	e.col = i
}

func (e *noteEditor) wordRight() {
	l := e.lines[e.row]
	if e.col == len(l) {
		e.moveRight()
		return
	}
	i := e.col
	for i < len(l) && !wordRune(l[i]) {
		i++
	}
	for i < len(l) && wordRune(l[i]) {
		i++
	}
	e.col = i
}

// wrapWord puts before and after around the word at the cursor. Without a word, it
// inserts both and puts the cursor between them.
func (e *noteEditor) wrapWord(before, after string) {
	l := e.lines[e.row]
	s, t := e.col, e.col
	for s > 0 && wordRune(l[s-1]) {
		s--
	}
	for t < len(l) && wordRune(l[t]) {
		t++
	}
	if s == t {
		e.insert([]rune(before + after))
		e.col -= len([]rune(after))
		if after == "]()" { // a link without text: the cursor goes into the brackets
			e.col = s + 1
		}
		return
	}
	word := l[s:t]
	nl := append(append([]rune{}, l[:s]...), []rune(before)...)
	nl = append(nl, word...)
	nl = append(nl, []rune(after)...)
	nl = append(nl, l[t:]...)
	e.lines[e.row] = nl
	e.col = s + len([]rune(before)) + len(word) + len([]rune(after))
	if after == "]()" { // a link: the cursor goes into the parentheses for the URL
		e.col--
	}
}

// toggleLineCheckbox switches the checkbox of the line, or makes the line a checkbox item.
func (e *noteEditor) toggleLineCheckbox() {
	l := string(e.lines[e.row])
	switch p := listPrefix.FindStringSubmatch(l); {
	case p != nil && p[4] != "":
		if src, ok := toggleCheckbox(l, 0); ok {
			e.lines[e.row] = []rune(src)
		}
		return
	case p != nil:
		e.lines[e.row] = []rune(p[0] + "[ ] " + l[len(p[0]):])
		e.col += 4
	default:
		e.lines[e.row] = []rune("- [ ] " + l)
		e.col += 6
	}
}

// ---- layout and drawing ----

// noteWidth is the text width of the editor.
func (m Model) noteWidth() int { return max(10, m.detailInnerWidth()-2) }

// wrapLine splits a line into screen segments of at most w runes, at spaces if possible.
// It returns the start of each segment.
func wrapLine(l []rune, w int) []int {
	starts := []int{0}
	for s := 0; len(l)-s > w; {
		cut := s + w
		for i := cut; i > s+w/2; i-- {
			if l[i-1] == ' ' {
				cut = i
				break
			}
		}
		starts = append(starts, cut)
		s = cut
	}
	return starts
}

// screenPos returns the screen line and column of the cursor.
func (e *noteEditor) screenPos(w int) (y, x int) {
	for i := 0; i < e.row; i++ {
		y += len(wrapLine(e.lines[i], w))
	}
	starts := wrapLine(e.lines[e.row], w)
	seg := 0
	for k, s := range starts {
		if e.col >= s {
			seg = k
		}
	}
	return y + seg, e.col - starts[seg]
}

// moveVertical moves the cursor d screen lines and keeps the column if it can.
func (e *noteEditor) moveVertical(d, w int) {
	y, x := e.screenPos(w)
	if e.want < 0 {
		e.want = x
	}
	target := y + d
	if target < 0 {
		return
	}
	n := 0
	for i, l := range e.lines {
		starts := wrapLine(l, w)
		if target < n+len(starts) {
			k := target - n
			end := len(l)
			if k+1 < len(starts) {
				end = starts[k+1] - 1
			}
			e.row, e.col = i, min(starts[k]+e.want, end)
			return
		}
		n += len(starts)
	}
}

// styleKinds of a markdown source line, per rune.
const (
	skText = iota
	skHeading
	skMarker
	skBold
	skItalic
	skCode
	skLink
	skQuote
	skFence
)

var (
	boldSpan   = regexp.MustCompile(`\*\*[^*]+\*\*|__[^_]+__`)
	italicSpan = regexp.MustCompile(`(^|[^*])\*[^*\s][^*]*\*`)
	codeSpan   = regexp.MustCompile("`[^`]+`")
	linkSpan   = regexp.MustCompile(`\[[^\]]*\]\([^)]*\)`)
)

// lineKinds gives each rune of a source line its style. inFence is true inside a fenced code block.
func lineKinds(l string, inFence bool) []int {
	rs := []rune(l)
	k := make([]int, len(rs))
	mark := func(re *regexp.Regexp, kind int) {
		for _, loc := range re.FindAllStringIndex(l, -1) {
			a, b := len([]rune(l[:loc[0]])), len([]rune(l[:loc[1]]))
			for i := a; i < b; i++ {
				if k[i] == skText {
					k[i] = kind
				}
			}
		}
	}
	trim := strings.TrimSpace(l)
	switch {
	case inFence || strings.HasPrefix(trim, "```"):
		for i := range k {
			k[i] = skFence
		}
		return k
	case strings.HasPrefix(trim, "#"):
		for i := range k {
			k[i] = skHeading
		}
		return k
	case strings.HasPrefix(trim, ">"):
		for i := range k {
			k[i] = skQuote
		}
		return k
	}
	if p := listPrefix.FindString(l); p != "" {
		for i := range []rune(p) {
			k[i] = skMarker
		}
	}
	mark(codeSpan, skCode)
	mark(linkSpan, skLink)
	mark(boldSpan, skBold)
	mark(italicSpan, skItalic)
	return k
}

func kindStyle(kind int) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(c(hexText))
	switch kind {
	case skHeading:
		return s.Foreground(c(fg(hexAccent))).Bold(true)
	case skMarker:
		return s.Foreground(c(fg(hexAccent)))
	case skBold:
		return s.Bold(true)
	case skItalic:
		return s.Italic(true)
	case skCode, skFence:
		return s.Foreground(c(hexWeek))
	case skLink:
		return s.Foreground(c(hexToday)).Underline(true)
	case skQuote:
		return s.Foreground(c(hexMuted)).Italic(true)
	}
	return s
}

// noteLines draws the editor text with live styling and the cursor.
func (m Model) noteLines(w, h int) []string {
	e := m.note
	tw := w - 2
	var out []string
	cy, cx := e.screenPos(tw)
	inFence := false
	for i, l := range e.lines {
		src := string(l)
		kinds := lineKinds(src, inFence && !strings.HasPrefix(strings.TrimSpace(src), "```"))
		if strings.HasPrefix(strings.TrimSpace(src), "```") {
			inFence = !inFence
		}
		starts := wrapLine(l, tw)
		level, _ := headingLevel(src)
		if inFence {
			level = 0
		}
		for k, s := range starts {
			end := len(l)
			if k+1 < len(starts) {
				end = starts[k+1]
			}
			var b strings.Builder
			for j := s; j < end; j++ {
				st := kindStyle(kinds[j])
				if level > 0 && kinds[j] == skHeading { // headings are bars in their level color
					st = headingStyle(level)
				}
				if ms, ok := e.matchStyle(i, j); ok { // find matches
					st = ms
				}
				if len(out) == cy && j-s == cx && i == e.row {
					st = st.Reverse(true)
				}
				b.WriteString(st.Render(string(l[j])))
			}
			if len(out) == cy && i == e.row && cx >= end-s { // the cursor after the last rune
				b.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
			}
			if level > 0 { // fill the bar to the full width
				if n := tw - lipgloss.Width(b.String()); n > 0 {
					b.WriteString(headingStyle(level).Render(strings.Repeat(" ", n)))
				}
			}
			out = append(out, " "+b.String())
		}
	}
	// Keep the cursor line on the screen. An open find box covers the top lines of the
	// pane, so the text starts below it.
	top := 0
	if e.search != nil {
		top = min(7, h-1)
	}
	view := h - top
	if cy < e.off {
		e.off = cy
	}
	if cy >= e.off+view {
		e.off = cy - view + 1
	}
	if e.off > 0 && e.off < len(out) {
		out = out[e.off:]
	}
	if top > 0 {
		out = append(make([]string, top), out...)
	}
	return padLines(out, w, h)
}

// noteTitle is the title of the editor pane with the autosave state.
func (m Model) noteTitle() string {
	e := m.note
	state := st(hexToday).Render("saved")
	switch {
	case e.err != "":
		state = st(hexOverdue).Render("not saved: " + e.err)
	case e.saving:
		state = st(hexTomorrow).Render("saving…")
	case e.dirty():
		state = st(hexTomorrow).Render("unsaved")
	}
	return st(fg(hexAccent)).Bold(true).Render(e.title) + st(hexMuted).Render(" · editing · ") + state
}

// noteClick puts the cursor where the user clicks in the editor. A click outside the
// editor saves the note and closes the editor.
func (m Model) noteClick(x, y int) (tea.Model, tea.Cmd) {
	r := m.sideRect()
	if !r.has(x, y) {
		return m.closeNoteEditor()
	}
	e := m.note
	row := r.row(y)
	if row < 0 {
		return m, nil
	}
	target, tw := e.off+row, r.w-4
	n := 0
	for i, l := range e.lines {
		starts := wrapLine(l, tw)
		if target < n+len(starts) {
			k := target - n
			end := len(l)
			if k+1 < len(starts) {
				end = starts[k+1] - 1
			}
			e.row, e.col = i, min(starts[k]+max(0, x-r.x-2), end)
			e.typing, e.want = false, -1
			return m, nil
		}
		n += len(starts)
	}
	e.row = len(e.lines) - 1
	e.col = len(e.lines[e.row])
	return m, nil
}
