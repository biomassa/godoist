package tui

import (
	"context"
	"fmt"
	"slices"
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// navLabel returns the label of the sidebar item under the cursor, or nil.
func (m Model) navLabel() *todoist.Label {
	cur := m.currentNav()
	if cur == nil || cur.kind != vkLabel {
		return nil
	}
	for i := range m.snap.Labels {
		if m.snap.Labels[i].ID == cur.labelID {
			return &m.snap.Labels[i]
		}
	}
	return nil
}

// inLabels reports whether the sidebar cursor is on a label or on the "A adds a label" line.
func (m Model) inLabels() bool {
	cur := m.currentNav()
	return cur != nil && (cur.kind == vkLabel || cur.kind == vkLabelHint)
}

// labelKeys handles the label keys in the sidebar. ok is false for other keys.
func (m Model) labelKeys(key string) (tea.Model, tea.Cmd, bool) {
	if !m.inLabels() {
		return m, nil, false
	}
	if key == "A" {
		next := m.openDialog(inputAddLabel, "")
		return m, next, true
	}
	l := m.navLabel()
	if l == nil {
		return m, nil, false
	}
	var next tea.Cmd
	switch key {
	case "e":
		m.targetLabel = l.ID
		next = m.openDialog(inputRenameLabel, l.Name)
	case "C":
		next = m.openLabelColorPicker(l)
	case "*":
		next = m.toggleLabelFavorite(l)
	case "[", "]":
		d := -1
		if key == "]" {
			d = 1
		}
		next = m.moveLabel(l, d)
	case "delete", "backspace":
		m.askDeleteLabel(l)
	default:
		return m, nil, false
	}
	return m, next, true
}

// addLabel creates a personal label.
func (m *Model) addLabel(name string) tea.Cmd {
	if name == "" {
		return nil
	}
	client := m.client
	cmd := m.simpleWrite("Added label @"+name, func(ctx context.Context) error {
		_, err := client.CreateLabel(ctx, name)
		return err
	})
	return withRetry(cmd, inputAddLabel, name)
}

// renameLabel saves a new label name. Todoist changes the label on its tasks too.
func (m *Model) renameLabel(name string) tea.Cmd {
	var l *todoist.Label
	for i := range m.snap.Labels {
		if m.snap.Labels[i].ID == m.targetLabel {
			l = &m.snap.Labels[i]
		}
	}
	if l == nil || name == "" || name == l.Name {
		return nil
	}
	id, old, client := l.ID, l.Name, m.client
	l.Name = name
	m.buildNav()
	cmd := m.simpleWrite("Renamed @"+old+" to @"+name, func(ctx context.Context) error {
		return client.UpdateLabel(ctx, id, map[string]any{"name": name})
	})
	return withRetry(cmd, inputRenameLabel, name)
}

// toggleLabelFavorite marks the label as a favorite or removes the mark.
func (m *Model) toggleLabelFavorite(l *todoist.Label) tea.Cmd {
	l.IsFavorite = !l.IsFavorite
	m.buildNav()
	id, fav, client := l.ID, l.IsFavorite, m.client
	done := "@" + l.Name + " is a favorite"
	if !fav {
		done = "@" + l.Name + " is not a favorite"
	}
	return m.simpleWrite(done, func(ctx context.Context) error {
		return client.UpdateLabel(ctx, id, map[string]any{"is_favorite": fav})
	})
}

// moveLabel moves a label d places in the label order. The sidebar changes at once.
// The new order also goes into the local replica, because an incremental sync does not
// send label orders.
func (m *Model) moveLabel(l *todoist.Label, d int) tea.Cmd {
	id, name := l.ID, l.Name // l points into m.snap.Labels, which the sort below changes
	labels := slices.Clone(m.snap.Labels)
	sort.SliceStable(labels, func(i, j int) bool { return labels[i].Order < labels[j].Order })
	order := make([]string, len(labels))
	for i, x := range labels {
		order[i] = x.ID
	}
	i := slices.Index(order, id)
	j := i + d
	if i < 0 || j < 0 || j >= len(order) {
		m.setStatus("the label is already at the edge", false)
		return nil
	}
	order[i], order[j] = order[j], order[i]
	setOrders := func(ls []todoist.Label) {
		for k := range ls {
			ls[k].Order = slices.Index(order, ls[k].ID) + 1
		}
		sort.SliceStable(ls, func(a, b int) bool { return ls[a].Order < ls[b].Order })
	}
	setOrders(m.snap.Labels)
	setOrders(m.st.Labels)
	m.buildNav()
	client := m.client
	return m.simpleWrite("Moved @"+name, func(ctx context.Context) error { return client.ReorderLabels(ctx, order) })
}

// askDeleteLabel asks, then deletes the label. Todoist removes it from its tasks.
func (m *Model) askDeleteLabel(l *todoist.Label) {
	n := 0
	for _, t := range m.snap.Tasks {
		if slices.Contains(t.Labels, l.Name) {
			n++
		}
	}
	id, name, client := l.ID, l.Name, m.client
	text := fmt.Sprintf("Delete label @%s? It is removed from %d task(s). The tasks stay.", name, n)
	m.confirm = yesNo("Delete label", text, "Delete", func(m *Model) tea.Cmd {
		return m.simpleWrite("Deleted label @"+name, func(ctx context.Context) error { return client.DeleteLabel(ctx, id) })
	})
}

// openLabelColorPicker opens the color picker for a label.
func (m *Model) openLabelColorPicker(l *todoist.Label) tea.Cmd {
	var items []pickItem
	cur := 0
	for _, name := range todoist.ColorNames {
		if name == l.Color {
			cur = len(items)
		}
		items = append(items, pickItem{text: colorLabel(name), match: colorLabel(name), color: todoist.ColorHex(name), colorName: name})
	}
	m.pick = &picker{kind: pickColor, title: "Color · @" + l.Name, items: items, filter: newPickerFilter(), cur: cur, labelID: l.ID}
	m.pick.filter.Placeholder = "type to filter, e.g. blue"
	m.pick.filter.SetWidth(pickerFilterWidth(m.pickerWidth()))
	return m.pick.filter.Focus()
}

// saveLabelColor sets the color of the picker label.
func (m *Model) saveLabelColor(id, color string) tea.Cmd {
	for i := range m.snap.Labels {
		if m.snap.Labels[i].ID == id {
			m.snap.Labels[i].Color = color
		}
	}
	m.buildNav()
	client := m.client
	return m.simpleWrite("Color → "+colorLabel(color), func(ctx context.Context) error {
		return client.UpdateLabel(ctx, id, map[string]any{"color": color})
	})
}

// labelMenu is the right-click menu of a sidebar label.
var labelMenu = []menuItem{
	{"New label", "A"}, {"Rename", "e"}, {"Color", "C"}, {"Favorite on / off", "*"},
	{"Move up", "["}, {"Move down", "]"}, {"Delete", "delete"},
}
