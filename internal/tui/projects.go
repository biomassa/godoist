package tui

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// navProject returns the project of the sidebar item under the cursor, or nil.
func (m Model) navProject() *todoist.Project {
	cur := m.currentNav()
	if cur == nil || cur.kind != vkProject {
		return nil
	}
	return m.projects[cur.projectID]
}

// projectKeys handles the project keys in the sidebar. ok is false for other keys.
func (m Model) projectKeys(key string) (tea.Model, tea.Cmd, bool) {
	if key == "A" {
		next := m.openDialog(inputAddProject, "")
		return m, next, true
	}
	switch key {
	case "e", "C", "*", "[", "]", ">", "<", "m", "delete", "backspace":
	default:
		return m, nil, false
	}
	p := m.navProject()
	if p == nil {
		m.setStatus("move the cursor to a project", true)
		return m, nil, true
	}
	if p.InboxProject {
		m.setStatus("the Inbox cannot be renamed, colored, moved, or deleted", true)
		return m, nil, true
	}
	var next tea.Cmd
	switch key {
	case "e":
		m.targetProject = p.ID
		next = m.openDialog(inputRenameProject, p.Name)
	case "C":
		next = m.openColorPicker(p.ID, "", "")
	case "*":
		next = m.toggleFavorite(p)
	case "[", "]":
		d := -1
		if key == "]" {
			d = 1
		}
		next = m.moveProject(p, d)
	case ">":
		next = m.indentProject(p)
	case "<":
		next = m.outdentProject(p)
	case "m":
		next = m.openParentPicker(p)
	case "delete", "backspace":
		m.askDeleteProject(p)
	}
	return m, next, true
}

// projectTaskCount counts the tasks of a project and of its sub-projects.
func (m Model) projectTaskCount(id string) (tasks, subs int) {
	ids := map[string]bool{id: true}
	for changed := true; changed; { // collect the sub-projects at every depth
		changed = false
		for _, p := range m.snap.Projects {
			if p.ParentID != nil && ids[*p.ParentID] && !ids[p.ID] {
				ids[p.ID], changed = true, true
				subs++
			}
		}
	}
	for _, t := range m.snap.Tasks {
		if ids[t.ProjectID] {
			tasks++
		}
	}
	return tasks, subs
}

// askDeleteProject opens a dialog: d deletes the project, a archives it, esc cancels.
func (m *Model) askDeleteProject(p *todoist.Project) {
	n, subs := m.projectTaskCount(p.ID)
	what := fmt.Sprintf("%d task(s)", n)
	if subs > 0 {
		what += fmt.Sprintf(" and %d sub-project(s)", subs)
	}
	text := fmt.Sprintf("Delete or archive “%s”?\n\nDelete removes the project with its %s. This cannot be undone.\nArchive hides the project and keeps everything.", p.Name, what)
	id, name, client := p.ID, p.Name, m.client
	m.confirm = &confirmPrompt{title: "Delete or archive project", text: text, buttons: []confirmButton{
		{key: "d", label: "Delete", danger: true, run: func(m *Model) tea.Cmd {
			return m.simpleWrite("Deleted project “"+name+"”", func(ctx context.Context) error { return client.DeleteProject(ctx, id) })
		}},
		{key: "a", label: "Archive", run: func(m *Model) tea.Cmd {
			return m.simpleWrite("Archived project “"+name+"”", func(ctx context.Context) error { return client.ArchiveProject(ctx, id) })
		}},
		{key: "esc", label: "Cancel"},
	}}
}

// toggleFavorite adds the project to Favorites or removes it. The sidebar changes at once.
func (m *Model) toggleFavorite(p *todoist.Project) tea.Cmd {
	fav := !p.IsFavorite
	p.IsFavorite = fav
	m.buildNav()
	id, name, client := p.ID, p.Name, m.client
	done := "Added “" + name + "” to Favorites"
	if !fav {
		done = "Removed “" + name + "” from Favorites"
	}
	return m.simpleWrite(done, func(ctx context.Context) error {
		return client.UpdateProject(ctx, id, map[string]any{"is_favorite": fav})
	})
}

// siblings returns the IDs of the projects with the same parent as p, in their order.
func (m Model) siblings(p *todoist.Project) []string {
	var sib []todoist.Project
	for _, q := range m.snap.Projects {
		if q.InboxProject || q.IsArchived {
			continue
		}
		if (q.ParentID == nil) == (p.ParentID == nil) && (p.ParentID == nil || *q.ParentID == *p.ParentID) {
			sib = append(sib, q)
		}
	}
	sort.SliceStable(sib, func(i, j int) bool { return sib[i].ChildOrder < sib[j].ChildOrder })
	ids := make([]string, len(sib))
	for i, q := range sib {
		ids[i] = q.ID
	}
	return ids
}

