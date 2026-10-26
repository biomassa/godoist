package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/glamour/v2"
	gansi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/biomassa/godoist/internal/todoist"
)

// mdCache keeps rendered markdown. View runs on every update, so each text is drawn one
// time. The cache is a pointer, so all copies of the Model share it.
type mdCache struct {
	out       map[string]mdText
	renderers map[int]*glamour.TermRenderer
	code      map[string][]edRow // rendered fenced code blocks by width and text
	preview   map[string][]edRow // formatted editor lines for the width previewW: see previewRows
	previewW  int
	tables    map[string][]edRow // table rows by width and text, with line relative to the table
}

// mdText is a rendered markdown text: its lines, and the first line of each checkbox item.
type mdText struct {
	lines  []string
	checks []int
}

// newMDCache returns an empty cache.
func newMDCache() *mdCache { return &mdCache{} }

// reset clears the cache. Call it when the theme changes.
func (c *mdCache) reset() {
	c.out = map[string]mdText{}
	c.renderers = map[int]*glamour.TermRenderer{}
	c.code = map[string][]edRow{}
	c.preview = map[string][]edRow{}
	c.tables = map[string][]edRow{}
}

// render returns markdown as lines that are at most w cells wide. The reader shows each
// line as the editor preview does (see previewRows). Fenced code blocks and tables show
// as blocks (see blockRows).
func (c *mdCache) render(src string, w int) mdText {
	// A drag of the pane border makes many widths: the cache starts again when it gets large.
	if c.out == nil || len(c.out) > 4000 {
		c.reset()
	}
	key := fmt.Sprintf("%d\x00%s", w, src)
	if t, ok := c.out[key]; ok {
		return t
	}
	tw := max(1, w-1) // the rows have one cell of margin
	var lines [][]rune
	for _, l := range strings.Split(src, "\n") {
		lines = append(lines, []rune(l))
	}
	blockAt := map[int]mdBlock{} // first line → block
	inBlock := make([]bool, len(lines))
	for _, b := range mdBlocks(lines) {
		blockAt[b.from] = b
		for i := b.from; i <= b.to; i++ {
			inBlock[i] = true
		}
	}
	isCheck := map[int]bool{}
	for _, i := range checkboxLines(src) {
		isCheck[i] = true
	}
	var t mdText
	for i, l := range lines {
		var rows []edRow
		switch b, first := blockAt[i]; {
		case first:
			rows = c.blockRows(lines, b, tw)
		case inBlock[i]: // the block rows came with its first line
		default:
			rows = c.previewRows(string(l), tw)
		}
		if isCheck[i] && !inBlock[i] {
			t.checks = append(t.checks, len(t.lines))
		}
		for _, r := range rows {
			t.lines = append(t.lines, r.pre)
		}
	}
	c.out[key] = t
	return t
}

// codeRows returns a fenced code block as rows that are at most w+1 cells wide, with
// syntax colors from Glamour. Each row has one cell of indent, like all body rows. If
// Glamour fails, the rows have plain wrapped text.
func (c *mdCache) codeRows(src string, w int) []edRow {
	if c.code == nil || len(c.code) > 2000 || len(c.renderers) > 12 {
		c.code, c.renderers = map[string][]edRow{}, map[int]*glamour.TermRenderer{}
	}
	key := fmt.Sprintf("%d\x00%s", w, src)
	if rows, ok := c.code[key]; ok {
		return rows
	}
	r := c.renderers[w]
	if r == nil {
		r, _ = glamour.NewTermRenderer(glamour.WithStyles(readerStyle()), glamour.WithWordWrap(max(10, w-2)))
		c.renderers[w] = r
	}
	var lines []string
	if r != nil {
		if out, err := r.Render(src); err == nil {
			lines = strings.Split(strings.Trim(out, "\n"), "\n")
		}
	}
	if lines == nil {
		lines = strings.Split(lipgloss.NewStyle().Width(w).Render(src), "\n")
	}
	// Glamour adds lines with only spaces around a block. Drop them.
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	rows := make([]edRow, len(lines))
	for k, l := range lines {
		rows[k] = edRow{pre: " " + l}
	}
	c.code[key] = rows
	return rows
}

