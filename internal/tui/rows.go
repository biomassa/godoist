package tui

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/biomassa/godoist/internal/todoist"
)

// row is a line in the task pane: a group header, a task, or a note's preview line.
type row struct {
	header    string
	headerHex string
	count     int
	sectionID string // for headers in project view
	inGroup   bool   // a section header under a project header (All tasks, labels): indented
	date      string // YYYY-MM-DD for day headers in Today/Upcoming
	task      *todoist.Task
	depth     int
	preview   string // non-empty: preview line under a note (never selected on its own)
	spacer    bool   // an empty line before a header (never selected)
	hasKids   bool   // a task with sub-tasks, or a section header with tasks
	collapsed bool   // its sub-tasks or its tasks are hidden
	hidden    int    // the number of hidden tasks when collapsed
	done      bool   // a completed task (Completed view, or doneSub)
	doneSub   bool   // a completed sub-task under its open task: struck through
	subDone   int    // the completed tasks below this task
	subAll    int    // all tasks below this task, with the completed ones
	upName    string // Completed view: the task above a sub-task that shows at the top
	kids      bool   // Completed view: the task has sub-tasks under it
	pulled    bool   // an overview parent that is only there because of its sub-tasks
	tree      string // notebook view: the tree lines before a sub-note or its preview
}

// buildRows makes the task list rows for the selected sidebar item.
// If reset is true, the cursor goes to the first task. If not, the cursor stays on the current task.
func (m *Model) buildRows(reset bool) {
	var keepID string
	if t := m.currentTask(); t != nil && !reset {
		keepID = t.ID
	}
	prevCur := m.rowCur

	m.rows = nil
	cur := m.currentNav()
	if cur == nil {
		return
	}
	now := time.Now()
	switch cur.kind {
	case vkProject:
		m.rows = m.projectRows(cur.projectID, m.notesMode())
	case vkToday:
		m.rows = m.todayRows(now)
	case vkUpcoming:
		m.rows = m.upcomingRows(now)
	case vkFilter:
		m.rows = m.filterRows()
	case vkAll:
		m.rows = m.allRows()
	case vkLabel:
		m.rows = m.labelRows(cur.name)
	case vkCompleted:
		m.rows = m.completedRows()
	}

	if m.find != "" {
		m.rows = applyFind(m.rows, m.find)
	}
	m.rows = addSpacers(m.rows)
	m.rows = append([]row{{spacer: true}}, m.rows...) // an empty line under the view title

	m.rowCur = -1
	if m.jumpTo != nil && *m.jumpTo != "" { // a new or an opened sub-note: the cursor goes to it
		for _, r := range m.rows {
			if r.task != nil && r.preview == "" && r.task.ID == *m.jumpTo {
				keepID, m.jumpTo = *m.jumpTo, nil
				break
			}
		}
	}
	if keepID != "" {
		for i, r := range m.rows {
			if r.task != nil && r.preview == "" && r.task.ID == keepID {
				m.rowCur = i
			}
		}
	}
	if m.rowCur < 0 {
		m.rowCur = 0
		if !reset {
			m.rowCur = max(0, min(prevCur, len(m.rows)-1))
		}
		if len(m.rows) > 0 && m.rows[m.rowCur].task == nil {
			for i := m.rowCur; i < len(m.rows); i++ {
				if m.rows[i].task != nil {
					m.rowCur = i
					break
				}
			}
		}
		if len(m.rows) > 0 && m.rows[m.rowCur].preview != "" {
			m.rowCur--
		}
	}
	if reset {
		m.rowOff = 0
	}
}

