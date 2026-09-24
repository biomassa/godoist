package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// confirmButton is a choice in a confirmation dialog. A nil run cancels.
type confirmButton struct {
	key    string
	label  string
	danger bool
	run    func(m *Model) tea.Cmd
}

// confirmPrompt is a confirmation dialog. The arrows, tab, and h/l select a button, enter
// pushes the selected button, a button key pushes that button, and esc closes the dialog.
// The first button (the action) is selected when the dialog opens.
type confirmPrompt struct {
	title   string
	text    string
	buttons []confirmButton
	sel     int // selected button
}

// yesNo makes a dialog with a dangerous "y" button and an "n" button that cancels.
func yesNo(title, text, yesLabel string, run func(m *Model) tea.Cmd) *confirmPrompt {
	return &confirmPrompt{title: title, text: text, buttons: []confirmButton{
		{key: "y", label: yesLabel, danger: true, run: run},
		{key: "n", label: "Cancel"},
	}}
}

// answerConfirm handles a key in the confirmation dialog. Other keys do nothing.
func (m Model) answerConfirm(key string) (tea.Model, tea.Cmd) {
	p := m.confirm
	switch key {
	case "esc":
		m.confirm = nil
		m.setStatus("cancelled", false)
		return m, nil
	case "left", "h", "shift+tab":
		p.sel = (p.sel - 1 + len(p.buttons)) % len(p.buttons)
		return m, nil
	case "right", "l", "tab":
		p.sel = (p.sel + 1) % len(p.buttons)
		return m, nil
	case "enter":
		return m.pushButton(p.sel)
	}
	for i, b := range p.buttons {
		if b.key == key {
			return m.pushButton(i)
		}
	}
	return m, nil
}

// pushButton closes the dialog and runs button i. A button without run cancels.
func (m Model) pushButton(i int) (tea.Model, tea.Cmd) {
	b := m.confirm.buttons[i]
	m.confirm = nil
	if b.run == nil {
		m.setStatus("cancelled", false)
		return m, nil
	}
	next := b.run(&m)
	return m, next
}

func (m Model) confirmWidth() int { return max(30, min(60, m.width-8)) }

// confirmLayout draws the dialog and returns the button areas inside it (x and y from
// the top-left corner of the dialog).
func (m Model) confirmLayout() (string, []rect) {
	p := m.confirm
	w := m.confirmWidth()
	inner := w - 2
	lines := []string{""}
	text := lipgloss.NewStyle().Width(inner - 2).Foreground(c(hexText)).Render(p.text)
	for _, l := range strings.Split(text, "\n") {
		lines = append(lines, " "+l)
	}
	lines = append(lines, "")

	var parts []string
	var widths []int
	for i, b := range p.buttons {
		style := lipgloss.NewStyle().Background(c(hexSelBg)).Foreground(c(hexText))
		if b.danger {
			style = lipgloss.NewStyle().Background(c(tint(hexAccent))).Foreground(c(fg(hexAccent)))
		}
		label := "  " + b.key + " " + b.label + "  "
		if i == p.sel { // the selected button is inverted and has arrows
			style = style.Reverse(true).Bold(true)
			label = "▸ " + b.key + " " + b.label + " ◂"
		}
		btn := style.Render(label)
		parts = append(parts, btn)
		widths = append(widths, lipgloss.Width(btn))
	}
	row := strings.Join(parts, "    ")
	left := max(0, (inner-lipgloss.Width(row))/2)
	y := len(lines) + 1 // +1 for the top border
	var hits []rect
	x := 1 + left
	for _, bw := range widths {
		hits = append(hits, rect{x, y, bw, 1})
		x += bw + 4
	}
	lines = append(lines, strings.Repeat(" ", left)+row, "")
	hint := lipgloss.NewStyle().Width(inner - 2).Foreground(c(hexDim)).Render("←/→ select · enter push · esc close")
	lines = append(lines, " "+hint)
	return box(st(fg(hexAccent)).Bold(true).Render(p.title), padLines(lines, inner, len(lines)), w, fg(hexAccent)), hits
}

// confirmRect is the area of the dialog on the screen.
func (m Model) confirmRect() rect {
	d, _ := m.confirmLayout()
	w, h := lipgloss.Width(d), lipgloss.Height(d)
	return rect{max(0, (m.width-w)/2), max(0, (m.height-h)/3), w, h}
}

// confirmClick runs the button under the pointer. Other clicks do nothing.
func (m Model) confirmClick(x, y int) (tea.Model, tea.Cmd) {
	r := m.confirmRect()
	_, hits := m.confirmLayout()
	for i, h := range hits {
		if (rect{r.x + h.x, r.y + h.y, h.w, h.h}).has(x, y) {
			return m.answerConfirm(m.confirm.buttons[i].key)
		}
	}
	return m, nil
}
