package todoist

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// A name that the user edits can have #project, /section, @label, and p1–p4, as in quick
// add. godoist reads them itself: the Todoist parser needs a temporary task, and the Google
// Calendar sync of Todoist can keep an event of that task. Dates in the name are not read:
// the date dialog sets dates.

// NameParts is what ParseName reads in a name.
type NameParts struct {
	Content   string   // the name without the parts below
	ProjectID string   // the project of a #project, or ""
	SectionID string   // the section of a /section, or ""
	Labels    []string // the names of the @labels
	Priority  int      // the API priority of a p1–p4 (4 is p1), or 0
}

var (
	priorityWord = regexp.MustCompile(`(?i)(^|\s)p([1-4])(\s|$)`)
	labelWord    = regexp.MustCompile(`(^|\s)@([^\s@#/]+)`)
)

// ParseName reads #project, /section, @label, and p1–p4 in text. A #word that is not a
// project, or a /word that is not a section of the project, stays in the name. A section
// is looked for in the named project, or else in curProject.
func ParseName(text string, projects []Project, sections []Section, curProject string) NameParts {
	var p NameParts
	// A longer name first, so that "#work stuff" wins over "#work".
	byLen := func(names []string) []string {
		sort.SliceStable(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
		return names
	}
	cut := func(mark, name string) bool { // removes mark+name if it is a whole word
		low, tag := strings.ToLower(text), strings.ToLower(mark+name)
		for i := strings.Index(low, tag); i >= 0; {
			end := i + len(tag)
			before := i == 0 || strings.ContainsRune(" \t", rune(low[i-1]))
			after := end == len(low) || strings.ContainsRune(" \t,.;:!?", rune(low[end]))
			if before && after {
				text = text[:i] + text[end:]
				return true
			}
			next := strings.Index(low[end:], tag)
			if next < 0 {
				break
			}
			i = end + next
		}
		return false
	}
	names := map[string]string{}
	for _, pr := range projects {
		names[pr.Name] = pr.ID
	}
	for _, n := range byLen(mapKeys(names)) {
		if cut("#", n) {
			p.ProjectID = names[n]
			break
		}
	}
	inProject := p.ProjectID
	if inProject == "" {
		inProject = curProject
	}
	secs := map[string]string{}
	for _, s := range sections {
		if s.ProjectID == inProject {
			secs[s.Name] = s.ID
		}
	}
	for _, n := range byLen(mapKeys(secs)) {
		if cut("/", n) {
			p.SectionID = secs[n]
			if p.ProjectID == "" {
				p.ProjectID = inProject
			}
			break
		}
	}
	for _, m := range labelWord.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(p.Labels, m[2]) {
			p.Labels = append(p.Labels, m[2])
		}
	}
	text = labelWord.ReplaceAllString(text, "$1")
	if m := priorityWord.FindStringSubmatch(text); m != nil {
		p.Priority = 5 - int(m[2][0]-'0')
		text = priorityWord.ReplaceAllString(text, " ")
	}
	p.Content = strings.Join(strings.Fields(text), " ")
	return p
}

func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ApplyName gives task cur a new name. ParseName reads #project, /section, @label, and
// p1–p4 in text, and they change only those fields: a label adds to the labels of the task,
// and a project or a section moves the task. desc, if not nil, is the new description.
// ApplyName returns what it read and the task fields that changed.
func (c *Client) ApplyName(ctx context.Context, s *SyncState, cur Task, text string, desc *string) (NameParts, map[string]any, error) {
	p := ParseName(text, s.Projects, s.Sections, cur.ProjectID)
	if p.Content == "" {
		return p, nil, fmt.Errorf("the name is empty")
	}
	fields := map[string]any{"content": p.Content}
	if desc != nil {
		fields["description"] = *desc
	}
	if p.Priority > 0 && p.Priority != cur.Priority {
		fields["priority"] = p.Priority
	}
	if len(p.Labels) > 0 {
		labels := append([]string(nil), cur.Labels...)
		for _, l := range p.Labels {
			if !slices.Contains(labels, l) {
				labels = append(labels, l)
			}
		}
		fields["labels"] = labels
	}
	if _, err := c.UpdateTask(ctx, cur.ID, fields); err != nil {
		return p, fields, err
	}
	if (p.ProjectID != "" && p.ProjectID != cur.ProjectID) || (p.SectionID != "" && p.SectionID != cur.Section()) {
		if err := c.Move(ctx, cur.ID, p.ProjectID, p.SectionID); err != nil {
			return p, fields, fmt.Errorf("renamed, but the move failed: %w", err)
		}
	}
	return p, fields, nil
}
