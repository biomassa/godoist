package tui

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/biomassa/godoist/internal/todoist"
)

// TestWheelEventsShareFrame checks that wheel events less than one frame apart move the
// cursor, give the last frame again, and get one new frame from the frameMsg.
func TestWheelEventsShareFrame(t *testing.T) {
	m := bigModel(t, 200)
	ref := m // ref draws a frame for each event
	ref.fc = &frameCache{noHold: true}
	m.fc.noHold = false
	first := m.View().Content
	m.fc.built = time.Now().Add(-frameTime) // the last frame is one frame old
	m = send(t, m, wheelAt(m, true))        // so the event gets a new frame at once
	ref = send(t, ref, wheelAt(ref, true))
	second := m.View().Content
	if second == first || second != ref.View().Content {
		t.Fatal("the first event must give its frame at once")
	}
	m.fc.built = time.Now() // the next events come less than one frame after the frame: held
	nm, cmd := m.Update(wheelAt(m, true))
	m = send(t, nm.(Model), wheelAt(m, true))
	ref = send(t, ref, wheelAt(ref, true), wheelAt(ref, true))
	if cmd == nil {
		t.Fatal("a held event must ask for a frame")
	}
	if m.rowCur != ref.rowCur || m.rowCur == 0 {
		t.Fatalf("held events: cursor %d, want %d", m.rowCur, ref.rowCur)
	}
	if got := m.View().Content; got != second {
		t.Fatal("a held event must give the last frame")
	}
	m = send(t, m, frameMsg{})
	if got := m.View().Content; got != ref.View().Content {
		t.Fatal("the frameMsg must give the frame of all held events")
	}
}

// TestRowCacheFollowsInPlaceEdits checks that a cached row changes when its task changes in
// place, as after the 1–4 keys, and that the cache gives the same screen as no cache.
func TestRowCacheFollowsInPlaceEdits(t *testing.T) {
	m := bigModel(t, 200)
	before := m.View().Content
	tk := m.currentTask()
	tk.Priority = 5 - tk.UIPriority()%4 - 1 // another priority, in place
	tk.Labels = append(tk.Labels, "new")
	cached := m.View().Content
	if cached == before {
		t.Fatal("the row did not change after an edit in place")
	}
	m.fc = nil
	if fresh := m.View().Content; fresh != cached {
		t.Fatal("the cached screen differs from the screen without the cache")
	}
}

