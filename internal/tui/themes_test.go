package tui

import (
	"strings"
	"testing"
)

// T opens the picker. A move previews the theme, esc goes back, and enter keeps the theme
// and saves it.
func TestThemePicker(t *testing.T) {
	var saved string
	m := openWork(t)
	m.saveTheme = func(n string) error { saved = n; return nil }
	defer applyTheme(todoistTheme, true)
	m = send(t, m, key("T"))
	if m.themes == nil || themeName != todoistTheme {
		t.Fatalf("picker = %v theme = %s", m.themes != nil, themeName)
	}
	m = send(t, m, key("j"))
	if themeName != palettes[0].name || themeBg != palettes[0].bg {
		t.Fatalf("after j: theme = %s bg = %s", themeName, themeBg)
	}
	if !strings.Contains(stripANSI(m.themeBox()), "catppuccin-mocha") {
		t.Error("the picker does not list catppuccin-mocha")
	}
	m = send(t, m, key("esc"))
	if m.themes != nil || themeName != todoistTheme || themeBg != "" {
		t.Fatalf("after esc: theme = %s bg = %q", themeName, themeBg)
	}
	nm, cmd := send(t, m, key("T"), key("j"), key("j")).Update(key("enter"))
	m = nm.(Model)
	if m.themes != nil || themeName != palettes[1].name || cmd == nil {
		t.Fatalf("after enter: theme = %s cmd = %v", themeName, cmd != nil)
	}
	m = send(t, m, cmd())
	if saved != palettes[1].name || !strings.Contains(m.status, "saved") {
		t.Errorf("saved = %q status = %q", saved, m.status)
	}
}

// In each theme, text and the meaning colors are readable on the theme background.
func TestThemesReadable(t *testing.T) {
	defer applyTheme(todoistTheme, true)
	for _, p := range palettes {
		applyTheme(p.name, true)
		for name, h := range map[string]string{"text": hexText, "muted": hexMuted, "overdue": hexOverdue,
			"today": hexToday, "tomorrow": hexTomorrow, "week": hexWeek, "p3": priorityHex[3]} {
			if cr := contrast(h, baseBg); cr < 2.9 {
				t.Errorf("%s: %s %s on %s has contrast %.2f", p.name, name, h, baseBg, cr)
			}
		}
	}
}

// An unknown theme name gives the "todoist" theme.
func TestUnknownTheme(t *testing.T) {
	applyTheme("no-such-theme", true)
	if themeName != todoistTheme || themeBg != "" {
		t.Errorf("theme = %s bg = %q", themeName, themeBg)
	}
}

// T is at the end of a pane legend, before "q quit". The open picker has its own keys.
func TestThemeLegend(t *testing.T) {
	m := onRow(t, 1)
	keys := m.legendKeys()
	if keys[len(keys)-1][0] != "T" {
		t.Errorf("task list legend ends with %v, want T theme", keys[len(keys)-1])
	}
	m.focus = paneNav
	for i, n := range m.nav { // Inbox: the sidebar legend with "q quit"
		if p := m.projects[n.projectID]; p != nil && p.InboxProject {
			m.navCur = i
		}
	}
	keys = m.legendKeys()
	if n := len(keys); keys[n-2][0] != "T" || keys[n-1][0] != "q" {
		t.Errorf("sidebar legend ends with %v, want T theme, q quit", keys[n-2:])
	}
	m.focus = paneTasks
	defer applyTheme(todoistTheme, true)
	m = send(t, m, key("T"))
	if bar := stripANSI(m.bottomBar()); !strings.Contains(bar, "enter keep") || !strings.Contains(bar, "esc back") {
		t.Errorf("picker bottom bar = %q", bar)
	}
}

// The markdown colors follow the theme: the heading bars, links, code, and the code style
// of the reader. The todoist theme keeps its colors.
func TestThemeMarkdownColors(t *testing.T) {
	defer applyTheme(todoistTheme, true)
	applyTheme(todoistTheme, true)
	if headingHex(1) != "#DC4C3E" || readerStyle().CodeBlock.Chroma == nil && readerStyle().CodeBlock.Theme != "" {
		t.Errorf("todoist: h1 = %s", headingHex(1))
	}
	applyTheme("nord", true)
	st := readerStyle()
	if headingHex(1) != hexOverdue || headingHex(4) != hexLink || st.CodeBlock.Theme != "nord" {
		t.Errorf("nord: h1 = %s h4 = %s code theme = %q", headingHex(1), headingHex(4), st.CodeBlock.Theme)
	}
	if st.Link.Color == nil || *st.Link.Color != hexLink {
		t.Errorf("nord: link color = %v, want %s", st.Link.Color, hexLink)
	}
	applyTheme("vt100", true) // no Chroma style: code colors from the palette
	if st := readerStyle(); st.CodeBlock.Chroma == nil || *st.CodeBlock.Chroma.Keyword.Color != hexLink {
		t.Error("vt100: no code colors from the palette")
	}
	for _, p := range palettes {
		applyTheme(p.name, true)
		for lv := 1; lv <= 6; lv++ {
			if cr := contrast(headingHex(lv), baseBg); cr < 2.9 {
				t.Errorf("%s: heading %d %s has contrast %.2f", p.name, lv, headingHex(lv), cr)
			}
		}
	}
}
