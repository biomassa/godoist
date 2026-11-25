package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Keys in text, such as "u to undo" in the status or "enter save" in a hint, show in the
// accent color, as in the legend. A text marks each key with kbd, and withKeys draws the
// text. The marks are control characters with no width, which no task name has, so a
// marked text can hold names from Todoist.

const (
	keyOpen  = "\x0e"
	keyClose = "\x0f"
)

// kbd marks k as a key in a text that withKeys draws.
func kbd(k string) string { return keyOpen + k + keyClose }

// keyed marks each {key} in s as a key, for example keyed("{enter} save · {esc} cancel").
// Use it only on text from the code: a name from Todoist can have braces.
func keyed(s string) string {
	return strings.NewReplacer("{", keyOpen, "}", keyClose).Replace(s)
}

// withKeys draws text in style base, and the keys that kbd marked in the accent color.
func withKeys(text string, base lipgloss.Style) string {
	if !strings.Contains(text, keyOpen) {
		return base.Render(text)
	}
	key := base.Foreground(c(fg(hexAccent)))
	var b strings.Builder
	for {
		i := strings.Index(text, keyOpen)
		if i < 0 {
			break
		}
		if i > 0 {
			b.WriteString(base.Render(text[:i]))
		}
		text = text[i+len(keyOpen):]
		j := strings.Index(text, keyClose)
		if j < 0 { // a cut text: the rest is the key
			j = len(text)
		}
		b.WriteString(key.Render(text[:j]))
		text = strings.TrimPrefix(text[j:], keyClose)
	}
	if text != "" {
		b.WriteString(base.Render(text))
	}
	return b.String()
}

// plainKeys removes the key marks from text.
func plainKeys(text string) string {
	return strings.NewReplacer(keyOpen, "", keyClose, "").Replace(text)
}
