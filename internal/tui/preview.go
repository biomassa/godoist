package tui

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The notebook editor shows a live preview: each line shows rendered markdown, as the
// reader does, except the line with the cursor, which shows its source. A multi-line
// block (fenced code or a table) shows as source when the cursor is in it. While the find
// box is open, all lines show as source, so that the matches are at their true place.

// The reader in the details pane draws its lines with the same code. The code comes from
// mded (github.com/biomassa/mded, internal/markdown).

// pcell is one rune on the editor screen with its style. src is the rune of the source
// line that it comes from, for a click. A marker that the preview adds (• or ☐) has the
// position of the source marker. A table border or padding cell has src -1.
type pcell struct {
	r   rune
	st  lipgloss.Style
	src int
}

// edRow is one screen row of the editor. A rendered block or a cached row has its text in
// pre. Other rows have cells.
type edRow struct {
	cells  []pcell
	pre    string
	line   int  // the source line of the row
	fill   int  // a heading of this level: the row gets its bar (see headingFill)
	source bool // the row shows source, so it can have the cursor
	start  int  // source rows: the first rune of the line on this row
}

var (
	// inlineToken matches the inline markdown that the preview renders, in order of
	// priority: backslash escape, code, autolink, bold, strike, link, bare URL, italic.
	inlineToken = regexp.MustCompile("\\\\[!-/:-@\\[-`{-~]|`[^`]+`|<[a-zA-Z][a-zA-Z0-9+.-]{1,31}:[^\\s<>]*>|\\*\\*[^*]+\\*\\*|__[^_]+__|~~[^~]+~~|\\[[^\\]]+\\]\\([^)\\s]+\\)|https?://[^\\s<>()\\[\\]]+|\\*[^*\\s][^*]*\\*|_[^_\\s][^_]*_")
	checkItem   = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])\s+\[([ xX])\]\s+(.*)$`)
	bulletItem  = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	numberItem  = regexp.MustCompile(`^(\s*)(\d+[.)])\s+(.*)$`)
	quoteLine   = regexp.MustCompile(`^\s*>\s?(.*)$`)
	ruleLine    = regexp.MustCompile(`^\s*([-*_])(\s*[-*_]){2,}\s*$`)
	// tableDelim matches the delimiter row of a table: cells of dashes with optional
	// colons, between pipes. The outer pipes are optional.
	tableDelim = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
)

// inlineCells renders the inline markdown of s. offset is the rune position of s in its
// source line. The text of bold, italic, strikethrough, and link spans gets its own inline
// markdown, for example code in bold text.
func inlineCells(s string, offset int, base lipgloss.Style) []pcell {
	rs := []rune(s)
	var out []pcell
	part := func(a, b int, st lipgloss.Style) {
		for i := a; i < b; i++ {
			out = append(out, pcell{rs[i], st, offset + i})
		}
	}
	inner := func(a, b int, st lipgloss.Style) {
		out = append(out, inlineCells(string(rs[a:b]), offset+a, st)...)
	}
	at := 0
	for _, loc := range inlineToken.FindAllStringIndex(s, -1) {
		a := utf8.RuneCountInString(s[:loc[0]])
		b := a + utf8.RuneCountInString(s[loc[0]:loc[1]])
		tok := s[loc[0]:loc[1]]
		// "_" and "*" inside a word (snake_case, 2*3*4) are not italic marks.
		if (tok[0] == '_' || (tok[0] == '*' && !strings.HasPrefix(tok, "**"))) && a > 0 && isWordRune(rs[a-1]) {
			continue
		}
		part(at, a, base)
		switch {
		case tok[0] == '\\': // an escaped mark shows as text
			part(a+1, b, base)
		case tok[0] == '`':
			part(a+1, b-1, base.Foreground(c(hexCode)))
		case tok[0] == '<': // an autolink: <https://example.com>
			url := tok[1 : len(tok)-1]
			part(a+1, b-1, base.Foreground(c(hexLink)).Underline(true).Hyperlink(url))
		case strings.HasPrefix(tok, "**") || strings.HasPrefix(tok, "__"):
			inner(a+2, b-2, base.Bold(true))
		case strings.HasPrefix(tok, "~~"):
			inner(a+2, b-2, base.Strikethrough(true))
		case tok[0] == '[':
			close := strings.Index(tok, "](")
			text := utf8.RuneCountInString(tok[1:close])
			url := tok[close+2 : len(tok)-1]
			inner(a+1, a+1+text, base.Foreground(c(hexLink)).Underline(true).Hyperlink(url))
		case strings.HasPrefix(tok, "http"):
			part(a, b, base.Foreground(c(hexLink)).Underline(true).Hyperlink(tok))
		default: // *italic* or _italic_
			inner(a+1, b-1, base.Italic(true))
		}
		at = b
	}
	part(at, len(rs), base)
	return out
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// previewCells renders one source line that is not in a block, as the reader shows it.
// It reports a heading level, so that the row can be a heading bar. Level -1 is a rule.
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
	runeAt := func(byteIdx int) int { return utf8.RuneCountInString(src[:byteIdx]) }
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
		if src[sm[6]:sm[7]] != " " {
			box = '☑'
		}
		cells = lead(src[sm[2]:sm[3]])
		if lit := src[sm[4]:sm[5]]; lit[0] >= '0' && lit[0] <= '9' { // a numbered item keeps its number
			for i, r := range lit {
				cells = append(cells, pcell{r, marker, runeAt(sm[4]) + i})
			}
			cells = append(cells, pcell{' ', text, runeAt(sm[5])})
		}
		mark := runeAt(sm[6])
		cells = append(cells, pcell{box, marker, mark}, pcell{' ', text, mark})
		return append(cells, inlineCells(src[sm[8]:sm[9]], runeAt(sm[8]), text)...), 0
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

