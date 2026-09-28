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