// moveProject moves a project d places among its siblings. The sidebar changes at once.
func (m *Model) moveProject(p *todoist.Project, d int) tea.Cmd {
	order := m.siblings(p)
	i := slices.Index(order, p.ID)
	j := i + d
	if i < 0 || j < 0 || j >= len(order) {
		m.setStatus("the project is already at the edge", false)
		return nil
	}
	order[i], order[j] = order[j], order[i]
	for k := range m.snap.Projects {
		if n := slices.Index(order, m.snap.Projects[k].ID); n >= 0 {
			m.snap.Projects[k].ChildOrder = n + 1
		}
	}
	m.buildNav() // keeps the selection on the project
	client, name := m.client, p.Name
	return m.simpleWrite("Moved project “"+name+"”", func(ctx context.Context) error {
		return client.ReorderProjects(ctx, order)
	})
}

// renameProject saves a new project name as typed.
func (m *Model) renameProject(name string) tea.Cmd {
	p := m.projects[m.targetProject]
	if p == nil || name == "" || name == p.Name {
		return nil
	}
	id, client := p.ID, m.client
	p.Name = name
	m.buildNav()
	cmd := m.simpleWrite("Renamed project to “"+name+"”", func(ctx context.Context) error {
		return client.UpdateProject(ctx, id, map[string]any{"name": name})
	})
	return withRetry(cmd, inputRenameProject, name)
}

// startNewProject takes the name from the dialog and opens the color picker. If the cursor
// is on a project other than the Inbox, the picker offers it as the parent.
func (m *Model) startNewProject(name string) tea.Cmd {
	if name == "" {
		return nil
	}
	parent := ""
	if p := m.navProject(); p != nil && !p.InboxProject {
		parent = p.ID
	}
	return m.openColorPicker("", name, parent)
}

// colorLabel is the display name of a Todoist color, e.g. "berry red".
func colorLabel(name string) string { return strings.ReplaceAll(name, "_", " ") }

// openColorPicker opens the color picker. With projectID, it changes the color of that
// project. With newName, it creates a project; parentID adds the sub-project row.
func (m *Model) openColorPicker(projectID, newName, parentID string) tea.Cmd {
	current := "charcoal"
	title := "New project · " + newName
	if p := m.projects[projectID]; p != nil {
		current, title = p.Color, "Color · "+p.Name
	}
	var items []pickItem
	if parentID != "" {
		items = append(items, pickItem{text: "sub-project of # " + m.projects[parentID].Name, toggleRow: true,
			color: todoist.ColorHex(m.projects[parentID].Color), projectID: parentID})
	}
	cur := 0
	for _, name := range todoist.ColorNames {
		if name == current {
			cur = len(items)
		}
		items = append(items, pickItem{text: colorLabel(name), match: colorLabel(name), color: todoist.ColorHex(name), colorName: name})
	}
	m.pick = &picker{kind: pickColor, title: title, items: items, filter: newPickerFilter(), cur: cur,
		projectID: projectID, newName: newName}
	m.pick.filter.Placeholder = "type to filter, e.g. blue"
	m.pick.filter.SetWidth(pickerFilterWidth(m.pickerWidth()))
	return m.pick.filter.Focus()
}

// pickColorEnter creates the project or saves the color under the cursor. On the
// sub-project row, enter switches the row instead.
func (m *Model) pickColorEnter(it pickItem) tea.Cmd {
	p := m.pick
	if it.toggleRow {
		p.toggleSub()
		return nil
	}
	m.pick = nil
	client, color := m.client, it.colorName
	if p.labelID != "" {
		return m.saveLabelColor(p.labelID, color)
	}
	if p.newName != "" {
		parent := ""
		for _, x := range p.items {
			if x.toggleRow && x.checked {
				parent = x.projectID
			}
		}
		name := p.newName
		cmd := m.simpleWrite("Added project “"+name+"”", func(ctx context.Context) error {
			_, err := client.AddProject(ctx, name, color, parent)
			return err
		})
		return withRetry(cmd, inputAddProject, name)
	}
	id := p.projectID
	if pr := m.projects[id]; pr != nil {
		pr.Color = color // show the new color at once. The sync confirms it.
		m.buildNav()
	}
	return m.simpleWrite("Color → "+colorLabel(color), func(ctx context.Context) error {
		return client.UpdateProject(ctx, id, map[string]any{"color": color})
	})
}

// toggleSub switches the sub-project row of the color picker.
func (p *picker) toggleSub() {
	for i := range p.items {
		if p.items[i].toggleRow {
			p.items[i].checked = !p.items[i].checked
		}
	}
}

// projectMenu is the right-click menu of a sidebar project.
var projectMenu = []menuItem{
	{"New project", "A"}, {"Rename", "e"}, {"Color", "C"}, {"Favorite on / off", "*"},
	{"Move up", "["}, {"Move down", "]"}, {"Indent", ">"}, {"Outdent", "<"}, {"Move under…", "m"},
	{"Delete or archive", "delete"},
}