// mdBlock is a fenced code block or a table: source lines from and to, both included.
// table is true for a table.
type mdBlock struct {
	from, to int
	table    bool
}

// hasPrefixTrim reports whether l, without its leading space, starts with p.
func hasPrefixTrim(l []rune, p string) bool {
	i := 0
	for i < len(l) && unicode.IsSpace(l[i]) {
		i++
	}
	n := 0
	for _, r := range p {
		if i+n >= len(l) || l[i+n] != r {
			return false
		}
		n++
	}
	return true
}

// isFence reports whether the line starts or ends a fenced code block.
func isFence(l []rune) bool { return hasPrefixTrim(l, "```") }

// mdBlocks finds the fenced code blocks and the tables of a text. An open fence goes to
// the last line.
func mdBlocks(lines [][]rune) []mdBlock {
	var out []mdBlock
	for i := 0; i < len(lines); i++ {
		if isFence(lines[i]) {
			j := i + 1
			for j < len(lines) && !isFence(lines[j]) {
				j++
			}
			out = append(out, mdBlock{from: i, to: min(j, len(lines)-1)})
			i = j
			continue
		}
		if isTableStart(lines, i) {
			j := i + 1
			for j+1 < len(lines) && hasPipe(lines[j+1]) && !isFence(lines[j+1]) {
				j++
			}
			out = append(out, mdBlock{from: i, to: j, table: true})
			i = j
		}
	}
	return out
}

// hasPipe reports whether l has a "|" that is not escaped.
func hasPipe(l []rune) bool {
	for i, r := range l {
		if r == '|' && (i == 0 || l[i-1] != '\\') {
			return true
		}
	}
	return false
}

// isTableStart reports whether line i is the header of a table, as in GitHub Flavored
// Markdown: a line with a "|", then a delimiter row with the same number of cells. A
// heading, a list item, or a quote is not a header.
func isTableStart(lines [][]rune, i int) bool {
	if !hasPipe(lines[i]) || i+1 >= len(lines) || !tableDelim.MatchString(string(lines[i+1])) {
		return false
	}
	src := string(lines[i])
	if lv, _ := headingLevel(src); lv > 0 || bulletItem.MatchString(src) || numberItem.MatchString(src) || quoteLine.MatchString(src) {
		return false
	}
	return len(splitTableRow(lines[i])) == len(splitTableRow(lines[i+1]))
}

