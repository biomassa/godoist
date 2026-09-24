// Package tui implements the three-pane godoist terminal UI.
package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/cache"
	"github.com/biomassa/godoist/internal/state"
	"github.com/biomassa/godoist/internal/todoist"
)

// autoSyncEvery is the interval of the background sync. A sync also runs at start,
// after each write, when the terminal gets focus, and when the user pushes r.
const autoSyncEvery = 60 * time.Second

// viewKind is the type of the sidebar item that the task list shows.
type viewKind int

const (
	vkToday viewKind = iota
	vkUpcoming
	vkProject
	vkFilter
	vkAll       // all tasks, grouped by project
	vkLabel     // tasks with one label, grouped by project
	vkCompleted // tasks completed in the last 30 days, grouped by project
	vkLabelHint // the "A adds a label" line when there are no labels
)

// pane identifies a focusable pane.
type pane int

const (
	paneNav pane = iota
	paneTasks
	paneDetail
)

// inputMode is the use of the one-line input in the bottom bar.
type inputMode int

const (
	inputNone inputMode = iota
	inputAdd
	inputAddNote
	inputFind
	inputQuery
	inputRename
	inputAddSection
	inputRenameSection
	inputAddProject
	inputRenameProject
	inputAddSubtask
	inputAddLabel
	inputRenameLabel
)

// navItem is a sidebar row. A row with a header is a label and the cursor skips it.
type navItem struct {
	header    string
	kind      viewKind
	projectID string
	name      string
	glyph     string
	color     string
	count     int
	countHex  string
	depth     int
	labelID   string // vkLabel
	hasKids   bool   // a project with sub-projects
	collapsed bool   // its sub-projects are hidden
	// tree is the tree prefix of a project in My Projects: │ ├ └ lines for sub-projects,
	// and empty for top-level projects. The ▾/▸ marker goes after the name.
	tree   string
	inTree bool
}

// row is a line in the task pane: a group header, a task, or a note's preview line.
type row struct {
	header    string
	headerHex string
	count     int
	sectionID string // for headers in project view
	date      string // YYYY-MM-DD for day headers in Today/Upcoming
	task      *todoist.Task
	depth     int
	preview   string // non-empty: preview line under a note (never selected on its own)
	spacer    bool   // an empty line before a header (never selected)
	hasKids   bool   // a task with sub-tasks, or a section header with tasks
	collapsed bool   // its sub-tasks or its tasks are hidden
	hidden    int    // the number of hidden tasks when collapsed
	done      bool   // a completed task (Completed view)
	pulled    bool   // an overview parent that is only there because of its sub-tasks
}

type (
	syncMsg struct {
		st  todoist.SyncState
		err error
	}
	filterMsg struct {
		query string
		tasks []todoist.Task
		err   error
	}
	// actionMsg is the result of a write. Each write sends exactly one.
	actionMsg struct {
		text string
		err  error
		// retryMode and retryText reopen the input with the typed text after a failed add.
		retryMode inputMode
		retryText string
	}
	autoSyncMsg  struct{}
	completedMsg struct {
		tasks []todoist.Task
		err   error
	}
)

// Model is the state of the TUI.
type Model struct {
	client *todoist.Client
	token  string

	st       todoist.SyncState
	snap     todoist.Snapshot
	projects map[string]*todoist.Project
	sections map[string]*todoist.Section
	loaded   bool
	syncing  int  // 1 while a sync runs
	resync   bool // a sync was requested while one was running
	ui       state.State

	pending  int  // writes in flight
	quitting bool // the user quit. The program waits for the pending writes.

	width, height int
	focus         pane
	showHelp      bool
	detailOpen    bool // narrow terminals: details replace the task list

	nav            []navItem
	navCur, navOff int

	rows           []row
	rowCur, rowOff int

	detOff int // details pane scroll
	comCur int // selected comment in the details pane, -1 = none
	chkCur int // selected checkbox of the note in the reader, -1 = none

	filterQuery    string
	filterTasks    []todoist.Task
	focusFilterNav bool
	preFilterNav   string // navKey of the view before the filter view, for esc
	find           string
	targetID       string // task that inputRename changes
	targetSection  string // section that inputRenameSection changes
	targetProject  string // project that inputRenameProject changes
	targetLabel    string // label that inputRenameLabel changes

	input     textinput.Model // bottom-bar input (find)
	dlg       textarea.Model  // dialog input (all other text inputs)
	inputMode inputMode

	editor textarea.Model
	edit   *editSession
	note   *noteEditor // inline markdown editor of notebook view
	pick   *picker

	confirm *confirmPrompt
	cal     *calDialog // date dialog

	completed        []todoist.Task // Completed view, last 30 days
	completedLoading bool
	selected         map[string]bool // multi-select: task IDs
	lastMark         string          // the task that s or a click marked last, for ranges
	menu             *ctxMenu        // right-click menu
	drag             *dragState      // task that the user drags with the mouse
	lastClick        clickInfo

	status    string
	statusErr bool
	closed    [][]string // undo stack: each entry is the one-time tasks of one completion

	md *mdCache
}

// New returns a Model. If a cache exists for the token, the first frame shows the cached data.
func New(client *todoist.Client, token string) Model {
	ti := textinput.New()
	ti.CharLimit = 500
	m := Model{client: client, token: token, input: ti, comCur: -1, chkCur: -1, md: newMDCache()}
	m.ui, _ = state.Load()
	m.syncing = 1 // Init starts the first sync
	if st := cache.Load(token); st.Token != "" {
		m.applyState(st) // show the cached data at once. Init starts a sync.
	}
	return m
}

// Init starts the first sync, asks for the terminal background color, and starts the sync timer.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.syncCmd(), tea.RequestBackgroundColor, autoSyncTick())
}

func autoSyncTick() tea.Cmd {
	return tea.Tick(autoSyncEvery, func(time.Time) tea.Msg { return autoSyncMsg{} })
}

// startSync starts a sync. Only one sync runs at a time: each sync works on a copy of the
// replica, and a sync that ends late would overwrite newer data. If a sync is running,
// startSync sets resync, and the syncMsg handler starts one more sync when it arrives.
func (m *Model) startSync() tea.Cmd {
	if m.syncing > 0 {
		m.resync = true
		return nil
	}
	m.syncing = 1
	return m.syncCmd()
}

// syncCmd syncs a copy of the replica and saves it to the cache.
func (m Model) syncCmd() tea.Cmd {
	st := m.st.Clone()
	client, token := m.client, m.token
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := client.Sync(ctx, &st); err != nil {
			return syncMsg{err: err}
		}
		_ = cache.Save(token, st)
		return syncMsg{st: st}
	}
}

// runFilter runs a Todoist filter query. Todoist rejects plain words, so if the query
// fails and does not start with "search:", runFilter tries "search: <query>" once.
func (m Model) runFilter(q string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		t, err := client.Filter(ctx, q)
		var apiErr *todoist.APIError
		if errors.As(err, &apiErr) && apiErr.Status == 400 && !strings.HasPrefix(strings.ToLower(q), "search:") {
			sq := "search: " + q
			if t2, err2 := client.Filter(ctx, sq); err2 == nil {
				return filterMsg{query: sq, tasks: t2}
			}
		}
		return filterMsg{query: q, tasks: t, err: err}
	}
}