// childProjects returns the IDs of the sub-projects of parentID ("" for the top level),
// in their order. The Inbox is not in the list.
func (m Model) childProjects(parentID string) []string {
	var ps []todoist.Project
	for _, q := range m.snap.Projects {
		if q.InboxProject || q.IsArchived {
			continue
		}
		if (parentID == "" && q.ParentID == nil) || (q.ParentID != nil && *q.ParentID == parentID) {
			ps = append(ps, q)
		}
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].ChildOrder < ps[j].ChildOrder })
	ids := make([]string, len(ps))
	for i, q := range ps {
		ids[i] = q.ID
	}
	return ids
}

// isInside reports whether project id is p or one of its sub-projects at any depth.
func (m Model) isInside(id, p string) bool {
	for id != "" {
		if id == p {
			return true
		}
		q := m.projects[id]
		if q == nil || q.ParentID == nil {
			return false
		}
		id = *q.ParentID
	}
	return false
}

// reparent moves project p under parentID ("" for the top level) at position pos of the
// new siblings (-1 for the end). The sidebar changes at once. The sub-projects move along.
func (m *Model) reparent(p *todoist.Project, parentID string, pos int, done string) tea.Cmd {
	if parentID != "" && m.isInside(parentID, p.ID) {
		m.setStatus("a project cannot go inside itself", true)
		return nil
	}
	order := slices.DeleteFunc(m.childProjects(parentID), func(id string) bool { return id == p.ID })
	if pos < 0 || pos > len(order) {
		pos = len(order)
	}
	order = slices.Insert(order, pos, p.ID)
	if parentID == "" {
		p.ParentID = nil
	} else {
		pp := parentID
		p.ParentID = &pp
	}
	for k := range m.snap.Projects {
		if n := slices.Index(order, m.snap.Projects[k].ID); n >= 0 {
			m.snap.Projects[k].ChildOrder = n + 1
		}
	}
	if q := m.projects[parentID]; q != nil {
		q.IsCollapsed = false // show the project in its new place
	}
	m.buildNav()
	id, client := p.ID, m.client
	return m.simpleWrite(done, func(ctx context.Context) error {
		if err := client.MoveProject(ctx, id, parentID); err != nil {
			return err
		}
		return client.ReorderProjects(ctx, order)
	})
}

// indentProject makes p a sub-project of the project above it at the same level.
func (m *Model) indentProject(p *todoist.Project) tea.Cmd {
	sib := m.siblings(p)
	i := slices.Index(sib, p.ID)
	if i <= 0 {
		m.setStatus("no project above at the same level", false)
		return nil
	}
	above := m.projects[sib[i-1]]
	return m.reparent(p, above.ID, -1, "Moved “"+p.Name+"” under “"+above.Name+"”")
}

// outdentProject moves p one level up, right after its old parent.
func (m *Model) outdentProject(p *todoist.Project) tea.Cmd {
	if p.ParentID == nil {
		m.setStatus("already at the top level", false)
		return nil
	}
	parent := m.projects[*p.ParentID]
	if parent == nil {
		return nil
	}
	up := ""
	if parent.ParentID != nil {
		up = *parent.ParentID
	}
	pos := slices.Index(m.childProjects(up), parent.ID) + 1
	return m.reparent(p, up, pos, "Moved “"+p.Name+"” one level up")
}

// moveProjectUnder makes p the last sub-project of parentID, or the last top-level project.
func (m *Model) moveProjectUnder(p *todoist.Project, parentID string) tea.Cmd {
	cur := ""
	if p.ParentID != nil {
		cur = *p.ParentID
	}
	if cur == parentID {
		m.setStatus("the project is already there", false)
		return nil
	}
	done := "Moved “" + p.Name + "” to the top level"
	if q := m.projects[parentID]; q != nil {
		done = "Moved “" + p.Name + "” under “" + q.Name + "”"
	}
	return m.reparent(p, parentID, -1, done)
}

// openParentPicker opens the list of possible parents: the top level and every project
// that is not p or inside p.
func (m *Model) openParentPicker(p *todoist.Project) tea.Cmd {
	items := []pickItem{{text: "⌂ top level", match: "top level", color: hexMuted}}
	for _, op := range m.orderedProjects() {
		q := op.p
		if q.InboxProject || m.isInside(q.ID, p.ID) {
			continue
		}
		items = append(items, pickItem{text: "# " + q.Name, match: strings.ToLower(q.Name),
			color: todoist.ColorHex(q.Color), depth: op.depth, projectID: q.ID})
	}
	m.pick = &picker{kind: pickParent, title: "Move under · " + p.Name, items: items, filter: newPickerFilter(), projectID: p.ID}
	m.pick.filter.Placeholder = "type to filter, e.g. work"
	m.pick.filter.SetWidth(pickerFilterWidth(m.pickerWidth()))
	return m.pick.filter.Focus()
}