// projectRows makes the rows of a project: tasks without a section first, then each section.
// In notebook view, a preview row follows each note.
func (m *Model) projectRows(projectID string, notes bool) []row {
	var secs []*todoist.Section
	for i := range m.snap.Sections {
		s := &m.snap.Sections[i]
		if s.ProjectID == projectID && !s.IsArchived {
			secs = append(secs, s)
		}
	}
	sort.SliceStable(secs, func(i, j int) bool { return secs[i].SectionOrder < secs[j].SectionOrder })
	bySection := map[string][]todoist.Task{}
	for _, t := range m.snap.Tasks {
		if t.ProjectID == projectID {
			sid := t.Section()
			if sid != "" && m.sections[sid] == nil {
				sid = ""
			}
			bySection[sid] = append(bySection[sid], t)
		}
	}
	group := func(ts []todoist.Task) []row {
		rows := treeRows(ts, m.withDoneKids, time.Now())
		if !notes {
			return rows
		}
		var out []row
		for _, r := range noteTree(rows) {
			out = append(out, r[0])
			if p := notePreview(r[0].task, m.snap.Comments[r[0].task.ID]); p != "" {
				out = append(out, row{task: r[0].task, depth: r[0].depth, preview: p, tree: r[1].tree, doneSub: r[0].doneSub})
			}
		}
		return out
	}
	rows := group(bySection[""])
	for _, s := range secs {
		ts := bySection[s.ID]
		rows = append(rows, row{header: s.Name, headerHex: fg(todoist.ColorHex(m.projects[projectID].Color)), count: len(ts),
			sectionID: s.ID, hasKids: len(ts) > 0, collapsed: s.IsCollapsed && len(ts) > 0})
		if !s.IsCollapsed {
			rows = append(rows, group(ts)...)
		}
	}
	return rows
}

// addSpacers puts an empty row before each header that is not the first row.
func addSpacers(rows []row) []row {
	out := make([]row, 0, len(rows)+8)
	for i, r := range rows {
		if r.header != "" && i > 0 {
			out = append(out, row{spacer: true})
		}
		out = append(out, r)
	}
	return out
}

// allRows groups all tasks by project in sidebar order, with sub-tasks nested.
func (m *Model) allRows() []row { return m.projectGroupedRows(m.snap.Tasks) }

// labelRows groups the tasks with one label by project.
func (m *Model) labelRows(name string) []row {
	var ts []todoist.Task
	for _, t := range m.snap.Tasks {
		if slices.Contains(t.Labels, name) {
			ts = append(ts, t)
		}
	}
	return m.projectGroupedRows(ts)
}

// completedRows groups the completed tasks by project, newest completion first. The
// sub-tasks of a completed task are under it. A sub-task without its task in the view
// shows the name of the task.
func (m *Model) completedRows() []row {
	top, kids := m.completedTree()
	var rows []row
	var addKids func(id string, depth int)
	addKids = func(id string, depth int) {
		for _, k := range kids[id] {
			rows = append(rows, row{task: k, depth: depth, done: true, kids: len(kids[k.ID]) > 0})
			addKids(k.ID, depth+1)
		}
	}
	head := -1
	for _, r := range m.groupByProject(top, true) {
		if r.task == nil {
			rows = append(rows, r)
			head = len(rows) - 1
			continue
		}
		r.done, r.kids = true, len(kids[r.task.ID]) > 0
		if p := r.task.Parent(); p != "" {
			r.upName = m.parentName(p)
		}
		rows = append(rows, r)
		n := len(rows)
		addKids(r.task.ID, 1)
		if head >= 0 {
			rows[head].count += len(rows) - n
		}
	}
	return rows
}

// groupByProject groups tasks by project in sidebar order. Inside a group, tasks with a
// due date come first, sorted by the date, and tasks without a date follow in project
// order. If byCompletion is true, the newest completion comes first instead.
func (m *Model) groupByProject(tasks []todoist.Task, byCompletion bool) []row {
	byProject := map[string][]todoist.Task{}
	for _, t := range tasks {
		byProject[t.ProjectID] = append(byProject[t.ProjectID], t)
	}
	var rows []row
	for _, op := range m.orderedProjects() {
		ts := byProject[op.p.ID]
		if len(ts) == 0 {
			continue
		}
		sort.SliceStable(ts, func(i, j int) bool {
			if byCompletion {
				return ts[i].CompletedAt > ts[j].CompletedAt
			}
			ai, aj := ts[i].Due != nil, ts[j].Due != nil
			if ai != aj {
				return ai
			}
			if !ai {
				return ts[i].ChildOrder < ts[j].ChildOrder
			}
			a, _, _ := ts[i].Due.Time()
			b, _, _ := ts[j].Due.Time()
			if !a.Equal(b) {
				return a.Before(b)
			}
			return ts[i].Priority > ts[j].Priority
		})
		glyph := "# "
		if op.p.InboxProject {
			glyph = "⌂ "
		}
		rows = append(rows, row{header: glyph + op.p.Name, headerHex: fg(todoist.ColorHex(op.p.Color)), count: len(ts)})
		rows = append(rows, flatRows(ts)...)
	}
	return rows
}

