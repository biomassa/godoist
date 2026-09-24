package tui

import (
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/state"
	"github.com/biomassa/godoist/internal/todoist"
)

// Overviews are the views that collect tasks from many places: Today, Upcoming, All tasks,
// labels, and filters. They show sub-tasks nested under their parents:
//
//   - A task of the view pulls in its parents up to the top-level task (the root). A parent
//     that is not in the view itself is dimmed.
//   - A root shows once, in the group of the earliest date among the view tasks below it.
//   - An expanded task shows all its sub-tasks. Tasks are collapsed by default, but a task
//     that is only there because of its sub-tasks is expanded by default.
//   - z changes the state. godoist saves the state per view in the local state file. The
//     Todoist collapsed state stays for the project views.

// isOverview reports whether a view kind is an overview.
func isOverview(k viewKind) bool {
	switch k {
	case vkToday, vkUpcoming, vkAll, vkLabel, vkFilter:
		return true
	}
	return false
}

// overviewKey names the current overview in the state file.
func (m Model) overviewKey() string {
	cur := m.currentNav()
	if cur == nil {
		return ""
	}
	switch cur.kind {
	case vkToday:
		return "today"
	case vkUpcoming:
		return "upcoming"
	case vkAll:
		return "all"
	case vkLabel:
		return "label:" + cur.labelID
	case vkFilter:
		return "filter:" + m.filterQuery
	}
	return ""
}

// ovTree is the task tree of an overview.
type ovTree struct {
	inView   map[string]bool
	children map[string][]*todoist.Task
	below    map[string]bool // the subtree has a view task below the task itself
	key      string
	open     map[string]bool // saved states of this view: ID → expanded
}

// ovRoot is a top-level task of an overview with its earliest view date.
type ovRoot struct {
	task    *todoist.Task
	when    time.Time
	hasWhen bool
	first   int // position of the first view task, for filters
}

// buildOverview makes the tree for the view tasks and returns the roots.
func (m *Model) buildOverview(view []todoist.Task) (ovTree, []ovRoot) {
	byID := map[string]*todoist.Task{}
	for i := range m.snap.Tasks {
		byID[m.snap.Tasks[i].ID] = &m.snap.Tasks[i]
	}
	tr := ovTree{inView: map[string]bool{}, children: map[string][]*todoist.Task{}, below: map[string]bool{}, key: m.overviewKey()}
	tr.open = m.ui.OverviewOpen[tr.key]
	for i := range m.snap.Tasks {
		t := &m.snap.Tasks[i]
		if p := t.Parent(); p != "" && byID[p] != nil {
			tr.children[p] = append(tr.children[p], t)
		}
	}
	for _, kids := range tr.children {
		sort.SliceStable(kids, func(i, j int) bool { return kids[i].ChildOrder < kids[j].ChildOrder })
	}
	roots := map[string]*ovRoot{}
	var order []string
	for pos, v := range view {
		t := byID[v.ID]
		if t == nil { // a filter result that the local copy does not have yet
			t = &view[pos]
			byID[t.ID] = t
		}
		tr.inView[t.ID] = true
		r := t
		for p := byID[r.Parent()]; p != nil; p = byID[p.Parent()] {
			tr.below[p.ID] = true
			r = p
		}
		root := roots[r.ID]
		if root == nil {
			root = &ovRoot{task: r, first: pos}
			roots[r.ID] = root
			order = append(order, r.ID)
		}
		if d, _, ok := t.Due.Time(); ok && (!root.hasWhen || d.Before(root.when)) {
			root.when, root.hasWhen = d, true
		}
	}
	out := make([]ovRoot, len(order))
	for i, id := range order {
		out[i] = *roots[id]
	}
	return tr, out
}

// expanded reports whether a task is open in this overview.
func (tr ovTree) expanded(id string) bool {
	if v, ok := tr.open[id]; ok {
		return v
	}
	return !tr.inView[id] && tr.below[id] // pulled in only by sub-tasks
}

// emit adds the rows of t and, if t is expanded, of all its sub-tasks.
func (tr ovTree) emit(rows []row, t *todoist.Task, depth int) []row {
	kids := tr.children[t.ID]
	open := tr.expanded(t.ID)
	r := row{task: t, depth: depth, hasKids: len(kids) > 0, collapsed: len(kids) > 0 && !open, pulled: !tr.inView[t.ID]}
	if r.collapsed {
		r.hidden = tr.count(t.ID)
	}
	rows = append(rows, r)
	if open {
		for _, k := range kids {
			rows = tr.emit(rows, k, depth+1)
		}
	}
	return rows
}

// count counts all tasks below id.
func (tr ovTree) count(id string) int {
	n := 0
	for _, k := range tr.children[id] {
		n += 1 + tr.count(k.ID)
	}
	return n
}

// viewCount counts the view tasks in the subtree of id.
func (tr ovTree) viewCount(id string) int {
	n := 0
	if tr.inView[id] {
		n++
	}
	for _, k := range tr.children[id] {
		n += tr.viewCount(k.ID)
	}
	return n
}

