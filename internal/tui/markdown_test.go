package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// minHeadingStep is the smallest perceived difference (CIE76 ΔE) between the bars of two
// adjacent heading levels, and between the level 4 bar and the background. A difference of
// about 2.3 is just visible. 5 is clear at a glance.
const minHeadingStep = 4.9

// In each theme, dark and light, the heading levels are clearly different: the bars of
// levels 1 to 4 have a clear step between them and to the background, and all heading
// text is readable.
func TestHeadingHierarchy(t *testing.T) {
	defer applyTheme(todoistTheme, true)
	for _, name := range themeNames() {
		for _, dark := range []bool{true, false} {
			applyTheme(name, dark)
			var bars []string
			for lv := 1; lv <= 6; lv++ {
				text, bar := headingColors(lv)
				if c := contrast(text, bar); c < 4.5 {
					t.Errorf("%s dark=%v level %d: text contrast %.1f", name, dark, lv, c)
				}
				if lv <= headingBarLevels {
					bars = append(bars, bar)
				}
				if bold := headingStyle(lv).GetBold(); bold != (lv <= 5) {
					t.Errorf("%s level %d: bold %v", name, lv, bold)
				}
			}
			bars = append(bars, baseBg)
			for i := 0; i+1 < len(bars); i++ {
				if d := deltaE(bars[i], bars[i+1]); d < minHeadingStep {
					t.Errorf("%s dark=%v: bar %d and the next differ by ΔE %.1f", name, dark, i+1, d)
				}
			}
		}
	}
}

// The bar length shows the level: the full width for levels 1 and 2, a short bar for
// levels 3 and 4, and no bar for levels 5 and 6. The text of level 1 is in the middle.
func TestHeadingBarLength(t *testing.T) {
	const tw = 40
	widths := make([]int, 7)
	c := newMDCache()
	for lv := 1; lv <= 6; lv++ {
		rows := c.previewRows(strings.Repeat("#", lv)+" Title", tw)
		widths[lv] = ansi.StringWidth(rows[0].pre)
	}
	if widths[1] != tw+1 || widths[2] != tw+1 {
		t.Errorf("levels 1 and 2: widths %d %d, want %d", widths[1], widths[2], tw+1)
	}
	if widths[3] != len("  Title")+2 || widths[4] != widths[3] {
		t.Errorf("levels 3 and 4: widths %d %d", widths[3], widths[4])
	}
	if widths[5] != len("  Title") {
		t.Errorf("level 5: width %d", widths[5])
	}
	h1 := ansi.Strip(c.previewRows("# Title", tw)[0].pre)
	if i := strings.Index(h1, "Title"); i < tw/2-5 || i > tw/2 {
		t.Errorf("level 1 is not centered: %q", h1)
	}
}

func TestDeltaE(t *testing.T) {
	if d := deltaE("#000000", "#FFFFFF"); d < 99 || d > 101 {
		t.Errorf("black to white: %.1f", d)
	}
	if d := deltaE("#336699", "#336699"); d != 0 {
		t.Errorf("same color: %.1f", d)
	}
}

func cellText(cells []pcell) string {
	var b strings.Builder
	for _, c := range cells {
		b.WriteRune(c.r)
	}
	return b.String()
}

// Inline markdown inside bold, italic, strike, and link text renders too.
func TestNestedInline(t *testing.T) {
	for src, want := range map[string]string{
		"**Search with `/`**, as in":  "Search with /, as in",
		"*see `x` here*":              "see x here",
		"~~old **bold**~~":            "old bold",
		"[**bold** link](u)":          "bold link",
		"**a [link](http://x) here**": "a link here",
		"1. [x] done":                 "1. ☑ done",
	} {
		cells, _ := previewCells(src)
		if got := cellText(cells); got != want {
			t.Errorf("%q: got %q, want %q", src, got, want)
		}
	}
	cells, _ := previewCells("**Search with `/`**")
	for _, c := range cells {
		if c.r == '/' && c.src != 15 {
			t.Errorf("/ src = %d", c.src)
		}
	}
}