// write runs an API change and counts it as pending, so that quit waits for it.
// The actionMsg handler starts a sync after the change.
func (m *Model) write(done string, f func(context.Context) (string, error)) tea.Cmd {
	m.pending++
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		text, err := f(ctx)
		if text == "" {
			text = done
		}
		return actionMsg{text: text, err: err}
	}
}

func (m *Model) simpleWrite(done string, f func(context.Context) error) tea.Cmd {
	return m.write(done, func(ctx context.Context) (string, error) { return "", f(ctx) })
}

func (m *Model) setStatus(s string, isErr bool) { m.status, m.statusErr = s, isErr }

// Update handles a message. If the task under the cursor changes, the details pane goes back to the top.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	prev := m.currentTaskID()
	prevView := ""
	if cur := m.currentNav(); cur != nil {
		prevView = navKey(*cur)
	}
	nm, cmd := m.update(msg)
	mm := nm.(Model)
	if mm.currentTaskID() != prev {
		mm.detOff, mm.comCur, mm.chkCur = 0, -1, -1
	}
	if cur := mm.currentNav(); cur != nil && navKey(*cur) != prevView {
		if mm.clearSelection() {
			mm.setStatus("selection cleared", false)
		}
		if cur.kind == vkCompleted {
			cmd = tea.Batch(cmd, mm.loadCompleted())
		}
	}
	if _, ok := msg.(syncMsg); ok {
		mm.pruneSelection()
	}
	mm.fixScroll()
	return mm, cmd
}

// update handles one message.
//
// Some commands change m when they are made (for example, write counts a pending write).
// The code makes such a command in a local variable before "return m, next". Go does not
// specify the order in which it evaluates the operands of a return statement.
func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(max(10, m.width-4))
		m.sizeEditor()
		return m, nil

	case tea.BackgroundColorMsg:
		setTheme(msg.IsDark())
		r, g, b, _ := msg.RGBA()
		baseBg = fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
		m.md.reset()
		if m.loaded {
			m.buildNav()
			m.buildRows(false)
		}
		return m, nil

	case autoSyncMsg:
		if m.syncing > 0 {
			return m, autoSyncTick()
		}
		next := tea.Batch(m.startSync(), autoSyncTick())
		return m, next

	case tea.FocusMsg:
		if m.syncing > 0 {
			return m, nil
		}
		next := m.startSync()
		return m, next

	case syncMsg:
		m.syncing = 0
		var cmds []tea.Cmd
		if m.resync {
			m.resync = false
			cmds = append(cmds, m.startSync())
		}
		if msg.err != nil {
			m.setStatus("sync failed: "+msg.err.Error(), true)
			return m, tea.Batch(cmds...)
		}
		m.applyState(msg.st)
		if m.status == "syncing…" { // the sync marker at the right now shows the time
			m.setStatus("", false)
		}
		if m.filterQuery != "" {
			cmds = append(cmds, m.runFilter(m.filterQuery))
		}
		return m, tea.Batch(cmds...)

	case filterMsg:
		if msg.err != nil {
			m.setStatus("filter failed: "+msg.err.Error()+" · see the examples in the dialog", true)
			if m.focusFilterNav && m.inputMode == inputNone && m.edit == nil && m.pick == nil {
				// The dialog opens again with the query, so the user can correct it.
				m.focusFilterNav = false
				next := m.openDialog(inputQuery, msg.query)
				return m, next
			}
			return m, nil
		}
		m.filterQuery, m.filterTasks = msg.query, msg.tasks
		m.buildNav()
		if m.focusFilterNav {
			if cur := m.currentNav(); cur != nil && cur.kind != vkFilter {
				m.preFilterNav = navKey(*cur)
			}
			m.selectNav(func(n navItem) bool { return n.kind == vkFilter })
			m.focus = paneTasks
			m.focusFilterNav = false
		}
		m.buildRows(false)
		return m, nil

	case actionMsg:
		m.pending = max(0, m.pending-1)
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		} else if msg.text != "" {
			m.setStatus(msg.text, false)
		}
		if m.quitting && m.pending == 0 {
			return m, tea.Quit
		}
		cmds := []tea.Cmd{m.startSync()}
		if msg.err != nil && msg.retryMode != inputNone && m.inputMode == inputNone && m.edit == nil && !m.quitting {
			cmds = append(cmds, m.openAdd(msg.retryMode, msg.retryText))
		}
		return m, tea.Batch(cmds...)

	case editSavedMsg:
		return m.editSaved(msg)

	case calParsedMsg:
		return m.calParsed(msg)

	case noteTickMsg:
		return m.noteTick(msg)

	case noteSavedMsg:
		return m.noteSaved(msg)

	case tea.PasteMsg:
		if m.note != nil {
			m.note.push(false)
			m.note.insertText(msg.Content)
			return m, m.note.changed()
		}

	case completedMsg:
		m.completedLoading = false
		if msg.err != nil {
			m.setStatus("could not load completed tasks: "+msg.err.Error(), true)
			return m, nil
		}
		m.completed = msg.tasks
		if cur := m.currentNav(); cur != nil && cur.kind == vkCompleted {
			m.buildRows(false)
		}
		return m, nil

	case calSavedMsg:
		return m.calSaved(msg)

	case editorDoneMsg:
		return m.externalEditorDone(msg)

	case tea.MouseClickMsg:
		return m.mouseClick(msg.Mouse())
	case tea.MouseReleaseMsg:
		return m.mouseRelease(msg.Mouse())
	case tea.MouseMotionMsg:
		return m.mouseMotion(msg.Mouse())
	case tea.MouseWheelMsg:
		return m.mouseWheel(msg.Mouse())

	case tea.KeyPressMsg:
		switch {
		case m.quitting:
			if msg.String() == "ctrl+c" { // second ctrl+c: quit without waiting
				return m, tea.Quit
			}
			return m, nil
		case m.cal != nil:
			return m.updateCalendar(msg)
		case m.menu != nil:
			return m.updateMenu(msg)
		case m.pick != nil:
			return m.updatePicker(msg)
		case m.note != nil:
			return m.updateNoteEditor(msg)
		case m.edit != nil:
			return m.updateEditor(msg)
		case m.confirm != nil:
			return m.answerConfirm(msg.String())
		case m.inputMode != inputNone:
			return m.updateInput(msg)
		}
		return m.updateKeys(msg)
	}
	if m.edit != nil { // cursor blink etc.
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	}
	return m, nil
}

// quit stops the program. If writes are pending, it waits for them first.
func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.pending == 0 {
		return m, tea.Quit
	}
	m.quitting = true
	m.setStatus(fmt.Sprintf("saving %d change(s) before quitting… (ctrl+c to force)", m.pending), false)
	return m, nil
}

