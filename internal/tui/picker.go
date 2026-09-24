package tui

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// pickKind is the type of list that a picker shows.
type pickKind int

const (
	pickLabels pickKind = iota // checklist of labels
	pickMove                   // projects and sections
	pickColor                  // Todoist colors, for a new project or a color change
)

// Label states in the label picker.
const (
	stateNone = iota // no task has the label
	stateSome        // some tasks have the label
	stateAll         // all tasks have the label
)

// pickItem is a row in a picker.
type pickItem struct {
	text      string // display text
	match     string // lowercase text for the filter
	color     string
	depth     int
	checked   bool   // the "sub-project of" row of the color picker
	state     int    // label picker: stateNone, stateSome, or stateAll
	orig      int    // label picker: the state when the picker opened
	create    bool   // the "+ create" row of the label picker
	fresh     bool   // a label that saveLabels must create first
	toggleRow bool   // the "sub-project of" row of the color picker
	colorName string // Todoist color name (color picker)
	project   string // lowercase project name (move picker)
	section   string // lowercase section name (move picker)
	projectID string
	sectionID string
}

// picker is a filterable list that replaces the details pane. Typed text goes to the filter.
type picker struct {
	kind    pickKind
	taskIDs []string
	title   string
	items   []pickItem
	filter  textinput.Model
	cur     int // index into visible()
	off     int // scroll offset of the list

	projectID string // color picker: the project to change
	labelID   string // color picker: the label to change
	newName   string // color picker: the name of the project to create
}

// matches reports whether an item matches the lowercase filter q. In the move picker,
// "pro/sec" matches a section whose project contains "pro" and whose name contains "sec".
func (p *picker) matches(it pickItem, q string) bool {
	if q == "" || it.toggleRow {
		return true
	}
	if p.kind == pickMove {
		if pq, sq, ok := strings.Cut(q, "/"); ok {
			return strings.Contains(it.project, pq) && it.sectionID != "" && strings.Contains(it.section, sq)
		}
	}
	return strings.Contains(it.match, q)
}

// visible returns the items that match the filter. The label picker adds a
// "+ create" row if no label has the exact filter text as its name.
func (p *picker) visible() []pickItem {
	q := strings.ToLower(strings.TrimSpace(p.filter.Value()))
	var out []pickItem
	exact := false
	for _, it := range p.items {
		if p.matches(it, q) {
			out = append(out, it)
		}
		if p.kind == pickLabels && strings.EqualFold(it.text, q) {
			exact = true
		}
	}
	if p.kind == pickLabels && q != "" && !exact {
		name := strings.TrimSpace(p.filter.Value())
		out = append(out, pickItem{text: name, match: q, create: true, color: todoist.ColorHex("charcoal")})
	}
	return out
}

// pickerFilterWidth is the filter width for a picker of outer width w. A textinput view is
// its prompt plus the width plus one cell for the cursor, and it must fit inside the border.
func pickerFilterWidth(w int) int { return max(10, w-2-lipgloss.Width(" filter › ")-2) }

func newPickerFilter() textinput.Model {
	ti := textinput.New()
	ti.Prompt = " filter › "
	ti.Placeholder = "type to filter"
	ti.CharLimit = 60
	return ti
}

// openLabelPicker opens the label checklist for the task under the cursor.
func (m *Model) openLabelPicker() tea.Cmd {
	t := m.currentTask()
	if t == nil {
		return nil
	}
	return m.openLabelPickerFor([]*todoist.Task{t})
}

