package tui

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// The heading styles come from mded (github.com/biomassa/mded, internal/theme). All levels
// use the accent of the theme. Levels 1 and 2 have a bar to the full width, levels 3 and 4
// a short bar, and levels 5 and 6 no bar.

// headingLevel returns the level (1 to 6) and the text of an ATX heading line, or 0.
func headingLevel(l string) (int, string) {
	t := strings.TrimLeft(l, " ")
	n := 0
	for n < len(t) && n < 7 && t[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || (n < len(t) && t[n] != ' ') {
		return 0, ""
	}
	return n, strings.TrimSpace(strings.TrimRight(strings.TrimSpace(t[n:]), "#"))
}

// headingAccent is the color of the heading bars: the accent of the theme, readable on the
// background.
func headingAccent() string { return readable(hexAccent, baseBg, 3) }

// headingLight is the bright text of levels 1 to 3: near white on a dark theme, near black
// on a light theme, with a tint of the accent.
func headingLight() string {
	if darkTheme {
		return mix(headingAccent(), "#FFFFFF", 0.85)
	}
	return mix(headingAccent(), "#000000", 0.6)
}

// headingTop is the bar of level 1: the accent, made deeper (toward black on a dark theme,
// toward white on a light theme) until the bright text is readable on it.
func headingTop() string {
	to := "#000000"
	if !darkTheme {
		to = "#FFFFFF"
	}
	h, text := headingAccent(), headingLight()
	bar := h
	for t := 0.05; t <= 0.9 && contrast(text, bar) < 4.5; t += 0.05 {
		bar = mix(h, to, t)
	}
	return bar
}

// headingBarLevels is the number of levels with a bar. Levels 5 and 6 have accent text and
// no bar.
const headingBarLevels = 4

// headingBarHex returns the bar of a level from 1 to 4. The bars go from the level 1 bar to
// the background in equal steps of perceived color difference (CIE Lab), so that each step
// is as large as the theme permits: there are 4 steps from level 1 to the background.
func headingBarHex(lv int) string {
	top := headingTop()
	if lv <= 1 {
		return top
	}
	want := float64(lv-1) / headingBarLevels * deltaE(top, baseBg)
	lo, hi := 0.0, 1.0 // the share of the background in the mix
	for range 30 {
		mid := (lo + hi) / 2
		if deltaE(top, mix(top, baseBg, mid)) < want {
			lo = mid
		} else {
			hi = mid
		}
	}
	return mix(top, baseBg, (lo+hi)/2)
}

// headingFill returns how far the bar of a heading level goes: -1 for the full width
// (levels 1 and 2), the number of bar cells after the text (levels 3 and 4), or 0 for no
// bar (levels 5 and 6). The length of the bar shows the level in all themes.
func headingFill(level int) int {
	switch lv := max(1, min(level, 6)); {
	case lv <= 2:
		return -1
	case lv <= headingBarLevels:
		return 2
	}
	return 0
}

// headingColors returns the text and the bar color of a heading level. Levels 1 to 4 have a
// bar in the accent: level 1 is the strongest, and each lower level goes toward the
// background. The text is bright for levels 1 to 3 and the text color for level 4.
// Levels 5 and 6 have no bar (bar is the background) and accent text.
func headingColors(level int) (text, bar string) {
	lv := max(1, min(level, 6))
	switch {
	case lv <= 3:
		text = headingLight()
	case lv <= headingBarLevels:
		text = hexText
	default:
		return readable(headingAccent(), baseBg, 4.5), baseBg
	}
	bar = headingBarHex(lv)
	return readable(text, bar, 4.5), bar
}

// headingStyles keeps the heading styles of the theme colors in key, because the bar
// colors take many steps to find and the preview asks for a style for each rune.
var headingStyles struct {
	key    [3]string // the accent, the background, and the text color
	dark   bool
	styles [6]lipgloss.Style
}

// headingStyle is the style of a heading level: a bar with bold text for levels 1 to 4,
// bold accent text for level 5, and plain accent text for level 6.
func headingStyle(level int) lipgloss.Style {
	hs := &headingStyles
	if key := [3]string{hexAccent, baseBg, hexText}; hs.key != key || hs.dark != darkTheme {
		hs.key, hs.dark = key, darkTheme
		for lv := 1; lv <= 6; lv++ {
			text, bar := headingColors(lv)
			s := lipgloss.NewStyle().Foreground(c(text)).Bold(lv <= 5)
			if headingFill(lv) != 0 {
				s = s.Background(c(bar))
			}
			hs.styles[lv-1] = s
		}
	}
	return hs.styles[max(1, min(level, 6))-1]
}

// deltaE is the perceived difference of two colors: the distance in CIE Lab (CIE76).
func deltaE(a, b string) float64 {
	l1, a1, b1 := toLab(a)
	l2, a2, b2 := toLab(b)
	return math.Sqrt((l1-l2)*(l1-l2) + (a1-a2)*(a1-a2) + (b1-b2)*(b1-b2))
}

// toLab converts "#RRGGBB" (sRGB, D65 white) to CIE Lab.
func toLab(hex string) (l, a, b float64) {
	r, g, bl := parseHex(hex)
	lin := func(v float64) float64 {
		v /= 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	R, G, B := lin(r), lin(g), lin(bl)
	x := (0.4124*R + 0.3576*G + 0.1805*B) / 0.95047
	y := 0.2126*R + 0.7152*G + 0.0722*B
	z := (0.0193*R + 0.1192*G + 0.9505*B) / 1.08883
	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116
	}
	return 116*f(y) - 16, 500 * (f(x) - f(y)), 200 * (f(y) - f(z))
}