func (m Model) updateInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.inputMode == inputFind {
			m.find = ""
			m.buildRows(true)
		}
		m.closeInput()
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if isDialog(m.inputMode) {
			val = m.dialogValue()
		}
		mode := m.inputMode
		m.closeInput()
		switch mode {
		case inputAdd:
			if val != "" {
				next := withRetry(m.quickAdd(val), inputAdd, val)
				return m, next
			}
		case inputAddNote:
			if val != "" {
				next := m.addNote(val)
				if next != nil {
					next = withRetry(next, inputAddNote, val)
				}
				return m, next
			}
		case inputRename:
			next := m.rename(val)
			return m, next
		case inputAddSection:
			next := m.addSection(val)
			return m, next
		case inputAddProject:
			next := m.startNewProject(val)
			return m, next
		case inputAddSubtask:
			next := m.addSubtask(val)
			return m, next
		case inputAddLabel:
			next := m.addLabel(val)
			return m, next
		case inputRenameLabel:
			next := m.renameLabel(val)
			return m, next
		case inputRenameProject:
			next := m.renameProject(val)
			return m, next
		case inputRenameSection:
			next := m.renameSection(val)
			return m, next
		case inputQuery:
			if val == "" && m.filterQuery != "" {
				// An empty query removes the filter view from the sidebar.
				m.clearFilter()
				return m, nil
			}
			if val != "" {
				m.focusFilterNav = true
				m.setStatus("filtering…", false)
				return m, m.runFilter(val)
			}
		case inputFind:
			m.find = val
			m.buildRows(true)
		}
		return m, nil
	}
	var cmd tea.Cmd
	if isDialog(m.inputMode) {
		m.dlg, cmd = m.dlg.Update(msg)
		return m, cmd
	}
	m.input, cmd = m.input.Update(msg)
	if m.inputMode == inputFind {
		m.find = m.input.Value()
		m.buildRows(true)
	}
	return m, cmd
}

// openInput opens the find input in the bottom bar, or the dialog for the other modes.
func (m *Model) openInput(mode inputMode, prompt, placeholder, value string) tea.Cmd {
	if isDialog(mode) {
		return m.openDialog(mode, value)
	}
	m.inputMode = mode
	m.input.Prompt = prompt
	m.input.Placeholder = placeholder
	m.input.SetValue(value)
	m.input.CursorEnd()
	return m.input.Focus()
}

// openAdd opens the dialog for a new task, a new note, a new name, or a due date.
func (m *Model) openAdd(mode inputMode, text string) tea.Cmd {
	return m.openDialog(mode, text)
}

// withRetry makes a failed add reopen the input with the typed text.
func withRetry(cmd tea.Cmd, mode inputMode, text string) tea.Cmd {
	return func() tea.Msg {
		msg := cmd()
		if a, ok := msg.(actionMsg); ok && a.err != nil {
			a.retryMode, a.retryText = mode, text
			return a
		}
		return msg
	}
}

func (m *Model) closeInput() {
	m.inputMode = inputNone
	m.input.Blur()
	m.input.SetValue("")
	m.dlg = textarea.Model{}
}

// namesProject reports whether text contains "#<name>" for an existing project, which
// Todoist's quick add would parse. Other "#" uses (e.g. "issue #42") stay in the content.
func (m Model) namesProject(text string) bool {
	lower := strings.ToLower(text)
	for _, p := range m.snap.Projects {
		tag := "#" + strings.ToLower(p.Name)
		for i := strings.Index(lower, tag); i >= 0; {
			end := i + len(tag)
			if end == len(lower) || strings.ContainsRune(" \t,.;:!?", rune(lower[end])) {
				return true
			}
			next := strings.Index(lower[end:], tag)
			if next < 0 {
				break
			}
			i = end + next
		}
	}
	return false
}

// quickAdd creates a task with natural-language parsing.
// In a project view, the task goes to that project and to the section under the cursor,
// unless the text names a different #project.
// In Today or Upcoming, the task gets the date of that day, unless the text sets a date.
func (m *Model) quickAdd(text string) tea.Cmd {
	var projectID, sectionID, date string
	if cur := m.currentNav(); cur != nil {
		switch {
		case cur.kind == vkProject && !m.namesProject(text):
			projectID = cur.projectID
			sectionID = m.cursorSection()
		case cur.kind == vkToday:
			date = time.Now().Format("2006-01-02")
		case cur.kind == vkUpcoming:
			date = m.cursorDate()
		}
	}
	client, projects := m.client, m.projects
	return m.write("", func(ctx context.Context) (string, error) {
		t, err := client.QuickAdd(ctx, text)
		if err != nil {
			return "", err
		}
		if projectID != "" && (t.ProjectID != projectID || sectionID != "") {
			if err := client.Move(ctx, t.ID, projectID, sectionID); err != nil {
				return "", fmt.Errorf("added, but move failed: %w", err)
			}
			t.ProjectID = projectID
		}
		if date != "" && t.Due == nil {
			if _, err := client.UpdateTask(ctx, t.ID, map[string]any{"due_date": date}); err != nil {
				return "", fmt.Errorf("added, but setting the date failed: %w", err)
			}
		}
		where := ""
		if p := projects[t.ProjectID]; p != nil {
			where = " → #" + p.Name
		}
		return "Added “" + t.Content + "”" + where, nil
	})
}

// addNote creates a note with a literal title (no date/#project parsing).
func (m *Model) addNote(title string) tea.Cmd {
	cur := m.currentNav()
	if cur == nil || cur.kind != vkProject {
		return nil
	}
	client, pid, sid := m.client, cur.projectID, m.cursorSection()
	return m.simpleWrite("Added note “"+title+"” · E to write", func(ctx context.Context) error {
		_, err := client.AddTask(ctx, title, pid, sid)
		return err
	})
}

// notesMode reports whether the current project is in notebook view.
// startRename opens the input with the name of the task under the cursor.
func (m *Model) startRename() tea.Cmd {
	t := m.currentTask()
	if t == nil {
		return nil
	}
	m.targetID = t.ID
	return m.openAdd(inputRename, t.Content)
}

