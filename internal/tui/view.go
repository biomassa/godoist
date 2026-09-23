package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/biomassa/godoist/internal/todoist"
)

const wideWidth = 110

// wide reports whether the terminal has space for the details pane.
func (m Model) wide() bool { return m.width >= wideWidth }

// navWidth is the width of the sidebar, borders included.
func (m Model) navWidth() int {
	if m.width < 80 {
		return 22
	}
	return 28
}

// detailWidth is the width of the details pane, borders included. It is 0 on narrow terminals.
// The task list and the details pane share the space after the sidebar equally.
func (m Model) detailWidth() int {
	if m.wide() {
		return (m.width - m.navWidth()) / 2
	}
	return 0
}

// paneHeight is the inner (content) height of each pane.
func (m Model) paneHeight() int { return max(1, m.height-2-2) } // two bottom lines, two borders

// fixScroll keeps the cursors of the sidebar and the task list on the screen.
func (m *Model) fixScroll() {
	h := m.paneHeight()
	scroll := func(cur, off, n int) int {
		if cur < off {
			off = cur
		}
		if cur >= off+h {
			off = cur - h + 1
		}
		return max(0, min(off, n-h))
	}
	// A note title and its preview line scroll into view together.
	cur := m.rowCur
	if cur+1 < len(m.rows) && m.rows[cur+1].preview != "" && cur+1 >= m.rowOff+h {
		m.rowOff = cur + 2 - h
	}
	m.rowOff = scroll(cur, m.rowOff, len(m.rows))
	m.navOff = scroll(m.navCur, m.navOff, len(m.nav))
	m.clampDetail()
}

// View renders the full screen.
func (m Model) View() tea.View {
	screen := m.render()
	if m.width > 0 {
		screen = m.overlays(screen)
	}
	v := tea.NewView(screen)
	v.MouseMode = tea.MouseModeCellMotion // clicks, wheel, and motion while a button is down
	v.AltScreen = true
	v.WindowTitle = "godoist"
	v.ReportFocus = true // a FocusMsg triggers a sync
	return v
}

