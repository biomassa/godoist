// Package tui implements the three-pane godoist terminal UI.
package tui

import (
	"fmt"
	"maps"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/cache"
	"github.com/biomassa/godoist/internal/state"
	"github.com/biomassa/godoist/internal/todoist"
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
	helpOff       int  // the first line of the help on the screen
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
	targetID       string  // task that inputRename changes
	jumpTo         *string // a note for the cursor: a new sub-note (set by its write) or an opened one
	targetSection  string  // section that inputRenameSection changes
	targetProject  string  // project that inputRenameProject changes
	targetLabel    string  // label that inputRenameLabel changes

	// The text inputs are pointers: they are large (15 KB for a textarea), and Bubble Tea
	// copies the Model for each message and each method call.
	input      *textinput.Model // bottom-bar input (find)
	dlg        *textarea.Model  // dialog input (all other text inputs), nil while closed
	dlgDesc    *noteEditor      // description field of the task dialog, nil while closed
	dlgDescH   int              // the height of the description field
	fx         *taskFx          // the running completion animation, or nil (see fx.go)
	fxGen      int
	animations bool        // the config option "animations"
	comp       *completion // the #project, @label, /section list of the name field, or nil
	compHide   int         // see updateCompletion
	dlgField   int         // focused field of the task dialog: 0 name, 1 description
	inputMode  inputMode

	note *noteEditor // the inline markdown editor: descriptions, notes, and comments
	pick *picker

	confirm *confirmPrompt
	cal     *calDialog // date dialog

	completed        []todoist.Task // Completed view, last 30 days
	completedLoading bool
	selected         map[string]bool // multi-select: task IDs
	lastMark         string          // the task that s or a click marked last, for ranges
	menu             *ctxMenu        // right-click menu
	drag             *dragState      // task that the user drags with the mouse
	lastClick        clickInfo

	status     string
	statusErr  bool
	closed     [][]string            // undo stack: each entry is the one-time tasks of one completion
	closedTask map[string]closedTask // the name and the project of each task in closed

	// The completed sub-tasks that show under their open tasks (see donesubs.go).
	doneLocal   map[string]localDone       // completed in this session, not synced yet
	reopenLocal map[string]localDone       // reopened in this session, not synced yet
	doneKids    map[string][]*todoist.Task // open or completed task ID → its completed sub-tasks
	doneIn      map[string]bool            // the completed sub-tasks in doneKids
	parentNames map[string]string          // Completed view: names of tasks from Todoist
	parentAsked map[string]bool
	syncSeq     int // the number of syncs started
	stSeq       int // the number of the sync that m.st comes from (0: the cache)            // Completed view: names asked for

	notesMoved bool                    // this run wrote the local notebook settings to Todoist
	themes     *themePicker            // the open theme picker (T), or nil
	termDark   bool                    // the terminal background is dark (for the "todoist" theme)
	saveTheme  func(name string) error // saves a kept theme in the config file, or nil
	listShare  float64                 // the share of the task list next to the details (see panes.go)
	shareGen   int                     // counts the border moves, for the pause before the save
	saveShare  func(float64) error     // saves the share in the config file, or nil
	paneDrag   bool                    // the mouse drags the border of the task list
	dragFrom   [2]int                  // the drag start: the mouse x and the list width
	offline    bool                    // the last sync could not reach Todoist
	pendingIDs map[string]bool         // tasks with a queued change that only Todoist can finish (⏳)

	md *mdCache
	fc *frameCache // the styled lines of the last frames and the wheel merge
}

// Settings are the saved settings that the TUI starts with, and the functions that save
// them. A nil function does not save.
type Settings struct {
	Theme         string  // the color theme (see ThemeNames)
	ListShare     float64 // the share of the task list next to the details pane, 0 for half
	Animations    bool    // the completion animation (config option "animations")
	SaveTheme     func(name string) error
	SaveListShare func(share float64) error
}

// New returns a Model. If a cache exists for the token, the first frame shows the cached data.
func New(client *todoist.Client, token string, set Settings) Model {
	ti := newTextInput()
	ti.CharLimit = 500
	m := Model{client: client, token: token, input: &ti, comCur: -1, chkCur: -1, md: newMDCache(), fc: newFrameCache(),
		termDark: true, saveTheme: set.SaveTheme, listShare: set.ListShare, saveShare: set.SaveListShare,
		animations: set.Animations}
	applyTheme(set.Theme, true)
	m.ui, _ = state.Load()
	m.syncing = 1      // Init starts the first sync
	client.TrackDone() // completed sub-tasks show under their open tasks
	// The offline queue keeps task changes while the computer is offline.
	if p, err := cache.QueuePath(token); err == nil {
		q, err := todoist.OpenQueue(p)
		if err != nil {
			m.setStatus(err.Error(), true)
		}
		client.SetQueue(q)
	}
	if st := cache.Load(token); st.Token != "" {
		m.applyState(st) // show the cached data at once. Init starts a sync.
	}
	return m
}

// Init starts the first sync, asks for the terminal background color, and starts the sync timer.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.syncCmd(1), tea.RequestBackgroundColor, autoSyncTick())
}

func (m *Model) setStatus(s string, isErr bool) { m.status, m.statusErr = s, isErr }

