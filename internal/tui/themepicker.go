package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The theme picker (T) lists the themes at the right of the screen. A move previews the
// highlighted theme on the whole screen with the real data. enter keeps the theme and
// saves it in the config file. esc goes back to the theme that was in use.

// themePicker is the open theme picker.
type themePicker struct {
	names []string
	cur   int
	orig  string // the theme in use when the picker opened
}

// themeSavedMsg is the result of a theme save.
type themeSavedMsg struct {
	name string
	err  error
}

const themePickerWidth = 40

// ThemeNames returns the names of all themes, "todoist" first. The CLI uses it for --theme.
func ThemeNames() []string { return themeNames() }

// openThemePicker opens the picker with the cursor on the theme in use.
func (m *Model) openThemePicker() {
	names := themeNames()
	cur := 0
	for i, n := range names {
		if n == themeName {
			cur = i
		}
	}
	m.themes = &themePicker{names: names, cur: cur, orig: themeName}
}

// previewTheme shows the screen in theme name. The markdown cache holds colored text, so
// it is cleared.
func (m *Model) previewTheme(name string) {
	applyTheme(name, m.termDark)
	m.md.reset()
	if m.loaded {
		m.buildNav()
		m.buildRows(false)
	}
}

// updateThemePicker handles a key while the picker is open.
func (m Model) updateThemePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.themes
	switch msg.String() {
	case "up", "k":
		p.cur = (p.cur - 1 + len(p.names)) % len(p.names)
	case "down", "j":
		p.cur = (p.cur + 1) % len(p.names)
	case "home", "g":
		p.cur = 0
	case "end", "G":
		p.cur = len(p.names) - 1
	case "enter":
		return m.keepTheme()
	case "esc", "T", "q":
		m.themes = nil
		m.previewTheme(p.orig)
		m.setStatus("theme "+p.orig, false)
		return m, nil
	default:
		return m, nil
	}
	m.previewTheme(p.names[p.cur])
	return m, nil
}

// keepTheme closes the picker with the highlighted theme and saves it for the next start.
func (m Model) keepTheme() (tea.Model, tea.Cmd) {
	name := m.themes.names[m.themes.cur]
	m.themes = nil
	m.previewTheme(name)
	save := m.saveTheme
	if save == nil {
		m.setStatus("theme "+name, false)
		return m, nil
	}
	return m, func() tea.Msg { return themeSavedMsg{name: name, err: save(name)} }
}

// themeSaved reports the result of the save in the status line.
func (m Model) themeSaved(msg themeSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setStatus("theme "+msg.name+" is in use, but it was not saved: "+msg.err.Error(), true)
		return m, nil
	}
	m.setStatus("theme "+msg.name+" · saved for the next start", false)
	return m, nil
}

// themeRect is the area of the picker: at the top right, so that most of the screen
// shows the preview.
func (m Model) themeRect() rect {
	w := min(themePickerWidth, max(20, m.width-4))
	h := min(len(themeNames())+5, m.height-3) // borders, a space, the list, a space, the hint
	return rect{max(0, m.width-w-1), 1, w, h}
}

// themeBox draws the picker.
func (m Model) themeBox() string {
	p := m.themes
	r := m.themeRect()
	inner := r.w - 2
	rows := r.h - 5
	off := max(0, min(p.cur-rows/2, len(p.names)-rows)) // keep the cursor in view
	lines := []string{""}
	for i := off; i < len(p.names) && i < off+rows; i++ {
		label := trunc(themeLabel(p.names[i]), inner-4)
		if i == p.cur {
			line := lipgloss.NewStyle().Background(c(hexSelBg)).Foreground(c(fg(hexAccent))).Bold(true).
				Render(" ▸ " + label + strings.Repeat(" ", max(0, inner-3-lipgloss.Width(label))))
			lines = append(lines, line)
			continue
		}
		lines = append(lines, "   "+st(hexText).Render(label))
	}
	lines = append(lines, "", " "+st(hexDim).Render(trunc("↑/↓ preview · enter keep · esc back", inner-1)))
	return box(st(fg(hexAccent)).Bold(true).Render("Theme"), padLines(lines, inner, r.h-2), r.w, fg(hexAccent))
}

// themeClick previews the theme under the mouse. A double-click keeps it. A click
// outside the picker goes back to the theme that was in use.
func (m Model) themeClick(x, y int, dbl bool) (tea.Model, tea.Cmd) {
	p := m.themes
	r := m.themeRect()
	if !r.has(x, y) {
		m.themes = nil
		m.previewTheme(p.orig)
		return m, nil
	}
	rows := r.h - 5
	off := max(0, min(p.cur-rows/2, len(p.names)-rows))
	i := off + y - r.y - 2 // the border and the empty line
	if i < off || i >= len(p.names) || i >= off+rows {
		return m, nil
	}
	p.cur = i
	if dbl {
		return m.keepTheme()
	}
	m.previewTheme(p.names[i])
	return m, nil
}
