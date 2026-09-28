package tui

import (
	"regexp"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
)

// The notebook editor shows a live preview: each line shows rendered markdown, as the
// reader does, except the line with the cursor, which shows its source. A multi-line
// block (fenced code or a table) shows as source when the cursor is in it. While the find
// box is open, all lines show as source, so that the matches are at their true place.

// pcell is one rune on the editor screen with its style. src is the rune of the source
// line that it comes from, for a click. A marker that the preview adds (• or ☐) has the
// position of the source marker.
type pcell struct {
	r   rune
	st  lipgloss.Style
	src int
}

// edRow is one screen row of the editor. A rendered block has its text in pre. Other
// rows have cells.
type edRow struct {
	cells  []pcell
	pre    string
	line   int  // the source line of the row
	fill   int  // a heading bar of this level: the row gets its style to the full width
	source bool // the row shows source, so it can have the cursor
	start  int  // source rows: the first rune of the line on this row
}

var (
	// inlineToken matches the inline markdown that the preview renders, in order of
	// priority: code, bold, strike, link, bare URL, italic.
	inlineToken = regexp.MustCompile("`[^`]+`|\\*\\*[^*]+\\*\\*|__[^_]+__|~~[^~]+~~|\\[[^\\]]+\\]\\([^)\\s]+\\)|https?://[^\\s<>()\\[\\]]+|\\*[^*\\s][^*]*\\*|_[^_\\s][^_]*_")
	checkItem   = regexp.MustCompile(`^(\s*)[-*+]\s+\[([ xX])\]\s+(.*)$`)
	bulletItem  = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	numberItem  = regexp.MustCompile(`^(\s*)(\d+[.)])\s+(.*)$`)
	quoteLine   = regexp.MustCompile(`^\s*>\s?(.*)$`)
	ruleLine    = regexp.MustCompile(`^\s*([-*_])(\s*[-*_]){2,}\s*$`)
	tableSep    = regexp.MustCompile(`^\s*\|?\s*:?-{3,}`)
)

