package tui

import (
	"fmt"
	"strings"
	"testing"
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