// readerStyle is the Glamour dark or light style with clean headings (no "##" marks) and
// ☐ / ☑ checkboxes. Links are OSC 8 hyperlinks, and fenced code gets syntax colors.
func readerStyle() gansi.StyleConfig {
	st := styles.DarkStyleConfig
	if !darkTheme {
		st = styles.LightStyleConfig
	}
	st.H1.Prefix, st.H1.Suffix = " ", " "
	for _, h := range []*gansi.StyleBlock{&st.H2, &st.H3, &st.H4, &st.H5, &st.H6} {
		h.Prefix = ""
	}
	st.Task.Ticked, st.Task.Unticked = "☑ ", "☐ "
	if themeBg != "" {
		themeReader(&st)
	}
	return st
}

// detailDoc is the content of the details pane before scrolling.
// comments[i] is the line range [start, end) of comment i.
type detailDoc struct {
	lines    []string
	comments [][2]int
	checks   []int // the first lines of the checkbox items of the note body, in order
	// editStart is the first line of the rendered note body (notebook view), for a
	// double-click that opens the editor at that place.
	editStart int
	// Notebook view: the parts of the path above the title (a click opens the note), and
	// the lines of the sub-notes list.
	crumbs   []crumb
	subnotes []crumb
}

// crumb is a note that the reader shows: on line, from column x0 to x1 (x1 -1: the
// whole line).
type crumb struct {
	line, x0, x1 int
	id           string
}

