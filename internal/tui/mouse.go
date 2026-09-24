package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// doubleClickTime is the longest time between the two clicks of a double-click.
const doubleClickTime = 400 * time.Millisecond

// rect is a screen area in cells.
type rect struct{ x, y, w, h int }

func (r rect) has(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

// row returns the content row under y (0 is the first line inside the top border), or -1.
func (r rect) row(y int) int {
	if y <= r.y || y >= r.y+r.h-1 {
		return -1
	}
	return y - r.y - 1
}

// layout is the position of each pane. render and the mouse handlers both use it,
// so that a click always maps to the item on the screen.
type layout struct {
	nav, mid, detail rect // detail.w is 0 on narrow terminals
	bar              int  // y of the legend line. The status line is bar+1.
}

func (m Model) layout() layout {
	h := m.paneHeight() + 2
	navW, detW := m.navWidth(), m.detailWidth()
	midW := m.width - navW - detW
	return layout{
		nav:    rect{0, 0, navW, h},
		mid:    rect{navW, 0, midW, h},
		detail: rect{navW + midW, 0, detW, h},
		bar:    m.height - 2,
	}
}

// listRect is the area of the task list, or an empty rect if another view covers it.
func (m Model) listRect() rect {
	if !m.wide() && (m.detailOpen || m.edit != nil || m.pick != nil) {
		return rect{}
	}
	return m.layout().mid
}

// sideRect is the area that shows the details, the editor, or a picker.
func (m Model) sideRect() rect {
	l := m.layout()
	if m.wide() {
		return l.detail
	}
	if m.detailOpen || m.edit != nil || m.pick != nil {
		return l.mid
	}
	return rect{}
}

// dialogRect is the area of the open dialog.
func (m Model) dialogRect() rect {
	d := m.dialogBox()
	w, h := lipgloss.Width(d), lipgloss.Height(d)
	return rect{max(0, (m.width-w)/2), max(0, (m.height-h)/3), w, h}
}

// keyMsg makes a key press from a key string such as "a", "enter", or "ctrl+z".
// Mouse actions use it to run the same code as the keys.
func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "delete":
		return tea.KeyPressMsg{Code: tea.KeyDelete}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	if k, ok := strings.CutPrefix(s, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: []rune(k)[0], Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// isDoubleClick records a click and reports whether it completes a double-click.
func (m *Model) isDoubleClick(x, y int) bool {
	now := time.Now()
	c := m.lastClick
	dbl := !c.t.IsZero() && now.Sub(c.t) < doubleClickTime && c.y == y && c.x-x <= 1 && x-c.x <= 1
	if dbl {
		m.lastClick = clickInfo{}
	} else {
		m.lastClick = clickInfo{x: x, y: y, t: now}
	}
	return dbl
}

// clickInfo is the last click, for double-click detection.
type clickInfo struct {
	x, y int
	t    time.Time
}

// dragState is a task that the user drags to a project or a section.
type dragState struct {
	taskID    string
	projectID string // a sidebar project instead of a task
	content   string
	x, y      int  // pointer position
	active    bool // the pointer moved after the press
}

func (m Model) mouseClick(ms tea.Mouse) (tea.Model, tea.Cmd) {
	x, y := ms.X, ms.Y
	dbl := m.isDoubleClick(x, y)
	if m.quitting {
		return m, nil
	}
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	// The legend runs its key in every mode, for example "enter save" in a dialog.
	if bar := m.layout().bar; (y == bar || y == bar+1) && ms.Button == tea.MouseLeft {
		_, hits := m.bottomBarLayout()
		for _, h := range hits {
			if y == bar+h.y && x >= h.x0 && x < h.x1 {
				return m.update(keyMsg(h.key))
			}
		}
		return m, nil
	}
	switch {
	case m.confirm != nil:
		return m.confirmClick(x, y)
	case m.cal != nil:
		return m.calClick(x, y, dbl)
	case m.menu != nil:
		return m.menuClick(x, y)
	case isDialog(m.inputMode):
		if !m.dialogRect().has(x, y) {
			m.closeInput()
			m.setStatus("cancelled", false)
		}
		return m, nil
	case m.note != nil:
		return m.noteClick(x, y)
	case m.edit != nil: // keep unsaved text safe from stray clicks
		return m, nil
	case m.pick != nil:
		return m.pickerClick(x, y, dbl)
	case m.inputMode == inputFind: // a click keeps the find text, as enter does
		m.find = m.input.Value()
		m.closeInput()
	}

	l := m.layout()
	switch {
	case l.nav.has(x, y):
		m.navClick(y)
		if ms.Button == tea.MouseRight {
			switch {
			case m.navProject() != nil:
				m.menu = &ctxMenu{x: x, y: y, items: projectMenu, focus: paneNav}
			case m.navLabel() != nil:
				m.menu = &ctxMenu{x: x, y: y, items: labelMenu, focus: paneNav}
			}
			return m, nil
		}
		// A click on the ▾/▸ marker of a project hides or shows its sub-projects.
		if r := l.nav.row(y); r >= 0 && m.navOff+r < len(m.nav) {
			n := m.nav[m.navOff+r]
			markX := l.nav.x + 1 + n.markCol // the border, then the marker column in the pane
			if n.hasKids && n.markCol >= 0 && (x == markX || x == markX+1) && m.navProject() != nil {
				next := m.toggleProjectCollapse(m.navProject())
				return m, next
			}
		}
		// A press on a project can start a drag to another parent.
		if p := m.navProject(); p != nil && !p.InboxProject && ms.Button == tea.MouseLeft {
			m.drag = &dragState{projectID: p.ID, content: p.Name, x: x, y: y}
		}
		return m, nil
	case m.listRect().has(x, y):
		return m.listClick(ms, dbl)
	case m.sideRect().has(x, y):
		return m.detailClick(y, dbl)
	}
	return m, nil
}

func (m *Model) navClick(y int) {
	r := m.layout().nav.row(y)
	i := m.navOff + r
	m.focus = paneNav
	if r < 0 || i >= len(m.nav) || m.nav[i].header != "" || i == m.navCur {
		return
	}
	m.navCur = i
	m.find = ""
	m.detailOpen = false
	m.buildRows(true)
}

// rowAt returns the task-list row under y, or -1. A preview line maps to its note.
func (m Model) rowAt(y int) int {
	r := m.listRect().row(y)
	i := m.rowOff + r
	if r < 0 || i >= len(m.rows) {
		return -1
	}
	if m.rows[i].spacer {
		return -1
	}
	if m.rows[i].preview != "" {
		i--
	}
	return i
}

func (m Model) listClick(ms tea.Mouse, dbl bool) (tea.Model, tea.Cmd) {
	i := m.rowAt(ms.Y)
	m.focus = paneTasks
	if i < 0 {
		return m, nil
	}
	m.rowCur = i
	r := m.rows[i]
	lx := m.listRect().x
	if r.task == nil {
		if ms.Button == tea.MouseRight && r.sectionID != "" {
			m.menu = &ctxMenu{x: ms.X, y: ms.Y, items: sectionMenu, focus: paneTasks}
		}
		// A click on the ▾/▸ marker of a section hides or shows its tasks.
		if s := m.sections[r.sectionID]; s != nil && r.hasKids && (ms.X == lx+2 || ms.X == lx+3) {
			next := m.toggleSectionCollapse(s)
			return m, next
		}
		return m, nil
	}
	if ms.Button == tea.MouseLeft && !r.done {
		switch {
		case ms.Mod&tea.ModCtrl != 0: // ctrl+click adds the task to the selection or removes it
			m.toggleSelect(r.task.ID)
			return m, nil
		case ms.Mod&tea.ModShift != 0: // shift+click selects a range
			m.selectRange(i)
			return m, nil
		}
	}
	// A click on the ▾/▸ marker after the circle hides or shows the sub-tasks.
	if markX := lx + 4 + 2*r.depth; r.hasKids && (ms.X == markX || ms.X == markX+1) && !m.notesMode() {
		if cur := m.currentNav(); cur != nil && isOverview(cur.kind) {
			next := m.toggleOverview(r)
			return m, next
		}
		next := m.toggleTaskCollapse(m.taskByID(r.task.ID))
		return m, next
	}
	if r.done { // completed tasks only select; x reopens
		return m, nil
	}
	if ms.Button == tea.MouseRight {
		m.openMenu(ms.X, ms.Y)
		return m, nil
	}
	// The circle is after the left border, one space, and the subtask indent.
	circleX := m.listRect().x + 2 + 2*r.depth
	if !m.notesMode() && (ms.X == circleX || ms.X == circleX+1) {
		if r.task.Due != nil && r.task.Due.IsRecurring {
			m.confirm = yesNo("Complete occurrence", "Complete this occurrence of “"+plain(r.task.Content)+"”? This cannot be undone.",
				"Complete", func(m *Model) tea.Cmd { return m.complete() })
			return m, nil
		}
		next := m.complete()
		return m, next
	}
	if dbl {
		m.drag = nil
		next := m.startRename()
		return m, next
	}
	m.drag = &dragState{taskID: r.task.ID, content: plain(r.task.Content), x: ms.X, y: ms.Y}
	return m, nil
}

func (m Model) detailClick(y int, dbl bool) (tea.Model, tea.Cmd) {
	m.focusDetail()
	r := m.sideRect().row(y)
	if r < 0 {
		return m, nil
	}
	line := m.detOff + r
	d := m.buildDetail(m.detailInnerWidth() - 1)
	for k, cl := range d.checks { // a click on a checkbox toggles it
		if cl == line && m.notesMode() {
			m.chkCur, m.comCur = k, -1
			next := m.toggleNoteCheck(k)
			return m, next
		}
	}
	m.comCur = -1
	for i, cr := range d.comments {
		if line >= cr[0] && line < cr[1] {
			m.comCur = i
		}
	}
	if dbl && m.comCur >= 0 {
		return m.detailKeys("e")
	}
	// A double-click on the note text opens the editor at that line.
	if dbl && m.notesMode() && line >= d.editStart && (len(d.comments) == 0 || line < d.comments[0][0]) {
		if t := m.currentTask(); t != nil && line < len(d.lines) {
			next := m.openNoteEditorAt(sourceLineFor(t.Description, d.lines[line]))
			return m, next
		}
	}
	return m, nil
}

func (m Model) pickerClick(x, y int, dbl bool) (tea.Model, tea.Cmd) {
	r := m.sideRect()
	if !r.has(x, y) {
		m.pick = nil
		m.setStatus("cancelled", false)
		return m, nil
	}
	// The list starts after the top border, the filter line, and one blank line.
	i := m.pick.off + (y - r.y - 3)
	vis := m.pick.visible()
	if y-r.y-3 < 0 || i >= len(vis) {
		return m, nil
	}
	m.pick.cur = i
	if m.pick.kind == pickLabels {
		m.pick.toggle(vis)
		return m, nil
	}
	if m.pick.kind == pickColor && vis[i].toggleRow {
		m.pick.toggleSub()
		return m, nil
	}
	if dbl {
		return m.updatePicker(keyMsg("enter"))
	}
	return m, nil
}

func (m Model) mouseWheel(ms tea.Mouse) (tea.Model, tea.Cmd) {
	d := 3
	if ms.Button == tea.MouseWheelUp {
		d = -3
	}
	l := m.layout()
	switch {
	case m.cal != nil:
		if m.calRect().has(ms.X, ms.Y) {
			m.cal.month = m.cal.month.AddDate(0, d/3, 0) // the wheel changes the month
		}
	case m.menu != nil || isDialog(m.inputMode) || m.showHelp:
	case m.pick != nil && m.sideRect().has(ms.X, ms.Y):
		m.pick.cur = max(0, min(len(m.pick.visible())-1, m.pick.cur+d))
	case l.nav.has(ms.X, ms.Y):
		m.moveNav(d / 3)
	case m.listRect().has(ms.X, ms.Y):
		m.moveRow(d)
	case m.sideRect().has(ms.X, ms.Y) && m.edit == nil:
		m.detOff = max(0, m.detOff+d)
	}
	return m, nil
}

func (m Model) mouseMotion(ms tea.Mouse) (tea.Model, tea.Cmd) {
	if m.drag == nil {
		return m, nil
	}
	if !m.drag.active && (ms.X != m.drag.x || ms.Y != m.drag.y) {
		m.drag.active = true
		if m.drag.projectID != "" {
			m.setStatus("moving # "+m.drag.content+" · release on a project, or on “My Projects” for the top level", false)
		} else {
			m.setStatus("moving “"+m.drag.content+"” · release on a project or a section header", false)
		}
	}
	m.drag.x, m.drag.y = ms.X, ms.Y
	return m, nil
}

func (m Model) mouseRelease(ms tea.Mouse) (tea.Model, tea.Cmd) {
	d := m.drag
	m.drag = nil
	if d == nil || !d.active {
		return m, nil
	}
	l := m.layout()
	if d.projectID != "" { // a project: drop on a project to nest it, on "My Projects" for the top level
		p := m.projects[d.projectID]
		if r := l.nav.row(ms.Y); p != nil && l.nav.has(ms.X, ms.Y) && r >= 0 && m.navOff+r < len(m.nav) {
			switch n := m.nav[m.navOff+r]; {
			case n.header == "My Projects":
				next := m.moveProjectUnder(p, "")
				return m, next
			case n.kind == vkProject && n.projectID != p.ID && m.projects[n.projectID] != nil && !m.projects[n.projectID].InboxProject:
				next := m.moveProjectUnder(p, n.projectID)
				return m, next
			}
		}
		m.setStatus("move cancelled · drop on a project, or on “My Projects” for the top level", false)
		return m, nil
	}
	if l.nav.has(ms.X, ms.Y) {
		if r := l.nav.row(ms.Y); r >= 0 && m.navOff+r < len(m.nav) {
			if n := m.nav[m.navOff+r]; n.header == "" && n.kind == vkProject {
				next := m.moveTask(d.taskID, n.projectID, "")
				return m, next
			}
		}
	}
	if m.listRect().has(ms.X, ms.Y) {
		if i := m.rowAt(ms.Y); i >= 0 && m.rows[i].sectionID != "" {
			next := m.moveTask(d.taskID, m.sections[m.rows[i].sectionID].ProjectID, m.rows[i].sectionID)
			return m, next
		}
	}
	m.setStatus("move cancelled · drop on a sidebar project or a section header", false)
	return m, nil
}

// moveTask moves a task to a project, or to a section if sectionID is not empty.
func (m *Model) moveTask(taskID, projectID, sectionID string) tea.Cmd {
	for _, t := range m.snap.Tasks {
		if t.ID == taskID && t.ProjectID == projectID && t.Section() == sectionID {
			m.setStatus("the task is already there", false)
			return nil
		}
	}
	where := "#" + m.projects[projectID].Name
	if s := m.sections[sectionID]; s != nil {
		where += " / " + s.Name
	}
	m.setStatus("moving to "+where+"…", false)
	client := m.client
	return m.simpleWrite("Moved to "+where, func(ctx context.Context) error {
		return client.Move(ctx, taskID, projectID, sectionID)
	})
}

// menuItem is a context menu entry. key is the key that the entry runs.
type menuItem struct {
	label string
	key   string
}

// ctxMenu is the right-click menu of a task.
type ctxMenu struct {
	x, y  int
	cur   int
	items []menuItem
	focus pane // the pane whose keys the menu runs
}

var taskMenu = []menuItem{
	{"Rename", "e"}, {"Due date", "t"}, {"Priority 1", "1"}, {"Priority 2", "2"},
	{"Priority 3", "3"}, {"Priority 4", "4"}, {"Labels", "@"}, {"Move", "m"},
	{"Description", "E"}, {"Comment", "c"}, {"Complete", "x"}, {"Delete", "delete"},
}

// sectionMenu is the right-click menu of a section header.
var sectionMenu = []menuItem{
	{"New section", "A"}, {"Rename", "e"}, {"Move up", "["}, {"Move down", "]"}, {"Delete", "delete"},
}

func (m *Model) openMenu(x, y int) {
	m.menu = &ctxMenu{x: x, y: y, items: taskMenu, focus: paneTasks}
}

// menuRect is the area of the menu, moved inside the screen if needed.
func (m Model) menuRect() rect {
	w, h := 26, len(m.menu.items)+2
	return rect{max(0, min(m.menu.x, m.width-w)), max(0, min(m.menu.y, m.height-2-h)), w, h}
}

// runMenu closes the menu and runs the key of item i on the task under the cursor.
func (m Model) runMenu(i int) (tea.Model, tea.Cmd) {
	key := m.menu.items[i].key
	f := m.menu.focus
	m.menu = nil
	m.focus = f
	return m.updateKeys(keyMsg(key))
}

func (m Model) menuClick(x, y int) (tea.Model, tea.Cmd) {
	r := m.menuRect()
	if !r.has(x, y) {
		m.menu = nil
		return m, nil
	}
	if i := r.row(y); i >= 0 && i < len(m.menu.items) {
		return m.runMenu(i)
	}
	return m, nil
}

// updateMenu handles keys while the menu is open. An item key, such as "t", runs that item.
func (m Model) updateMenu(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k := msg.String(); k {
	case "esc", "q":
		m.menu = nil
	case "j", "down":
		m.menu.cur = min(len(m.menu.items)-1, m.menu.cur+1)
	case "k", "up":
		m.menu.cur = max(0, m.menu.cur-1)
	case "enter":
		return m.runMenu(m.menu.cur)
	default:
		for i, it := range m.menu.items {
			if it.key == k {
				return m.runMenu(i)
			}
		}
	}
	return m, nil
}

// menuBox draws the context menu.
func (m Model) menuBox() string {
	r := m.menuRect()
	inner := r.w - 2
	var lines []string
	for i, it := range m.menu.items {
		base := lipgloss.NewStyle()
		if i == m.menu.cur {
			base = base.Background(c(hexSelBg))
		}
		label := base.Foreground(c(hexText)).Render(" " + it.label)
		keyText := it.key
		if keyText == "delete" {
			keyText = "del"
		}
		key := base.Foreground(c(fg(hexAccent))).Bold(true).Render(keyText + " ")
		gap := inner - lipgloss.Width(label) - lipgloss.Width(key)
		lines = append(lines, label+base.Render(strings.Repeat(" ", max(0, gap)))+key)
	}
	return box("", lines, r.w, fg(hexAccent))
}

// overlays draws the dialog, the menu, and the drag label over the screen.
func (m Model) overlays(screen string) string {
	layers := []*lipgloss.Layer{lipgloss.NewLayer(screen)}
	if isDialog(m.inputMode) {
		r := m.dialogRect()
		layers = append(layers, lipgloss.NewLayer(m.dialogBox()).X(r.x).Y(r.y).Z(1))
	}
	if m.note != nil && m.note.search != nil { // the find box covers the top of the editor
		r := m.sideRect()
		layers = append(layers, lipgloss.NewLayer(m.findBox(r.w-2)).X(r.x+1).Y(r.y+1).Z(1))
	}
	if m.confirm != nil {
		r := m.confirmRect()
		d, _ := m.confirmLayout()
		layers = append(layers, lipgloss.NewLayer(d).X(r.x).Y(r.y).Z(4))
	}
	if m.cal != nil {
		r := m.calRect()
		layers = append(layers, lipgloss.NewLayer(m.calBox()).X(r.x).Y(r.y).Z(1))
	}
	if m.menu != nil {
		r := m.menuRect()
		layers = append(layers, lipgloss.NewLayer(m.menuBox()).X(r.x).Y(r.y).Z(2))
	}
	if m.drag != nil && m.drag.active {
		label := lipgloss.NewStyle().Background(c(hexSelBg)).Foreground(c(fg(hexAccent))).
			Render(" ⇢ " + trunc(m.drag.content, 30) + " ")
		layers = append(layers, lipgloss.NewLayer(label).X(min(m.drag.x+2, max(0, m.width-lipgloss.Width(label)))).Y(m.drag.y).Z(3))
	}
	if len(layers) == 1 {
		return screen
	}
	return lipgloss.NewCompositor(layers...).Render()
}