// renderRowCells is the reference of renderRow: one Render call for each cell, as before
// renderRow joined the runs of cells with the same style.
func renderRowCells(r edRow, tw, cx int) string {
	if r.pre != "" || r.cells == nil && !r.source && r.fill == 0 {
		return r.pre
	}
	lead := " "
	switch {
	case r.indent() == 0:
	case r.fill > 0:
		lead += headingStyle(r.fill).Render(" ")
	default:
		lead += " "
	}
	var b strings.Builder
	for k := 0; k < len(r.cells); k++ {
		var st lipgloss.Style
		if r.cells[k].st != nil {
			st = *r.cells[k].st
		}
		if k == cx {
			st = st.Reverse(true)
		}
		text := []rune{r.cells[k].r}
		for k+1 < len(r.cells) && joins(r.cells[k+1].r) {
			k++
			text = append(text, r.cells[k].r)
		}
		b.WriteString(st.Render(string(text)))
	}
	if cx >= 0 && cx >= len(r.cells) {
		b.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
	}
	if r.fill > 0 {
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

// sameCells fails if a and b differ on the screen in a cell: in text, style, or link.
func sameCells(t *testing.T, name, a, b string, w int) {
	t.Helper()
	sa, sb := uv.NewScreenBuffer(w, 1), uv.NewScreenBuffer(w, 1)
	uv.NewStyledString(a).Draw(sa, uv.Rect(0, 0, w, 1))
	uv.NewStyledString(b).Draw(sb, uv.Rect(0, 0, w, 1))
	for x := range w {
		ca, cb := sa.CellAt(x, 0), sb.CellAt(x, 0)
		if (ca == nil) != (cb == nil) || ca != nil && (ca.Content != cb.Content || !ca.Style.Equal(&cb.Style) || ca.Link != cb.Link) {
			t.Fatalf("%s: cell %d differs\nruns:  %q\ncells: %q", name, x, a, b)
		}
	}
}

// TestRenderRowRunsSameScreen checks that an editor row drawn with one Render call for
// each run of cells looks the same on the screen as a row drawn with one call for each
// cell: rendered lines, source lines with a cursor, find matches, and a selection.
func TestRenderRowRunsSameScreen(t *testing.T) {
	doc := strings.Join([]string{
		"# Heading **bold** `code`",
		"## Second level",
		"Text with **bold**, *italic*, ~~strike~~, `code`, and a [link](https://example.com).",
		"- [ ] a task with `code` and ✔️ an emoji with a variation selector",
		"1. a numbered item with é a combining mark",
		"> a quote with *style*",
		"---",
		"```go",
		"func f() int { return 1 } // comment",
		"```",
		"| a | b |",
		"|---|---|",
		"| **x** | `y` |",
	}, "\n")
	const tw = 60
	md := newMDCache()
	e := &noteEditor{lines: splitRunes(doc), want: -1}
	for i := range e.lines {
		for _, r := range md.previewRows(string(e.lines[i]), tw) {
			sameCells(t, "preview "+string(e.lines[i]), renderRow(r, tw, -1), renderRowCells(r, tw, -1), tw+2)
		}
	}
	e.search = &noteFind{matches: []noteMatch{{row: 2, a: 5, b: 9}, {row: 2, a: 20, b: 26}}, cur: 1}
	e.selOn, e.selRow, e.selCol = true, 3, 2
	e.row, e.col = 3, 12
	for i := range e.lines {
		for _, r := range e.sourceRows(i, false, tw) {
			for _, cx := range []int{-1, 0, 4, len(r.cells)} {
				sameCells(t, "source "+string(e.lines[i]), renderRow(r, tw, cx), renderRowCells(r, tw, cx), tw+2)
			}
		}
	}
}

// nearBefore is the reference order of nearKey.before: the comparison that found the days
// in each comparison.
func nearBefore(a, b, now time.Time) bool {
	da, db, today := dayOf(a), dayOf(b), dayOf(now)
	pastA, pastB := !da.After(today), !db.After(today)
	switch {
	case pastA != pastB:
		return pastA
	case !da.Equal(db) && pastA: // today and earlier: the later day first
		return da.After(db)
	}
	return a.Before(b)
}

// TestSortByDateOrder checks that the project view gives the order of the reference sort,
// which parsed the due dates in each comparison.
func TestSortByDateOrder(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.Local)
	rng := rand.New(rand.NewPCG(3, 4))
	ts := make([]todoist.Task, 300)
	for i := range ts {
		ts[i] = todoist.Task{ID: fmt.Sprintf("t%d", i), ChildOrder: rng.IntN(50)}
		d := now.AddDate(0, 0, rng.IntN(20)-10)
		switch rng.IntN(3) {
		case 0: // no date
		case 1:
			ts[i].Due = &todoist.Due{Date: d.Format("2006-01-02")}
		default:
			ts[i].Due = &todoist.Due{Date: d.Format("2006-01-02") + fmt.Sprintf("T%02d:%02d:00", rng.IntN(24), rng.IntN(4)*15)}
		}
	}
	want := make([]*todoist.Task, len(ts))
	for i := range ts {
		want[i] = &ts[i]
	}
	sort.SliceStable(want, func(i, j int) bool {
		a, b := want[i], want[j]
		ta, _, okA := a.Due.Time()
		tb, _, okB := b.Due.Time()
		switch {
		case okA != okB:
			return okA
		case okA && !ta.Equal(tb):
			return nearBefore(ta, tb, now)
		}
		return a.ChildOrder < b.ChildOrder
	})
	got := treeRows(ts, nil, now)
	for i := range got {
		if got[i].task.ID != want[i].ID {
			t.Fatalf("position %d: %s, want %s", i, got[i].task.ID, want[i].ID)
		}
	}
}

// TestSortRootsNearOrder checks that sortRootsNear gives the order of nearBefore.
func TestSortRootsNearOrder(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.Local)
	rng := rand.New(rand.NewPCG(1, 2))
	tasks := make([]todoist.Task, 300)
	rs := make([]ovRoot, len(tasks))
	for i := range tasks {
		tasks[i] = todoist.Task{ID: string(rune('a'+i%26)) + strings.Repeat("x", i/26), ChildOrder: rng.IntN(50)}
		rs[i] = ovRoot{task: &tasks[i], first: i}
		if rng.IntN(4) > 0 {
			rs[i].hasWhen = true
			rs[i].when = now.AddDate(0, 0, rng.IntN(20)-10).Add(time.Duration(rng.IntN(48)-24) * time.Hour)
		}
	}
	want := append([]ovRoot(nil), rs...)
	sortRootsRef := func(rs []ovRoot) { // the sort before the change
		for i := 1; i < len(rs); i++ {
			for j := i; j > 0; j-- {
				a, b := rs[j], rs[j-1]
				less := rootLess(a, b)
				if a.hasWhen && b.hasWhen && !a.when.Equal(b.when) {
					less = nearBefore(a.when, b.when, now)
				}
				if !less {
					break
				}
				rs[j], rs[j-1] = rs[j-1], rs[j]
			}
		}
	}
	sortRootsRef(want)
	sortRootsNear(rs, now)
	for i := range rs {
		if rs[i].task != want[i].task {
			t.Fatalf("position %d: %s, want %s", i, rs[i].task.ID, want[i].task.ID)
		}
	}
}

// TestCacheFollowsTheme checks that the cached rows of the task list and the sidebar
// change with the theme, as in the live preview of the theme picker.
func TestCacheFollowsTheme(t *testing.T) {
	m := bigModel(t, 200)
	before := m.View().Content
	applyTheme(ThemeNames()[1], true)
	defer applyTheme(todoistTheme, true)
	cached := m.View().Content
	m.fc = nil
	if fresh := m.View().Content; cached == before || fresh != cached {
		t.Fatalf("after a theme change: changed %v, same as without the cache %v", cached != before, fresh == cached)
	}
}