// rename saves a new name. In task view, Todoist parses the text as in quick add:
// a date, #project, /section, @label or p1–p4 in the text changes that field, and the
// other fields stay the same. In notebook view, the name is saved as typed.
func (m *Model) rename(text string) tea.Cmd {
	id := m.targetID
	var old *todoist.Task
	for i := range m.snap.Tasks {
		if m.snap.Tasks[i].ID == id {
			old = &m.snap.Tasks[i]
		}
	}
	if text == "" || old == nil || text == old.Content {
		return nil
	}
	client, cur, names := m.client, *old, m.namesProject(text)
	if m.notesMode() {
		cmd := m.simpleWrite("Renamed to “"+text+"”", func(ctx context.Context) error {
			_, err := client.UpdateTask(ctx, id, map[string]any{"content": text})
			return err
		})
		return withRetry(cmd, inputRename, text)
	}
	projects, sections := m.projects, m.sections
	cmd := m.write("", func(ctx context.Context) (string, error) {
		p, err := client.Parse(ctx, text)
		if err != nil {
			return "", err
		}
		fields := map[string]any{"content": p.Content}
		changes := []string{"Renamed to “" + p.Content + "”"}
		if p.Due != nil {
			fields["due_string"] = p.Due.String
			changes = append(changes, "due "+p.Due.String)
		}
		if p.Priority > 1 && p.Priority != cur.Priority {
			fields["priority"] = p.Priority
			changes = append(changes, fmt.Sprintf("P%d", p.UIPriority()))
		}
		if len(p.Labels) > 0 {
			labels := append([]string(nil), cur.Labels...)
			for _, l := range p.Labels {
				if !slices.Contains(labels, l) {
					labels = append(labels, l)
				}
			}
			fields["labels"] = labels
			changes = append(changes, "@"+strings.Join(p.Labels, " @"))
		}
		if _, err := client.UpdateTask(ctx, id, fields); err != nil {
			return "", err
		}
		// Quick add puts a task without #project in the Inbox. Move only if the text named a project.
		if names && (p.ProjectID != cur.ProjectID || p.Section() != cur.Section()) {
			if err := client.Move(ctx, id, p.ProjectID, p.Section()); err != nil {
				return "", fmt.Errorf("renamed, but the move failed: %w", err)
			}
			where := "#" + projects[p.ProjectID].Name
			if s := sections[p.Section()]; s != nil {
				where += " / " + s.Name
			}
			changes = append(changes, "→ "+where)
		}
		return strings.Join(changes, " · "), nil
	})
	return withRetry(cmd, inputRename, text)
}

// startDue opens the date dialog with the due string of the task under the cursor.
// The due string holds the recurrence too, for example "every Thursday at 12:00".
func (m *Model) startDue() tea.Cmd {
	t := m.currentTask()
	if t == nil {
		return nil
	}
	text := ""
	if t.Due != nil {
		text = t.Due.String
	}
	return m.openCalendar(t, text)
}

// setPriority sets p1 to p4 on the task under the cursor.
func (m *Model) setPriority(p int) tea.Cmd {
	t := m.taskByID(m.currentTaskID())
	if t == nil {
		return nil
	}
	if t.UIPriority() == p {
		m.setStatus(fmt.Sprintf("already P%d", p), false)
		return nil
	}
	id, client := t.ID, m.client
	t.Priority = 5 - p // show the new color at once. The sync confirms it.
	return m.simpleWrite(fmt.Sprintf("Priority → P%d", p), func(ctx context.Context) error {
		_, err := client.UpdateTask(ctx, id, map[string]any{"priority": 5 - p})
		return err
	})
}

// propertyKeys handles the keys that change task properties. They work in the
// task list and in the details pane. ok is false for other keys.
func (m Model) propertyKeys(key string) (tea.Model, tea.Cmd, bool) {
	var next tea.Cmd
	switch key {
	case "t":
		next = m.startDue()
	case "1", "2", "3", "4":
		next = m.setPriority(int(key[0] - '0'))
	case "@":
		next = m.openLabelPicker()
	case "m":
		next = m.openMovePicker()
	default:
		return m, nil, false
	}
	return m, next, true
}

// notesMode reports whether the current project is in notebook view.
func (m Model) notesMode() bool {
	cur := m.currentNav()
	return cur != nil && cur.kind == vkProject && m.ui.ProjectModes[cur.projectID] == state.ModeNotes
}

// visiblePanes lists focusable panes in tab order for the current layout.
func (m Model) visiblePanes() []pane {
	switch {
	case m.wide():
		return []pane{paneNav, paneTasks, paneDetail}
	case m.detailOpen:
		return []pane{paneNav, paneDetail}
	}
	return []pane{paneNav, paneTasks}
}

func (m *Model) cycleFocus(d int) {
	ps := m.visiblePanes()
	i := 0
	for j, p := range ps {
		if p == m.focus {
			i = j
		}
	}
	m.focus = ps[(i+d+len(ps))%len(ps)]
}

func (m *Model) focusDetail() {
	if !m.wide() {
		m.detailOpen = true
	}
	m.focus = paneDetail
}

func (m *Model) leaveDetail() {
	m.detailOpen = false
	m.focus = paneTasks
}

func (m Model) updateKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.showHelp {
		m.showHelp = false
		if key == "q" || key == "ctrl+c" {
			return m.quit()
		}
		return m, nil
	}
	switch key {
	case "ctrl+c", "q":
		return m.quit()
	case "?":
		m.showHelp = true
		return m, nil
	case "r":
		m.setStatus("syncing…", false)
		next := m.startSync()
		return m, next
	case "tab", "shift+tab":
		// In the reader of a note with checkboxes, tab moves between the checkboxes.
		d := 1
		if key == "shift+tab" {
			d = -1
		}
		if m.noteChecks() > 0 {
			m.moveCheck(d)
			return m, nil
		}
		m.cycleFocus(d)
		return m, nil
	case "a":
		mode := inputAdd
		if m.notesMode() {
			mode = inputAddNote
		}
		next := m.openAdd(mode, "")
		return m, next
	case "/":
		m.focus = paneTasks
		m.detailOpen = false
		next := m.openInput(inputFind, " / ", "find in this view · enter keep · esc clear", m.find)
		return m, next
	case "f":
		next := m.openDialog(inputQuery, m.filterQuery)
		return m, next
	case "v":
		return m.toggleNotes()
	case "ctrl+z":
		return m.undo()
	}

	switch m.focus {
	case paneNav:
		return m.navKeys(key)
	case paneDetail:
		return m.detailKeys(key)
	}
	return m.taskKeys(key)
}

// inFilterView reports whether the selected sidebar item is the filter view.
func (m Model) inFilterView() bool {
	cur := m.currentNav()
	return cur != nil && cur.kind == vkFilter
}

// clearFilter removes the filter view and selects the view that was open before it.
func (m *Model) clearFilter() {
	back := m.preFilterNav
	m.filterQuery, m.filterTasks, m.preFilterNav = "", nil, ""
	m.buildNav()
	m.selectNav(func(n navItem) bool { return navKey(n) == back })
	m.buildRows(true)
	m.setStatus("filter cleared", false)
}

func (m Model) navKeys(key string) (tea.Model, tea.Cmd) {
	if nm, cmd, ok := m.labelKeys(key); ok {
		return nm, cmd
	}
	if key == "z" {
		if p := m.navProject(); p != nil && m.nav[m.navCur].hasKids {
			next := m.toggleProjectCollapse(p)
			return m, next
		}
		m.setStatus("this item has no sub-projects", false)
		return m, nil
	}
	if nm, cmd, ok := m.projectKeys(key); ok {
		return nm, cmd
	}
	switch key {
	case "esc":
		if m.inFilterView() {
			m.clearFilter()
		}
	case "j", "down":
		m.moveNav(1)
	case "k", "up":
		m.moveNav(-1)
	case "g", "home":
		m.moveNav(-len(m.nav))
	case "G", "end":
		m.moveNav(len(m.nav))
	case "enter", "l", "right":
		m.focus = paneTasks
	}
	return m, nil
}

