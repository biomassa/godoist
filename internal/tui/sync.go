package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/cache"
	"github.com/biomassa/godoist/internal/todoist"
)

// autoSyncEvery is the interval of the background sync. A sync also runs at start,
// after each write, when the terminal gets focus, and when the user pushes r.
const autoSyncEvery = 60 * time.Second

type (
	syncMsg struct {
		st  todoist.SyncState
		err error
		seq int // the number of the sync
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
		retryDesc string
		queued    bool // the change went into the offline queue
		offline   bool // it went into the queue because Todoist was not available
	}
	autoSyncMsg  struct{}
	completedMsg struct {
		tasks []todoist.Task
		err   error
	}
)

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
	m.syncSeq++
	return m.syncCmd(m.syncSeq)
}

// syncCmd syncs a copy of the replica and saves it to the cache. seq is the number of
// the sync.
func (m Model) syncCmd(seq int) tea.Cmd {
	st := m.st.Clone()
	client, token := m.client, m.token
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := client.Sync(ctx, &st); err != nil {
			return syncMsg{err: err, seq: seq}
		}
		_ = cache.Save(token, st)
		return syncMsg{st: st, seq: seq}
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
// A task change that goes into the offline queue gets " · offline" after its message.
// Other changes fail offline with "offline · try again later".
func (m *Model) write(done string, f func(context.Context) (string, error)) tea.Cmd {
	m.pending++
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ctx = todoist.WithQueuedFlag(ctx)
		text, err := f(ctx)
		if text == "" {
			text = done
		}
		switch {
		case err != nil && todoist.IsOffline(err):
			err = fmt.Errorf("offline · try again later")
		}
		return actionMsg{text: text, err: err, queued: err == nil && todoist.Queued(ctx), offline: todoist.QueuedOffline(ctx)}
	}
}

func (m *Model) simpleWrite(done string, f func(context.Context) error) tea.Cmd {
	return m.write(done, func(ctx context.Context) (string, error) { return "", f(ctx) })
}

// applyState shows a new replica.
func (m *Model) applyState(st todoist.SyncState) {
	m.st = st
	// The screen shows the replica with the changes that wait in the offline queue.
	view := st.Clone()
	m.pendingIDs = m.client.Queue().Overlay(&view)
	m.snap = view.Snapshot()
	m.mergeDone()
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