// notePreview is the first line of a note's body, or of its first comment.
func notePreview(t *todoist.Task, comments []todoist.Comment) string {
	if line := firstLine(t.Description); line != "" {
		return line
	}
	if len(comments) > 0 {
		return fmt.Sprintf("✎%d %s", len(comments), firstLine(comments[0].Content))
	}
	return ""
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(plain(strings.TrimLeft(strings.TrimSpace(l), "#>*-+ ")))
		if l != "" {
			return l
		}
	}
	return ""
}

// noteTree gives the rows of a notebook their tree lines: "├ " or "└ " before a sub-note,
// and "│ " for each level above it that has more notes below. Each item is the note row
// and a row with the tree lines for its preview line.
func noteTree(rows []row) [][2]row {
	out := make([][2]row, len(rows))
	var more []bool // more[l]: the note at level l on the current path has a sibling below
	for i, r := range rows {
		last := true
		for _, x := range rows[i+1:] {
			if x.depth < r.depth {
				break
			}
			if x.depth == r.depth {
				last = false
				break
			}
		}
		more = append(more[:min(len(more), r.depth)], !last)
		var b strings.Builder
		for l := 1; l < r.depth; l++ {
			b.WriteString(map[bool]string{true: "│ ", false: "  "}[more[l]])
		}
		lead := b.String()
		if r.depth > 0 {
			r.tree = lead + map[bool]string{true: "└ ", false: "├ "}[last]
			lead += map[bool]string{true: "│ ", false: "  "}[!last]
		}
		out[i] = [2]row{r, {tree: lead}}
	}
	return out
}

// treeRows orders tasks by child_order with subtasks nested under their parents.
// A collapsed task shows no sub-tasks. Its row has the number of hidden tasks.
func treeRows(ts []todoist.Task, addDone func(map[string][]*todoist.Task, []string), now time.Time) []row {
	ids := map[string]bool{}
	var all []string
	for _, t := range ts {
		ids[t.ID] = true
		all = append(all, t.ID)
	}
	children := map[string][]*todoist.Task{}
	for i := range ts {
		parent := ts[i].Parent()
		if !ids[parent] {
			parent = ""
		}
		children[parent] = append(children[parent], &ts[i])
	}
	if addDone != nil {
		addDone(children, all)
	}
	var out []row
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		kids := children[parent]
		if depth == 0 {
			sortByDate(kids, now)
		} else {
			sort.SliceStable(kids, func(i, j int) bool { return kids[i].ChildOrder < kids[j].ChildOrder })
		}
		for _, t := range kids {
			r := row{task: t, depth: depth, hasKids: len(children[t.ID]) > 0, collapsed: t.IsCollapsed,
				done: t.Checked, doneSub: t.Checked}
			if r.hasKids {
				r.subAll, r.subDone = countTree(children, t.ID), countDone(children, t.ID)
			}
			if r.hasKids && t.IsCollapsed {
				r.hidden = r.subAll
				out = append(out, r)
				continue
			}
			out = append(out, r)
			walk(t.ID, depth+1)
		}
	}
	walk("", 0)
	return out
}

// sortByDate orders the top-level tasks of a project view by their due dates. A task with
// a date comes before a task without one. Tasks due today or earlier come first, the
// closest to now first: today, then yesterday, then older dates. Future tasks follow, the
// nearest first. Tasks on the same day go by time. If the dates do not decide the order,
// the manual order does.
func sortByDate(ts []*todoist.Task, now time.Time) {
	type dated struct {
		t *todoist.Task
		k nearKey
	}
	today := dayOf(now)
	ds := make([]dated, len(ts))
	for i, t := range ts {
		when, _, ok := t.Due.Time()
		ds[i] = dated{t, newNearKey(when, ok, today)}
	}
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i].k, ds[j].k
		switch {
		case a.ok != b.ok:
			return a.ok
		case a.ok && !a.when.Equal(b.when):
			return a.before(b)
		}
		return ds[i].t.ChildOrder < ds[j].t.ChildOrder
	})
	for i := range ds {
		ts[i] = ds[i].t
	}
}

