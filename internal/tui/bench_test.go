package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// The benchmarks measure the work of one screen draw. Bubble Tea draws the screen after
// each message, so this work must stay small, also on old computers.
// Run them with: go test ./internal/tui -run '^$' -bench . -benchmem

// BenchmarkNoteEditor draws the inline editor with a long note (400 lines) after a key press.
func BenchmarkNoteEditor(b *testing.B) {
	m := notesModel(b)
	var sb strings.Builder
	for range 50 {
		sb.WriteString("## Heading\n\nSome **bold** and *it* with `code` and [a link](https://example.com/x) here.\n- [ ] item one\n- bullet two\n> a quote line\n```go\nfunc x() {}\n```\n")
	}
	m.taskByID("t1").Description = sb.String()
	m.buildRows(false)
	m = send(b, m, key("E"))
	r := m.sideRect()
	b.ResetTimer()
	for range b.N {
		m.note.col = (m.note.col + 1) % 3 // a cursor move, as after a key press
		_ = m.noteLines(r.w-2, r.h-2)
	}
}

// BenchmarkListView draws the whole screen with a project of 300 tasks.
func BenchmarkListView(b *testing.B) {
	m := testModel(b)
	for i := range 300 {
		t := m.st.Tasks[0]
		t.ID, t.ChildOrder = fmt.Sprintf("x%d", i), 10+i
		m.st.Tasks = append(m.st.Tasks, t)
	}
	m.applyState(m.st)
	m = send(b, m, click(5, 8))
	b.ResetTimer()
	for range b.N {
		_ = m.View()
	}
}

// bigState is an account with 40 projects of 3 sections, 15 labels, and n tasks. Half of
// the tasks are in the first project. Half of the tasks have a due date, every fifth has
// a markdown description.
func bigState(n int) todoist.SyncState {
	st := todoist.SyncState{Token: "test"}
	st.Projects = append(st.Projects, todoist.Project{ID: "inbox", Name: "Inbox", InboxProject: true, Color: "charcoal"})
	for p := range 40 {
		id := fmt.Sprintf("p%d", p)
		st.Projects = append(st.Projects, todoist.Project{ID: id, Name: fmt.Sprintf("Project %d", p), Color: "green", ChildOrder: p + 1})
		for s := range 3 {
			st.Sections = append(st.Sections, todoist.Section{ID: fmt.Sprintf("%ss%d", id, s), ProjectID: id, Name: fmt.Sprintf("Section %d", s), SectionOrder: s + 1})
		}
	}
	for l := range 15 {
		st.Labels = append(st.Labels, todoist.Label{ID: fmt.Sprintf("l%d", l), Name: fmt.Sprintf("label%d", l), Color: "blue", Order: l})
	}
	now := time.Now()
	for i := range n {
		proj := "p0"
		if i%2 == 1 {
			proj = fmt.Sprintf("p%d", i%40)
		}
		t := todoist.Task{ID: fmt.Sprintf("t%d", i), ProjectID: proj, Priority: 1 + i%4, ChildOrder: i,
			Content: fmt.Sprintf("Task number %d with a [link](https://example.com/%d) and **bold** words in it", i, i),
			Labels:  []string{fmt.Sprintf("label%d", i%15)}}
		if i%3 == 0 {
			sec := fmt.Sprintf("%ss%d", proj, i%3)
			t.SectionID = &sec
		}
		if i%2 == 0 {
			t.Due = &todoist.Due{Date: now.AddDate(0, 0, i%30-5).Format("2006-01-02"), String: "x"}
		}
		if i%5 == 0 {
			t.Description = "## Goal\nShip the **notes** before the review.\n\n- [x] one\n- [ ] two\n\n```go\nfunc f() int { return 1 }\n```\n\n| a | b |\n|---|---|\n| 1 | 2 |"
		}
		st.Tasks = append(st.Tasks, t)
	}
	return st
}

// bigModel shows the first project of bigState(n) in a screen of 160x45 cells.
func bigModel(b testing.TB, n int) Model {
	m := testModel(b)
	m.width, m.height = 160, 45
	m.applyState(bigState(n))
	m.selectNav(func(n navItem) bool { return n.kind == vkProject && n.projectID == "p0" })
	m.buildRows(true)
	m.focus = paneTasks
	return m
}

// wheelAt is a wheel event over the task list.
func wheelAt(m Model, down bool) tea.Msg {
	btn := tea.MouseWheelDown
	if !down {
		btn = tea.MouseWheelUp
	}
	r := m.listRect()
	return tea.MouseWheelMsg{X: r.x + 5, Y: r.y + 3, Button: btn}
}

// BenchmarkWheelFrame handles one wheel event in a project of 1000 tasks (an account of
// 2000) and draws the screen. The scroll goes down 300 events, then up.
func BenchmarkWheelFrame(b *testing.B) {
	m := bigModel(b, 2000)
	down := true
	b.ResetTimer()
	for i := range b.N {
		if i%300 == 0 {
			down = !down
		}
		nm, _ := m.Update(wheelAt(m, down))
		m = nm.(Model)
		_ = m.View()
	}
}

// BenchmarkAllTasksRows makes the rows of All tasks in an account of 2000 tasks. This runs
// at each view change and each sync.
func BenchmarkAllTasksRows(b *testing.B) {
	m := bigModel(b, 2000)
	m.selectNav(func(n navItem) bool { return n.kind == vkAll })
	b.ResetTimer()
	for range b.N {
		m.buildRows(false)
	}
}