// openLabelPickerFor opens the label checklist for one or more tasks. A label that all
// tasks have is [x], a label that some have is [~], and other labels are [ ].
func (m *Model) openLabelPickerFor(ts []*todoist.Task) tea.Cmd {
	n := labelStates(ts)
	state := func(name string) int {
		switch {
		case n[name] == 0:
			return stateNone
		case n[name] == len(ts):
			return stateAll
		}
		return stateSome
	}
	var items []pickItem
	known := map[string]bool{}
	for _, l := range m.snap.Labels {
		known[l.Name] = true
		st := state(l.Name)
		items = append(items, pickItem{text: l.Name, match: strings.ToLower(l.Name), color: todoist.ColorHex(l.Color), state: st, orig: st})
	}
	// Keep labels that are on a task but not in the label list (for example, shared labels).
	for name := range n {
		if !known[name] {
			known[name] = true
			st := state(name)
			items = append(items, pickItem{text: name, match: strings.ToLower(name), color: todoist.ColorHex("charcoal"), state: st, orig: st})
		}
	}
	title := "Labels · " + plain(ts[0].Content)
	if len(ts) > 1 {
		title = fmt.Sprintf("Labels · %d tasks", len(ts))
	}
	m.pick = &picker{kind: pickLabels, taskIDs: joinIDs(ts), title: title, items: items, filter: newPickerFilter()}
	m.pick.filter.SetWidth(pickerFilterWidth(m.pickerWidth()))
	return m.pick.filter.Focus()
}

// openMovePicker opens the list of projects and sections for the task under the cursor.
func (m *Model) openMovePicker() tea.Cmd {
	t := m.currentTask()
	if t == nil {
		return nil
	}
	return m.openMovePickerFor([]*todoist.Task{t})
}

// openMovePickerFor opens the move picker for one or more tasks. The cursor starts on the
// place of the first task.
func (m *Model) openMovePickerFor(ts []*todoist.Task) tea.Cmd {
	t := ts[0]
	secs := map[string][]*todoist.Section{}
	for i := range m.snap.Sections {
		s := &m.snap.Sections[i]
		if !s.IsArchived {
			secs[s.ProjectID] = append(secs[s.ProjectID], s)
		}
	}
	var items []pickItem
	cur := 0
	for _, op := range m.orderedProjects() {
		p := op.p
		col := todoist.ColorHex(p.Color)
		if p.ID == t.ProjectID && t.Section() == "" {
			cur = len(items)
		}
		items = append(items, pickItem{text: "# " + p.Name, match: strings.ToLower(p.Name), color: col, depth: op.depth,
			projectID: p.ID, project: strings.ToLower(p.Name)})
		ss := secs[p.ID]
		sort.SliceStable(ss, func(i, j int) bool { return ss[i].SectionOrder < ss[j].SectionOrder })
		for _, s := range ss {
			if s.ID == t.Section() {
				cur = len(items)
			}
			items = append(items, pickItem{text: "└ " + s.Name, match: strings.ToLower(p.Name + " " + s.Name),
				color: col, depth: op.depth + 1, projectID: p.ID, sectionID: s.ID,
				project: strings.ToLower(p.Name), section: strings.ToLower(s.Name)})
		}
	}
	title := "Move · " + plain(t.Content)
	if len(ts) > 1 {
		title = fmt.Sprintf("Move · %d tasks", len(ts))
	}
	m.pick = &picker{kind: pickMove, taskIDs: joinIDs(ts), title: title, items: items, filter: newPickerFilter(), cur: cur}
	m.pick.filter.Placeholder = "type to filter, e.g. pack or pack/urg"
	m.pick.filter.SetWidth(pickerFilterWidth(m.pickerWidth()))
	return m.pick.filter.Focus()
}