func (m Model) taskKeys(key string) (tea.Model, tea.Cmd) {
	if key == "esc" && m.clearSelection() {
		m.setStatus("selection cleared", false)
		return m, nil
	}
	if nm, cmd, ok := m.completedKeys(key); ok {
		return nm, cmd
	}
	if nm, cmd, ok := m.sectionKeys(key); ok {
		return nm, cmd
	}
	if nm, cmd, ok := m.selectionKeys(key); ok {
		return nm, cmd
	}
	if nm, cmd, ok := m.orderKeys(key); ok {
		return nm, cmd
	}
	if m.currentTask() == nil && taskKey(key) {
		m.setStatus("no task under the cursor · move to a task with j/k", true)
		return m, nil
	}
	if nm, cmd, ok := m.propertyKeys(key); ok {
		return nm, cmd
	}
	switch key {
	case "j", "down":
		m.moveRow(1)
	case "k", "up":
		m.moveRow(-1)
	case "g", "home":
		m.moveRow(-len(m.rows))
	case "G", "end":
		m.moveRow(len(m.rows))
	case "ctrl+d", "pgdown":
		m.moveRow(max(1, m.paneHeight()/2))
	case "ctrl+u", "pgup":
		m.moveRow(-max(1, m.paneHeight()/2))
	case "esc":
		switch {
		case m.find != "":
			m.find = ""
			m.buildRows(true)
		case m.inFilterView():
			m.clearFilter()
		default:
			m.focus = paneNav
		}
	case "h", "left":
		m.focus = paneNav
	case "enter", "l", "right":
		if m.currentTask() != nil {
			m.focusDetail()
		}
	case "x", "space":
		return m.completeTask()
	case "delete", "backspace":
		m.askDeleteTask()
		return m, nil
	case "c":
		if t := m.currentTask(); t != nil {
			next := m.openEditor(editCommentNew, t.ID, "", "New comment · "+plain(t.Content), "")
			return m, next
		}
	case "e":
		next := m.startRename()
		return m, next
	case "E":
		if m.notesMode() { // notebook view: the inline markdown editor
			next := m.openNoteEditor()
			return m, next
		}
		if t := m.currentTask(); t != nil {
			next := m.openEditor(editDescription, t.ID, "", editTitleFor(t, m.notesMode()), t.Description)
			return m, next
		}
	}
	return m, nil
}

// taskKey reports whether key acts on the task under the cursor.
func taskKey(key string) bool {
	switch key {
	case "x", "space", "e", "E", "c", "t", "1", "2", "3", "4", "@", "m", "delete", "backspace":
		return true
	}
	return false
}

func (m Model) detailKeys(key string) (tea.Model, tea.Cmd) {
	if key == "space" && m.noteChecks() > 0 {
		if m.chkCur < 0 {
			m.setStatus("tab selects a checkbox", false)
			return m, nil
		}
		next := m.toggleNoteCheck(m.chkCur)
		return m, next
	}
	if (key == "j" || key == "k") && m.chkCur >= 0 {
		m.chkCur = -1 // j/k go back to the comments
	}
	if key == "enter" && m.notesMode() && m.comCur < 0 {
		next := m.openNoteEditor()
		return m, next
	}
	if nm, cmd, ok := m.propertyKeys(key); ok {
		return nm, cmd
	}
	t := m.currentTask()
	var comments []todoist.Comment
	if t != nil {
		comments = m.snap.Comments[t.ID]
	}
	switch key {
	case "h", "left", "esc":
		m.leaveDetail()
	case "j", "down":
		if len(comments) == 0 {
			m.detOff++
		} else if m.comCur < len(comments)-1 {
			m.comCur++
			m.scrollToComment()
		}
	case "k", "up":
		if m.comCur >= 0 {
			m.comCur--
			m.scrollToComment()
		} else {
			m.detOff--
		}
	case "ctrl+d", "pgdown":
		m.detOff += max(1, m.paneHeight()/2)
	case "ctrl+u", "pgup":
		m.detOff -= max(1, m.paneHeight()/2)
	case "g", "home":
		m.detOff, m.comCur = 0, -1
	case "G", "end":
		m.detOff = 1 << 20
		if len(comments) > 0 {
			m.comCur = len(comments) - 1
		}
	case "x", "space":
		return m.completeTask()
	case "c":
		if t != nil {
			next := m.openEditor(editCommentNew, t.ID, "", "New comment · "+plain(t.Content), "")
			return m, next
		}
	case "e":
		if t == nil {
			return m, nil
		}
		if m.comCur >= 0 && m.comCur < len(comments) {
			cm := comments[m.comCur]
			next := m.openEditor(editCommentEdit, t.ID, cm.ID, "Edit comment · "+plain(t.Content), cm.Content)
			return m, next
		}
		next := m.startRename()
		return m, next
	case "E":
		if m.notesMode() {
			next := m.openNoteEditor()
			return m, next
		}
		if t != nil {
			next := m.openEditor(editDescription, t.ID, "", editTitleFor(t, m.notesMode()), t.Description)
			return m, next
		}
	case "d":
		if t != nil && m.comCur >= 0 && m.comCur < len(comments) {
			id, client := comments[m.comCur].ID, m.client
			m.confirm = yesNo("Delete comment", "Delete this comment? This cannot be undone.", "Delete", func(m *Model) tea.Cmd {
				m.comCur--
				return m.simpleWrite("Comment deleted", func(ctx context.Context) error { return client.DeleteComment(ctx, id) })
			})
		}
	}
	return m, nil
}

func editTitleFor(t *todoist.Task, notes bool) string {
	if notes {
		return "Note · " + plain(t.Content)
	}
	return "Description · " + plain(t.Content)
}

// completeTask completes the task under the cursor. It removes a one-time task from the list at once.
func (m Model) completeTask() (tea.Model, tea.Cmd) {
	next := m.complete()
	return m, next
}

// askDeleteTask asks y/n and then deletes the task under the cursor. The API has no undo
// for a deletion, so the prompt says so.
func (m *Model) askDeleteTask() {
	t := m.currentTask()
	if t == nil {
		return
	}
	id, name, client := t.ID, plain(t.Content), m.client
	text := "Delete “" + name + "”? This cannot be undone."
	if n := len(m.snap.Comments[id]); n > 0 {
		text = fmt.Sprintf("Delete “%s” and its %d comment(s)? This cannot be undone.", name, n)
	}
	m.confirm = yesNo("Delete task", text, "Delete", func(m *Model) tea.Cmd {
		m.removeTask(id)
		return m.simpleWrite("Deleted “"+name+"”", func(ctx context.Context) error {
			return client.Delete(ctx, id)
		})
	})
}

