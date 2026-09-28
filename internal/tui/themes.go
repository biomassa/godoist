package tui

import (
	"fmt"
	"math"
)

// A palette is a color theme. The "todoist" theme has no palette: it uses the Todoist app
// colors, and the terminal background selects its dark or light form. The other palettes
// come from tideui by Allie Bayless (github.com/allisonhere/tideui, MIT license).
type palette struct {
	name          string
	bg, fg        string // background and text
	border, focus string // unfocused and focused borders. focus is also the accent
	selected      string // selected row
	good          string // "done" green: today, completed
	dimmed        string // secondary text
	errorHex      string // overdue, p1, errors
}

// todoistTheme is the name of the default theme.
const todoistTheme = "todoist"

// palettes are the themes after "todoist", in picker order.
var palettes = []palette{
	{"catppuccin-mocha", "#1e1e2e", "#cdd6f4", "#6c7086", "#89b4fa", "#89b4fa", "#a6e3a1", "#585b70", "#f38ba8"},
	{"catppuccin-latte", "#eff1f5", "#4c4f69", "#9ca0b0", "#1e66f5", "#1e66f5", "#40a02b", "#8c8fa1", "#d20f39"},
	{"catppuccin-frappe", "#303446", "#c6d0f5", "#626880", "#8caaee", "#8caaee", "#a6d189", "#51576d", "#e78284"},
	{"catppuccin-macchiato", "#24273a", "#cad3f5", "#5b6078", "#8aadf4", "#8aadf4", "#a6da95", "#494d64", "#ed8796"},
	{"nord", "#2e3440", "#eceff4", "#4c566a", "#88c0d0", "#88c0d0", "#a3be8c", "#4c566a", "#bf616a"},
	{"dracula", "#282a36", "#f8f8f2", "#6272a4", "#bd93f9", "#bd93f9", "#50fa7b", "#6272a4", "#ff5555"},
	{"gruvbox-dark", "#282828", "#ebdbb2", "#504945", "#83a598", "#83a598", "#b8bb26", "#504945", "#fb4934"},
	{"gruvbox-light", "#fbf1c7", "#3c3836", "#bdae93", "#076678", "#076678", "#79740e", "#bdae93", "#cc241d"},
	{"tokyo-night", "#1a1b26", "#c0caf5", "#414868", "#7aa2f7", "#7aa2f7", "#9ece6a", "#414868", "#f7768e"},
	{"tokyo-night-day", "#e1e2e7", "#3760bf", "#a8aecb", "#2e7de9", "#2e7de9", "#587539", "#a8aecb", "#f52a65"},
	{"rose-pine", "#191724", "#e0def4", "#403d52", "#c4a7e7", "#c4a7e7", "#9ccfd8", "#403d52", "#eb6f92"},
	{"rose-pine-moon", "#232136", "#e0def4", "#44415a", "#c4a7e7", "#c4a7e7", "#9ccfd8", "#44415a", "#eb6f92"},
	{"rose-pine-dawn", "#faf4ed", "#575279", "#d7d2be", "#907aa9", "#907aa9", "#286983", "#d7d2be", "#b4637a"},
	{"one-dark", "#282c34", "#abb2bf", "#3e4451", "#61afef", "#61afef", "#98c379", "#3e4451", "#e06c75"},
	{"magenta-geode", "#47003c", "#f3b0dc", "#aa4d84", "#c83fa9", "#c83fa9", "#f3b0dc", "#77176e", "#ff7062"},
	{"coral-sunset", "#444154", "#fec9c1", "#fc8b79", "#ff7062", "#ff7062", "#fec9c1", "#7a637f", "#ff7062"},
	{"lavender-fields-forever", "#382d72", "#e5ccf4", "#b7c2c6", "#a080e1", "#a080e1", "#e5ccf4", "#5c509c", "#ff7062"},
	{"vt100", "#000000", "#33ff33", "#145214", "#00ff00", "#00ff00", "#66ff66", "#3dcc3d", "#ff6b6b"},
	{"vt52", "#000000", "#ffcc66", "#6b4e14", "#ffb020", "#ffb020", "#ffe6a8", "#a67c2e", "#ff6666"},
}

// themeNames returns all theme names, "todoist" first.
func themeNames() []string {
	names := []string{todoistTheme}
	for _, p := range palettes {
		names = append(names, p.name)
	}
	return names
}

// themeName is the theme in use. themeBg is its background, or "" for the terminal
// background ("todoist" theme).
var (
	themeName = todoistTheme
	themeBg   string
)

// applyTheme sets the colors of theme name. termDark is the terminal background of the
// last report, for the "todoist" theme. An unknown name gives the "todoist" theme.
func applyTheme(name string, termDark bool) {
	for _, p := range palettes {
		if p.name == name {
			applyPalette(p)
			return
		}
	}
	themeName, themeBg = todoistTheme, ""
	setTheme(termDark)
}

// applyPalette maps a palette onto the godoist colors. The Todoist meanings stay:
// overdue and p1 are the error color, today is the green, the accent is the focus color.
// Tomorrow, this week, p2, and p3 are mixes that keep their hue family where the palette
// has none: orange from the error color and yellow, purple from the error and focus colors.
func applyPalette(p palette) {
	themeName, themeBg = p.name, p.bg
	darkTheme = luminance(p.bg) < 0.5
	baseBg = p.bg
	hexText = p.fg
	hexAccent = p.focus
	hexBorder = p.border
	hexDim = readable(p.dimmed, p.bg, 3)
	hexMuted = readable(mix(p.fg, p.bg, 0.4), p.bg, 4.5)
	hexSelBg, hexSelBgDim = mix(p.selected, p.bg, 0.72), mix(p.selected, p.bg, 0.85)
	hexOverdue = readable(p.errorHex, p.bg, 3)
	hexToday = readable(p.good, p.bg, 3)
	hexTomorrow = readable(mix(p.errorHex, "#FFC000", 0.55), p.bg, 3)
	hexWeek = readable(mix(p.errorHex, p.focus, 0.6), p.bg, 3)
	priorityHex = map[int]string{1: hexOverdue, 2: hexTomorrow, 3: readable(p.focus, p.bg, 3), 4: hexMuted}
}

// contrast is the WCAG contrast ratio of two colors, from 1 to 21.
func contrast(a, b string) float64 {
	la, lb := relLum(a), relLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// relLum is the WCAG relative luminance of a color.
func relLum(h string) float64 {
	r, g, b := parseHex(h)
	lin := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// readable moves h toward white or black (away from bg) until it has the contrast min
// against bg, so that dim palette colors stay readable. It keeps the hue.
func readable(h, bg string, min float64) string {
	to := "#FFFFFF"
	if luminance(bg) > 0.5 {
		to = "#000000"
	}
	out := h
	for t := 0.1; contrast(out, bg) < min && t <= 1; t += 0.1 {
		out = mix(h, to, t)
	}
	return out
}

// themeLabel is the name shown in the picker: "todoist" shows that it follows the terminal.
func themeLabel(name string) string {
	if name == todoistTheme {
		return fmt.Sprintf("%s (terminal colors)", name)
	}
	return name
}
