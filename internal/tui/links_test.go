package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestTitleSegments(t *testing.T) {
	got := titleSegments("see [the repo](https://github.com/x/y) and https://go.dev/doc. **now**")
	want := []titleSeg{
		{text: "see "}, {text: "the repo", url: "https://github.com/x/y"}, {text: " and "},
		{text: "https://go.dev/doc", url: "https://go.dev/doc"}, {text: ". now"},
	}
	if len(got) != len(want) {
		t.Fatalf("segments = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("segment %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A link in a title is an OSC 8 hyperlink. The visible text is the link text.
func TestTitleLineHyperlink(t *testing.T) {
	s := titleLine("fix [issue](https://example.com/1) today", 40, lipgloss.NewStyle())
	if !strings.Contains(s, "\x1b]8;") || !strings.Contains(s, "https://example.com/1") {
		t.Errorf("no hyperlink in %q", s)
	}
	if got := stripANSI(s); got != "fix issue today" {
		t.Errorf("text = %q", got)
	}
	if got := stripANSI(titleLine("fix [issue](https://example.com/1) today", 7, lipgloss.NewStyle())); got != "fix is…" {
		t.Errorf("cut text = %q, want %q", got, "fix is…")
	}
}

// The details title wraps by words, and no line is wider than w.
func TestTitleLinesWrap(t *testing.T) {
	lines := titleLines("read https://example.com/a/long/path before friday", 20, lipgloss.NewStyle())
	for _, l := range lines {
		if w := lipgloss.Width(l); w > 20 {
			t.Errorf("line %q is %d wide", stripANSI(l), w)
		}
	}
	if len(lines) < 2 {
		t.Errorf("lines = %d, want a wrap", len(lines))
	}
}

// o opens the first link: the title first, then the description.
func TestOpenLinkKey(t *testing.T) {
	var opened string
	orig := openURL
	openURL = func(u string) error { opened = u; return nil }
	defer func() { openURL = orig }()
	m := onRow(t, 1)
	m = send(t, m, key("o"))
	if opened != "" || !strings.Contains(m.status, "no link") {
		t.Fatalf("opened = %q status = %q", opened, m.status)
	}
	m.taskByID("t1").Description = "docs at https://example.com/d"
	m.buildRows(false)
	m = send(t, m, key("o"))
	if opened != "https://example.com/d" {
		t.Errorf("opened = %q", opened)
	}
}