// complete is completeTask for callers that hold a *Model, such as a confirm prompt.
func (m *Model) complete() tea.Cmd {
	t := m.currentTask()
	if t == nil {
		return nil
	}
	id, content, recurring := t.ID, t.Content, t.Due != nil && t.Due.IsRecurring
	// Reopen does not move the date of a recurring task back. Thus undo is only for one-time tasks.
	text := "Completed occurrence of “" + content + "” · next one scheduled"
	if !recurring {
		m.closed = append(m.closed, []string{id})
		m.removeTask(id)
		text = "Completed “" + content + "” · ctrl+z to undo"
	}
	m.setStatus(text, false)
	client := m.client
	return m.simpleWrite(text, func(ctx context.Context) error { return client.Close(ctx, id) })
}

func (m Model) undo() (tea.Model, tea.Cmd) {
	if len(m.closed) == 0 {
		m.setStatus("nothing to undo", false)
		return m, nil
	}
	ids := m.closed[len(m.closed)-1]
	m.closed = m.closed[:len(m.closed)-1]
	client := m.client
	done := "Reopened task"
	if len(ids) > 1 {
		done = fmt.Sprintf("Reopened %d tasks", len(ids))
	}
	next := m.simpleWrite(done, func(ctx context.Context) error {
		for _, id := range ids {
			if err := client.Reopen(ctx, id); err != nil {
				return err
			}
		}
		return nil
	})
	return m, next
}

// toggleNotes switches the current project between task view and notebook view and saves the choice.
func (m Model) toggleNotes() (tea.Model, tea.Cmd) {
	cur := m.currentNav()
	if cur == nil || cur.kind != vkProject {
		m.setStatus("notes mode applies to projects", false)
		return m, nil
	}
	if m.ui.ProjectModes[cur.projectID] == state.ModeNotes {
		delete(m.ui.ProjectModes, cur.projectID)
		m.setStatus("#"+cur.name+" · tasks view", false)
	} else {
		m.ui.ProjectModes[cur.projectID] = state.ModeNotes
		m.setStatus("#"+cur.name+" · notebook view", false)
	}
	if err := state.Save(m.ui); err != nil {
		m.setStatus("could not save view mode: "+err.Error(), true)
	}
	m.buildRows(false)
	return m, nil
}

// ---- data → view models ----

// applyState shows a new replica.
func (m *Model) applyState(st todoist.SyncState) {
	m.st = st
	m.snap = st.Snapshot()
	m.loaded = true
	m.projects = map[string]*todoist.Project{}
	for i := range m.snap.Projects {
		m.projects[m.snap.Projects[i].ID] = &m.snap.Projects[i]
	}
	m.sections = map[string]*todoist.Section{}
	for i := range m.snap.Sections {
		m.sections[m.snap.Sections[i].ID] = &m.snap.Sections[i]
	}
	m.buildNav()
	m.buildRows(false)
}

func (m *Model) removeTask(id string) {
	keep := func(ts []todoist.Task) []todoist.Task {
		out := ts[:0:0]
		for _, t := range ts {
			if t.ID != id {
				out = append(out, t)
			}
		}
		return out
	}
	m.snap.Tasks = keep(m.snap.Tasks)
	m.filterTasks = keep(m.filterTasks)
	m.buildNav()
	m.buildRows(false)
}

// buildNav makes the sidebar rows and keeps the selection.
func (m *Model) buildNav() {
	var prevKey string
	if cur := m.currentNav(); cur != nil {
		prevKey = navKey(*cur)
	}
	now := time.Now()
	perProject := map[string]int{}
	var today, overdue int
	for _, t := range m.snap.Tasks {
		perProject[t.ProjectID]++
		switch todoist.Classify(t.Due, now) {
		case todoist.Overdue:
			overdue++
		case todoist.DueToday:
			today++
		}
	}
	todayHex := hexMuted
	if overdue > 0 {
		todayHex = hexOverdue
	}
	projItem := func(p *todoist.Project, depth int) navItem {
		glyph := "#"
		if p.InboxProject {
			glyph = "⌂"
		}
		return navItem{kind: vkProject, projectID: p.ID, name: p.Name, glyph: glyph,
			color: todoist.ColorHex(p.Color), count: perProject[p.ID], depth: depth}
	}
	// Order: Inbox, Today and Upcoming, Favorites, My Projects, All tasks. An empty
	// line is at the top and before each group.
	nav := []navItem{{header: " "}}
	ordered := m.orderedProjects()
	for _, op := range ordered {
		if op.p.InboxProject {
			nav = append(nav, projItem(op.p, 0))
		}
	}
	nav = append(nav, navItem{header: " "},
		navItem{kind: vkToday, name: "Today", glyph: "◉", color: hexToday, count: today + overdue, countHex: todayHex},
		navItem{kind: vkUpcoming, name: "Upcoming", glyph: "▦", color: hexWeek})
	if m.filterQuery != "" {
		nav = append(nav, navItem{kind: vkFilter, name: m.filterQuery, glyph: "⌕", color: hexTomorrow, count: len(m.filterTasks)})
	}
	var favs []navItem
	for _, op := range ordered {
		if op.p.IsFavorite {
			favs = append(favs, projItem(op.p, 0))
		}
	}
	if len(favs) > 0 {
		nav = append(nav, navItem{header: " "}, navItem{header: "Favorites"})
		nav = append(nav, favs...)
	}
	nav = append(nav, navItem{header: " "}, navItem{header: "My Projects"})
	hasKids := map[string]bool{}
	for _, op := range ordered {
		if p := op.p.ParentID; p != nil {
			hasKids[*p] = true
		}
	}
	hiddenUnder := map[string]bool{} // projects inside a collapsed project
	var shown []orderedProject
	for _, op := range ordered {
		if op.p.InboxProject {
			continue
		}
		if p := op.p.ParentID; p != nil && (hiddenUnder[*p] || (m.projects[*p] != nil && m.projects[*p].IsCollapsed)) {
			hiddenUnder[op.p.ID] = true
			continue
		}
		shown = append(shown, op)
	}
	// last[i] is true if shown[i] is the last shown project among its siblings.
	last := make([]bool, len(shown))
	for i, op := range shown {
		last[i] = true
		for j := i + 1; j < len(shown) && shown[j].depth >= op.depth; j++ {
			if shown[j].depth == op.depth {
				last[i] = false
				break
			}
		}
	}
	var open []bool // for each ancestor level: does a later sibling follow (draw │)?
	for i, op := range shown {
		it := projItem(op.p, op.depth)
		it.hasKids, it.collapsed = hasKids[op.p.ID], op.p.IsCollapsed && hasKids[op.p.ID]
		it.inTree = true
		open = append(open[:min(op.depth, len(open))], !last[i])
		if op.depth > 0 {
			var b strings.Builder
			b.WriteString("  ")
			for d := 1; d < op.depth; d++ {
				if open[d] {
					b.WriteString("│ ")
				} else {
					b.WriteString("  ")
				}
			}
			if last[i] {
				b.WriteString("└ ")
			} else {
				b.WriteString("├ ")
			}
			it.tree = b.String()
		}
		nav = append(nav, it)
	}
	nav = append(nav, navItem{header: " "}, navItem{header: "Labels"})
	perLabel := map[string]int{}
	for _, t := range m.snap.Tasks {
		for _, l := range t.Labels {
			perLabel[l]++
		}
	}
	for _, l := range m.snap.Labels {
		nav = append(nav, navItem{kind: vkLabel, labelID: l.ID, name: l.Name, glyph: "@",
			color: todoist.ColorHex(l.Color), count: perLabel[l.Name]})
	}
	if len(m.snap.Labels) == 0 {
		nav = append(nav, navItem{kind: vkLabelHint, name: "A adds a label", glyph: " ", color: hexDim})
	}
	nav = append(nav, navItem{header: " "},
		navItem{kind: vkAll, name: "All tasks", glyph: "≡", color: hexMuted, count: len(m.snap.Tasks)},
		navItem{kind: vkCompleted, name: "Completed", glyph: "✓", color: hexToday})
	m.nav = nav

	m.navCur = -1
	for i, n := range m.nav {
		if n.header == "" && prevKey != "" && navKey(n) == prevKey {
			m.navCur = i
			break
		}
	}
	if m.navCur < 0 { // start in Today
		m.selectNav(func(n navItem) bool { return n.kind == vkToday })
	}
}

