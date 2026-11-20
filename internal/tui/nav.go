package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/biomassa/godoist/internal/todoist"
)

// viewKind is the type of the sidebar item that the task list shows.
type viewKind int

const (
	vkToday viewKind = iota
	vkUpcoming
	vkProject
	vkFilter
	vkAll       // all tasks, grouped by project
	vkLabel     // tasks with one label, grouped by project
	vkCompleted // tasks completed in the last 30 days, grouped by project
	vkLabelHint // the "A adds a label" line when there are no labels
)

// navItem is a sidebar row. A row with a header is a label and the cursor skips it.
type navItem struct {
	header    string
	kind      viewKind
	projectID string
	name      string
	glyph     string
	color     string
	count     int
	countHex  string
	depth     int
	labelID   string // vkLabel
	hasKids   bool   // a project with sub-projects
	collapsed bool   // its sub-projects are hidden
	// tree is the tree prefix of a project in My Projects: │ ├ └ lines for sub-projects,
	// and empty for top-level projects. The ▾/▸ marker goes after the name.
	tree   string
	inTree bool
}

// inFilterView reports whether the selected sidebar item is the filter view.
func (m Model) inFilterView() bool {
	cur := m.currentNav()
	return cur != nil && cur.kind == vkFilter
}

// clearFilter removes the filter view and selects the view that was open before it.
func (m *Model) clearFilter() {
	back := m.preFilterNav
	m.filterQuery, m.filterTasks, m.preFilterNav = "", nil, ""
	m.buildNav()
	m.selectNav(func(n navItem) bool { return navKey(n) == back })
	m.buildRows(true)
	m.setStatus("filter cleared", false)
}

// buildNav makes the sidebar rows and keeps the selection.
func (m *Model) buildNav() {
	var prevKey string
	if cur := m.currentNav(); cur != nil {
		prevKey = navKey(*cur)
	}
	now := time.Now()
	perProject := map[string]int{}
	var today, overdue int
	for _, t := range m.snap.Tasks {
		perProject[t.ProjectID]++
		switch todoist.Classify(t.Due, now) {
		case todoist.Overdue:
			overdue++
		case todoist.DueToday:
			today++
		}
	}
	todayHex := hexMuted
	if overdue > 0 {
		todayHex = hexOverdue
	}
	projItem := func(p *todoist.Project, depth int) navItem {
		glyph := "#"
		if p.InboxProject {
			glyph = "⌂"
		}
		return navItem{kind: vkProject, projectID: p.ID, name: p.Name, glyph: glyph,
			color: todoist.ColorHex(p.Color), count: perProject[p.ID], depth: depth}
	}
	// Order: Inbox, Today and Upcoming, Favorites, My Projects, All tasks. An empty
	// line is at the top and before each group.
	nav := []navItem{{header: " "}}
	ordered := m.orderedProjects()
	for _, op := range ordered {
		if op.p.InboxProject {
			nav = append(nav, projItem(op.p, 0))
		}
	}
	nav = append(nav, navItem{header: " "},
		navItem{kind: vkToday, name: "Today", glyph: "◉", color: hexToday, count: today + overdue, countHex: todayHex},
		navItem{kind: vkUpcoming, name: "Upcoming", glyph: "▦", color: hexWeek})
	if m.filterQuery != "" {
		nav = append(nav, navItem{kind: vkFilter, name: m.filterQuery, glyph: "⌕", color: hexTomorrow, count: len(m.filterTasks)})
	}
	var favs []navItem
	for _, op := range ordered {
		if op.p.IsFavorite {
			favs = append(favs, projItem(op.p, 0))
		}
	}
	if len(favs) > 0 {
		nav = append(nav, navItem{header: " "}, navItem{header: "Favorites"})
		nav = append(nav, favs...)
	}
	nav = append(nav, navItem{header: " "}, navItem{header: "My Projects"})
	hasKids := map[string]bool{}
	for _, op := range ordered {
		if p := op.p.ParentID; p != nil {
			hasKids[*p] = true
		}
	}
	hiddenUnder := map[string]bool{} // projects inside a collapsed project
	var shown []orderedProject
	for _, op := range ordered {
		if op.p.InboxProject {
			continue
		}
		if p := op.p.ParentID; p != nil && (hiddenUnder[*p] || (m.projects[*p] != nil && m.projects[*p].IsCollapsed)) {
			hiddenUnder[op.p.ID] = true
			continue
		}
		shown = append(shown, op)
	}
	// last[i] is true if shown[i] is the last shown project among its siblings.
	last := make([]bool, len(shown))
	for i, op := range shown {
		last[i] = true
		for j := i + 1; j < len(shown) && shown[j].depth >= op.depth; j++ {
			if shown[j].depth == op.depth {
				last[i] = false
				break
			}
		}
	}
	var open []bool // for each ancestor level: does a later sibling follow (draw │)?
	for i, op := range shown {
		it := projItem(op.p, op.depth)
		it.hasKids, it.collapsed = hasKids[op.p.ID], op.p.IsCollapsed && hasKids[op.p.ID]
		it.inTree = true
		open = append(open[:min(op.depth, len(open))], !last[i])
		if op.depth > 0 {
			var b strings.Builder
			b.WriteString("  ")
			for d := 1; d < op.depth; d++ {
				if open[d] {
					b.WriteString("│ ")
				} else {
					b.WriteString("  ")
				}
			}
			if last[i] {
				b.WriteString("└ ")
			} else {
				b.WriteString("├ ")
			}
			it.tree = b.String()
		}
		nav = append(nav, it)
	}
	nav = append(nav, navItem{header: " "}, navItem{header: "Labels"})
	perLabel := map[string]int{}
	for _, t := range m.snap.Tasks {
		for _, l := range t.Labels {
			perLabel[l]++
		}
	}
	for _, l := range m.snap.Labels {
		nav = append(nav, navItem{kind: vkLabel, labelID: l.ID, name: l.Name, glyph: "@",
			color: todoist.ColorHex(l.Color), count: perLabel[l.Name]})
	}
	if len(m.snap.Labels) == 0 {
		nav = append(nav, navItem{kind: vkLabelHint, name: "A adds a label", glyph: " ", color: hexDim})
	}
	nav = append(nav, navItem{header: " "},
		navItem{kind: vkAll, name: "All tasks", glyph: "≡", color: hexMuted, count: len(m.snap.Tasks)},
		navItem{kind: vkCompleted, name: "Completed", glyph: "✓", color: hexToday})
	m.nav = nav

	m.navCur = -1
	for i, n := range m.nav {
		if n.header == "" && prevKey != "" && navKey(n) == prevKey {
			m.navCur = i
			break
		}
	}
	if m.navCur < 0 { // start in Today
		m.selectNav(func(n navItem) bool { return n.kind == vkToday })
	}
}