// codeLines returns the first and the last code line of a fenced block, without the fence
// lines. Glamour does not show the fences, so row k of the block shows code line first+k
// (a long line that wraps makes this an approximation). A block with no code gives its
// first line.
func codeLines(lines [][]rune, b mdBlock) (first, last int) {
	first, last = b.from+1, b.to
	if last > b.from && isFence(lines[last]) {
		last--
	}
	if last < first {
		return b.from, b.from
	}
	return first, last
}

// sourceRows makes the rows of source line i with the live styles of the source view.
func (e *noteEditor) sourceRows(i int, inFence bool, tw int) []edRow {
	l := e.lines[i]
	src := string(l)
	kinds := lineKinds(src, inFence && !isFence(l))
	level, _ := headingLevel(src)
	if inFence {
		level = 0
	}
	var rows []edRow
	starts := wrapLine(l, bodyWidth(tw)) // every source row has the indent
	for k, s := range starts {
		end := len(l)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		row := edRow{line: i, source: true, start: s}
		for j := s; j < end; j++ {
			st := kindStyle(kinds[j])
			if level > 0 && kinds[j] == skHeading { // headings have the style of their level
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

// noteLayout returns the rows of the inline editor that are on the screen (see
// editorLayout).
func (m Model) noteLayout(tw, view int) (rows []edRow, cursor int) {
	return m.md.editorLayout(m.note, tw, view, true)
}

// editorLayout returns the rows of editor e that are on the screen, for text width tw
// and view rows, and the screen row of the cursor in them. It scrolls the editor so that
// the cursor is on the screen. Each line uses its rows from the cache, so a redraw formats
// only the cursor line (see previewRows). Only the visible rows are copied. If focused is
// false, all lines show rendered.
func (c *mdCache) editorLayout(e *noteEditor, tw, view int, focused bool) (rows []edRow, cursor int) {
	blockAt := map[int]mdBlock{} // first line → block
	inBlock := make([]bool, len(e.lines))
	active := mdBlock{from: -1, to: -1} // the block with the cursor shows as source
	for _, b := range mdBlocks(e.lines) {
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
		asSource := focused && (e.search != nil || i == e.row || (i >= active.from && i <= active.to) || e.selLine(i))
		switch {
		case asSource:
			if i == e.row {
				cy = total
				for k, s := range wrapLine(e.lines[i], bodyWidth(tw)) {
					if e.col >= s {
						cy = total + k
					}
				}
			}
			lineRows[i] = e.sourceRows(i, fence, tw)
		case inBlock[i]:
			if b, first := blockAt[i]; first { // the block rows come with its first line
				lineRows[i] = c.blockRows(e.lines, b, tw)
			}
		default:
			lineRows[i] = c.previewRows(string(e.lines[i]), tw)
		}
		total += len(lineRows[i])
		if isFence(e.lines[i]) {
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
				if b, ok := blockAt[i]; ok && !r.source {
					if !b.table { // a code block row: its source line, for a click
						first, last := codeLines(e.lines, b)
						r.line = max(first, min(first+k, last))
					}
				} else {
					r.line = i
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
// A table row has its source line. The markdown cache keeps the rendered text.
func (c *mdCache) blockRows(lines [][]rune, b mdBlock, tw int) []edRow {
	cw := bodyWidth(tw) // a block is body content: it has the indent
	if b.table {
		return c.tableRows(lines, b, cw)
	}
	text := make([]string, 0, b.to-b.from+1)
	for j := b.from; j <= b.to; j++ {
		text = append(text, string(lines[j]))
	}
	return c.codeRows(strings.Join(text, "\n"), cw+2)
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
	var rows []edRow
	cells, level := previewCells(src)
	if level == -1 { // a rule: the margin, the indent, and the line
		rows = []edRow{{pre: "  " + st(hexDim).Render(strings.Repeat("─", bodyWidth(tw)))}}
	} else {
		wrapW := bodyWidth(tw) // a heading has no indent: its first cell is the padding
		if level > 0 {
			wrapW = tw
		}
		rs := make([]rune, len(cells))
		for k, cl := range cells {
			rs[k] = cl.r
		}
		starts := wrapLine(rs, wrapW)
		for k, s := range starts {
			end := len(cells)
			if k+1 < len(starts) {
				end = starts[k+1]
			}
			row := edRow{cells: cells[s:end]}
			if level > 0 {
				row.fill = level
			}
			if level == 1 {
				row.cells = centerCells(row.cells, tw)
			}
			row.pre = renderRow(row, tw, -1)
			rows = append(rows, row)
		}
	}
	c.preview[src] = rows
	return rows
}

// centerCells puts the text of a level 1 heading row in the middle of a bar of width tw.
// It drops the spaces around the text and adds padding cells before it. The padding cells
// have the bar style and the source position of the first text cell, for a click.
func centerCells(cells []pcell, tw int) []pcell {
	a, b := 0, len(cells)
	for a < b && cells[a].r == ' ' {
		a++
	}
	for b > a && cells[b-1].r == ' ' {
		b--
	}
	if a == b {
		return cells
	}
	text := cells[a:b]
	var sb strings.Builder
	for _, cl := range text {
		sb.WriteRune(cl.r)
	}
	pad := max(1, (tw-ansi.StringWidth(sb.String()))/2)
	out := make([]pcell, 0, pad+len(text))
	for range pad {
		out = append(out, pcell{' ', headingStyle(1), text[0].src})
	}
	return append(out, text...)
}

// bodyWidth returns the width of the text of a row that is not a heading. The row has
// one cell of indent, so that its text starts in the same column as the text of a heading.
func bodyWidth(tw int) int { return max(1, tw-1) }

// indent returns the number of cells between the row margin and the first cell of the
// row: 0 for a heading (its first cell is the padding of the bar), else 1. A heading
// line that shows as source has the indent inside its bar.
func (r edRow) indent() int {
	if r.fill > 0 && !r.source {
		return 0
	}
	return 1
}

// renderRow draws a row of width tw. cx is the cursor column in the row, or -1.
func renderRow(r edRow, tw, cx int) string {
	if r.pre != "" || r.cells == nil && !r.source && r.fill == 0 {
		return r.pre
	}
	lead := " " // the row margin
	switch {
	case r.indent() == 0:
	case r.fill > 0: // a heading as source: the indent belongs to the bar
		lead += headingStyle(r.fill).Render(" ")
	default:
		lead += " "
	}
	var b strings.Builder
	for k := 0; k < len(r.cells); k++ {
		cl := r.cells[k]
		st := cl.st
		if k == cx {
			st = st.Reverse(true)
		}
		// A variation selector or a combining mark goes into the same styled text as the
		// rune before it. An escape sequence between them breaks the glyph in the terminal.
		text := []rune{cl.r}
		for k+1 < len(r.cells) && joins(r.cells[k+1].r) {
			k++
			text = append(text, r.cells[k].r)
		}
		b.WriteString(st.Render(string(text)))
	}
	if cx >= 0 && cx >= len(r.cells) { // the cursor after the last rune
		b.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
	}
	if r.fill > 0 { // the bar after the text: to the full width, a short bar, or none
		n := tw - r.indent() - lipgloss.Width(b.String())
		if f := headingFill(r.fill); f >= 0 {
			n = min(n, f)
		}
		if n > 0 {
			b.WriteString(headingStyle(r.fill).Render(strings.Repeat(" ", n)))
		}
	}
	return lead + b.String()
}

// joins reports whether r joins the rune before it: a variation selector or a
// combining mark.
func joins(r rune) bool {
	return r >= 0xFE00 && r <= 0xFE0F || unicode.Is(unicode.Mn, r)
}