func navKey(n navItem) string { return fmt.Sprintf("%d/%s/%s", n.kind, n.projectID, n.labelID) }

type orderedProject struct {
	p     *todoist.Project
	depth int
}

// orderedProjects returns active projects in sidebar (tree) order.
func (m *Model) orderedProjects() []orderedProject {
	children := map[string][]*todoist.Project{}
	for i := range m.snap.Projects {
		p := &m.snap.Projects[i]
		if p.IsArchived {
			continue
		}
		parent := ""
		if p.ParentID != nil && m.projects[*p.ParentID] != nil {
			parent = *p.ParentID
		}
		children[parent] = append(children[parent], p)
	}
	var out []orderedProject
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		kids := children[parent]
		sort.SliceStable(kids, func(i, j int) bool {
			if kids[i].InboxProject != kids[j].InboxProject {
				return kids[i].InboxProject
			}
			return kids[i].ChildOrder < kids[j].ChildOrder
		})
		for _, p := range kids {
			out = append(out, orderedProject{p, depth})
			walk(p.ID, depth+1)
		}
	}
	walk("", 0)
	return out
}

func (m *Model) currentNav() *navItem {
	if m.navCur < 0 || m.navCur >= len(m.nav) {
		return nil
	}
	return &m.nav[m.navCur]
}

func (m *Model) selectNav(pred func(navItem) bool) {
	for i, n := range m.nav {
		if n.header == "" && pred(n) {
			m.navCur = i
			return
		}
	}
}

func (m *Model) moveNav(d int) {
	prev := m.navCur
	step := 1
	if d < 0 {
		step, d = -1, -d
	}
	for ; d > 0; d-- {
		i := m.navCur + step
		for i >= 0 && i < len(m.nav) && m.nav[i].header != "" {
			i += step
		}
		if i < 0 || i >= len(m.nav) {
			break
		}
		m.navCur = i
	}
	if m.navCur != prev {
		m.find = ""
		m.detailOpen = false
		m.buildRows(true)
	}
}

// buildRows makes the task list rows for the selected sidebar item.
// If reset is true, the cursor goes to the first task. If not, the cursor stays on the current task.
func (m *Model) buildRows(reset bool) {
	var keepID string
	if t := m.currentTask(); t != nil && !reset {
		keepID = t.ID
	}
	prevCur := m.rowCur

	m.rows = nil
	cur := m.currentNav()
	if cur == nil {
		return
	}
	now := time.Now()
	switch cur.kind {
	case vkProject:
		m.rows = m.projectRows(cur.projectID, m.notesMode())
	case vkToday:
		m.rows = m.todayRows(now)
	case vkUpcoming:
		m.rows = m.upcomingRows(now)
	case vkFilter:
		m.rows = m.filterRows()
	case vkAll:
		m.rows = m.allRows()
	case vkLabel:
		m.rows = m.labelRows(cur.name)
	case vkCompleted:
		m.rows = m.completedRows()
	}

	if m.find != "" {
		m.rows = applyFind(m.rows, m.find)
	}
	m.rows = addSpacers(m.rows)
	m.rows = append([]row{{spacer: true}}, m.rows...) // an empty line under the view title

	m.rowCur = -1
	if keepID != "" {
		for i, r := range m.rows {
			if r.task != nil && r.preview == "" && r.task.ID == keepID {
				m.rowCur = i
			}
		}
	}
	if m.rowCur < 0 {
		m.rowCur = 0
		if !reset {
			m.rowCur = max(0, min(prevCur, len(m.rows)-1))
		}
		if len(m.rows) > 0 && m.rows[m.rowCur].task == nil {
			for i := m.rowCur; i < len(m.rows); i++ {
				if m.rows[i].task != nil {
					m.rowCur = i
					break
				}
			}
		}
		if len(m.rows) > 0 && m.rows[m.rowCur].preview != "" {
			m.rowCur--
		}
	}
	if reset {
		m.rowOff = 0
	}
}

// projectRows makes the rows of a project: tasks without a section first, then each section.
// In notebook view, a preview row follows each note.
func (m *Model) projectRows(projectID string, notes bool) []row {
	var secs []*todoist.Section
	for i := range m.snap.Sections {
		s := &m.snap.Sections[i]
		if s.ProjectID == projectID && !s.IsArchived {
			secs = append(secs, s)
		}
	}
	sort.SliceStable(secs, func(i, j int) bool { return secs[i].SectionOrder < secs[j].SectionOrder })
	bySection := map[string][]todoist.Task{}
	for _, t := range m.snap.Tasks {
		if t.ProjectID == projectID {
			sid := t.Section()
			if sid != "" && m.sections[sid] == nil {
				sid = ""
			}
			bySection[sid] = append(bySection[sid], t)
		}
	}
	group := func(ts []todoist.Task) []row {
		rows := treeRows(ts)
		if !notes {
			return rows
		}
		var out []row
		for _, r := range rows {
			out = append(out, r)
			if p := notePreview(r.task, m.snap.Comments[r.task.ID]); p != "" {
				out = append(out, row{task: r.task, depth: r.depth, preview: p})
			}
		}
		return out
	}
	rows := group(bySection[""])
	for _, s := range secs {
		ts := bySection[s.ID]
		rows = append(rows, row{header: s.Name, headerHex: fg(todoist.ColorHex(m.projects[projectID].Color)), count: len(ts),
			sectionID: s.ID, hasKids: len(ts) > 0, collapsed: s.IsCollapsed && len(ts) > 0})
		if !s.IsCollapsed {
			rows = append(rows, group(ts)...)
		}
	}
	return rows
}

