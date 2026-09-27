package todoist

import (
	"time"
)

// Overlay applies the changes that wait in the queue to s, so that the TUI shows them
// before they are in Todoist. s must be a copy (see Clone): Overlay changes it.
//
// pending has the IDs of the tasks with a change that only Todoist can finish: a quick
// add or a name that Todoist parses, a due text, or a completed recurring task. The TUI
// marks these tasks with ⏳.
func (q *Queue) Overlay(s *SyncState) (pending map[string]bool) {
	pending = map[string]bool{}
	if q == nil {
		return pending
	}
	inbox := ""
	for _, p := range s.Projects {
		if p.InboxProject {
			inbox = p.ID
		}
	}
	find := func(id string) int {
		for i := range s.Tasks {
			if s.Tasks[i].ID == id {
				return i
			}
		}
		return -1
	}
	drop := func(id string) {
		if i := find(id); i >= 0 {
			s.Tasks = append(s.Tasks[:i], s.Tasks[i+1:]...)
		}
	}
	str := func(v any) string { x, _ := v.(string); return x }
	ops := q.Ops()
	// A task that is completed and then opened again (ctrl+z) in the queue stays: Todoist
	// has not completed it yet.
	reopened := map[string]bool{}
	for _, op := range ops {
		if op.Type == "item_uncomplete" {
			reopened[str(op.Args["id"])] = true
		}
	}
	for _, op := range ops {
		switch op.Kind {
		case OpQuick:
			s.Tasks = append(s.Tasks, Task{ID: op.TempID, Content: op.Text, ProjectID: inbox, Priority: 1, ChildOrder: 1 << 20})
			pending[op.TempID] = true
		case OpRename:
			if i := find(op.TaskID); i >= 0 {
				s.Tasks[i].Content = op.Text
				if op.Desc != nil {
					s.Tasks[i].Description = *op.Desc
				}
				pending[op.TaskID] = true
			}
		case OpCommand:
			a := op.Args
			id := str(a["id"])
			switch op.Type {
			case "item_add":
				t := Task{ID: op.TempID, Content: str(a["content"]), ProjectID: str(a["project_id"]), Priority: 1, ChildOrder: 1 << 20}
				if sid := str(a["section_id"]); sid != "" {
					t.SectionID = &sid
				}
				s.Tasks = append(s.Tasks, t)
			case "item_update":
				i := find(id)
				if i < 0 {
					continue
				}
				t := &s.Tasks[i]
				for k, v := range a {
					switch k {
					case "content":
						t.Content = str(v)
					case "description":
						t.Description = str(v)
					case "priority":
						if f, ok := v.(float64); ok {
							t.Priority = int(f)
						} else if n, ok := v.(int); ok {
							t.Priority = n
						}
					case "labels":
						t.Labels = toStrings(v)
					case "is_collapsed":
						t.IsCollapsed, _ = v.(bool)
					case "due":
						due, _ := v.(map[string]any)
						switch {
						case due == nil:
							t.Due = nil
						case str(due["date"]) != "":
							d := Due{Date: str(due["date"]), String: str(due["string"]), Lang: str(due["lang"])}
							d.IsRecurring, _ = due["is_recurring"].(bool)
							if d.String == "" {
								d.String = d.Date
							}
							t.Due = &d
						default: // a due text: only Todoist can parse it
							d := Due{String: str(due["string"])}
							if t.Due != nil {
								d.Date = t.Due.Date // keep the old date until then
							}
							t.Due = &d
							pending[t.ID] = true
						}
					}
				}
			case "item_move":
				i := find(id)
				if i < 0 {
					continue
				}
				t := &s.Tasks[i]
				switch {
				case str(a["parent_id"]) != "":
					pid := str(a["parent_id"])
					t.ParentID = &pid
					if p := find(pid); p >= 0 {
						t.ProjectID, t.SectionID = s.Tasks[p].ProjectID, s.Tasks[p].SectionID
					}
				case str(a["section_id"]) != "":
					sid := str(a["section_id"])
					t.SectionID, t.ParentID = &sid, nil
					for _, sec := range s.Sections {
						if sec.ID == sid {
							t.ProjectID = sec.ProjectID
						}
					}
				case str(a["project_id"]) != "":
					t.ProjectID, t.SectionID, t.ParentID = str(a["project_id"]), nil, nil
				}
			case "item_reorder":
				items, _ := a["items"].([]any)
				for _, it := range items {
					m, _ := it.(map[string]any)
					if i := find(str(m["id"])); i >= 0 {
						if f, ok := m["child_order"].(float64); ok {
							s.Tasks[i].ChildOrder = int(f)
						} else if n, ok := m["child_order"].(int); ok {
							s.Tasks[i].ChildOrder = n
						}
					}
				}
			case "item_close":
				if i := find(id); i >= 0 && !reopened[id] {
					if s.Tasks[i].Due != nil && s.Tasks[i].Due.IsRecurring {
						pending[id] = true // Todoist moves it to the next date
					} else {
						drop(id)
					}
				}
			case "item_delete":
				drop(id)
			case "note_add":
				s.Comments = append(s.Comments, Comment{ID: op.TempID, TaskID: str(a["item_id"]), Content: str(a["content"]),
					PostedAt: op.Added.UTC().Format(time.RFC3339)})
			case "note_update":
				for i := range s.Comments {
					if s.Comments[i].ID == id {
						s.Comments[i].Content = str(a["content"])
					}
				}
			case "note_delete":
				for i := range s.Comments {
					if s.Comments[i].ID == id {
						s.Comments = append(s.Comments[:i], s.Comments[i+1:]...)
						break
					}
				}
			}
		}
	}
	return pending
}

// toStrings converts a JSON list (or a []string) to a []string.
func toStrings(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