// buildDetail builds the details for the row under the cursor. In notebook view,
// it is a reader with the note body as markdown.
func (m Model) buildDetail(w int) detailDoc {
	d := detailDoc{lines: []string{""}} // an empty line under the pane title
	r := m.currentRow()
	if r == nil {
		return d
	}
	label := func(k, v string) string {
		return " " + st(hexMuted).Render(fmt.Sprintf("%-9s", k)) + v
	}
	wrap := func(s, hex string, bold bool) []string {
		out := lipgloss.NewStyle().Width(max(1, w-2)).Foreground(c(hex)).Bold(bold).Render(s)
		lines := strings.Split(out, "\n")
		for i := range lines {
			lines[i] = " " + lines[i]
		}
		return lines
	}
	if r.task == nil {
		d.lines = append(d.lines, wrap(r.header, hexText, true)...)
		d.lines = append(d.lines, "", label("Items", fmt.Sprint(r.count)))
		if r.sectionID != "" {
			d.lines = append(d.lines, "")
			d.lines = append(d.lines, wrapHint("a add a task here · A new section · e rename · [ ] move · del delete", hexMuted, w-1)...)
		}
		return d
	}
	t := r.task
	notes := m.notesMode()
	now := time.Now()
	if notes && t.Parent() != "" {
		d.crumbs = m.crumbLine(&d, t, w)
	}
	for _, l := range titleLines(t.Content, max(1, w-2), st(hexText).Bold(true)) {
		d.lines = append(d.lines, " "+l)
	}
	comments := m.snap.Comments[t.ID]

	if notes {
		// Notebook reader: a meta line, the body as markdown, then the comments.
		var meta []string
		if s := m.sections[t.Section()]; s != nil {
			meta = append(meta, s.Name)
		}
		if at, err := time.Parse(time.RFC3339, t.AddedAt); err == nil {
			meta = append(meta, "added "+at.Local().Format("2 Jan 2006"))
		}
		if len(t.Labels) > 0 {
			meta = append(meta, "@"+strings.Join(t.Labels, " @"))
		}
		switch n := len(comments); {
		case n == 1:
			meta = append(meta, "1 comment")
		case n > 1:
			meta = append(meta, fmt.Sprintf("%d comments", n))
		}
		d.lines = append(d.lines, " "+st(hexMuted).Render(strings.Join(meta, " · ")))
		d.lines = append(d.lines, " "+st(fg(hexAccent)).Render("enter or E")+st(hexDim).Render(" · edit this note"), "")
		d.editStart = len(d.lines)
		if body := strings.TrimSpace(t.Description); body != "" {
			d.addMarkdown(m.md.render(body, w))
		} else {
			d.lines = append(d.lines, " "+st(hexDim).Render("empty note"))
		}
		m.addSubnoteList(&d, t, w)
	} else {
		d.lines = append(d.lines, "")
		if p := m.projects[t.ProjectID]; p != nil {
			d.lines = append(d.lines, label("Project", st(fg(todoist.ColorHex(p.Color))).Render("# "+p.Name)))
		}
		if s := m.sections[t.Section()]; s != nil {
			d.lines = append(d.lines, label("Section", s.Name))
		}
		if t.Due != nil {
			d.lines = append(d.lines, label("Due", st(dueHex(t.Due, now)).Render(todoist.FormatDue(t.Due, now))))
			if t.Due.IsRecurring {
				d.lines = append(d.lines, label("Repeats", st(hexMuted).Render("↻ "+t.Due.String)))
			}
		}
		prio := t.UIPriority()
		d.lines = append(d.lines, label("Priority", st(priorityHex[prio]).Render(fmt.Sprintf("⚑ P%d", prio))))
		if len(t.Labels) > 0 {
			d.lines = append(d.lines, label("Labels", st(hexTomorrow).Render("@"+strings.Join(t.Labels, " @"))))
		}
		if desc := strings.TrimSpace(t.Description); desc != "" {
			d.lines = append(d.lines, "", " "+st(hexMuted).Render("Description"))
			d.addMarkdown(m.md.render(desc, w))
		}
	}

	if len(comments) > 0 || m.focus == paneDetail {
		d.lines = append(d.lines, "", " "+st(hexMuted).Bold(true).Render(fmt.Sprintf("Comments %d", len(comments))))
	}
	for _, cm := range comments {
		start := len(d.lines)
		stamp := " ── " + cm.Posted().Format("2 Jan 15:04") + " "
		d.lines = append(d.lines, st(hexDim).Render(stamp+strings.Repeat("─", max(0, w-lipgloss.Width(stamp)-1))))
		if body := strings.TrimSpace(cm.Content); body != "" {
			d.addMarkdown(m.md.render(body, w))
		}
		if a := cm.FileAttachment; a != nil {
			if a.URL != "" && strings.Contains(cm.Content, a.URL) {
				a = &todoist.Attachment{FileName: a.FileName, Title: a.Title}
			}
			if line := attachmentLine(a); line != "" {
				d.lines = append(d.lines, wrap(line, hexWeek, false)...)
			}
		}
		d.comments = append(d.comments, [2]int{start, len(d.lines)})
	}
	if m.focus == paneDetail {
		d.lines = append(d.lines, "")
		d.lines = append(d.lines, wrapHint("j/k select comment · c add · e edit · y copy · d delete", hexDim, w-1)...)
	} else {
		d.lines = append(d.lines, "")
		d.lines = append(d.lines, wrapHint("enter or l · comments and editing here", hexDim, w-1)...)
	}
	d.lines = append(d.lines, "", " "+st(hexDim).Render("id "+t.ID))
	return d
}

// crumbLine adds the path of note t above its title: the notes above it, from the top
// level down, each followed by "›". It returns the place of each note on the line.
func (m Model) crumbLine(d *detailDoc, t *todoist.Task, w int) []crumb {
	var path []*todoist.Task
	for p := m.taskByID(t.Parent()); p != nil; p = m.taskByID(p.Parent()) {
		path = append([]*todoist.Task{p}, path...)
	}
	line, x := len(d.lines), 1
	var out []crumb
	var b strings.Builder
	b.WriteString(" ")
	for _, p := range path {
		name := trunc(plain(p.Content), max(4, (w-4)/max(1, len(path))-3))
		out = append(out, crumb{line: line, x0: x, x1: x + lipgloss.Width(name), id: p.ID})
		b.WriteString(st(hexMuted).Underline(true).Render(name) + st(hexDim).Render(" › "))
		x += lipgloss.Width(name) + 3
	}
	d.lines = append(d.lines, b.String())
	return out
}

