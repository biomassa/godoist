package tui

import (
	"net"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// offlineModel is the work project with a client that has an offline queue and no network.
func offlineModel(t *testing.T) Model {
	t.Helper()
	m := openWork(t)
	q, err := todoist.OpenQueue(filepath.Join(t.TempDir(), "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	m.client = todoist.New("test")
	m.client.SetBaseURL("http://" + addr)
	m.client.SetQueue(q)
	return m
}

// run runs cmd and gives its messages to the model, as the program does.
func run(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, c := range b {
			m = run(t, m, c)
		}
		return m
	}
	if msg == nil {
		return m
	}
	nm, next := m.Update(msg)
	m = nm.(Model)
	if _, isSync := msg.(syncMsg); isSync { // one sync is enough for a test
		return m
	}
	return run(t, m, next)
}

// press sends a key and runs the commands that it makes.
func press(t *testing.T, m Model, k string) Model {
	t.Helper()
	nm, cmd := m.Update(key(k))
	return run(t, nm.(Model), cmd)
}

// Offline, a completion waits in the queue and the task goes from the list at once.
// The status line shows the waiting change, and quit asks first.
func TestOfflineComplete(t *testing.T) {
	m := offlineModel(t)
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 2), release(x, 2)) // alpha
	m = press(t, m, "x")
	if got := rowNames(m); strings.Contains(got, "alpha") {
		t.Errorf("rows = %q, want alpha gone", got)
	}
	if m.client.Queue().Len() != 1 || !strings.Contains(m.status, "offline") {
		t.Errorf("queue = %d status = %q", m.client.Queue().Len(), m.status)
	}
	if !m.offline || !strings.Contains(stripANSI(m.bottomBar()), "offline · 1 change waiting") {
		t.Errorf("offline = %v bar = %q", m.offline, stripANSI(m.bottomBar()))
	}
	m = press(t, m, "q")
	if m.confirm == nil || !strings.Contains(m.confirm.text, "1 change is not in Todoist yet") {
		t.Fatalf("quit did not ask: %+v", m.confirm)
	}
}

// Offline, a quick add waits in the queue and shows with ⏳. ctrl+z after a queued
// completion brings the task back.
func TestOfflineAddAndUndo(t *testing.T) {
	m := offlineModel(t)
	m = press(t, m, "a")
	m = typeText(t, m, "milk")
	m = press(t, m, "enter")
	found := false
	for _, r := range m.rows {
		if r.task != nil && r.task.Content == "milk" && m.pendingIDs[r.task.ID] {
			found = true
		}
	}
	if !found {
		t.Fatalf("rows = %q, want milk with ⏳", rowNames(m))
	}
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 2), release(x, 2)) // alpha
	m = press(t, m, "x")
	m = press(t, m, "ctrl+z")
	if !strings.Contains(rowNames(m), "alpha") {
		t.Errorf("rows = %q, want alpha back after ctrl+z", rowNames(m))
	}
}

// Offline, e puts the name into the queue for Todoist to parse later.
func TestOfflineRename(t *testing.T) {
	m := offlineModel(t)
	x := m.layout().mid.x + 10
	m = send(t, m, click(x, 2), release(x, 2)) // alpha
	m = press(t, m, "e")
	for range len("alpha") {
		m = send(t, m, key("backspace"))
	}
	m = typeText(t, m, "omega fri")
	m = press(t, m, "enter")
	ops := m.client.Queue().Ops()
	if len(ops) != 1 || ops[0].Kind != todoist.OpRename || !strings.Contains(m.status, "parses it later") {
		t.Fatalf("ops = %+v status = %q", ops, m.status)
	}
	if !strings.Contains(rowNames(m), "omega fri") {
		t.Errorf("rows = %q, want the typed name", rowNames(m))
	}
}

// Offline, a task added in Today shows in Today.
func TestOfflineAddInToday(t *testing.T) {
	m := offlineModel(t)
	m = send(t, m, click(5, 4)) // Today
	m = press(t, m, "a")
	m = typeText(t, m, "milk")
	m = press(t, m, "enter")
	if !strings.Contains(rowNames(m), "milk") {
		t.Errorf("Today rows = %q, want milk", rowNames(m))
	}
}