func navKey(n navItem) string { return fmt.Sprintf("%d/%s/%s", n.kind, n.projectID, n.labelID) }

type orderedProject struct {
	p     *todoist.Project
	depth int
}

// orderedProjects returns active projects in sidebar (tree) order.
func (m *Model) orderedProjects() []orderedProject {
	children := map[string][]*todoist.Project{}
	for i := range m.snap.Projects {
		p := &m.snap.Projects[i]
		if p.IsArchived {
			continue
		}
		parent := ""
		if p.ParentID != nil && m.projects[*p.ParentID] != nil {
			parent = *p.ParentID
		}
		children[parent] = append(children[parent], p)
	}
	var out []orderedProject
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		kids := children[parent]
		sort.SliceStable(kids, func(i, j int) bool {
			if kids[i].InboxProject != kids[j].InboxProject {
				return kids[i].InboxProject
			}
			return kids[i].ChildOrder < kids[j].ChildOrder
		})
		for _, p := range kids {
			out = append(out, orderedProject{p, depth})
			walk(p.ID, depth+1)
		}
	}
	walk("", 0)
	return out
}

func (m *Model) currentNav() *navItem {
	if m.navCur < 0 || m.navCur >= len(m.nav) {
		return nil
	}
	return &m.nav[m.navCur]
}

func (m *Model) selectNav(pred func(navItem) bool) {
	for i, n := range m.nav {
		if n.header == "" && pred(n) {
			m.navCur = i
			return
		}
	}
}

func (m *Model) moveNav(d int) {
	prev := m.navCur
	step := 1
	if d < 0 {
		step, d = -1, -d
	}
	for ; d > 0; d-- {
		i := m.navCur + step
		for i >= 0 && i < len(m.nav) && m.nav[i].header != "" {
			i += step
		}
		if i < 0 || i >= len(m.nav) {
			break
		}
		m.navCur = i
	}
	if m.navCur != prev {
		m.find = ""
		m.detailOpen = false
		m.buildRows(true)
	}
}
