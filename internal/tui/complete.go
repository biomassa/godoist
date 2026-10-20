package tui

import (
	"context"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/biomassa/godoist/internal/todoist"
)

// The name field of the task dialog completes #project, @label, and /section. When the
// word at the cursor starts with one of these marks, a list of the matching names shows
// below it. up and down choose, tab or enter put the name in the text, and esc closes
// the list.

// completion is the open list of the name field.
type completion struct {
	mark  rune // '#', '@', or '/'
	start int  // the rune index of the mark in the name
	end   int  // the rune index of the cursor: the typed part is start+1 to end
	items []compItem
	cur   int // the selected item
	off   int // the first visible item
}

// compItem is one name in the list.
type compItem struct {
	name    string // the text after the mark
	project string // a section: the name of its project
	hex     string // the color of the dot
	create  bool   // the "+ create" row of a new label
}

// compRows is the largest number of items that the list shows at a time.
const compRows = 6

// compMarks returns the marks that the name field of the open dialog completes.
func (m Model) compMarks() string {
	switch {
	case m.inputMode == inputAdd:
		return "#@/"
	case m.inputMode == inputRename && !m.notesMode():
		return "#@/"
	case m.inputMode == inputAddSubtask: // a sub-task stays in the project of its parent
		return "@"
	}
	return ""
}

// updateCompletion opens, changes, or closes the list for the word at the cursor of the
// name field. A list that esc closed stays closed until the cursor leaves its word:
// compHide is the mark index of that word plus one, or 0.
func (m *Model) updateCompletion() {
	m.comp = nil
	marks := m.compMarks()
	if marks == "" || m.dlg == nil || m.dlgField != 0 || m.dlg.Line() != 0 {
		return
	}
	text := []rune(m.dlg.Value())
	end := min(m.dlg.Column(), len(text))
	start := end - 1
	for start >= 0 && !unicode.IsSpace(text[start]) && !strings.ContainsRune(marks, text[start]) {
		start--
	}
	if start < 0 || !strings.ContainsRune(marks, text[start]) || start > 0 && !unicode.IsSpace(text[start-1]) {
		m.compHide = 0
		return
	}
	if start+1 == m.compHide {
		return
	}
	m.compHide = 0
	c := &completion{mark: text[start], start: start, end: end}
	c.items = m.compItems(c.mark, string(text[start+1:end]))
	if len(c.items) > 0 {
		m.comp = c
	}
}

// compItems returns the names for mark that contain q, without regard to case. The names
// that start with q come first.
func (m Model) compItems(mark rune, q string) []compItem {
	lq := strings.ToLower(q)
	var first, rest []compItem
	add := func(it compItem) {
		ln := strings.ToLower(it.name)
		switch {
		case strings.HasPrefix(ln, lq):
			first = append(first, it)
		case strings.Contains(ln, lq):
			rest = append(rest, it)
		}
	}
	exact := false
	switch mark {
	case '#':
		for _, op := range m.orderedProjects() {
			add(compItem{name: op.p.Name, hex: todoist.ColorHex(op.p.Color)})
		}
	case '@':
		labels := append([]todoist.Label(nil), m.snap.Labels...)
		sort.SliceStable(labels, func(i, j int) bool { return labels[i].Order < labels[j].Order })
		for _, l := range labels {
			add(compItem{name: l.Name, hex: todoist.ColorHex(l.Color)})
			exact = exact || strings.EqualFold(l.Name, q)
		}
	case '/':
		for _, op := range m.orderedProjects() {
			for _, sid := range m.projectSections(op.p.ID) {
				if s := m.sections[sid]; s != nil {
					add(compItem{name: s.Name, project: op.p.Name, hex: todoist.ColorHex(op.p.Color)})
				}
			}
		}
	}
	out := append(first, rest...)
	if mark == '@' && q != "" && !exact {
		out = append(out, compItem{name: q, hex: todoist.ColorHex("charcoal"), create: true})
	}
	return out
}

// compKeys handles the keys of the open list. ok is false for keys that go to the field.
func (m Model) compKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	c := m.comp
	switch msg.String() {
	case "up":
		c.cur = (c.cur - 1 + len(c.items)) % len(c.items)
	case "down":
		c.cur = (c.cur + 1) % len(c.items)
	case "tab", "enter":
		next := m.acceptCompletion()
		return m, next, true
	case "esc":
		m.compHide, m.comp = c.start+1, nil
		return m, nil, true
	default:
		return m, nil, false
	}
	if c.cur < c.off {
		c.off = c.cur
	}
	if c.cur >= c.off+compRows {
		c.off = c.cur - compRows + 1
	}
	return m, nil, true
}

