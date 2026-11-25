package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/biomassa/godoist/internal/todoist"
)

// writeTheme writes a theme file.
func writeTheme(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// resetFileThemes drops the file themes and goes back to the default theme after a test.
func resetFileThemes(t *testing.T) {
	t.Cleanup(func() {
		fileThemes, themeLoadErr = nil, nil
		applyTheme(todoistTheme, true)
	})
}

// A theme file adds a theme named after the file. The colors that the file leaves out
// come from its other colors, and the theme works like a built-in one.
func TestLoadThemes(t *testing.T) {
	resetFileThemes(t)
	dir := t.TempDir()
	writeTheme(t, filepath.Join(dir, "desk.toml"), `background = "#101820"
foreground = "#e0e0e0"
accent = "#ff8800"
`)
	if err := LoadThemes(dir); err != nil {
		t.Fatal(err)
	}
	if names := themeNames(); names[len(names)-1] != "desk" {
		t.Fatalf("themes = %v, want desk last", names)
	}
	if got := themeLabel("desk"); got != "desk (file)" {
		t.Errorf("label = %q", got)
	}
	applyTheme("desk", true)
	if themeName != "desk" || themeBg != "#101820" || hexAccent != "#ff8800" || chromaStyle != "" {
		t.Errorf("theme = %s bg = %s accent = %s chroma = %q", themeName, themeBg, hexAccent, chromaStyle)
	}
	if hexBorder == "" || hexOverdue == "" || hexToday == "" {
		t.Errorf("missing colors: border %q overdue %q today %q", hexBorder, hexOverdue, hexToday)
	}
}

// A missing theme directory is no error.
func TestLoadThemesNoDir(t *testing.T) {
	resetFileThemes(t)
	if err := LoadThemes(filepath.Join(t.TempDir(), "themes")); err != nil || len(fileThemes) != 0 {
		t.Errorf("err = %v themes = %d", err, len(fileThemes))
	}
}

// A bad theme file gives an error that names it, and the good files still load.
func TestLoadThemesErrors(t *testing.T) {
	resetFileThemes(t)
	dir := t.TempDir()
	good := "background = \"#000000\"\nforeground = \"#ffffff\"\naccent = \"#3366ff\"\n"
	writeTheme(t, filepath.Join(dir, "good.toml"), good)
	writeTheme(t, filepath.Join(dir, "nofg.toml"), "background = \"#000000\"\naccent = \"#3366ff\"\n")
	writeTheme(t, filepath.Join(dir, "badhex.toml"), good+"error = \"red\"\n")
	writeTheme(t, filepath.Join(dir, "typo.toml"), good+"acent = \"#ffffff\"\n")
	writeTheme(t, filepath.Join(dir, "nord.toml"), good)
	writeTheme(t, filepath.Join(dir, ".#good.toml"), "an editor lock file")
	writeTheme(t, filepath.Join(dir, "notes.txt"), "not a theme")
	if err := os.Mkdir(filepath.Join(dir, "dir.toml"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := LoadThemes(dir)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"nofg.toml: foreground is missing", `"red" is not a #RRGGBB color`,
		`unknown key "acent"`, `"nord" is the name of a built-in theme`, "dir.toml: not a file"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if len(fileThemes) != 1 || fileThemes[0].name != "good" {
		t.Errorf("loaded %d themes, want only good", len(fileThemes))
	}
	if slices.Contains(themeNames(), "nofg") {
		t.Error("a broken file is in the theme list")
	}
}

// A reload shows the new colors of the theme file in use. A broken file keeps the colors
// on the screen and says why in the status line.
func TestThemeFileReload(t *testing.T) {
	resetFileThemes(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "desk.toml")
	writeTheme(t, path, "background = \"#101820\"\nforeground = \"#e0e0e0\"\naccent = \"#ff8800\"\n")
	if err := LoadThemes(dir); err != nil {
		t.Fatal(err)
	}
	m := openWork(t)
	applyTheme("desk", true)

	writeTheme(t, path, "background = \"#f5f5f5\"\nforeground = \"#202020\"\naccent = \"#0055cc\"\n")
	gen := themeGen
	m = send(t, m, ThemeReloadMsg{})
	if themeBg != "#f5f5f5" || hexAccent != "#0055cc" || darkTheme || themeGen == gen {
		t.Fatalf("after the reload: bg = %s accent = %s dark = %v", themeBg, hexAccent, darkTheme)
	}
	writeTheme(t, path, "background = \"#000000\"\n")
	m = send(t, m, ThemeReloadMsg{})
	if themeBg != "#f5f5f5" || !m.statusErr || !strings.Contains(m.status, "foreground is missing") {
		t.Errorf("after a broken file: bg = %s status = %q", themeBg, m.status)
	}
}

// With a built-in theme in use, a reload changes nothing, even when a theme file changed.
func TestThemeReloadBuiltIn(t *testing.T) {
	resetFileThemes(t)
	dir := t.TempDir()
	writeTheme(t, filepath.Join(dir, "desk.toml"), "background = \"#101820\"\nforeground = \"#e0e0e0\"\naccent = \"#ff8800\"\n")
	if err := LoadThemes(dir); err != nil {
		t.Fatal(err)
	}
	m := openWork(t)
	applyTheme("nord", true)
	gen := themeGen
	m = send(t, m, ThemeReloadMsg{})
	if themeName != "nord" || themeGen != gen || m.statusErr {
		t.Errorf("theme = %s gen %d -> %d status = %q", themeName, gen, themeGen, m.status)
	}
}

// startModel calls New with the files of godoist in a temporary directory.
func startModel(t *testing.T, theme string) Model {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_STATE_HOME", dir)
	t.Setenv("HOME", dir)
	return New(todoist.New("test"), "test", Settings{Theme: theme})
}

// A kept theme whose file is gone gives the "todoist" theme and says so, and godoist
// still starts.
func TestStartMissingThemeFile(t *testing.T) {
	resetFileThemes(t)
	m := startModel(t, "gone")
	if themeName != todoistTheme || !m.statusErr || !strings.Contains(m.status, "theme gone not found") {
		t.Errorf("theme = %s status = %q", themeName, m.status)
	}
}

// A broken theme file doesn't stop the start: the status line shows the error.
func TestStartBrokenThemeFile(t *testing.T) {
	resetFileThemes(t)
	dir := t.TempDir()
	writeTheme(t, filepath.Join(dir, "bad.toml"), "background = 1\n")
	if LoadThemes(dir) == nil {
		t.Fatal("no error for a broken file")
	}
	m := startModel(t, "")
	if !m.statusErr || !strings.Contains(m.status, "bad.toml") {
		t.Errorf("status = %q", m.status)
	}
}

// The Noctalia template in docs/themes gives a theme file that godoist reads, once
// Noctalia has filled in the colors.
func TestNoctaliaTemplate(t *testing.T) {
	resetFileThemes(t)
	tmpl, err := os.ReadFile("../../docs/themes/noctalia.toml")
	if err != nil {
		t.Fatal(err)
	}
	body := regexp.MustCompile(`\{\{[^}]*\}\}`).ReplaceAllString(string(tmpl), "#336699")
	if body == string(tmpl) {
		t.Fatal("the template has no placeholders")
	}
	dir := t.TempDir()
	writeTheme(t, filepath.Join(dir, "noctalia.toml"), body)
	if err := LoadThemes(dir); err != nil || findFileTheme("noctalia") == nil {
		t.Errorf("err = %v", err)
	}
}
