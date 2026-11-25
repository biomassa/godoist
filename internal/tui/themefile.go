package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/BurntSushi/toml"
)

// Theme files are themes that the user writes, or that another program writes for them:
// each themes/NAME.toml in the config directory is the theme NAME. A program that makes
// color schemes (a desktop shell, a wallpaper tool) can fill such a file from a template
// and then send godoist SIGUSR2, so that the new colors show at once.

// themeFile is the content of a theme file. Only background, foreground and accent must
// be set; the other colors come from them or from the Todoist colors.
type themeFile struct {
	Background string `toml:"background"`
	Foreground string `toml:"foreground"`
	Accent     string `toml:"accent"`    // focused borders, headings, the cursor in lists
	Border     string `toml:"border"`    // unfocused borders
	Selection  string `toml:"selection"` // the selected row, mixed with the background
	Success    string `toml:"success"`   // today and completed tasks, usually a green
	Dimmed     string `toml:"dimmed"`    // secondary text
	Error      string `toml:"error"`     // overdue tasks, p1, errors
}

// fileTheme is a theme read from a theme file. Its name is never the name of a built-in
// theme, so it never gets a Chroma style (see chromaStyles).
type fileTheme struct {
	palette
	path string
}

// fileThemes are the themes read from theme files, sorted by file name.
var fileThemes []fileTheme

// themeLoadErr is the error of LoadThemes. New shows it in the status line.
var themeLoadErr error

// themeDir is the directory of the theme files, from LoadThemes. A reload reads it again,
// so that it finds the files that were added or fixed after the start.
var themeDir string

// LoadThemes reads the theme files (*.toml) in dir. A missing dir is no error. A file
// that can't be read, or that has the name of a built-in theme, gives an error, and the
// other files are still read. The TUI also shows the error when it starts.
func LoadThemes(dir string) error {
	themeDir, fileThemes = dir, nil
	themeLoadErr = scanThemes()
	return themeLoadErr
}

// scanThemes reads the theme files in themeDir again. A file that can't be read keeps
// the colors that it had: a tool may be in the middle of writing it. A file that is gone
// drops its theme.
func scanThemes() error {
	if themeDir == "" {
		return nil // LoadThemes was not called
	}
	entries, err := os.ReadDir(themeDir)
	if errors.Is(err, fs.ErrNotExist) {
		fileThemes = nil
		return nil
	}
	if err != nil {
		return err
	}
	var themes []fileTheme
	var errs []error
	for _, e := range entries {
		// Editors keep lock and backup files next to the file, as ".#name.toml".
		if !strings.HasSuffix(e.Name(), ".toml") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(themeDir, e.Name())
		p, err := readThemeFile(path)
		if err != nil {
			errs = append(errs, err)
			if old := findFileTheme(strings.TrimSuffix(e.Name(), ".toml")); old != nil {
				themes = append(themes, *old)
			}
			continue
		}
		if p.name == todoistTheme || slices.ContainsFunc(palettes, func(b palette) bool { return b.name == p.name }) {
			errs = append(errs, fmt.Errorf("theme file %s: %q is the name of a built-in theme", path, p.name))
			continue
		}
		themes = append(themes, p)
	}
	fileThemes = themes
	return errors.Join(errs...)
}

// errLine makes err fit the status line: errors.Join puts each error on its own line,
// and a second line breaks the layout of the screen.
func errLine(err error) string {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		if errs := j.Unwrap(); len(errs) > 1 {
			return fmt.Sprintf("%s (and %d more)", oneLine(errs[0].Error()), len(errs)-1)
		}
	}
	return oneLine(err.Error())
}

// setThemeErr shows the theme file error err in the status line.
func (m *Model) setThemeErr(err error) {
	m.themeErr = errLine(err)
	m.setStatus(m.themeErr, true)
}

