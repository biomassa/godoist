package tui

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The border between the task list and the details pane can move: { and } move it by
// paneStep columns, and a mouse drag on the border moves it too. The width persists as the
// share of the task list in the space after the sidebar, so that it fits any terminal width.

const (
	paneStep = 2  // columns for one { or }
	minPaneW = 30 // the smallest width of the task list and of the details pane
)

// shareSaveDelay is the pause after the last move before the width is saved. Holding a key
// or dragging does not write the config file for each step.
const shareSaveDelay = 700 * time.Millisecond

type (
	shareSaveMsg  struct{ gen int }
	shareSavedMsg struct{ err error }
)

// detailWidth is the width of the details pane, borders included. It is 0 on narrow
// terminals. The task list gets its share of the space after the sidebar (half by default).
func (m Model) detailWidth() int {
	if !m.wide() {
		return 0
	}
	avail := m.width - m.navWidth()
	share := m.listShare
	if share <= 0 || share >= 1 {
		share = 0.5
	}
	mid := int(math.Round(float64(avail) * share))
	mid = max(minPaneW, min(mid, avail-minPaneW))
	return avail - mid
}

// setListWidth gives the task list the width w (borders included) and starts the pause
// before the save.
func (m *Model) setListWidth(w int) tea.Cmd {
	avail := m.width - m.navWidth()
	w = max(minPaneW, min(w, avail-minPaneW))
	m.listShare = float64(w) / float64(avail)
	m.sizeDialog()
	m.shareGen++
	gen := m.shareGen
	return tea.Tick(shareSaveDelay, func(time.Time) tea.Msg { return shareSaveMsg{gen} })
}

// moveBorder moves the border between the task list and the details pane by d columns.
func (m Model) moveBorder(d int) (tea.Model, tea.Cmd) {
	if !m.wide() {
		m.setStatus("the details pane is next to the task list only on a wide terminal", false)
		return m, nil
	}
	next := m.setListWidth(m.layout().mid.w + d)
	return m, next
}

// shareSave saves the width if no move came during the pause.
func (m Model) shareSave(msg shareSaveMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.shareGen || m.saveShare == nil {
		return m, nil
	}
	save, share := m.saveShare, m.listShare
	return m, func() tea.Msg { return shareSavedMsg{err: save(share)} }
}

// onPaneBorder reports whether x, y is on the border between the task list and the
// details pane.
func (m Model) onPaneBorder(x, y int) bool {
	l := m.layout()
	return m.wide() && y > 0 && y < l.bar-1 && (x == l.detail.x-1 || x == l.detail.x)
}