// Update handles a message. A wheel event less than one frame after the last frame shares
// the next frame with the events after it (see holdsWheel).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	fc := m.fc
	if fc == nil {
		return m.updateAll(msg)
	}
	if _, ok := msg.(frameMsg); ok { // the frame of the held wheel events
		fc.frameDue, fc.held = false, false
		return m, nil
	}
	if fc.holdsWheel(msg) {
		nm, cmd := m.updateAll(msg)
		return nm, tea.Batch(cmd, fc.hold())
	}
	fc.held = false
	return m.updateAll(msg)
}

// updateAll handles a message. If the task under the cursor changes, the details pane goes
// back to the top.
func (m Model) updateAll(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.input != nil {
			m.input.SetWidth(max(10, m.width-4))
		}
		m.sizeDialog()
		return m, nil

	case themeSavedMsg:
		return m.themeSaved(msg)
	case shareSaveMsg:
		return m.shareSave(msg)
	case shareSavedMsg:
		if msg.err != nil {
			m.setStatus("the pane width was not saved: "+msg.err.Error(), true)
		}
		return m, nil

	case tea.BackgroundColorMsg:
		m.termDark = msg.IsDark()
		if themeName != todoistTheme {
			return m, nil // a palette theme paints its own background
		}
		setTheme(msg.IsDark())
		r, g, b, _ := msg.RGBA()
		baseBg = fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
		themeGen++
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
			if todoist.IsOffline(msg.err) {
				m.offline = true // the status line shows it at the right
				if m.status == "syncing…" {
					m.setStatus("", false)
				}
			} else {
				m.setStatus("sync failed: "+msg.err.Error(), true)
			}
			return m, tea.Batch(cmds...)
		}
		m.offline = false
		m.stSeq = msg.seq
		m.applyState(msg.st)
		if errs := m.client.Queue().TakeErrors(); len(errs) > 0 {
			m.setStatus("Todoist refused a change made offline: "+strings.Join(errs, " · "), true)
		}
		cmds = append(cmds, m.moveNotesModes())
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
			if m.focusFilterNav && m.inputMode == inputNone && m.note == nil && m.pick == nil {
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
		if msg.offline {
			m.offline = true
		}
		switch {
		case msg.err != nil:
			m.setStatus(msg.err.Error(), true)
		case msg.queued && m.offline: // online, the queue goes to Todoist with the next sync
			text := msg.text
			if text == "" {
				text = "Saved"
			}
			m.setStatus(text+" · offline", false)
		case msg.text != "":
			m.setStatus(msg.text, false)
		}
		if m.client.Queue().Len() > 0 {
			m.applyState(m.st) // show the queued changes. A sync cannot while offline.
		}
		if m.quitting && m.pending == 0 {
			return m.endQuit()
		}
		cmds := []tea.Cmd{m.startSync()}
		if msg.err != nil && msg.retryMode != inputNone && m.inputMode == inputNone && m.note == nil && !m.quitting {
			cmds = append(cmds, m.openTaskDialog(msg.retryMode, msg.retryText, msg.retryDesc))
		}
		return m, tea.Batch(cmds...)

	case noteTickMsg:
		return m.noteTick(msg)

	case noteSavedMsg:
		return m.noteSaved(msg)

	case tea.PasteMsg:
		return m.paste(msg)
	case tea.ClipboardMsg: // the answer to ctrl+v: the system clipboard from the terminal
		return m.paste(tea.PasteMsg{Content: msg.Content})

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
		return m, m.fetchParentNames()

	case parentNamesMsg:
		if m.parentNames == nil {
			m.parentNames = map[string]string{}
		}
		maps.Copy(m.parentNames, msg)
		if m.inCompleted() {
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

	case fxTickMsg:
		return m.fxFrameMsg(msg)
	case tea.KeyPressMsg:
		m.dropGhosts() // a key acts on the list without the rows of the flying tasks
		switch {
		case m.quitting:
			if msg.String() == "ctrl+c" { // second ctrl+c: quit without waiting
				return m, tea.Quit
			}
			return m, nil
		case m.themes != nil:
			return m.updateThemePicker(msg)
		case m.cal != nil:
			return m.updateCalendar(msg)
		case m.menu != nil:
			return m.updateMenu(msg)
		case m.pick != nil:
			return m.updatePicker(msg)
		case m.note != nil:
			return m.updateNoteEditor(msg)
		case m.confirm != nil:
			return m.answerConfirm(msg.String())
		case m.inputMode != inputNone:
			return m.updateInput(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

// quit stops the program. If writes are pending, it waits for them first.
func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.pending == 0 {
		return m.endQuit()
	}
	m.quitting = true
	m.setStatus(fmt.Sprintf("saving %d change(s) before quitting… (ctrl+c to force)", m.pending), false)
	return m, nil
}

// endQuit quits when no write runs. If changes wait in the offline queue, a dialog asks
// first: the changes go to Todoist at the next start.
func (m Model) endQuit() (tea.Model, tea.Cmd) {
	m.quitting = false
	n := m.client.Queue().Len()
	if n == 0 {
		return m, tea.Quit
	}
	verb := "are"
	if n == 1 {
		verb = "is"
	}
	m.confirm = &confirmPrompt{title: "Quit?",
		text: plural(n, "change") + " " + verb + " not in Todoist yet. They go at the next start.",
		buttons: []confirmButton{
			{key: "q", label: "Quit", danger: true, run: func(*Model) tea.Cmd { return tea.Quit }},
			{key: "s", label: "Stay"},
		}}
	return m, nil
}
