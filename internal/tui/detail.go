package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/glamour/v2"
	gansi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/biomassa/godoist/internal/todoist"
)

// mdCache keeps rendered markdown. Glamour is slow, and View runs on every update.
// The cache is a pointer, so all copies of the Model share it.
type mdCache struct {
	out       map[string][]string
	renderers map[int]*glamour.TermRenderer
}

// newMDCache returns an empty cache.
func newMDCache() *mdCache { return &mdCache{} }

// reset clears the cache. Call it when the theme changes.
func (c *mdCache) reset() {
	c.out = map[string][]string{}
	c.renderers = map[int]*glamour.TermRenderer{}
}

// render returns markdown as lines that are at most w cells wide.
// If glamour fails, it returns plain wrapped text.
func (c *mdCache) render(src string, w int) []string {
	if c.out == nil {
		c.reset()
	}
	key := fmt.Sprintf("%d\x00%s", w, src)
	if lines, ok := c.out[key]; ok {
		return lines
	}
	r := c.renderers[w]
	if r == nil {
		r, _ = glamour.NewTermRenderer(glamour.WithStyles(readerStyle()), glamour.WithWordWrap(max(10, w-2)))
		c.renderers[w] = r
	}
	// Headings are full-width bars in the color of their level. Glamour renders the text
	// between them.
	var lines []string
	var chunk []string
	flush := func() {
		text := strings.Trim(strings.Join(chunk, "\n"), "\n")
		chunk = nil
		if strings.TrimSpace(text) == "" {
			return
		}
		var part []string
		if r != nil {
			if out, err := r.Render(text); err == nil {
				part = strings.Split(strings.Trim(out, "\n"), "\n")
			}
		}
		if part == nil {
			part = strings.Split(lipgloss.NewStyle().Width(w).Render(text), "\n")
		}
		// Glamour adds lines with only spaces around a part. Drop them, so that one empty
		// line separates a part from a heading.
		for len(part) > 0 && strings.TrimSpace(ansi.Strip(part[0])) == "" {
			part = part[1:]
		}
		for len(part) > 0 && strings.TrimSpace(ansi.Strip(part[len(part)-1])) == "" {
			part = part[:len(part)-1]
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, part...)
	}
	inFence := false
	for _, l := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inFence = !inFence
		}
		level, text := headingLevel(l)
		if inFence || level == 0 {
			chunk = append(chunk, l)
			continue
		}
		flush()
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, headingBar(level, plain(text), w))
	}
	flush()
	c.out[key] = lines
	return lines
}

// headingLevel returns the level (1 to 6) and the text of an ATX heading line, or 0.
func headingLevel(l string) (int, string) {
	t := strings.TrimLeft(l, " ")
	n := 0
	for n < len(t) && n < 7 && t[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || (n < len(t) && t[n] != ' ') {
		return 0, ""
	}
	return n, strings.TrimSpace(strings.TrimRight(strings.TrimSpace(t[n:]), "#"))
}

// headingHex is the color of a heading level: red, yellow, green, blue, orange, purple.
func headingHex(level int) string {
	hexes := []string{"#DC4C3E", "#E0B000", "#369307", "#4180FF", "#FF9A14", "#A970FF"}
	return hexes[max(1, min(level, 6))-1]
}

// headingStyle is the bar style of a heading level: a tinted background and bright text.
func headingStyle(level int) lipgloss.Style {
	h := headingHex(level)
	text := mix(h, "#FFFFFF", 0.55)
	if !darkTheme {
		text = mix(h, "#000000", 0.45)
	}
	return lipgloss.NewStyle().Background(c(mix(h, baseBg, 0.62))).Foreground(c(text)).Bold(true)
}

// headingBar draws a heading as a full-width bar.
func headingBar(level int, text string, w int) string {
	st := headingStyle(level)
	return st.Render(" " + trunc(text, max(1, w-3)) + strings.Repeat(" ", max(0, w-2-lipgloss.Width(trunc(text, max(1, w-3))))))
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
	return st
}

// detailDoc is the content of the details pane before scrolling.
// comments[i] is the line range [start, end) of comment i.
type detailDoc struct {
	lines    []string
	comments [][2]int
	checks   []int // lines with a ☐ or ☑ of the note body, in order
	// editStart is the first line of the rendered note body (notebook view), for a
	// double-click that opens the editor at that place.
	editStart int
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
	d.lines = append(d.lines, wrap(plain(t.Content), hexText, true)...)
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
			start := len(d.lines)
			d.lines = append(d.lines, m.md.render(body, w)...)
			for i := start; i < len(d.lines); i++ {
				if l := strings.TrimSpace(ansi.Strip(d.lines[i])); strings.HasPrefix(l, "☐ ") || strings.HasPrefix(l, "☑ ") {
					d.checks = append(d.checks, i)
				}
			}
		} else {
			d.lines = append(d.lines, " "+st(hexDim).Render("empty note"))
		}
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
			for _, para := range strings.Split(desc, "\n") {
				d.lines = append(d.lines, wrap(para, hexBody, false)...)
			}
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
			if notes {
				start := len(d.lines)
				d.lines = append(d.lines, m.md.render(body, w)...)
				for i := start; i < len(d.lines); i++ {
					if l := strings.TrimSpace(ansi.Strip(d.lines[i])); strings.HasPrefix(l, "☐ ") || strings.HasPrefix(l, "☑ ") {
						d.checks = append(d.checks, i)
					}
				}
			} else {
				for _, para := range strings.Split(body, "\n") {
					d.lines = append(d.lines, wrap(para, hexBody, false)...)
				}
			}
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
		d.lines = append(d.lines, wrapHint("j/k select comment · c add · e edit · d delete", hexDim, w-1)...)
	} else {
		d.lines = append(d.lines, "")
		d.lines = append(d.lines, wrapHint("enter or l · comments and editing here", hexDim, w-1)...)
	}
	d.lines = append(d.lines, "", " "+st(hexDim).Render("id "+t.ID))
	return d
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
