package tui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"

	"charm.land/lipgloss/v2"
)

// Theme colors. setTheme sets them after the terminal reports its background color.
// The default is the dark theme.
var (
	hexAccent   = "#DC4C3E" // Todoist red
	hexOverdue  string
	hexToday    string
	hexTomorrow string
	hexWeek     string
	hexMuted    string
	hexDim      string
	hexBorder   string
	hexText     string
	hexBody     string
	hexSelBg    string
	hexSelBgDim string
	baseBg      string // terminal background, used for tinting
	darkTheme   bool
	priorityHex map[int]string
)

func init() { setTheme(true) }

// setTheme selects the dark or the light palette. The values come from the Todoist app themes.
func setTheme(dark bool) {
	darkTheme = dark
	if dark {
		hexOverdue, hexToday, hexTomorrow, hexWeek = "#FF7066", "#25B84C", "#FF9A14", "#A970FF"
		hexMuted, hexDim, hexBorder = "#8A8A8A", "#5C5C5C", "#3D3D3D"
		hexText, hexBody = "#E6E6E6", "#C8C8C8"
		hexSelBg, hexSelBgDim, baseBg = "#363636", "#2A2A2A", "#1F1F1F"
		priorityHex = map[int]string{1: "#FF7066", 2: "#FF9A14", 3: "#5297FF", 4: hexMuted}
		return
	}
	hexOverdue, hexToday, hexTomorrow, hexWeek = "#D1453B", "#058527", "#AD6200", "#692FC2"
	hexMuted, hexDim, hexBorder = "#666666", "#9A9A9A", "#D0D0D0"
	hexText, hexBody = "#202020", "#3A3A3A"
	hexSelBg, hexSelBgDim, baseBg = "#E4E4E4", "#EFEFEF", "#FFFFFF"
	priorityHex = map[int]string{1: "#D1453B", 2: "#EB8909", 3: "#246FE0", 4: hexMuted}
}

// c converts a hex string to a color.
func c(hex string) color.Color { return lipgloss.Color(hex) }

// parseHex splits "#RRGGBB" into its components. It returns gray for a bad value.
func parseHex(h string) (r, g, b float64) {
	v, err := strconv.ParseUint(h[1:], 16, 32)
	if err != nil || len(h) != 7 {
		return 128, 128, 128
	}
	return float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff)
}

// mix blends a toward b by t (0 = a, 1 = b).
func mix(a, b string, t float64) string {
	ar, ag, ab := parseHex(a)
	br, bg, bb := parseHex(b)
	l := func(x, y float64) int { return int(math.Round(x + (y-x)*t)) }
	return fmt.Sprintf("#%02X%02X%02X", l(ar, br), l(ag, bg), l(ab, bb))
}

// luminance is the relative brightness of a color, from 0 to 1.
func luminance(h string) float64 {
	r, g, b := parseHex(h)
	return (0.2126*r + 0.7152*g + 0.0722*b) / 255
}

// fg makes a Todoist palette color readable on the terminal background.
func fg(h string) string {
	if darkTheme && luminance(h) < 0.35 {
		return mix(h, "#FFFFFF", 0.35)
	}
	if !darkTheme && luminance(h) > 0.6 {
		return mix(h, "#000000", 0.35)
	}
	return h
}

// tint is a subtle background derived from a project color, like Todoist's selected row.
func tint(h string) string { return mix(h, baseBg, 0.72) }