// The reader draws body text with one cell of indent, a table with its own renderer, and
// fenced code with Glamour. It records the first row of each checkbox item.
func TestReaderRender(t *testing.T) {
	src := "text\n1. [ ] one\n- [x] two\n\n| Name | Qty |\n|:-----|----:|\n| **a** | 1 |\n\n```\n- [ ] code\n```"
	got := newMDCache().render(src, 41)
	var plain []string
	for _, l := range got.lines {
		plain = append(plain, strings.TrimRight(ansi.Strip(l), " "))
	}
	if plain[0] != "  text" || plain[1] != "  1. ☐ one" || plain[2] != "  ☑ two" {
		t.Errorf("lines = %q", plain)
	}
	if len(got.checks) != 2 || got.checks[0] != 1 || got.checks[1] != 2 {
		t.Errorf("checks = %v", got.checks)
	}
	if plain[4] != "    Name │ Qty" || plain[5] != "   ──────┼─────" || plain[6] != "    a    │   1" {
		t.Errorf("table = %q", plain[4:7])
	}
	if s := strings.Join(plain, "\n"); !strings.Contains(s, "- [ ] code") || strings.Contains(s, "```") {
		t.Errorf("code block:\n%s", s)
	}
}

// A table that is too wide shrinks its widest column and wraps the text. In the editor,
// a table row refers to its own source line.
func TestTableShrink(t *testing.T) {
	lines := [][]rune{[]rune("| a | note |"), []rune("|---|---|"), []rune("| b | long text here that wraps around |")}
	rows := newMDCache().blockRows(lines, mdBlock{from: 0, to: 2, table: true}, 20)
	if len(rows) < 4 {
		t.Fatalf("rows = %d", len(rows))
	}
	for i, r := range rows {
		if w := ansi.StringWidth(r.pre); w > 21 {
			t.Errorf("row %d too wide: %q", i, ansi.Strip(r.pre))
		}
		if i >= 2 && r.line != 2 {
			t.Errorf("row %d line %d", i, r.line)
		}
	}
}

// Wrapping counts screen cells: CJK text and emoji are two cells wide, and no reader
// row is wider than the pane. A click on a wide rune finds its cell.
func TestWideRunes(t *testing.T) {
	src := "日本語のテキスト 日本語のテキスト 日本語のテキスト\n🎉🎉🎉 party 🎉🎉🎉 party 🎉🎉🎉 party\n| 名前 | n |\n|---|---|\n| 日本語のテキスト 日本語 | 1 |"
	for _, l := range newMDCache().render(src, 30).lines {
		if w := ansi.StringWidth(l); w > 30 {
			t.Errorf("width %d: %q", w, ansi.Strip(l))
		}
	}
	if got := wrapLine([]rune("日本語 日本語"), 8); len(got) != 2 || got[1] != 4 {
		t.Errorf("wrapLine = %v, want [0 4]", got)
	}
	cells, _ := previewCells("日本x")
	for x, want := range []int{0, 0, 1, 1, 2, 3} {
		if got := cellAt(cells, x); got != want {
			t.Errorf("cellAt(%d) = %d, want %d", x, got, want)
		}
	}
}

// Tables follow GitHub Flavored Markdown: a delimiter row with one dash or more, and
// optional outer pipes. A list item, a rule, or a header with another number of cells
// is not a table.
func TestTableRule(t *testing.T) {
	for src, want := range map[string]bool{
		"| a | b |\n|:-|-:|\n| 1 | 2 |": true,
		"a | b\n--|--\n1 | 2":           true,
		"| a |\n| --- |":                true,
		"a | b\n---":                    false,
		"- a | b\n--|--":                false,
		"text\n---":                     false,
		"a \\| b\n--|--":                false,
	} {
		var lines [][]rune
		for _, l := range strings.Split(src, "\n") {
			lines = append(lines, []rune(l))
		}
		got := mdBlocks(lines)
		if is := len(got) == 1 && got[0].table; is != want {
			t.Errorf("%q: table %v, want %v", src, is, want)
		}
	}
}

// A backslash escape shows the mark as text, and an autolink shows its URL as a link.
func TestEscapeAndAutolink(t *testing.T) {
	for src, want := range map[string]string{
		`\*not italic\*`:         "*not italic*",
		"a \\`b\\` c":            "a `b` c",
		"see <https://x.org/a>.": "see https://x.org/a.",
		"`\\*`":                  "\\*",
	} {
		cells, _ := previewCells(src)
		if got := cellText(cells); got != want {
			t.Errorf("%q: got %q, want %q", src, got, want)
		}
	}
}