// sameScreen fails if screens a and b differ in a cell: in text, style, or link.
func sameScreen(t *testing.T, name, a, b string) {
	t.Helper()
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	if len(la) != len(lb) {
		t.Fatalf("%s: %d lines, want %d", name, len(la), len(lb))
	}
	for i := range la {
		sameCells(t, fmt.Sprintf("%s line %d", name, i), la[i], lb[i], 200)
	}
}

// TestCacheSameScreen checks, in many states, that the screen with the caches is the same
// as the screen without them, in each cell, and that a second frame from the caches is the
// same as the first.
func TestCacheSameScreen(t *testing.T) {
	base := func(w, h int) Model {
		m := testModel(t)
		ti := newTextInput()
		ti.CharLimit = 500
		m.input = &ti
		m.width, m.height = w, h
		m.applyState(bigState(300))
		return m
	}
	big := func() Model {
		m := base(160, 45)
		m.selectNav(func(n navItem) bool { return n.kind == vkProject && n.projectID == "p0" })
		m.buildRows(true)
		m.focus = paneTasks
		return m
	}
	nav := func(m Model, f func(navItem) bool) Model {
		m.selectNav(f)
		m.buildRows(true)
		m.focus = paneTasks
		return m
	}
	wheel := func(m Model, n int) Model {
		for range n {
			m = send(t, m, wheelAt(m, true))
		}
		return m
	}
	notes := func() Model {
		m := notesModel(t)
		ti := newTextInput()
		m.input = &ti
		m.width, m.height = 160, 45
		return m
	}
	states := []struct {
		name string
		make func() Model
	}{
		{"today", func() Model { return base(160, 45) }},
		{"today 90", func() Model { return base(90, 30) }},
		{"today 70", func() Model { return base(70, 24) }},
		{"project", big},
		{"project wheel", func() Model { return wheel(big(), 7) }},
		{"project sidebar focus", func() Model { m := big(); m.focus = paneNav; return m }},
		{"project details focus", func() Model { m := wheel(big(), 2); m.focus = paneDetail; return m }},
		{"all tasks", func() Model { return wheel(nav(base(160, 45), func(n navItem) bool { return n.kind == vkAll }), 4) }},
		{"label", func() Model { return nav(base(160, 45), func(n navItem) bool { return n.kind == vkLabel }) }},
		{"upcoming", func() Model { return nav(base(160, 45), func(n navItem) bool { return n.kind == vkUpcoming }) }},
		{"completed", func() Model {
			m := base(160, 45)
			now := time.Now()
			m.completed = []todoist.Task{
				{ID: "c1", ProjectID: "p1", Content: "done today", CompletedAt: now.Add(-time.Hour).UTC().Format(time.RFC3339)},
				{ID: "c2", ProjectID: "p2", Content: "done before", CompletedAt: now.AddDate(0, 0, -3).UTC().Format(time.RFC3339)},
			}
			return nav(m, func(n navItem) bool { return n.kind == vkCompleted })
		}},
		{"filter", func() Model {
			m := base(160, 45)
			m.filterQuery, m.filterTasks = "today", append([]todoist.Task(nil), m.snap.Tasks[:20]...)
			m.buildNav()
			return nav(m, func(n navItem) bool { return n.kind == vkFilter })
		}},
		{"help", func() Model { return send(t, big(), key("?")) }},
		{"date dialog", func() Model { return send(t, big(), key("j"), key("t")) }},
		{"task dialog", func() Model { return send(t, big(), key("a")) }},
		{"move picker", func() Model { return send(t, big(), key("j"), key("m")) }},
		{"label picker", func() Model { return send(t, big(), key("j"), key("@")) }},
		{"menu", func() Model { m := big(); return send(t, m, rightClick(m.layout().mid.x+10, 5)) }},
		{"selection", func() Model { return send(t, big(), key("j"), key("s"), key("j"), key("j"), key("s"), key("j")) }},
		{"find", func() Model { return send(t, big(), key("/"), key("1"), key("2")) }},
		{"narrow details", func() Model {
			m := nav(base(70, 24), func(n navItem) bool { return n.kind == vkProject && n.projectID == "p0" })
			return send(t, m, key("enter"))
		}},
		{"collapse", func() Model { return send(t, big(), key("j"), key("j"), key("z")) }},
		{"theme picker", func() Model { return send(t, big(), key("T")) }},
		{"notebook", notes},
		{"notebook editor", func() Model {
			m := send(t, notes(), key("E"), key("j"))
			if m.note == nil {
				t.Fatal("E did not open the editor")
			}
			return m
		}},
	}
	for _, s := range states {
		m := s.make()
		cached := m.View().Content
		if again := m.View().Content; again != cached {
			t.Errorf("%s: the second frame differs from the first", s.name)
		}
		m.fc = nil
		sameScreen(t, s.name, cached, m.View().Content)
	}
}