// nearKey is a due time for a sort that shows the dates closest to now first. A sort finds
// the keys once before it starts: to find the day in each comparison took most of the
// sort time.
type nearKey struct {
	when, day time.Time
	ok        bool // there is a due time
	past      bool // the day is today or earlier
}

// newNearKey makes the key of due time when. ok is false if there is no due time.
func newNearKey(when time.Time, ok bool, today time.Time) nearKey {
	k := nearKey{when: when, ok: ok}
	if ok {
		k.day = dayOf(when)
		k.past = !k.day.After(today)
	}
	return k
}

// before reports whether k comes before o in a list that shows the dates closest to now
// first: today, then the earlier days from the latest, then the future days from the
// nearest. On the same day, the earlier time comes first. Both keys must have a due time.
func (k nearKey) before(o nearKey) bool {
	switch {
	case k.past != o.past:
		return k.past
	case k.past && !k.day.Equal(o.day): // today and earlier: the later day first
		return k.day.After(o.day)
	}
	return k.when.Before(o.when)
}

// countTree counts all tasks below id.
func countTree(children map[string][]*todoist.Task, id string) int {
	n := 0
	for _, t := range children[id] {
		n += 1 + countTree(children, t.ID)
	}
	return n
}

func flatRows(ts []todoist.Task) []row {
	out := make([]row, len(ts))
	for i := range ts {
		out[i] = row{task: &ts[i]}
	}
	return out
}

// applyFind keeps tasks matching q (case-insensitive) and headers that still have tasks.
func applyFind(rows []row, q string) []row {
	q = strings.ToLower(q)
	var out []row
	for _, r := range rows {
		if r.task == nil {
			out = append(out, r)
			continue
		}
		hay := strings.ToLower(r.task.Content + " " + r.task.Description + " " + strings.Join(r.task.Labels, " "))
		if strings.Contains(hay, q) {
			r.depth = 0
			out = append(out, r)
		}
	}
	var pruned []row
	for i, r := range out {
		if r.task == nil && (i+1 >= len(out) || out[i+1].task == nil) {
			continue
		}
		pruned = append(pruned, r)
	}
	return pruned
}

func (m *Model) currentRow() *row {
	if m.rowCur < 0 || m.rowCur >= len(m.rows) {
		return nil
	}
	return &m.rows[m.rowCur]
}

func (m *Model) currentTask() *todoist.Task {
	if r := m.currentRow(); r != nil {
		return r.task
	}
	return nil
}

func (m Model) currentTaskID() string {
	if t := m.currentTask(); t != nil {
		return t.ID
	}
	return ""
}

// cursorSection is the section of the row under the cursor (project view).
func (m *Model) cursorSection() string {
	for i := m.rowCur; i >= 0 && i < len(m.rows); i-- {
		if m.rows[i].header != "" {
			return m.rows[i].sectionID
		}
	}
	return ""
}

// cursorDate is the day group under the cursor (Today/Upcoming).
func (m *Model) cursorDate() string {
	for i := m.rowCur; i >= 0 && i < len(m.rows); i-- {
		if m.rows[i].date != "" {
			return m.rows[i].date
		}
	}
	return ""
}

// moveRow moves the cursor by d rows. It skips note preview lines and spacers.
func (m *Model) moveRow(d int) {
	if len(m.rows) == 0 {
		return
	}
	step := 1
	if d < 0 {
		step, d = -1, -d
	}
	cur := m.rowCur
	for ; d > 0; d-- {
		i := cur + step
		for i >= 0 && i < len(m.rows) && (m.rows[i].preview != "" || m.rows[i].spacer) {
			i += step
		}
		if i < 0 || i >= len(m.rows) {
			break
		}
		cur = i
	}
	m.rowCur = cur
}
