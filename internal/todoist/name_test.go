package todoist

import (
	"slices"
	"testing"
)

func TestParseName(t *testing.T) {
	projects := []Project{{ID: "p1", Name: "work"}, {ID: "p2", Name: "work stuff"}, {ID: "in", Name: "Inbox"}}
	sections := []Section{{ID: "s1", ProjectID: "p1", Name: "urgent"}, {ID: "s2", ProjectID: "in", Name: "later"}}
	for _, c := range []struct {
		text, cur           string
		content, proj, sect string
		labels              []string
		prio                int
	}{
		{"pay rent #work p1 @home", "in", "pay rent", "p1", "", []string{"home"}, 4},
		{"call bob #work stuff", "in", "call bob", "p2", "", nil, 0},
		{"fix /urgent #work", "in", "fix", "p1", "s1", nil, 0},
		{"read /later", "in", "read", "in", "s2", nil, 0},
		{"issue #42 and #nope stay", "in", "issue #42 and #nope stay", "", "", nil, 0},
		{"buy milk tomorrow 9am", "in", "buy milk tomorrow 9am", "", "", nil, 0},
		{"p5 and top1 stay", "in", "p5 and top1 stay", "", "", nil, 0},
		{"mail me@x.com", "in", "mail me@x.com", "", "", nil, 0},
	} {
		p := ParseName(c.text, projects, sections, c.cur)
		if p.Content != c.content || p.ProjectID != c.proj || p.SectionID != c.sect || !slices.Equal(p.Labels, c.labels) || p.Priority != c.prio {
			t.Errorf("%q: %+v", c.text, p)
		}
	}
}