// inlineCells renders the inline markdown of s. offset is the rune position of s in its
// source line.
func inlineCells(s string, offset int, base lipgloss.Style) []pcell {
	rs := []rune(s)
	var out []pcell
	plainPart := func(a, b int) {
		for i := a; i < b; i++ {
			out = append(out, pcell{rs[i], base, offset + i})
		}
	}
	part := func(a, b int, st lipgloss.Style) {
		for i := a; i < b; i++ {
			out = append(out, pcell{rs[i], st, offset + i})
		}
	}
	at := 0
	for _, loc := range inlineToken.FindAllStringIndex(s, -1) {
		a, b := len([]rune(s[:loc[0]])), len([]rune(s[:loc[1]]))
		tok := string(rs[a:b])
		// "_" and "*" inside a word (snake_case, 2*3*4) are not italic marks.
		if (tok[0] == '_' || (tok[0] == '*' && !strings.HasPrefix(tok, "**"))) && a > 0 && isWordRune(rs[a-1]) {
			continue
		}
		plainPart(at, a)
		switch {
		case tok[0] == '`':
			part(a+1, b-1, base.Foreground(c(hexCode)))
		case strings.HasPrefix(tok, "**") || strings.HasPrefix(tok, "__"):
			part(a+2, b-2, base.Bold(true))
		case strings.HasPrefix(tok, "~~"):
			part(a+2, b-2, base.Strikethrough(true))
		case tok[0] == '[':
			close := strings.Index(tok, "](")
			text := []rune(tok[1:close])
			url := tok[close+2 : len(tok)-1]
			part(a+1, a+1+len(text), base.Foreground(c(hexLink)).Underline(true).Hyperlink(url))
		case strings.HasPrefix(tok, "http"):
			part(a, b, base.Foreground(c(hexLink)).Underline(true).Hyperlink(tok))
		default: // *italic* or _italic_
			part(a+1, b-1, base.Italic(true))
		}
		at = b
	}
	plainPart(at, len(rs))
	return out
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// previewCells renders one source line that is not in a block, as the reader shows it.
// It reports a heading level, so that the row can be a full-width bar.
func previewCells(src string) (cells []pcell, level int) {
	text := kindStyle(skText)
	marker := kindStyle(skMarker)
	lead := func(indent string) []pcell {
		var out []pcell
		for i, r := range indent {
			out = append(out, pcell{r, text, i})
		}
		return out
	}
	runeAt := func(byteIdx int) int { return len([]rune(src[:byteIdx])) }
	if lv, h := headingLevel(src); lv > 0 {
		st := headingStyle(lv)
		off := runeAt(strings.Index(src, h))
		return append([]pcell{{' ', st, 0}}, inlineCells(h, off, st)...), lv
	}
	if ruleLine.MatchString(src) {
		return []pcell{{'─', st(hexDim), 0}}, -1 // -1: a rule to the full width
	}
	if sm := checkItem.FindStringSubmatchIndex(src); sm != nil {
		box := '☐'
		if src[sm[4]:sm[5]] != " " {
			box = '☑'
		}
		cells = lead(src[sm[2]:sm[3]])
		cells = append(cells, pcell{box, marker, runeAt(sm[4])}, pcell{' ', text, runeAt(sm[4])})
		return append(cells, inlineCells(src[sm[6]:sm[7]], runeAt(sm[6]), text)...), 0
	}
	if sm := bulletItem.FindStringSubmatchIndex(src); sm != nil {
		cells = lead(src[sm[2]:sm[3]])
		cells = append(cells, pcell{'•', marker, runeAt(sm[3])}, pcell{' ', text, runeAt(sm[3])})
		return append(cells, inlineCells(src[sm[4]:sm[5]], runeAt(sm[4]), text)...), 0
	}
	if sm := numberItem.FindStringSubmatchIndex(src); sm != nil {
		cells = lead(src[sm[2]:sm[3]])
		for i, r := range src[sm[4]:sm[5]] {
			cells = append(cells, pcell{r, marker, runeAt(sm[4]) + i})
		}
		cells = append(cells, pcell{' ', text, runeAt(sm[5])})
		return append(cells, inlineCells(src[sm[6]:sm[7]], runeAt(sm[6]), text)...), 0
	}
	if sm := quoteLine.FindStringSubmatchIndex(src); sm != nil {
		q := kindStyle(skQuote)
		cells = []pcell{{'│', st(hexMuted), 0}, {' ', q, 0}}
		return append(cells, inlineCells(src[sm[2]:sm[3]], runeAt(sm[2]), q)...), 0
	}
	return inlineCells(src, 0, text), 0
}

// noteBlock is a fenced code block or a table: source lines from and to, both included.
type noteBlock struct{ from, to int }

// noteBlocks finds the fenced code blocks and the tables of the note.
func (e *noteEditor) noteBlocks() []noteBlock {
	var out []noteBlock
	isRow := func(i int) bool { return strings.HasPrefix(strings.TrimSpace(string(e.lines[i])), "|") }
	for i := 0; i < len(e.lines); i++ {
		l := strings.TrimSpace(string(e.lines[i]))
		if strings.HasPrefix(l, "```") {
			j := i + 1
			for j < len(e.lines) && !strings.HasPrefix(strings.TrimSpace(string(e.lines[j])), "```") {
				j++
			}
			out = append(out, noteBlock{i, min(j, len(e.lines)-1)})
			i = j
			continue
		}
		if isRow(i) && i+1 < len(e.lines) && tableSep.MatchString(string(e.lines[i+1])) {
			j := i + 1
			for j+1 < len(e.lines) && isRow(j+1) {
				j++
			}
			out = append(out, noteBlock{i, j})
			i = j
		}
	}
	return out
}

// sourceRows makes the rows of source line i with the live styles of the source view.
func (e *noteEditor) sourceRows(i int, inFence bool, tw int) []edRow {
	l := e.lines[i]
	src := string(l)
	kinds := lineKinds(src, inFence && !strings.HasPrefix(strings.TrimSpace(src), "```"))
	level, _ := headingLevel(src)
	if inFence {
		level = 0
	}
	var rows []edRow
	starts := wrapLine(l, tw)
	for k, s := range starts {
		end := len(l)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		row := edRow{line: i, source: true, start: s}
		for j := s; j < end; j++ {
			st := kindStyle(kinds[j])
			if level > 0 && kinds[j] == skHeading { // headings are bars in their level color
				st = headingStyle(level)
			}
			if ms, ok := e.matchStyle(i, j); ok { // find matches
				st = ms
			}
			if e.inSel(i, j) {
				st = selectionStyle(st)
			}
			row.cells = append(row.cells, pcell{l[j], st, j})
		}
		if level > 0 {
			row.fill = level
		}
		rows = append(rows, row)
	}
	return rows
}

// noteLayout returns the rows of the editor that are on the screen, for text width tw
// and view rows, and the screen row of the cursor in them. It scrolls the editor so that
// the cursor is on the screen. Each line uses its rows from the cache, so a redraw formats
// only the cursor line (see previewRows). Only the visible rows are copied.
func (m Model) noteLayout(tw, view int) (rows []edRow, cursor int) {
	e := m.note
	blocks := e.noteBlocks()
	blockAt := map[int]noteBlock{} // first line → block
	inBlock := make([]bool, len(e.lines))
	active := noteBlock{-1, -1} // the block with the cursor shows as source
	for _, b := range blocks {
		blockAt[b.from] = b
		for i := b.from; i <= b.to; i++ {
			inBlock[i] = true
		}
		if e.row >= b.from && e.row <= b.to {
			active = b
		}
	}
	// Pass 1: the rows of each line, and the row of the cursor.
	lineRows := make([][]edRow, len(e.lines))
	total, cy := 0, 0
	fence := false // for the source styles: the line is in a fenced block
	for i := range e.lines {
		src := string(e.lines[i])
		isFence := strings.HasPrefix(strings.TrimSpace(src), "```")
		asSource := e.search != nil || i == e.row || (i >= active.from && i <= active.to) || e.selLine(i)
		switch {
		case asSource:
			if i == e.row {
				cy = total
				for k, s := range wrapLine(e.lines[i], tw) {
					if e.col >= s {
						cy = total + k
					}
				}
			}
			lineRows[i] = e.sourceRows(i, fence, tw)
		case inBlock[i]:
			if b, first := blockAt[i]; first { // the block rows come with its first line
				lineRows[i] = m.md.blockRows(e, b, tw)
			}
		default:
			lineRows[i] = m.md.previewRows(src, tw)
		}
		total += len(lineRows[i])
		if isFence {
			fence = !fence
		}
	}
	// Scroll so that the cursor row is on the screen.
	if cy < e.off {
		e.off = cy
	}
	if cy >= e.off+view {
		e.off = cy - view + 1
	}
	e.off = max(0, min(e.off, total-1))
	// Pass 2: copy the visible rows.
	n := 0
	for i, lr := range lineRows {
		for k := range lr {
			if n >= e.off && n < e.off+view {
				r := lr[k]
				r.line = i
				if b, ok := blockAt[i]; ok && r.cells == nil && !r.source {
					r.line = min(b.from+k, b.to) // a block row: its source line, for a click
				}
				rows = append(rows, r)
			}
			n++
		}
		if n >= e.off+view {
			break
		}
	}
	return rows, cy - e.off
}

// blockRows returns the rows of a fenced code block or a table as the reader shows it.
// The markdown cache keeps the rendered text.
func (c *mdCache) blockRows(e *noteEditor, b noteBlock, tw int) []edRow {
	text := make([]string, 0, b.to-b.from+1)
	for j := b.from; j <= b.to; j++ {
		text = append(text, string(e.lines[j]))
	}
	rendered := c.render(strings.Join(text, "\n"), tw+2)
	rows := make([]edRow, len(rendered))
	for k, l := range rendered {
		rows[k] = edRow{pre: l}
	}
	return rows
}

// previewRows returns the formatted rows of a source line for width tw. Each row is drawn
// one time and kept in the cache with its cells (for a click), so that a key press
// draws only the lines that change: the cursor line and the line that it leaves.
// The cache is cleared when the theme changes, and when it gets large.
// It is a method of the cache, not of Model, because a Model is large to copy.
func (c *mdCache) previewRows(src string, tw int) []edRow {
	if c.preview == nil || len(c.preview) > 20000 || c.previewW != tw {
		c.preview, c.previewW = map[string][]edRow{}, tw // one width at a time
	}
	if rows, ok := c.preview[src]; ok {
		return rows
	}
	key := src
	var rows []edRow
	cells, level := previewCells(src)
	if level == -1 { // a rule
		rows = []edRow{{pre: " " + st(hexDim).Render(strings.Repeat("─", tw))}}
	} else {
		rs := make([]rune, len(cells))
		for k, cl := range cells {
			rs[k] = cl.r
		}
		starts := wrapLine(rs, tw)
		for k, s := range starts {
			end := len(cells)
			if k+1 < len(starts) {
				end = starts[k+1]
			}
			row := edRow{cells: cells[s:end]}
			if level > 0 {
				row.fill = level
			}
			row.pre = renderRow(row, tw, -1)
			rows = append(rows, row)
		}
	}
	c.preview[key] = rows
	return rows
}

// renderRow draws a row of width tw. cx is the cursor column in the row, or -1.
func renderRow(r edRow, tw, cx int) string {
	if r.pre != "" || r.cells == nil && !r.source && r.fill == 0 {
		if r.pre == "" {
			return ""
		}
		return r.pre
	}
	var b strings.Builder
	for k, cl := range r.cells {
		st := cl.st
		if k == cx {
			st = st.Reverse(true)
		}
		b.WriteString(st.Render(string(cl.r)))
	}
	if cx >= len(r.cells) { // the cursor after the last rune
		b.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
	}
	if r.fill > 0 { // fill the bar to the full width
		if n := tw - lipgloss.Width(b.String()); n > 0 {
			b.WriteString(headingStyle(r.fill).Render(strings.Repeat(" ", n)))
		}
	}
	return " " + b.String()
}