// updatePicker handles keys while a picker is open.
func (m Model) updatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.pick
	vis := p.visible()
	switch msg.String() {
	case "esc":
		m.pick = nil
		m.setStatus("cancelled", false)
		return m, nil
	case "up", "ctrl+p":
		p.cur = max(0, p.cur-1)
		return m, nil
	case "down", "ctrl+n":
		p.cur = min(len(vis)-1, p.cur+1)
		return m, nil
	case "pgup":
		p.cur = max(0, p.cur-m.paneHeight()/2)
		return m, nil
	case "pgdown":
		p.cur = min(len(vis)-1, p.cur+m.paneHeight()/2)
		return m, nil
	case "space":
		if p.kind == pickLabels {
			p.toggle(vis)
			return m, nil
		}
		if p.kind == pickColor {
			p.toggleSub()
			return m, nil
		}
	case "enter":
		if p.kind == pickLabels {
			next := m.saveLabels()
			return m, next
		}
		if p.kind == pickColor {
			if p.cur >= 0 && p.cur < len(vis) {
				next := m.pickColorEnter(vis[p.cur])
				return m, next
			}
			return m, nil
		}
		if p.cur >= 0 && p.cur < len(vis) {
			next := m.moveTo(vis[p.cur])
			return m, next
		}
		return m, nil
	}
	before := p.filter.Value()
	var cmd tea.Cmd
	p.filter, cmd = p.filter.Update(msg)
	if p.filter.Value() != before {
		p.cur, p.off = 0, 0
	}
	return m, cmd
}

// toggle changes the checkbox of the row under the cursor. [x] and [ ] alternate. A label
// that was [~] cycles [~] → [x] → [ ] → [~]. On the "+ create" row, it adds a new
// checked label and clears the filter.
func (p *picker) toggle(vis []pickItem) {
	if p.cur < 0 || p.cur >= len(vis) {
		return
	}
	it := vis[p.cur]
	if it.create {
		p.items = append(p.items, pickItem{text: it.text, match: it.match, color: it.color, state: stateAll, orig: stateNone, fresh: true})
		p.filter.SetValue("")
		p.cur = len(p.items) - 1
		return
	}
	for i := range p.items {
		if p.items[i].text != it.text {
			continue
		}
		switch x := &p.items[i]; {
		case x.orig == stateSome && x.state == stateSome:
			x.state = stateAll
		case x.orig == stateSome && x.state == stateAll:
			x.state = stateNone
		case x.orig == stateSome:
			x.state = stateSome
		case x.state == stateAll:
			x.state = stateNone
		default:
			x.state = stateAll
		}
	}
}

// saveLabels creates new labels and applies only the changed labels: [x] adds the label
// to every task, [ ] removes it from every task, and [~] keeps each task as it is.
func (m *Model) saveLabels() tea.Cmd {
	p := m.pick
	m.pick = nil
	var add, remove, create []string
	for _, it := range p.items {
		if it.state == it.orig {
			continue
		}
		switch it.state {
		case stateAll:
			add = append(add, it.text)
			if it.fresh {
				create = append(create, it.text)
			}
		case stateNone:
			remove = append(remove, it.text)
		}
	}
	if len(add)+len(remove) == 0 {
		m.setStatus("no label changes", false)
		return nil
	}
	newLabels := map[string][]string{}
	for _, id := range p.taskIDs {
		t := m.taskByID(id)
		if t == nil {
			continue
		}
		var ls []string
		for _, l := range t.Labels {
			if !slices.Contains(remove, l) {
				ls = append(ls, l)
			}
		}
		for _, l := range add {
			if !slices.Contains(ls, l) {
				ls = append(ls, l)
			}
		}
		if ls == nil {
			ls = []string{}
		}
		newLabels[id] = ls
		t.Labels = ls // show the labels at once. The sync confirms them.
	}
	var parts []string
	if len(add) > 0 {
		parts = append(parts, "+@"+strings.Join(add, " +@"))
	}
	if len(remove) > 0 {
		parts = append(parts, "−@"+strings.Join(remove, " −@"))
	}
	done := "Labels " + strings.Join(parts, " ")
	if len(p.taskIDs) > 1 {
		done += fmt.Sprintf(" on %d tasks", len(p.taskIDs))
	}
	client := m.client
	return m.simpleWrite(done, func(ctx context.Context) error {
		for _, n := range create {
			if _, err := client.CreateLabel(ctx, n); err != nil {
				return fmt.Errorf("create label %q: %w", n, err)
			}
		}
		for id, ls := range newLabels {
			if _, err := client.UpdateTask(ctx, id, map[string]any{"labels": ls}); err != nil {
				return err
			}
		}
		return nil
	})
}

