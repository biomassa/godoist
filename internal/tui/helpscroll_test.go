package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The help is taller than a 30-line terminal. j and G scroll it so that the last
// section shows, and q closes the help without quitting.
func TestHelpScroll(t *testing.T) {
	m := send(t, testModel(t), key("?"))
	if !m.showHelp || m.helpMaxOff() == 0 {
		t.Fatalf("help open %v, max offset %d: the test needs a help taller than the pane", m.showHelp, m.helpMaxOff())
	}
	if strings.Contains(ansi.Strip(m.render()), "this help") {
		t.Fatal("the end of the help shows before scrolling")
	}
	m = send(t, m, key("j"))
	if m.helpOff != 1 || !m.showHelp {
		t.Fatalf("after j: offset %d, open %v", m.helpOff, m.showHelp)
	}
	m = send(t, m, key("G"))
	if m.helpOff != m.helpMaxOff() {
		t.Fatalf("after G: offset %d, want %d", m.helpOff, m.helpMaxOff())
	}
	if !strings.Contains(ansi.Strip(m.render()), "this help") {
		t.Error("the end of the help does not show after G")
	}
	m = send(t, m, key("j"))
	if m.helpOff != m.helpMaxOff() {
		t.Errorf("j scrolled past the end: %d", m.helpOff)
	}
	m = send(t, m, key("q"))
	if m.showHelp || m.quitting {
		t.Errorf("after q: help open %v, quitting %v", m.showHelp, m.quitting)
	}
	m = send(t, m, key("?"))
	if m.helpOff != 0 {
		t.Errorf("the help opens at offset %d, want 0", m.helpOff)
	}
	if m = send(t, m, key("esc")); m.showHelp {
		t.Error("esc did not close the help")
	}
	m.pending = 1 // with a write in flight, quit waits and sets quitting
	if m = send(t, m, key("?"), key("ctrl+c")); !m.quitting {
		t.Error("ctrl+c in the help did not quit")
	}
}
