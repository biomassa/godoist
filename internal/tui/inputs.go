package tui

import (
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
)

// The text inputs have a steady cursor. A blinking cursor sends a message two times a
// second, and each message draws the screen again, which uses CPU for nothing.

// newTextInput returns a one-line text input with a steady cursor.
func newTextInput() textinput.Model {
	ti := textinput.New()
	s := textinput.DefaultStyles(darkTheme)
	s.Cursor.Blink = false
	ti.SetStyles(s)
	return ti
}

// areaStyles returns the textarea styles of the theme with a steady cursor.
func areaStyles() textarea.Styles {
	s := textarea.DefaultStyles(darkTheme)
	s.Cursor.Blink = false
	return s
}