// moveTo moves the picker tasks to the project or section of it.
func (m *Model) moveTo(it pickItem) tea.Cmd {
	ids := m.pick.taskIDs
	m.pick = nil
	if len(ids) == 1 {
		return m.moveTask(ids[0], it.projectID, it.sectionID)
	}
	where := "#" + m.projects[it.projectID].Name
	if s := m.sections[it.sectionID]; s != nil {
		where += " / " + s.Name
	}
	client := m.client
	return m.simpleWrite(fmt.Sprintf("Moved %d task(s) to %s", len(ids), where), func(ctx context.Context) error {
		for _, id := range ids {
			if err := client.Move(ctx, id, it.projectID, it.sectionID); err != nil {
				return err
			}
		}
		return nil
	})
}

// pickerWidth is the outer width of the pane that shows the picker.
func (m Model) pickerWidth() int {
	if m.wide() {
		return m.detailWidth()
	}
	return m.width - m.navWidth()
}

// pickerBox draws the picker: the filter line, the list, and a key hint.
func (m Model) pickerBox(w, h int) string {
	p := m.pick
	inner := w - 2
	vis := p.visible()
	listH := max(1, h-5) // filter line, blank line, and up to two hint lines
	p.cur = max(0, min(p.cur, len(vis)-1))
	if p.cur < p.off {
		p.off = p.cur
	}
	if p.cur >= p.off+listH {
		p.off = p.cur - listH + 1
	}
	lines := []string{p.filter.View(), ""}
	for i := p.off; i < len(vis) && i < p.off+listH; i++ {
		it := vis[i]
		base := lipgloss.NewStyle()
		if i == p.cur {
			base = base.Background(c(hexSelBg))
		}
		var mark string
		switch {
		case it.toggleRow && it.checked:
			mark = base.Foreground(c(hexToday)).Render("[x] ")
		case it.toggleRow:
			mark = base.Foreground(c(hexMuted)).Render("[ ] ")
		case p.kind == pickColor:
			mark = base.Foreground(c(fg(it.color))).Render("■ ")
		case it.create:
			mark = base.Foreground(c(hexToday)).Render("+ create ")
		case p.kind == pickLabels && it.state == stateAll:
			mark = base.Foreground(c(hexToday)).Render("[x] ")
		case p.kind == pickLabels && it.state == stateSome:
			mark = base.Foreground(c(hexTomorrow)).Render("[~] ")
		case p.kind == pickLabels:
			mark = base.Foreground(c(hexMuted)).Render("[ ] ")
		}
		text := it.text
		if p.kind == pickLabels {
			text = "@" + text
		}
		line := base.Render(" "+strings.Repeat("  ", it.depth)) + mark + base.Foreground(c(fg(it.color))).Render(trunc(text, inner-12))
		lines = append(lines, pad(line, inner, base))
	}
	if len(vis) == 0 {
		lines = append(lines, " "+st(hexMuted).Render("no match"))
	}
	hint := "↑/↓ select · space check · enter save · esc cancel"
	switch {
	case p.kind == pickMove:
		hint = "↑/↓ select · enter move here · esc cancel"
	case p.kind == pickColor && p.newName != "":
		hint = "↑/↓ select · space sub-project on / off · enter create · esc cancel"
	case p.kind == pickColor:
		hint = "↑/↓ select · enter save the color · esc cancel"
	}
	hintLines := wrapHint(hint, hexDim, inner-1)
	all := padLines(lines, inner, h-len(hintLines))
	all = append(all, padLines(hintLines, inner, len(hintLines))...)
	return box(st(fg(hexAccent)).Bold(true).Render(p.title), all, w, fg(hexAccent))
}