// addSubnoteList adds the sub-notes of note t after its body: a header with the count and
// one line for each sub-note. tab selects them, and enter or a click opens one.
func (m Model) addSubnoteList(d *detailDoc, t *todoist.Task, w int) {
	var kids []*todoist.Task
	for i := range m.snap.Tasks {
		if m.snap.Tasks[i].Parent() == t.ID {
			kids = append(kids, &m.snap.Tasks[i])
		}
	}
	if len(kids) == 0 {
		return
	}
	sort.SliceStable(kids, func(i, j int) bool { return kids[i].ChildOrder < kids[j].ChildOrder })
	d.lines = append(d.lines, "", " "+st(hexMuted).Bold(true).Render(fmt.Sprintf("Sub-notes %d", len(kids))))
	for _, k := range kids {
		more := ""
		if n := len(m.subtreeIDs(k.ID)) - 1; n > 0 {
			more = st(hexMuted).Render(fmt.Sprintf(" · %d sub-notes", n))
			if n == 1 {
				more = st(hexMuted).Render(" · 1 sub-note")
			}
		}
		d.subnotes = append(d.subnotes, crumb{line: len(d.lines), x1: -1, id: k.ID})
		d.lines = append(d.lines, "  "+st(fg(hexAccent)).Render("○ ")+st(hexText).Render(trunc(plain(k.Content), w-6-lipgloss.Width(more)))+more)
	}
}

// addMarkdown adds rendered markdown lines and records the lines with a checkbox.
func (d *detailDoc) addMarkdown(t mdText) {
	for _, k := range t.checks {
		d.checks = append(d.checks, len(d.lines)+k)
	}
	d.lines = append(d.lines, t.lines...)
}

// detailInnerWidth is the content width of the pane that shows details.
func (m Model) detailInnerWidth() int {
	if m.wide() {
		return m.detailWidth() - 2
	}
	return m.width - m.navWidth() - 2
}

// detailLines renders the visible part of the details pane. A gutter marks the selected comment.
func (m Model) detailLines(w, h int) []string {
	d := m.buildDetail(w - 1)
	sel := [2]int{-1, -1}
	if m.focus == paneDetail && m.comCur >= 0 && m.comCur < len(d.comments) {
		sel = d.comments[m.comCur]
	}
	if m.focus == paneDetail && m.chkCur >= 0 && m.chkCur < len(d.checks) {
		sel = [2]int{d.checks[m.chkCur], d.checks[m.chkCur] + 1}
	}
	if k := m.chkCur - len(d.checks); m.focus == paneDetail && k >= 0 && k < len(d.subnotes) {
		sel = [2]int{d.subnotes[k].line, d.subnotes[k].line + 1}
	}
	lines := make([]string, 0, h)
	for i := m.detOff; i < len(d.lines) && len(lines) < h; i++ {
		gutter := " "
		if i >= sel[0] && i < sel[1] {
			gutter = st(fg(hexAccent)).Render("▌")
		}
		lines = append(lines, gutter+d.lines[i])
	}
	return padLines(lines, w, h)
}

// scrollToComment scrolls the details pane so that the selected comment is visible.
func (m *Model) scrollToComment() {
	if m.comCur < 0 {
		m.detOff = 0
		return
	}
	d := m.buildDetail(m.detailInnerWidth() - 1)
	if m.comCur >= len(d.comments) {
		return
	}
	h := m.paneHeight()
	r := d.comments[m.comCur]
	if r[0] < m.detOff {
		m.detOff = r[0]
	}
	if r[1] > m.detOff+h {
		m.detOff = min(r[0], r[1]-h)
	}
}

// clampDetail keeps the details scroll offset inside the document.
func (m *Model) clampDetail() {
	if m.detOff <= 0 {
		m.detOff = 0
		return
	}
	n := len(m.buildDetail(m.detailInnerWidth() - 1).lines)
	m.detOff = max(0, min(m.detOff, n-m.paneHeight()))
}
