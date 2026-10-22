package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/biomassa/godoist/internal/todoist"
)

// x on a one-time task starts a flight to Completed. The row stays, struck through, until
// the flight ends. Then the Completed item lights up, and the animation stops.
func TestCompleteFlight(t *testing.T) {
	m := onRow(t, 1) // alpha, a one-time task
	m.animations = true
	m = send(t, m, key("x"))
	fx := m.fx
	if fx == nil || fx.label != "✓ alpha" || fx.glow < 0 || m.nav[fx.glow].kind != vkCompleted || len(fx.ghosts) != 1 {
		t.Fatalf("fx = %+v", fx)
	}
	if !strings.Contains(rowNames(m), "alpha") {
		t.Fatalf("the row went before the flight ended: %q", rowNames(m))
	}
	if l := m.fxLayer(); l == nil {
		t.Error("no flying label")
	}
	fx.start = time.Now().Add(-fxFlight - fxGlow/2) // the flight is over
	m = send(t, m, fxTickMsg{fx.gen})
	if strings.Contains(rowNames(m), "alpha") || !m.fxGlowing(fx.glow) || m.fx == nil {
		t.Fatalf("after the flight: rows = %q glow = %v", rowNames(m), m.fxGlowing(fx.glow))
	}
	m.fx.start = time.Now().Add(-fxFlight - fxGlow)
	m = send(t, m, fxTickMsg{fx.gen})
	if m.fx != nil {
		t.Error("the animation did not stop")
	}
}

// A key during the flight removes the row at once, and the label flies on.
func TestFlightKeyDropsRow(t *testing.T) {
	m := onRow(t, 1)
	m.animations = true
	m = send(t, m, key("x"), key("j"))
	if strings.Contains(rowNames(m), "alpha") || m.fx == nil || m.fx.label == "" {
		t.Errorf("rows = %q fx = %+v", rowNames(m), m.fx)
	}
}

// A recurring task blinks in place and does not fly. With animations off, nothing runs.
func TestRecurringBlinksAndOff(t *testing.T) {
	m := onRow(t, 2) // beta, every day
	m.animations = true
	m = send(t, m, key("x"))
	if m.fx == nil || m.fx.label != "" || len(m.fx.blinks) != 1 {
		t.Fatalf("fx = %+v", m.fx)
	}
	line := m.fxRow("t2", "beta", 10)
	if line != "beta" && strings.TrimSpace(line) != "" {
		t.Errorf("blink row = %q", line)
	}
	m = send(t, onRow(t, 1), key("x")) // animations off
	if m.fx != nil || strings.Contains(rowNames(m), "alpha") {
		t.Errorf("off: fx = %v rows = %q", m.fx != nil, rowNames(m))
	}
}

// x in the Completed view flies the task from its row to its project, and the row stays
// until the flight ends. ctrl+z flies from the Completed item to the project.
func TestReopenFlight(t *testing.T) {
	m := send(t, testModel(t), click(5, 14)) // Completed
	m.animations = true
	m = send(t, m, completedMsg{tasks: []todoist.Task{{ID: "c1", ProjectID: "p1", Content: "old", CompletedAt: "2026-09-20T10:00:00Z"}}})
	m.focus, m.rowCur = paneTasks, len(m.rows)-1
	m = send(t, m, key("x"))
	fx := m.fx
	if fx == nil || fx.label != "○ old" || !fx.reopen || fx.glow < 0 || m.nav[fx.glow].projectID != "p1" {
		t.Fatalf("fx = %+v", fx)
	}
	if len(m.completed) != 1 {
		t.Fatal("the row went before the flight ended")
	}
	fx.start = time.Now().Add(-fxFlight)
	m = send(t, m, fxTickMsg{fx.gen})
	if len(m.completed) != 0 {
		t.Error("the row stayed after the flight")
	}

	m = onRow(t, 1)
	m.animations = true
	m = send(t, m, key("x"), key("j"), key("ctrl+z"))
	from, _ := m.navCell(func(n navItem) bool { return n.kind == vkCompleted })
	if fx := m.fx; fx == nil || fx.label != "○ alpha" || fx.from != from || m.nav[fx.glow].projectID != "p1" {
		t.Errorf("undo fx = %+v", m.fx)
	}
}