// addSpacers puts an empty row before each header that is not the first row.
func addSpacers(rows []row) []row {
	out := make([]row, 0, len(rows)+8)
	for i, r := range rows {
		if r.header != "" && i > 0 {
			out = append(out, row{spacer: true})
		}
		out = append(out, r)
	}
	return out
}

// allRows groups all tasks by project in sidebar order, with sub-tasks nested.
func (m *Model) allRows() []row { return m.projectGroupedRows(m.snap.Tasks) }

// labelRows groups the tasks with one label by project.
func (m *Model) labelRows(name string) []row {
	var ts []todoist.Task
	for _, t := range m.snap.Tasks {
		if slices.Contains(t.Labels, name) {
			ts = append(ts, t)
		}
	}
	return m.projectGroupedRows(ts)
}

// completedRows groups the completed tasks by project, newest completion first.
func (m *Model) completedRows() []row {
	rows := m.groupByProject(m.completed, true)
	for i := range rows {
		if rows[i].task != nil {
			rows[i].done = true
		}
	}
	return rows
}

// groupByProject groups tasks by project in sidebar order. Inside a group, tasks with a
// due date come first, sorted by the date, and tasks without a date follow in project
// order. If byCompletion is true, the newest completion comes first instead.
func (m *Model) groupByProject(tasks []todoist.Task, byCompletion bool) []row {
	byProject := map[string][]todoist.Task{}
	for _, t := range tasks {
		byProject[t.ProjectID] = append(byProject[t.ProjectID], t)
	}
	var rows []row
	for _, op := range m.orderedProjects() {
		ts := byProject[op.p.ID]
		if len(ts) == 0 {
			continue
		}
		sort.SliceStable(ts, func(i, j int) bool {
			if byCompletion {
				return ts[i].CompletedAt > ts[j].CompletedAt
			}
			ai, aj := ts[i].Due != nil, ts[j].Due != nil
			if ai != aj {
				return ai
			}
			if !ai {
				return ts[i].ChildOrder < ts[j].ChildOrder
			}
			a, _, _ := ts[i].Due.Time()
			b, _, _ := ts[j].Due.Time()
			if !a.Equal(b) {
				return a.Before(b)
			}
			return ts[i].Priority > ts[j].Priority
		})
		glyph := "# "
		if op.p.InboxProject {
			glyph = "⌂ "
		}
		rows = append(rows, row{header: glyph + op.p.Name, headerHex: fg(todoist.ColorHex(op.p.Color)), count: len(ts)})
		rows = append(rows, flatRows(ts)...)
	}
	return rows
}

// notePreview is the first line of a note's body, or of its first comment.
func notePreview(t *todoist.Task, comments []todoist.Comment) string {
	if line := firstLine(t.Description); line != "" {
		return line
	}
	if len(comments) > 0 {
		return fmt.Sprintf("✎%d %s", len(comments), firstLine(comments[0].Content))
	}
	return ""
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(plain(strings.TrimLeft(strings.TrimSpace(l), "#>*-+ ")))
		if l != "" {
			return l
		}
	}
	return ""
}

// treeRows orders tasks by child_order with subtasks nested under their parents.
// A collapsed task shows no sub-tasks. Its row has the number of hidden tasks.
func treeRows(ts []todoist.Task) []row {
	ids := map[string]bool{}
	for _, t := range ts {
		ids[t.ID] = true
	}
	children := map[string][]*todoist.Task{}
	for i := range ts {
		parent := ts[i].Parent()
		if !ids[parent] {
			parent = ""
		}
		children[parent] = append(children[parent], &ts[i])
	}
	var out []row
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		kids := children[parent]
		sort.SliceStable(kids, func(i, j int) bool { return kids[i].ChildOrder < kids[j].ChildOrder })
		for _, t := range kids {
			r := row{task: t, depth: depth, hasKids: len(children[t.ID]) > 0, collapsed: t.IsCollapsed}
			if r.hasKids && t.IsCollapsed {
				r.hidden = countTree(children, t.ID)
				out = append(out, r)
				continue
			}
			out = append(out, r)
			walk(t.ID, depth+1)
		}
	}
	walk("", 0)
	return out
}

// countTree counts all tasks below id.
func countTree(children map[string][]*todoist.Task, id string) int {
	n := 0
	for _, t := range children[id] {
		n += 1 + countTree(children, t.ID)
	}
	return n
}

func flatRows(ts []todoist.Task) []row {
	out := make([]row, len(ts))
	for i := range ts {
		out[i] = row{task: &ts[i]}
	}
	return out
}

func sortByDue(ts []todoist.Task) {
	sort.SliceStable(ts, func(i, j int) bool {
		a, _, _ := ts[i].Due.Time()
		b, _, _ := ts[j].Due.Time()
		if !a.Equal(b) {
			return a.Before(b)
		}
		return ts[i].Priority > ts[j].Priority
	})
}

// applyFind keeps tasks matching q (case-insensitive) and headers that still have tasks.
func applyFind(rows []row, q string) []row {
	q = strings.ToLower(q)
	var out []row
	for _, r := range rows {
		if r.task == nil {
			out = append(out, r)
			continue
		}
		hay := strings.ToLower(r.task.Content + " " + r.task.Description + " " + strings.Join(r.task.Labels, " "))
		if strings.Contains(hay, q) {
			r.depth = 0
			out = append(out, r)
		}
	}
	var pruned []row
	for i, r := range out {
		if r.task == nil && (i+1 >= len(out) || out[i+1].task == nil) {
			continue
		}
		pruned = append(pruned, r)
	}
	return pruned
}

func (m *Model) currentRow() *row {
	if m.rowCur < 0 || m.rowCur >= len(m.rows) {
		return nil
	}
	return &m.rows[m.rowCur]
}

func (m *Model) currentTask() *todoist.Task {
	if r := m.currentRow(); r != nil {
		return r.task
	}
	return nil
}

func (m Model) currentTaskID() string {
	if t := m.currentTask(); t != nil {
		return t.ID
	}
	return ""
}

// cursorSection is the section of the row under the cursor (project view).
func (m *Model) cursorSection() string {
	for i := m.rowCur; i >= 0 && i < len(m.rows); i-- {
		if m.rows[i].header != "" {
			return m.rows[i].sectionID
		}
	}
	return ""
}

// cursorDate is the day group under the cursor (Today/Upcoming).
func (m *Model) cursorDate() string {
	for i := m.rowCur; i >= 0 && i < len(m.rows); i-- {
		if m.rows[i].date != "" {
			return m.rows[i].date
		}
	}
	return ""
}

// moveRow moves the cursor by d rows. It skips note preview lines and spacers.
func (m *Model) moveRow(d int) {
	if len(m.rows) == 0 {
		return
	}
	step := 1
	if d < 0 {
		step, d = -1, -d
	}
	cur := m.rowCur
	for ; d > 0; d-- {
		i := cur + step
		for i >= 0 && i < len(m.rows) && (m.rows[i].preview != "" || m.rows[i].spacer) {
			i += step
		}
		if i < 0 || i >= len(m.rows) {
			break
		}
		cur = i
	}
	m.rowCur = cur
}