func (m Model) render() string {
	if m.width == 0 {
		return ""
	}
	h := m.paneHeight()
	l := m.layout()
	navW, midW, detW := l.nav.w, l.mid.w, l.detail.w

	borderFor := func(p pane) string {
		if m.focus == p && m.inputMode == inputNone {
			if cur := m.currentNav(); cur != nil {
				return fg(cur.color)
			}
			return hexAccent
		}
		return hexBorder
	}

	nav := box(st(hexMuted).Bold(true).Render("godoist"), m.navLines(navW-2, h), navW, borderFor(paneNav))

	detailTitle := st(hexMuted).Bold(true).Render("Details")
	if m.notesMode() {
		detailTitle = st(hexMuted).Bold(true).Render("Reader")
	}
	// On narrow terminals the editor and the details replace the task list.
	var mid string
	switch {
	case m.showHelp:
		// The help spans the task list and the details pane.
		helpW := midW + detW
		return lipgloss.JoinHorizontal(lipgloss.Top, nav,
			box(st(hexText).Bold(true).Render("Help"), padLines(helpLines(helpW-2), helpW-2, h), helpW, hexAccent)) +
			"\n" + m.bottomBar()
	case m.pick != nil && !m.wide():
		mid = m.pickerBox(midW, h)
	case m.edit != nil && !m.wide():
		mid = m.editorBox(midW, h)
	case m.detailOpen && !m.wide():
		mid = box(detailTitle, m.detailLines(midW-2, h), midW, borderFor(paneDetail))
	default:
		mid = box(m.taskTitle(), m.taskLines(midW-2, h), midW, borderFor(paneTasks))
	}

	panes := []string{nav, mid}
	switch {
	case detW > 0 && m.pick != nil:
		panes = append(panes, m.pickerBox(detW, h))
	case detW > 0 && m.edit != nil:
		panes = append(panes, m.editorBox(detW, h))
	case detW > 0:
		panes = append(panes, box(detailTitle, m.detailLines(detW-2, h), detW, borderFor(paneDetail)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, panes...) + "\n" + m.bottomBar()
}

// ---- helpers ----

func st(hex string) lipgloss.Style { return lipgloss.NewStyle().Foreground(c(hex)) }

// wrapHint wraps a hint to lines at most w cells wide, each with a one-space indent.
// It breaks lines only between items (at " · "), so that an item such as "1–5 picks"
// stays on one line. An item that is wider than a line wraps at spaces.
func wrapHint(text, hex string, w int) []string {
	const sep = " · "
	var lines []string
	cur := ""
	for _, item := range strings.Split(text, sep) {
		switch {
		case cur == "":
			cur = item
		case lipgloss.Width(cur+sep+item) <= w-1:
			cur += sep + item
		default:
			lines = append(lines, cur)
			cur = item
		}
	}
	lines = append(lines, cur)
	var out []string
	for _, l := range lines {
		for _, part := range strings.Split(lipgloss.NewStyle().Width(max(1, w-1)).Render(l), "\n") {
			out = append(out, " "+st(hex).Render(part))
		}
	}
	return out
}

// trunc cuts s to w cells and adds an ellipsis.
func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// pad makes a styled string exactly w cells wide. It truncates or fills with the fill style.
func pad(s string, w int, fill lipgloss.Style) string {
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	if n := w - lipgloss.Width(s); n > 0 {
		s += fill.Render(strings.Repeat(" ", n))
	}
	return s
}

// padLines returns exactly h lines, each w cells wide.
func padLines(lines []string, w, h int) []string {
	out := make([]string, h)
	for i := range out {
		if i < len(lines) {
			out[i] = pad(lines[i], w, lipgloss.NewStyle())
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

// box draws a rounded border with the title embedded in the top edge.
func box(title string, lines []string, w int, borderHex string) string {
	b := st(borderHex)
	inner := w - 2
	if title == "" {
		var sb strings.Builder
		sb.WriteString(b.Render("╭" + strings.Repeat("─", inner) + "╮"))
		for _, l := range lines {
			sb.WriteString("\n" + b.Render("│") + l + b.Render("│"))
		}
		sb.WriteString("\n" + b.Render("╰"+strings.Repeat("─", inner)+"╯"))
		return sb.String()
	}
	title = trunc(title, max(0, inner-4))
	fill := max(0, inner-3-lipgloss.Width(title))
	var sb strings.Builder
	sb.WriteString(b.Render("╭─ ") + title + b.Render(" "+strings.Repeat("─", fill)+"╮"))
	for _, l := range lines {
		sb.WriteString("\n" + b.Render("│") + l + b.Render("│"))
	}
	sb.WriteString("\n" + b.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return sb.String()
}

var mdLink = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

// plain strips markdown links and bold markers for single-line display.
func plain(s string) string {
	s = mdLink.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "**", "")
	return strings.ReplaceAll(s, "\n", " ")
}

// ---- sidebar ----

// navLines renders the sidebar. The selected project gets a tint of its own color.
func (m Model) navLines(w, h int) []string {
	lines := make([]string, 0, h)
	for i := m.navOff; i < len(m.nav) && len(lines) < h; i++ {
		n := m.nav[i]
		if n.header != "" {
			if strings.TrimSpace(n.header) == "" {
				lines = append(lines, strings.Repeat(" ", w))
			} else {
				lines = append(lines, pad(" "+st(hexMuted).Bold(true).Render(n.header), w, lipgloss.NewStyle()))
			}
			continue
		}
		base := lipgloss.NewStyle()
		nameHex := hexText
		if i == m.navCur {
			if m.focus == paneNav {
				base = base.Background(c(tint(n.color)))
			} else {
				base = base.Background(c(hexSelBgDim))
			}
			nameHex = fg(n.color)
		}
		count := ""
		if n.count > 0 {
			ch := n.countHex
			if ch == "" {
				ch = hexMuted
			}
			count = base.Foreground(c(ch)).Render(fmt.Sprint(n.count)) + base.Render(" ")
		}
		left := base.Render(" "+strings.Repeat("  ", n.depth)) +
			base.Foreground(c(fg(n.color))).Render(n.glyph) + base.Render(" ")
		nameW := w - lipgloss.Width(left) - lipgloss.Width(count) - 1
		name := base.Foreground(c(nameHex)).Render(trunc(n.name, nameW))
		gap := w - lipgloss.Width(left) - lipgloss.Width(name) - lipgloss.Width(count)
		lines = append(lines, left+name+base.Render(strings.Repeat(" ", max(0, gap)))+count)
	}
	return padLines(lines, w, h)
}

// ---- task list ----

// taskTitle is the title of the task list pane: the view name and the item count.
func (m Model) taskTitle() string {
	cur := m.currentNav()
	if cur == nil {
		return ""
	}
	n := 0
	for _, r := range m.rows {
		if r.task != nil && r.preview == "" {
			n++
		}
	}
	t := st(fg(cur.color)).Bold(true).Render(cur.glyph+" "+cur.name) + st(hexMuted).Render(fmt.Sprintf(" %d", n))
	if m.notesMode() {
		t += st(hexMuted).Render(" · notebook")
	}
	if m.find != "" {
		t += st(hexTomorrow).Render("  / " + m.find)
	}
	return t
}

// taskLines renders the visible rows of the task list.
func (m Model) taskLines(w, h int) []string {
	if !m.loaded {
		msg := "Loading…"
		if m.statusErr {
			msg = "Could not load tasks — press r to retry"
		}
		return padLines([]string{"", "  " + st(hexMuted).Render(msg)}, w, h)
	}
	cur := m.currentNav()
	crossProject := cur != nil && cur.kind != vkProject
	notes := m.notesMode()
	now := time.Now()

	lines := make([]string, 0, h)
	for i := m.rowOff; i < len(m.rows) && len(lines) < h; i++ {
		r := m.rows[i]
		base := lipgloss.NewStyle()
		// A preview line shares the highlight of the note above it.
		if i == m.rowCur || (r.preview != "" && i-1 == m.rowCur) {
			if m.focus == paneTasks {
				base = base.Background(c(hexSelBg))
			} else {
				base = base.Background(c(hexSelBgDim))
			}
		}
		switch {
		case r.spacer:
			lines = append(lines, "")
		case r.task == nil:
			lines = append(lines, headerLine(r, w, base))
		case r.preview != "":
			indent := "    " + strings.Repeat("  ", r.depth)
			lines = append(lines, pad(base.Render(indent)+base.Foreground(c(hexMuted)).Render(trunc(r.preview, w-len(indent)-1)), w, base))
		case notes:
			lines = append(lines, m.noteLine(r, w, base, i == m.rowCur))
		default:
			lines = append(lines, m.taskLine(r, w, base, crossProject, now))
		}
	}
	if len(m.rows) == 0 || (len(lines) == 0 && h > 1) {
		msg := "Nothing here · a to add a task"
		if m.notesMode() {
			msg = "No notes · a to add a note"
		}
		if m.find != "" {
			msg = "No matches · esc to clear"
		}
		lines = append(lines, "", "  "+st(hexMuted).Render(msg))
	}
	return padLines(lines, w, h)
}

// noteLine is a note title in notebook view. It has no checkbox.
func (m Model) noteLine(r row, w int, base lipgloss.Style, selected bool) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}
	left := base.Render(" "+strings.Repeat("  ", r.depth)) + base.Foreground(c(fg(hexAccent))).Render(marker)
	right := ""
	if n := len(m.snap.Comments[r.task.ID]); n > 0 && firstLine(r.task.Description) != "" {
		right = base.Foreground(c(hexMuted)).Render(fmt.Sprintf("✎%d ", n))
	}
	title := base.Foreground(c(hexText)).Bold(true).Render(trunc(plain(r.task.Content), w-lipgloss.Width(left)-lipgloss.Width(right)-1))
	gap := w - lipgloss.Width(left) - lipgloss.Width(title) - lipgloss.Width(right)
	return left + title + base.Render(strings.Repeat(" ", max(0, gap))) + right
}

// editorBox draws the built-in editor with a key hint below it.
func (m Model) editorBox(w, h int) string {
	lines := strings.Split(m.editor.View(), "\n")
	hint := "ctrl+s save · esc cancel · ctrl+e $EDITOR"
	if m.edit.saving {
		hint = "saving…"
	}
	lines = append(lines, "", " "+st(hexDim).Render(hint))
	return box(st(fg(hexAccent)).Bold(true).Render(m.edit.title), padLines(lines, w-2, h), w, fg(hexAccent))
}

// headerLine renders a section or day header with a count and a rule.
func headerLine(r row, w int, base lipgloss.Style) string {
	name := base.Foreground(c(r.headerHex)).Bold(true).Render(" " + trunc(r.header, w-8))
	count := ""
	if r.count > 0 {
		count = base.Foreground(c(hexMuted)).Render(fmt.Sprintf(" %d", r.count))
	}
	used := lipgloss.Width(name) + lipgloss.Width(count) + 1
	rule := base.Render(" ") + base.Foreground(c(hexBorder)).Render(strings.Repeat("─", max(0, w-used-1)))
	return pad(name+count+rule, w, base)
}

// taskLine renders a task: the priority circle, the content, and the meta items at the right.
func (m Model) taskLine(r row, w int, base lipgloss.Style, crossProject bool, now time.Time) string {
	t := r.task
	prio := t.UIPriority()
	left := base.Render(" "+strings.Repeat("  ", r.depth)) +
		base.Foreground(c(priorityHex[prio])).Render("○") + base.Render(" ")

	// The meta items are in display order. If the line is too narrow, the item with the
	// lowest keep value goes first.
	type item struct {
		s    string
		keep int
	}
	var meta []item
	dimS := base.Foreground(c(hexMuted))
	for _, l := range t.Labels {
		meta = append(meta, item{base.Foreground(c(hexTomorrow)).Render("@" + l), 1})
	}
	if strings.TrimSpace(t.Description) != "" {
		meta = append(meta, item{dimS.Render("≡"), 2})
	}
	if n := len(m.snap.Comments[t.ID]); n > 0 {
		meta = append(meta, item{dimS.Render(fmt.Sprintf("✎%d", n)), 3})
	}
	if t.Due != nil {
		d := todoist.FormatDue(t.Due, now)
		if t.Due.IsRecurring {
			d += " ↻"
		}
		meta = append(meta, item{base.Foreground(c(dueHex(t.Due, now))).Render(d), 5})
	}
	if crossProject {
		if p := m.projects[t.ProjectID]; p != nil {
			name := p.Name
			if s := m.sections[t.Section()]; s != nil {
				name += "/" + s.Name
			}
			meta = append(meta, item{base.Foreground(c(fg(todoist.ColorHex(p.Color)))).Render("# " + trunc(name, 20)), 4})
		}
	}
	var right string
	contentW := 0
	for {
		parts := make([]string, len(meta))
		for i, it := range meta {
			parts[i] = it.s
		}
		right = strings.Join(parts, base.Render("  "))
		if right != "" {
			right += base.Render(" ")
		}
		contentW = w - lipgloss.Width(left) - lipgloss.Width(right) - 1
		if contentW >= min(24, lipgloss.Width(plain(t.Content))) || len(meta) == 0 {
			break
		}
		drop := 0
		for i, it := range meta {
			if it.keep < meta[drop].keep {
				drop = i
			}
		}
		meta = append(meta[:drop], meta[drop+1:]...)
	}
	content := base.Foreground(c(hexText)).Render(trunc(plain(t.Content), contentW))
	gap := w - lipgloss.Width(left) - lipgloss.Width(content) - lipgloss.Width(right)
	return left + content + base.Render(strings.Repeat(" ", max(0, gap))) + right
}

// dueHex is the color of a due label, as in the Todoist app.
func dueHex(d *todoist.Due, now time.Time) string {
	if todoist.PastDue(d, now) {
		return hexOverdue
	}
	switch todoist.Classify(d, now) {
	case todoist.Overdue:
		return hexOverdue
	case todoist.DueToday:
		return hexToday
	case todoist.DueTomorrow:
		return hexTomorrow
	case todoist.DueThisWeek:
		return hexWeek
	}
	return hexMuted
}

// attachmentLine describes a comment attachment in one line.
func attachmentLine(a *todoist.Attachment) string {
	name := a.FileName
	if name == "" {
		name = a.Title
	}
	link := a.FileURL
	if link == "" {
		link = a.URL
	}
	switch {
	case name != "" && link != "":
		return "📎 " + name + " · " + link
	case name != "":
		return "📎 " + name
	}
	if link == "" {
		return ""
	}
	return "📎 " + link
}

// ---- bottom bar ----

// bottomBar renders the input line, a y/n prompt, or the key legend with the status.
func (m Model) bottomBar() string {
	bar, _ := m.bottomBarLayout()
	return bar
}

// legendHit is the x range of one legend item in the bottom bar, its line, and the key it runs.
type legendHit struct {
	x0, x1 int
	y      int // 0 is the legend line, 1 is the status line
	key    string
}

// legendKey converts a legend label to the key string that runs it. Labels for a
// group of keys, such as "1-4" or "j/k", return "".
func legendKey(label string) string {
	switch label {
	case "^z":
		return "ctrl+z"
	case "^s":
		return "ctrl+s"
	case "^e":
		return "ctrl+e"
	case "enter", "esc", "tab", "space":
		return label
	case "del":
		return "delete"
	}
	if len([]rune(label)) == 1 {
		return label
	}
	return ""
}

// legendKeys returns the legend items for the current mode and focus. "? help" is first.
func (m Model) legendKeys() [][2]string {
	var keys [][2]string
	switch {
	case m.cal != nil:
		keys = [][2]string{{"tab", "text / calendar / time"}, {"enter", "save"}, {"esc", "cancel"}}
	case m.menu != nil:
		keys = [][2]string{{"↑/↓", "select"}, {"enter", "run"}, {"esc", "close"}}
	case isDialog(m.inputMode):
		keys = [][2]string{{"enter", "save"}, {"esc", "cancel"}, {"←/→", "move cursor"}}
	case m.pick != nil && m.pick.kind == pickLabels:
		keys = [][2]string{{"type", "filter"}, {"↑/↓", "select"}, {"space", "check"}, {"enter", "save"}, {"esc", "cancel"}}
	case m.pick != nil:
		keys = [][2]string{{"type", "filter"}, {"↑/↓", "select"}, {"enter", "move"}, {"esc", "cancel"}}
	case m.edit != nil:
		keys = [][2]string{{"^s", "save"}, {"esc", "cancel"}, {"^e", "$EDITOR"}}
	case m.focus == paneDetail:
		keys = [][2]string{{"j/k", "select comment"}, {"c", "comment"}, {"e", "edit comment/name"}, {"d", "delete comment"}, {"E", "description"}, {"t", "due"}, {"1-4", "priority"}, {"@", "labels"}, {"m", "move"}, {"h", "back"}}
	case m.focus == paneNav:
		keys = [][2]string{{"j/k", "move"}, {"enter", "open"}, {"tab", "next pane"}, {"a", "add"}, {"v", "notes view"}, {"f", "filter"}, {"r", "sync"}, {"q", "quit"}}
	case m.headerSection() != nil:
		keys = [][2]string{{"a", "add task here"}, {"A", "new section"}, {"e", "rename section"}, {"[", "move up"}, {"]", "move down"}, {"del", "delete section"}}
	case m.notesMode():
		keys = [][2]string{{"a", "new note"}, {"E", "write"}, {"e", "rename"}, {"c", "comment"}, {"v", "tasks view"}, {"/", "find"}}
	default:
		keys = [][2]string{{"a", "add"}, {"A", "section"}, {"x", "done"}, {"e", "rename"}, {"t", "due"}, {"1-4", "priority"}, {"@", "labels"}, {"m", "move"}, {"E", "description"}, {"c", "comment"}, {"del", "delete"}, {"^z", "undo"}, {"v", "notes view"}, {"/", "find"}, {"f", "filter"}}
	}
	// In the task list and the sidebar, esc clears an active find or filter.
	if m.inputMode == inputNone && m.cal == nil && m.menu == nil && m.pick == nil && m.edit == nil && m.focus != paneDetail {
		switch {
		case m.find != "" && m.focus == paneTasks:
			keys = append([][2]string{{"esc", "clear find"}}, keys...)
		case m.inFilterView():
			keys = append([][2]string{{"esc", "clear filter"}}, keys...)
		}
	}
	if m.confirm != nil {
		keys = [][2]string{{"y", "yes"}, {"n", "no"}}
	}
	if m.inputMode == inputFind {
		keys = [][2]string{{"enter", "keep the find"}, {"esc", "clear the find"}}
	}
	return append([][2]string{{"?", "help"}}, keys...)
}

// bottomBarLayout renders the two bottom lines and returns the positions of the legend items.
// Line 1 is the key legend. Line 2 is the last operation at the left and the sync state at
// the right. Legend items that do not fit on line 1 go to the free space on line 2.
// Line 2 shows the find input or a y/n prompt instead of the last operation while they are open.
func (m Model) bottomBarLayout() (string, []legendHit) {
	keys := m.legendKeys()
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = st(fg(hexAccent)).Bold(true).Render(k[0]) + " " + st(hexMuted).Render(k[1])
	}
	sep := st(hexDim).Render(" · ")
	var hits []legendHit

	// Line 1: as many items as fit.
	line1, x, n := " ", 1, 0
	for i, part := range parts {
		w := lipgloss.Width(part)
		extra := w
		if i > 0 {
			extra += 3
		}
		if x+extra > m.width-1 {
			break
		}
		if i > 0 {
			line1 += sep
			x += 3
		}
		if k := legendKey(keys[i][0]); k != "" {
			hits = append(hits, legendHit{x0: x, x1: x + w, y: 0, key: k})
		}
		line1 += part
		x += w
		n++
	}

	// Line 2, left: the find input, the y/n prompt, or the last operation.
	sync := st(hexDim).Render("synced " + m.st.SyncedAt.Format("15:04") + " ")
	switch {
	case m.pending > 0:
		sync = st(hexTomorrow).Render(fmt.Sprintf("saving %d… ", m.pending))
	case m.syncing > 0:
		sync = st(hexTomorrow).Render("syncing… ")
	}
	var left string
	switch {
	case m.inputMode == inputFind:
		left = m.input.View()
	case m.confirm != nil:
		left = " " + st(hexOverdue).Bold(true).Render(m.confirm.prompt)
	case m.status != "" && m.statusErr:
		left = " " + st(hexOverdue).Render("✗ "+m.status)
	case m.status != "":
		left = " " + st(hexToday).Render("✓ ") + st(hexText).Render(m.status)
	}
	room := m.width - lipgloss.Width(sync) - 1
	left = trunc(left, room)

	// Line 2, middle: the legend items that did not fit on line 1, if there is room.
	x = lipgloss.Width(left) + 3
	var rest string
	for i := n; i < len(parts); i++ {
		w := lipgloss.Width(parts[i])
		if x+w > room-1 {
			break
		}
		if rest != "" {
			rest += sep
			x += 3
		}
		if k := legendKey(keys[i][0]); k != "" {
			hits = append(hits, legendHit{x0: x, x1: x + w, y: 1, key: k})
		}
		rest += parts[i]
		x += w
	}
	line2 := left
	if rest != "" {
		line2 += "   " + rest
	}
	gap := m.width - lipgloss.Width(line2) - lipgloss.Width(sync)
	line2 += strings.Repeat(" ", max(0, gap)) + sync
	return pad(line1, m.width, lipgloss.NewStyle()) + "\n" + line2, hits
}

// helpSections is the content of the help overlay, one list of lines per section.
func helpSections() [][]string {
	k := func(key, desc string) string {
		return "  " + st(fg(hexAccent)).Bold(true).Render(fmt.Sprintf("%-12s", key)) + st(hexText).Render(desc)
	}
	h := func(s string) string { return " " + st(hexMuted).Bold(true).Render(s) }
	return [][]string{
		{h("Navigation"),
			k("tab", "next pane: sidebar → list → details"),
			k("h / l", "previous / next pane"),
			k("j / k", "move down / up"),
			k("g / G", "top / bottom"),
			k("ctrl+d/u", "half page down / up"),
			k("enter", "open · details on narrow terminals")},
		{h("Tasks"),
			k("a", "quick add: tomorrow 9am #proj @label p1"),
			k("", "goes to the section under the cursor"),
			k("x / space", "complete"),
			k("ctrl+z", "undo the last completion"),
			k("e", "rename (parsed like quick add)"),
			k("E", "edit the description / note body"),
			k("t", "due date: text, calendar, time"),
			k("", "text: fri 9am · every mon · no date"),
			k("1 – 4", "priority p1 – p4"),
			k("@", "labels picker"),
			k("m", "move to a project or section"),
			k("c", "add a comment"),
			k("del", "delete the task (asks first)")},
		{h("Sections (cursor on a header)"),
			k("A", "new section after the cursor's section"),
			k("e", "rename the section"),
			k("[ / ]", "move the section up / down"),
			k("del", "delete the section and its tasks"),
			k("right-click", "section menu")},
		{h("Details pane"),
			k("j / k", "select a comment"),
			k("e / d", "edit / delete the selected comment"),
			k("", "no comment selected: e edits the name")},
		{h("Editor and pickers"),
			k("ctrl+s", "save the editor text"),
			k("ctrl+e", "open the text in $EDITOR"),
			k("type", "filter a picker list"),
			k("↑ / ↓", "select in a picker"),
			k("space", "check a label"),
			k("enter", "save the picker choice"),
			k("esc", "cancel")},
		{h("Date dialog (t)"),
			k("tab", "text → calendar → time · parses the text"),
			k("arrows", "day / week · PgUp/PgDn month"),
			k("Home", "today · 1–5 quick picks"),
			k("enter", "save the input you changed last"),
			k("o / r", "recurring: this occurrence / replace")},
		{h("Mouse"),
			k("click", "select · click ○ to complete"),
			k("dbl-click", "rename a task · edit a comment"),
			k("right-click", "task menu"),
			k("drag", "task to a project or section header"),
			k("wheel", "scroll · shift+drag selects text"),
			k("legend", "click a key hint to run it")},
		{h("Views"),
			k("v", "project: tasks / notebook view"),
			k("/", "find in this view (esc clears)"),
			k("f", "Todoist filter: today | overdue"),
			k("esc", "clear the filter or the find"),
			k("r", "sync now (also every 60 s, on focus)"),
			k("?", "this help"),
			k("q", "quit (waits for pending saves)")},
	}
}

// helpLines lays out the help sections in one or two columns for a w-wide pane.
func helpLines(w int) []string {
	secs := helpSections()
	cols := 1
	if w >= 90 {
		cols = 2
	}
	colW := w / cols
	var col [2][]string
	total := 0
	for _, sec := range secs {
		total += len(sec) + 1
	}
	i := 0
	for _, sec := range secs {
		// Start the second column after about half of the lines.
		if cols == 2 && i == 0 && len(col[0]) >= total/2 {
			i = 1
		}
		col[i] = append(col[i], "")
		col[i] = append(col[i], sec...)
	}
	n := max(len(col[0]), len(col[1]))
	out := make([]string, 0, n+2)
	for r := 0; r < n; r++ {
		var line string
		for c := 0; c < cols; c++ {
			cell := ""
			if r < len(col[c]) {
				cell = col[c][r]
			}
			line += pad(cell, colW, lipgloss.NewStyle())
		}
		out = append(out, line)
	}
	return append(out, "", " "+st(hexDim).Render("press any key to close"))
}