// readThemeFile reads one theme file. The file name without ".toml" is the theme name.
func readThemeFile(path string) (fileTheme, error) {
	st, err := os.Stat(path)
	if err != nil {
		return fileTheme{}, err
	}
	if !st.Mode().IsRegular() {
		return fileTheme{}, fmt.Errorf("theme file %s: not a file", path)
	}
	var f themeFile
	md, err := toml.DecodeFile(path, &f)
	if err != nil {
		return fileTheme{}, fmt.Errorf("theme file %s: %w", path, err)
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		return fileTheme{}, fmt.Errorf("theme file %s: unknown key %q", path, keys[0].String())
	}
	for _, v := range []struct{ key, hex string }{{"background", f.Background}, {"foreground", f.Foreground}, {"accent", f.Accent}} {
		if v.hex == "" {
			return fileTheme{}, fmt.Errorf("theme file %s: %s is missing", path, v.key)
		}
	}
	def := func(h, d string) string {
		if h == "" {
			return d
		}
		return h
	}
	p := fileTheme{palette: palette{
		name:     strings.TrimSuffix(filepath.Base(path), ".toml"),
		bg:       f.Background,
		fg:       f.Foreground,
		border:   def(f.Border, mix(f.Foreground, f.Background, 0.7)),
		focus:    f.Accent,
		selected: def(f.Selection, f.Accent),
		good:     def(f.Success, "#25B84C"),
		dimmed:   def(f.Dimmed, mix(f.Foreground, f.Background, 0.55)),
		errorHex: def(f.Error, "#FF7066"),
	}, path: path}
	for _, h := range []string{p.bg, p.fg, p.border, p.focus, p.selected, p.good, p.dimmed, p.errorHex} {
		if !isHex(h) {
			return fileTheme{}, fmt.Errorf("theme file %s: %q is not a #RRGGBB color", path, h)
		}
	}
	return p, nil
}

// isHex reports whether h is a "#RRGGBB" color.
func isHex(h string) bool {
	if len(h) != 7 || h[0] != '#' {
		return false
	}
	for _, r := range strings.ToLower(h[1:]) {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// allPalettes returns the built-in palettes and then the file themes.
func allPalettes() []palette {
	all := slices.Clip(palettes)
	for _, f := range fileThemes {
		all = append(all, f.palette)
	}
	return all
}

// findFileTheme returns the file theme name, or nil.
func findFileTheme(name string) *fileTheme {
	for i := range fileThemes {
		if fileThemes[i].name == name {
			return &fileThemes[i]
		}
	}
	return nil
}

// ThemeReloadMsg asks the TUI to read the theme files again. godoist sends it on
// SIGUSR2, so a tool that writes a theme file can tell godoist with
// "pkill -USR2 -x godoist".
type ThemeReloadMsg struct{}

// reloadTheme reads the theme files again and shows the new colors. It also finds the
// theme files that were added, so the wanted theme comes back when its file was missing
// or broken at the start. A built-in theme stays as it is. With the picker open, the
// picker gets the new list, and the theme under the cursor shows again.
func (m Model) reloadTheme() (tea.Model, tea.Cmd) {
	err := scanThemes()
	if err == nil && m.statusErr && m.status == m.themeErr {
		m.setStatus("", false) // the files are good now: the old error goes
	}
	names := themeNames()
	// The theme in use: the one that the picker goes back to, or the one on the screen.
	inUse := themeName
	if m.themes != nil {
		inUse = m.themes.orig
	}
	want := inUse
	if m.wantTheme != "" && slices.Contains(names, m.wantTheme) {
		want = m.wantTheme
	}
	switch {
	case !slices.Contains(names, want):
		m.setStatus("theme "+want+" not found, the theme is "+todoistTheme, true)
		want = todoistTheme
	case want != inUse:
		m.setStatus("theme "+want, false) // the wanted theme is back
	}
	if err != nil {
		m.setThemeErr(err)
	}
	show := want
	if p := m.themes; p != nil {
		cur := p.names[p.cur]
		p.names, p.orig = names, want
		p.cur = slices.Index(names, cur)
		if p.cur < 0 {
			p.cur = slices.Index(names, want)
		}
		show = names[p.cur]
	}
	// A built-in theme has no new colors, so the screen stays.
	if show != themeName || findFileTheme(show) != nil {
		m.previewTheme(show)
	}
	return m, nil
}
