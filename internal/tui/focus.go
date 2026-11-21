package tui

// pane identifies a focusable pane.
type pane int

const (
	paneNav pane = iota
	paneTasks
	paneDetail
)

// visiblePanes lists focusable panes in tab order for the current layout.
func (m Model) visiblePanes() []pane {
	switch {
	case m.wide():
		return []pane{paneNav, paneTasks, paneDetail}
	case m.detailOpen:
		return []pane{paneNav, paneDetail}
	}
	return []pane{paneNav, paneTasks}
}

func (m *Model) cycleFocus(d int) {
	ps := m.visiblePanes()
	i := 0
	for j, p := range ps {
		if p == m.focus {
			i = j
		}
	}
	m.focus = ps[(i+d+len(ps))%len(ps)]
}

func (m *Model) focusDetail() {
	if !m.wide() {
		m.detailOpen = true
	}
	m.focus = paneDetail
}

func (m *Model) leaveDetail() {
	m.detailOpen = false
	m.focus = paneTasks
}
