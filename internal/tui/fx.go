package tui

import (
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// A completed task flies to "✓ Completed" in the sidebar. Its name flies on an arc, and
// the sidebar item lights up when it arrives. The row of the task stays, dimmed and struck
// through, until the flight ends or the next key. A recurring task does not go to
// Completed: its row blinks three times in place. A reopened task flies the other way:
// from Completed (the row in the Completed view, or the sidebar item for ctrl+z) to its
// project. The config option "animations = false" turns this off.

const (
	fxFlight = 700 * time.Millisecond // the flight of the label
	fxGlow   = 200 * time.Millisecond // the light of the Completed item after the flight
	fxBlink  = 75 * time.Millisecond  // one half of a blink of a recurring task
	fxBlinks = 3                      // the number of blinks
	fxFrame  = 33 * time.Millisecond  // the time between two frames
	fxLabelW = 20                     // the largest width of the flying label
)

// taskFx is a running completion animation.
type taskFx struct {
	gen      int
	start    time.Time
	label    string // "✓ name", "○ name", or "✓ 3 tasks", or "" for no flight
	mark     string // the first glyph of the label: only it flies at the end
	from, to [2]int // the screen cells where the flight starts and ends
	glow     int    // the sidebar item that lights up at the end, or -1
	ghosts   []string
	reopen   bool // the ghosts are rows of the Completed view
	blinks   []string
}

// fxTickMsg asks for the next frame of animation gen.
type fxTickMsg struct{ gen int }

// fxTick waits one frame.
func fxTick(gen int) tea.Cmd {
	return tea.Tick(fxFrame, func(time.Time) tea.Msg { return fxTickMsg{gen} })
}

// startFx starts the animation for completed tasks: once are one-time tasks, which fly to
// Completed, and recurring are recurring tasks, which blink. With animations off, the
// rows of the one-time tasks go at once.
func (m *Model) startFx(once, recurring []string, label string) tea.Cmd {
	m.endFx()
	if !m.animations {
		for _, id := range once {
			m.removeTask(id)
		}
		return nil
	}
	fx := m.newFx()
	fx.ghosts, fx.blinks = once, recurring
	if len(once) > 0 {
		fx.setLabel("✓", label)
		fx.from = m.cursorCell()
		fx.to, fx.glow = m.navCell(func(n navItem) bool { return n.kind == vkCompleted })
	}
	return fxTick(fx.gen)
}

// startReopenFx starts the flight of a reopened task to its project. A task of the
// Completed view flies from its row, which stays until the flight ends. A task of ctrl+z
// (row is false) flies from the Completed item.
func (m *Model) startReopenFx(id, projectID, label string, row bool) tea.Cmd {
	m.endFx()
	if !m.animations {
		if row {
			m.completed = removeByID(m.completed, id)
			m.buildRows(false)
		}
		return nil
	}
	fx := m.newFx()
	fx.setLabel("○", label)
	if row {
		fx.ghosts, fx.reopen = []string{id}, true
		fx.from = m.cursorCell()
	} else {
		fx.from, _ = m.navCell(func(n navItem) bool { return n.kind == vkCompleted })
	}
	fx.to, fx.glow = m.navCell(func(n navItem) bool { return n.kind == vkProject && n.projectID == projectID })
	return fxTick(fx.gen)
}

// newFx makes a new animation that starts now.
func (m *Model) newFx() *taskFx {
	m.fxGen++
	m.fx = &taskFx{gen: m.fxGen, start: time.Now(), glow: -1}
	return m.fx
}

// setLabel sets the flying text: the mark and the name, cut to fxLabelW cells.
func (fx *taskFx) setLabel(mark, name string) {
	fx.mark = mark
	fx.label = mark + " " + trunc(plain(name), fxLabelW-2)
}

// cursorCell is the screen cell of the start of the cursor row of the task list.
func (m Model) cursorCell() [2]int {
	l := m.layout()
	return [2]int{l.mid.x + 3, l.mid.y + 1 + max(0, min(m.rowCur-m.rowOff, m.paneHeight()-1))}
}

// navCell returns the screen cell of the glyph of the last sidebar item that is, and its
// index. An item that is scrolled away gives the top or the bottom row of the sidebar in
// its direction, and the index -1: it does not light up.
func (m Model) navCell(is func(navItem) bool) (cell [2]int, index int) {
	l := m.layout()
	top, bottom := l.nav.y+1, l.nav.y+l.nav.h-2
	index = -1
	for i, n := range m.nav {
		if n.header == "" && is(n) {
			index = i
		}
	}
	switch k := index - m.navOff; {
	case index < 0 || k >= m.paneHeight():
		return [2]int{l.nav.x + 2, bottom}, -1
	case k < 0:
		return [2]int{l.nav.x + 2, top}, -1
	default:
		return [2]int{l.nav.x + 2, top + k}, index
	}
}

// fxEnd is the time the animation takes.
func (fx *taskFx) fxEnd() time.Duration {
	var d time.Duration
	if fx.label != "" {
		d = fxFlight
		if fx.glow >= 0 {
			d += fxGlow
		}
	}
	if len(fx.blinks) > 0 {
		d = max(d, 2*fxBlinks*fxBlink)
	}
	return d
}

// fxFrameMsg draws the next frame. The rows of the one-time tasks go when the flight ends.
func (m Model) fxFrameMsg(msg fxTickMsg) (tea.Model, tea.Cmd) {
	fx := m.fx
	if fx == nil || msg.gen != fx.gen {
		return m, nil
	}
	t := time.Since(fx.start)
	if t >= fxFlight {
		m.dropGhosts()
	}
	if t >= fx.fxEnd() {
		m.fx = nil
		return m, nil
	}
	return m, fxTick(fx.gen)
}

// dropGhosts removes the rows that stay during the flight.
func (m *Model) dropGhosts() {
	if m.fx == nil || len(m.fx.ghosts) == 0 {
		return
	}
	ids := m.fx.ghosts
	m.fx.ghosts = nil
	for _, id := range ids {
		if m.fx.reopen {
			m.completed = removeByID(m.completed, id)
			m.buildRows(false)
		} else {
			m.removeTask(id)
		}
	}
}

// endFx stops the animation at once.
func (m *Model) endFx() {
	m.dropGhosts()
	m.fx = nil
}

// fxRow changes a task list row while the animation runs: a task that flies is dimmed
// and struck through, and a recurring task blinks.
func (m Model) fxRow(id, line string, w int) string {
	fx := m.fx
	if fx == nil {
		return line
	}
	for _, g := range fx.ghosts {
		if g == id {
			return st(hexDim).Strikethrough(true).Render(ansi.Strip(line))
		}
	}
	for _, b := range fx.blinks {
		if b == id && int(time.Since(fx.start)/fxBlink)%2 == 0 {
			return strings.Repeat(" ", w)
		}
	}
	return line
}

// fxGlowing reports whether sidebar item i lights up now.
func (m Model) fxGlowing(i int) bool {
	fx := m.fx
	if fx == nil || fx.glow < 0 || fx.glow != i {
		return false
	}
	t := time.Since(fx.start)
	return t >= fxFlight && t < fxFlight+fxGlow
}

// fxLayer is the flying label at its place now, or nil. The label goes on an arc above
// the line from the start to the end, slow at both ends. In the last quarter, only the
// mark flies.
func (m Model) fxLayer() *lipgloss.Layer {
	fx := m.fx
	if fx == nil || fx.label == "" {
		return nil
	}
	p := float64(time.Since(fx.start)) / float64(fxFlight)
	if p >= 1 {
		return nil
	}
	u := (1 - math.Cos(math.Pi*p)) / 2 // ease in and out
	x0, y0 := float64(fx.from[0]), float64(fx.from[1])
	x1, y1 := float64(fx.to[0]), float64(fx.to[1])
	cx, cy := (x0+x1)/2, math.Max(0, math.Min(y0, y1)-6) // the top of the arc
	x := (1-u)*(1-u)*x0 + 2*(1-u)*u*cx + u*u*x1
	y := (1-u)*(1-u)*y0 + 2*(1-u)*u*cy + u*u*y1
	text := fx.label
	if u > 0.75 {
		text = fx.mark
	}
	label := st(fg(hexAccent)).Bold(true).Render(text)
	lx := max(0, min(int(math.Round(x)), m.width-lipgloss.Width(label)))
	return lipgloss.NewLayer(label).X(lx).Y(max(0, int(math.Round(y)))).Z(3)
}