// acceptCompletion puts the selected name and a space in place of the typed word. A
// section also gets its #project, unless the text names that project already. Then a
// #project of another project goes. A new label is created in Todoist.
func (m *Model) acceptCompletion() tea.Cmd {
	c := m.comp
	it := c.items[c.cur]
	text := []rune(m.dlg.Value())
	before, after := string(text[:c.start]), string(text[c.end:])
	insert := string(c.mark) + it.name + " "
	if c.mark == '/' && !m.namesProjectName(before+after, it.project) {
		for _, p := range m.snap.Projects { // the text can name only one project
			before, after = cutTag(before, "#"+p.Name), cutTag(after, "#"+p.Name)
		}
		insert += "#" + it.project + " "
	}
	after = strings.TrimLeft(after, " ")
	m.dlg.SetValue(before + insert + after)
	m.dlg.SetCursorColumn(len([]rune(before + insert)))
	m.comp = nil
	if !it.create {
		return nil
	}
	name, client := it.name, m.client
	return m.simpleWrite("Added label @"+name, func(ctx context.Context) error {
		_, err := client.CreateLabel(ctx, name)
		return err
	})
}

// namesProjectName reports whether text has "#name" as a whole word, without regard to case.
func (m Model) namesProjectName(text, name string) bool {
	return cutTag(text, "#"+name) != text
}

// cutTag removes the first tag (such as "#work") in text that is a whole word, without
// regard to case, and the space before it (at the start of text, the space after it).
func cutTag(text, tag string) string {
	low, lt := strings.ToLower(text), strings.ToLower(tag)
	for i := strings.Index(low, lt); i >= 0; {
		end := i + len(lt)
		before := i == 0 || low[i-1] == ' '
		after := end == len(low) || strings.ContainsRune(" \t,.;:!?", rune(low[end]))
		if before && after && i == 0 {
			return strings.TrimLeft(text[end:], " ")
		}
		if before && after {
			return strings.TrimRight(text[:i], " ") + text[end:]
		}
		next := strings.Index(low[end:], lt)
		if next < 0 {
			break
		}
		i = end + next
	}
	return text
}

// compBox draws the list: a dot in the color of the project or the label, and the name.
// A section shows its project after it.
func (m Model) compBox() string {
	cp := m.comp
	maxW := m.dialogWidth() - 4
	label := func(it compItem) string {
		switch {
		case it.create:
			return "+ create " + string(cp.mark) + it.name
		case it.project != "":
			return "● " + it.name + " · " + it.project
		}
		return "● " + it.name
	}
	w := 0
	for _, it := range cp.items {
		w = max(w, ansi.StringWidth(label(it)))
	}
	w = min(w+2, maxW-2) // one space at each side, inside the border
	var lines []string
	for i := cp.off; i < len(cp.items) && i < cp.off+compRows; i++ {
		it := cp.items[i]
		base := lipgloss.NewStyle()
		if i == cp.cur {
			base = base.Background(c(hexSelBg))
		}
		text := trunc(label(it), w-2)
		var line string
		switch {
		case it.create:
			line = base.Foreground(c(hexMuted)).Render(" " + text)
		default:
			name := strings.TrimPrefix(text, "● ")
			line = base.Render(" ") + base.Foreground(c(fg(it.hex))).Render("●") + base.Foreground(c(hexText)).Render(" "+name)
		}
		lines = append(lines, line+base.Render(strings.Repeat(" ", max(0, w-lipgloss.Width(line)))))
	}
	return box("", lines, w+2, fg(hexAccent))
}

// compRect is the place of the list: below the cursor row, starting at the mark.
func (m Model) compRect() rect {
	c := m.comp
	d := m.dialogRect()
	li := m.dlg.LineInfo()
	text := []rune(m.dlg.Value())
	x := li.ColumnOffset - ansi.StringWidth(string(text[c.start:min(c.end, len(text))]))
	row := li.RowOffset - m.dlg.ScrollYOffset()
	b := m.compBox()
	w, h := lipgloss.Width(b), lipgloss.Height(b)
	bx := min(d.x+2+max(0, x), max(0, m.width-w))
	by := d.y + 1 + 2 + row + 1 // the border, the empty line, the "Name" label, the cursor row
	return rect{bx, by, w, h}
}