// sortRoots sorts roots by date (tasks without a date last, in project order), then priority.
func sortRoots(rs []ovRoot) {
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if a.hasWhen != b.hasWhen {
			return a.hasWhen
		}
		if a.hasWhen && !a.when.Equal(b.when) {
			return a.when.Before(b.when)
		}
		if !a.hasWhen {
			return a.task.ChildOrder < b.task.ChildOrder
		}
		return a.task.Priority > b.task.Priority
	})
}

// todayRows builds Overdue and Today.
func (m *Model) todayRows(now time.Time) []row {
	var view []todoist.Task
	for _, t := range m.snap.Tasks {
		if k := todoist.Classify(t.Due, now); k == todoist.Overdue || k == todoist.DueToday {
			view = append(view, t)
		}
	}
	tr, roots := m.buildOverview(view)
	sortRoots(roots)
	var over, today []row
	var nOver, nToday int
	for _, r := range roots {
		if r.hasWhen && dayOf(r.when).Before(dayOf(now)) {
			over = tr.emit(over, r.task, 0)
			nOver += tr.viewCount(r.task.ID)
		} else {
			today = tr.emit(today, r.task, 0)
			nToday += tr.viewCount(r.task.ID)
		}
	}
	var rows []row
	if len(over) > 0 {
		rows = append(rows, row{header: "Overdue", headerHex: hexOverdue, count: nOver})
		rows = append(rows, over...)
	}
	rows = append(rows, row{header: "Today · " + now.Format("Mon 2 Jan"), headerHex: hexToday,
		count: nToday, date: now.Format("2006-01-02")})
	return append(rows, today...)
}

// upcomingRows builds one group per day after today.
func (m *Model) upcomingRows(now time.Time) []row {
	var view []todoist.Task
	for _, t := range m.snap.Tasks {
		if todoist.Classify(t.Due, now) >= todoist.DueTomorrow {
			view = append(view, t)
		}
	}
	tr, roots := m.buildOverview(view)
	sortRoots(roots)
	var rows []row
	lastDay, head := "", -1
	for _, r := range roots {
		day := r.when.Format("Mon 2 Jan")
		if r.when.Year() != now.Year() {
			day = r.when.Format("Mon 2 Jan 2006")
		}
		if day != lastDay {
			due := &todoist.Due{Date: r.when.Format("2006-01-02")}
			hex, label := hexText, day
			switch todoist.Classify(due, now) {
			case todoist.DueTomorrow:
				label += " · Tomorrow"
				hex = hexTomorrow
			case todoist.DueThisWeek:
				hex = hexWeek
			}
			rows = append(rows, row{header: label, headerHex: hex, date: r.when.Format("2006-01-02")})
			head, lastDay = len(rows)-1, day
		}
		rows[head].count += tr.viewCount(r.task.ID)
		rows = tr.emit(rows, r.task, 0)
	}
	return rows
}

// projectGroupedRows groups the view tasks by project in sidebar order (All tasks, labels).
func (m *Model) projectGroupedRows(view []todoist.Task) []row {
	tr, roots := m.buildOverview(view)
	byProject := map[string][]ovRoot{}
	for _, r := range roots {
		byProject[r.task.ProjectID] = append(byProject[r.task.ProjectID], r)
	}
	var rows []row
	for _, op := range m.orderedProjects() {
		rs := byProject[op.p.ID]
		if len(rs) == 0 {
			continue
		}
		sortRoots(rs)
		glyph := "# "
		if op.p.InboxProject {
			glyph = "⌂ "
		}
		n := 0
		for _, r := range rs {
			n += tr.viewCount(r.task.ID)
		}
		rows = append(rows, row{header: glyph + op.p.Name, headerHex: fg(todoist.ColorHex(op.p.Color)), count: n})
		for _, r := range rs {
			rows = tr.emit(rows, r.task, 0)
		}
	}
	return rows
}

// filterRows keeps the order of the filter results, with the tree around each result.
func (m *Model) filterRows() []row {
	tr, roots := m.buildOverview(m.filterTasks)
	sort.SliceStable(roots, func(i, j int) bool { return roots[i].first < roots[j].first })
	var rows []row
	for _, r := range roots {
		rows = tr.emit(rows, r.task, 0)
	}
	return rows
}

// toggleOverview opens or closes the task of row r in this overview and saves the state.
func (m *Model) toggleOverview(r row) tea.Cmd {
	key := m.overviewKey()
	if key == "" || r.task == nil || !r.hasKids {
		return nil
	}
	if m.ui.OverviewOpen == nil {
		m.ui.OverviewOpen = map[string]map[string]bool{}
	}
	if m.ui.OverviewOpen[key] == nil {
		m.ui.OverviewOpen[key] = map[string]bool{}
	}
	m.ui.OverviewOpen[key][r.task.ID] = r.collapsed // collapsed → open, open → collapsed
	if err := state.Save(m.ui); err != nil {
		m.setStatus("could not save the view state: "+err.Error(), true)
	}
	m.buildRows(false)
	return nil
}
