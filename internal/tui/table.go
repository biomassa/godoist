package tui

import (
	"strconv"
	"strings"

	"github.com/clipperhouse/displaywidth"
)

// tableCell is the text of one table cell and the rune offset of the text in its line.
type tableCell struct {
	text string
	off  int
}

// splitTableRow splits a table row at its "|" marks. An escaped "\|" does not split. Each
// cell text has no space at the start and the end.
func splitTableRow(l []rune) []tableCell {
	var segs [][2]int
	a := 0
	for i := 0; i <= len(l); i++ {
		if i == len(l) || l[i] == '|' && (i == 0 || l[i-1] != '\\') {
			segs = append(segs, [2]int{a, i})
			a = i + 1
		}
	}
	blank := func(s [2]int) bool { return strings.TrimSpace(string(l[s[0]:s[1]])) == "" }
	if len(segs) > 0 && blank(segs[0]) && strings.HasPrefix(strings.TrimSpace(string(l)), "|") {
		segs = segs[1:]
	}
	if len(segs) > 0 && blank(segs[len(segs)-1]) {
		segs = segs[:len(segs)-1]
	}
	out := make([]tableCell, 0, len(segs))
	for _, s := range segs {
		a, b := s[0], s[1]
		for a < b && l[a] == ' ' || a < b && l[a] == '\t' {
			a++
		}
		for b > a && (l[b-1] == ' ' || l[b-1] == '\t') {
			b--
		}
		out = append(out, tableCell{string(l[a:b]), a})
	}
	return out
}

// tableAlign is the alignment of a column: 0 left, 1 center, 2 right.
func tableAlign(sep string) int {
	sep = strings.TrimSpace(sep)
	switch {
	case strings.HasPrefix(sep, ":") && strings.HasSuffix(sep, ":") && len(sep) > 1:
		return 1
	case strings.HasSuffix(sep, ":"):
		return 2
	}
	return 0
}

// tableRows draws a table as rows with cells: natural column widths, and the widest columns
// shrink and wrap if the table is wider than tw. The cells have the source position of
// their text in src. A border or a padding cell has src -1. The line of a row is relative
// to the first line of the table. The cache keeps the rows.
func (c *mdCache) tableRows(lines [][]rune, b mdBlock, tw int) []edRow {
	var sb strings.Builder
	for j := b.from; j <= b.to; j++ {
		sb.WriteString(string(lines[j]))
		sb.WriteByte('\n')
	}
	key := strconv.Itoa(tw) + "\x00" + sb.String()
	if c.tables == nil || len(c.tables) > 2000 {
		c.tables = map[string][]edRow{}
	}
	rows, ok := c.tables[key]
	if !ok {
		rows = buildTable(lines[b.from:b.to+1], tw)
		c.tables[key] = rows
	}
	out := make([]edRow, len(rows))
	for i, r := range rows {
		r.line += b.from
		out[i] = r
	}
	return out
}

// buildTable makes the rows of a table. lines[0] is the header, lines[1] the separator.
func buildTable(lines [][]rune, tw int) []edRow {
	text := kindStyle(skText)
	hdr := splitTableRow(lines[0])
	n := len(hdr)
	aligns := make([]int, n)
	for i, sc := range splitTableRow(lines[1]) {
		if i < n {
			aligns[i] = tableAlign(sc.text)
		}
	}
	// cells[r][i] are the cells of column i in line r. Line 1 (the separator) has none.
	cells := make([][][]pcell, len(lines))
	widths := make([]int, n)
	for r, l := range lines {
		if r == 1 {
			continue
		}
		cells[r] = make([][]pcell, n)
		for i, tc := range splitTableRow(l) {
			if i >= n {
				break
			}
			cells[r][i] = inlineCells(tc.text, tc.off, text)
			widths[i] = max(widths[i], cellsWidth(cells[r][i]))
		}
	}
	// Shrink the widest columns until the table fits: each column has 2 spaces of padding
	// and a border between columns.
	avail := tw - 3*n
	sum := func() (s int) {
		for _, w := range widths {
			s += w
		}
		return
	}
	for sum() > max(avail, n) {
		wi := 0
		for i, w := range widths {
			if w > widths[wi] {
				wi = i
			}
		}
		if widths[wi] <= 1 {
			break
		}
		widths[wi]--
	}
	for i := range widths {
		widths[i] = max(1, widths[i])
	}
	line := st(hexText)
	bar := func(r rune) pcell { return pcell{r, line, -1} }
	pad := func(k int) (out []pcell) {
		for ; k > 0; k-- {
			out = append(out, pcell{' ', text, -1})
		}
		return
	}
	var rows []edRow
	for r := range lines {
		if r == 1 { // the header separator
			row := edRow{line: 1, cells: []pcell{bar(' ')}}
			for i, w := range widths {
				if i > 0 {
					row.cells = append(row.cells, bar('┼'))
				}
				for k := 0; k < w+2; k++ {
					row.cells = append(row.cells, bar('─'))
				}
			}
			row.pre = renderRow(row, tw, -1)
			rows = append(rows, row)
			continue
		}
		// Wrap each cell of the row, then make as many screen rows as the tallest cell has.
		wrapped := make([][][]pcell, n)
		height := 1
		for i := range widths {
			wrapped[i] = wrapCells(cells[r][i], widths[i])
			height = max(height, len(wrapped[i]))
		}
		for k := 0; k < height; k++ {
			row := edRow{line: r, cells: []pcell{bar(' ')}}
			for i, w := range widths {
				if i > 0 {
					row.cells = append(row.cells, bar('│'))
				}
				var seg []pcell
				if k < len(wrapped[i]) {
					seg = wrapped[i][k]
				}
				left, right := 0, w-cellsWidth(seg)
				switch aligns[i] {
				case 1:
					left = right / 2
					right -= left
				case 2:
					left, right = right, 0
				}
				row.cells = append(row.cells, pad(1+left)...)
				row.cells = append(row.cells, seg...)
				row.cells = append(row.cells, pad(right+1)...)
			}
			row.pre = renderRow(row, tw, -1)
			rows = append(rows, row)
		}
	}
	return rows
}

// cellsWidth is the number of screen cells of cs. A wide rune (CJK, emoji) is two cells.
func cellsWidth(cs []pcell) int {
	n := 0
	for _, c := range cs {
		n += displaywidth.Rune(c.r)
	}
	return n
}

// wrapCells splits the cells of one table cell into segments of at most w cells. A space
// at a break goes away.
func wrapCells(cs []pcell, w int) [][]pcell {
	rs := make([]rune, len(cs))
	for i, c := range cs {
		rs[i] = c.r
	}
	starts := wrapLine(rs, w)
	out := make([][]pcell, len(starts))
	for k, s := range starts {
		end := len(cs)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		seg := cs[s:end]
		if k+1 < len(starts) {
			for len(seg) > 1 && seg[len(seg)-1].r == ' ' {
				seg = seg[:len(seg)-1]
			}
		}
		out[k] = seg
	}
	return out
}
