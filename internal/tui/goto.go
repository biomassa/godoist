package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Jump keys, so that a view or a project is one or two keys away from anywhere:
// g is a prefix like in vim and Todoist. gg goes to the top, gt to Today, gu to
// Upcoming, gi to the Inbox, and gp opens the go-to picker. ctrl+k opens the picker too.
// A key after g that is not one of these cancels the prefix and works as usual.
// If the second key does not come in gotoDelay, a small box shows the jump keys, so a
// quick "gt" never shows it.

// gotoHint is the status while g waits for the second key.
const gotoHint = "g…"

// gotoDelay is the time after g until the box of the jump keys shows.
const gotoDelay = 500 * time.Millisecond

// gotoPopupMsg shows the box of the jump keys if g number seq still waits.
type gotoPopupMsg struct{ seq int }

// gotoItems are the jump keys that the box shows.
var gotoItems = [][2]string{{"g", "top"}, {"t", "Today"}, {"u", "Upcoming"}, {"i", "Inbox"}, {"p", "go to…"}}

// startGoto starts to wait for the key after g. The returned command shows the box of
// the jump keys after gotoDelay.
func (m *Model) startGoto() tea.Cmd {
	m.gPrefix, m.gPopup = true, false
	m.gSeq++
	seq := m.gSeq
	m.setStatus(gotoHint, false)
	return tea.Tick(gotoDelay, func(time.Time) tea.Msg { return gotoPopupMsg{seq} })
}

// gotoPopup shows the box of the jump keys if g still waits for its key. If a dialog
// opened since g, g stops waiting.
func (m *Model) gotoPopup(msg gotoPopupMsg) {
	if msg.seq != m.gSeq || !m.gPrefix {
		return
	}
	if m.keysTaken() {
		m.cancelGoto()
		return
	}
	m.gPopup = true
}

// keysTaken reports whether a dialog, a picker, or the editor gets the keys, so that
// they do not go to the jump keys.
func (m Model) keysTaken() bool {
	return m.quitting || m.themes != nil || m.cal != nil || m.menu != nil || m.pick != nil ||
		m.note != nil || m.confirm != nil || m.inputMode != inputNone
}

// gotoBox draws the box of the jump keys.
func (m Model) gotoBox() string {
	r := m.gotoRect()
	lines := make([]string, len(gotoItems))
	for i, it := range gotoItems {
		lines[i] = pad(" "+st(fg(hexAccent)).Bold(true).Render(it[0])+"  "+st(hexText).Render(it[1]), r.w-2, lipgloss.NewStyle())
	}
	return box(st(hexText).Bold(true).Render("Go to"), lines, r.w, fg(hexAccent))
}

// gotoRect is the area of the box of the jump keys: at the bottom right, inside the
// borders of the panes.
func (m Model) gotoRect() rect {
	w, h := 18, len(gotoItems)+2
	return rect{max(0, m.width-2-w), max(0, m.paneHeight()+1-h), w, h}
}

// gotoKeys handles the key after g. ok is false if the key is not a jump key: it then
// works as usual.
func (m Model) gotoKeys(key string) (tea.Model, tea.Cmd, bool) {
	m.cancelGoto()
	switch key {
	case "g": // the same as home in each pane
		nm, cmd := m.updateKeys(tea.KeyPressMsg{Code: tea.KeyHome})
		return nm, cmd, true
	case "t":
		m.gotoKind(vkToday)
	case "u":
		m.gotoKind(vkUpcoming)
	case "i":
		m.gotoInbox()
	case "p":
		next := m.openGotoPicker()
		return m, next, true
	case "esc":
	default:
		return m, nil, false
	}
	return m, nil, true
}

// cancelGoto stops waiting for the key after g.
func (m *Model) cancelGoto() {
	m.gPrefix, m.gPopup = false, false
	if m.status == gotoHint {
		m.setStatus("", false)
	}
}

// gotoNav opens the sidebar item with navKey key and puts the focus on the task list.
// Like a move in the sidebar, it clears the find text of the old view.
func (m *Model) gotoNav(key string) {
	m.pick = nil
	before := m.navCur
	m.selectNav(func(n navItem) bool { return navKey(n) == key })
	if m.navCur != before {
		m.find = ""
		m.buildRows(true)
	}
	m.detailOpen = false
	m.focus = paneTasks
}

// gotoKind opens the first sidebar item of kind k.
func (m *Model) gotoKind(k viewKind) {
	for _, n := range m.nav {
		if n.header == "" && n.kind == k {
			m.gotoNav(navKey(n))
			return
		}
	}
}

// gotoInbox opens the Inbox project.
func (m *Model) gotoInbox() {
	for _, n := range m.nav {
		if p := m.projects[n.projectID]; n.header == "" && n.kind == vkProject && p != nil && p.InboxProject {
			m.gotoNav(navKey(n))
			return
		}
	}
	m.setStatus("no Inbox project", false)
}

// openGotoPicker opens a filter list of the sidebar items: views, projects, and labels.
func (m *Model) openGotoPicker() tea.Cmd {
	var items []pickItem
	cur := 0
	seen := map[string]int{} // a favorite is in the sidebar twice: the index of its item
	for i, n := range m.nav {
		if n.header != "" || n.kind == vkLabelHint || n.name == "" {
			continue
		}
		if j, ok := seen[navKey(n)]; ok {
			if i == m.navCur {
				cur = j
			}
			continue
		}
		seen[navKey(n)] = len(items)
		if i == m.navCur {
			cur = len(items)
		}
		text := n.name
		switch n.kind {
		case vkProject:
			text = "# " + n.name
		case vkLabel:
			text = "@ " + n.name
		}
		items = append(items, pickItem{text: text, match: strings.ToLower(n.name), color: n.color, depth: n.depth, nav: navKey(n)})
	}
	m.pick = &picker{kind: pickGoto, title: "Go to", items: items, filter: newPickerFilter(), cur: cur}
	m.pick.filter.Placeholder = "type a view, project, or label"
	m.pick.filter.SetWidth(pickerFilterWidth(m.pickerWidth()))
	return m.pick.filter.Focus()
}
