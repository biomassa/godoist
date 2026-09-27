package todoist

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The offline queue keeps the task changes that could not go to Todoist, because the
// computer was offline. Sync sends them first, in their order, when Todoist is available
// again. Plain changes go as Sync API commands: each has a UUID, so Todoist does not apply
// a command two times. A quick add and a parsed name change need the REST parser, so
// they go as REST calls.
//
// While the queue has changes, new task changes also go into the queue, so that the
// order stays correct.

// Op kinds.
const (
	OpCommand = "command" // a Sync API command
	OpQuick   = "quick"   // a quick add: Todoist parses Text
	OpRename  = "rename"  // a new name that Todoist parses, as the TUI rename does
)

// Op is a change in the offline queue.
type Op struct {
	UUID   string         `json:"uuid"`
	Kind   string         `json:"kind"`
	Type   string         `json:"type,omitempty"`    // command type, e.g. "item_update"
	Args   map[string]any `json:"args,omitempty"`    // command arguments
	TempID string         `json:"temp_id,omitempty"` // the ID of a new item until Todoist gives the real ID
	Text   string         `json:"text,omitempty"`    // OpQuick, OpRename
	TaskID string         `json:"task_id,omitempty"` // OpRename
	Desc   *string        `json:"desc,omitempty"`    // OpRename: a new description, or nil
	Names  bool           `json:"names,omitempty"`   // OpRename: the text names a project
	Date   string         `json:"date,omitempty"`    // OpQuick: the date if the text has no date
	Added  time.Time      `json:"added"`
}

// Queue is the offline queue. It is safe for concurrent use. Each change is saved to the
// file at once, so that the queue stays after a crash or a quit.
type Queue struct {
	mu     sync.Mutex
	path   string
	ops    []Op
	temp   map[string]string // temporary ID → real ID, for changes that are made later
	errs   []string          // changes that Todoist refused, for the status line
	flushM sync.Mutex        // one flush at a time
}

// queueFile is the stored form of the queue.
type queueFile struct {
	Ops  []Op              `json:"ops"`
	Temp map[string]string `json:"temp,omitempty"`
}

// OpenQueue loads the queue from path. A missing file gives an empty queue.
func OpenQueue(path string) (*Queue, error) {
	q := &Queue{path: path, temp: map[string]string{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return q, nil
	}
	if err != nil {
		return q, err
	}
	var f queueFile
	if err := json.Unmarshal(b, &f); err != nil {
		return q, fmt.Errorf("the offline queue %s is damaged: %w", path, err)
	}
	q.ops = f.Ops
	if f.Temp != nil {
		q.temp = f.Temp
	}
	return q, nil
}

// save writes the queue to its file. The caller holds q.mu.
func (q *Queue) save() error {
	if q.path == "" {
		return nil
	}
	f := queueFile{Ops: q.ops}
	if len(q.ops) > 0 {
		// The file keeps the ID map only while changes wait. The map in memory stays for
		// this run: the screen can show a temporary ID until the next sync.
		f.Temp = q.temp
	}
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(q.path), 0o700); err != nil {
		return err
	}
	tmp := q.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, q.path)
}

// Len is the number of changes that wait.
func (q *Queue) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.ops)
}

// Ops returns a copy of the changes that wait, in their order.
func (q *Queue) Ops() []Op {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]Op(nil), q.ops...)
}

// TakeErrors returns the refused changes since the last call and forgets them.
func (q *Queue) TakeErrors() []string {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	e := q.errs
	q.errs = nil
	return e
}

// add puts op at the end of the queue. IDs that Todoist gave already replace temporary IDs.
func (q *Queue) add(op Op) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if op.UUID == "" {
		op.UUID = newUUID()
	}
	op.Added = time.Now()
	if op.Args != nil { // the JSON form, so that the args look the same after a restart
		b, err := json.Marshal(op.Args)
		if err != nil {
			return err
		}
		op.Args = nil
		if err := json.Unmarshal(b, &op.Args); err != nil {
			return err
		}
	}
	op.Args = replaceIDs(op.Args, q.temp).(map[string]any)
	if r, ok := q.temp[op.TaskID]; ok {
		op.TaskID = r
	}
	q.ops = append(q.ops, op)
	return q.save()
}

// remove takes the change with uuid out of the queue.
func (q *Queue) remove(uuid string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, op := range q.ops {
		if op.UUID == uuid {
			q.ops = append(q.ops[:i], q.ops[i+1:]...)
			break
		}
	}
	_ = q.save()
}

// mapIDs records the real IDs of new items and puts them in the changes that wait.
func (q *Queue) mapIDs(m map[string]string) {
	if len(m) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for t, r := range m {
		q.temp[t] = r
	}
	for i := range q.ops {
		q.ops[i].Args = replaceIDs(q.ops[i].Args, m).(map[string]any)
		if r, ok := m[q.ops[i].TaskID]; ok {
			q.ops[i].TaskID = r
		}
	}
	_ = q.save()
}

// refuse records a change that Todoist did not accept.
func (q *Queue) refuse(msg string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.errs = append(q.errs, msg)
}

// replaceIDs replaces the strings that are keys of m in v (maps, lists, and strings).
func replaceIDs(v any, m map[string]string) any {
	switch x := v.(type) {
	case nil:
		return map[string]any(nil)
	case string:
		if r, ok := m[x]; ok {
			return r
		}
		return x
	case map[string]any:
		if x == nil {
			return x
		}
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = replaceIDs(e, m)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = replaceIDs(e, m)
		}
		return out
	}
	return v
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80 // UUID version 4
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// NewTempID returns a temporary ID for a new item.
func NewTempID() string { return "tmp-" + newUUID() }

// IsOffline reports whether err means that the request did not get to Todoist or that
// no answer came back: no network, a failed name lookup, a refused connection, or a time-out.
func IsOffline(err error) bool {
	if err == nil {
		return false
	}
	var ne net.Error
	var ue *url.Error
	return errors.As(err, &ne) || errors.As(err, &ue) || errors.Is(err, context.DeadlineExceeded)
}

// notSent reports whether err means that the request surely did not get to Todoist: the
// name lookup or the connection failed. After a time-out, Todoist can have done the change.
func notSent(err error) bool {
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) {
		return true
	}
	var op *net.OpError
	return errors.As(err, &op) && (op.Op == "dial" || op.Op == "proxyconnect")
}

// ErrMaybeAdded is the error of an add that got no answer from Todoist. The add does not
// go into the queue, because Todoist can have done it already.
var ErrMaybeAdded = errors.New("no answer from Todoist, so it may be added already. Look before you try again")

// queuedKey marks a context whose writes went into the queue. See WithQueuedFlag.
type queuedKey struct{}

// queuedFlag is what a write with a WithQueuedFlag context did.
type queuedFlag struct{ queued, offline bool }

// WithQueuedFlag returns a context that records if a write goes into the queue.
// Queued and QueuedOffline report it after the writes.
func WithQueuedFlag(ctx context.Context) context.Context {
	return context.WithValue(ctx, queuedKey{}, &queuedFlag{})
}

// Queued reports whether a write with ctx went into the offline queue.
func Queued(ctx context.Context) bool {
	p, _ := ctx.Value(queuedKey{}).(*queuedFlag)
	return p != nil && p.queued
}

// QueuedOffline reports whether a write with ctx went into the queue because Todoist was
// not available. Other queued writes wait only behind older changes.
func QueuedOffline(ctx context.Context) bool {
	p, _ := ctx.Value(queuedKey{}).(*queuedFlag)
	return p != nil && p.offline
}

func markQueued(ctx context.Context, offline bool) {
	if p, _ := ctx.Value(queuedKey{}).(*queuedFlag); p != nil {
		p.queued = true
		p.offline = p.offline || offline
	}
}

// taskWrite runs a task change. Without a queue, it runs rest. With a queue, it runs
// rest if no change waits; if rest fails because the computer is offline, or if changes
// wait, op goes into the queue. An add goes into the queue only if the request surely did
// not get to Todoist, so that a retry cannot add it two times.
// queued is true if op went into the queue.
func (c *Client) taskWrite(ctx context.Context, op Op, isAdd bool, rest func() error) (queued bool, err error) {
	if c.queue == nil {
		return false, rest()
	}
	offline := false
	if c.queue.Len() == 0 {
		err := rest()
		if err == nil || !IsOffline(err) {
			return false, err
		}
		if isAdd && !notSent(err) {
			// Not %w: this is not an offline error, so the TUI shows this message as it is.
			return false, fmt.Errorf("%w (%v)", ErrMaybeAdded, err)
		}
		offline = true
	}
	if err := c.queue.add(op); err != nil {
		return false, fmt.Errorf("could not save the change for later: %w", err)
	}
	markQueued(ctx, offline)
	return true, nil
}

// command makes a Sync command op.
func command(typ string, args map[string]any) Op {
	return Op{Kind: OpCommand, Type: typ, Args: args}
}

// flush sends the queue to Todoist in order. It stops at the first offline error and
// returns it, and the rest waits for the next sync. A change that Todoist refuses goes
// out of the queue and into the queue errors. After a refusal, s gets a full sync, because
// the local data can have the refused change.
func (c *Client) flush(ctx context.Context, s *SyncState) error {
	q := c.queue
	q.flushM.Lock()
	defer q.flushM.Unlock()
	raw := *c
	raw.queue = nil // the calls of the flush go to Todoist directly
	for {
		ops := q.Ops()
		if len(ops) == 0 {
			return nil
		}
		var batch []Op
		for _, op := range ops {
			if op.Kind != OpCommand || len(batch) == 100 {
				break
			}
			batch = append(batch, op)
		}
		if len(batch) > 0 {
			if err := raw.sendCommands(ctx, q, batch, s); err != nil {
				return err
			}
			continue
		}
		op := ops[0]
		var err error
		switch op.Kind {
		case OpQuick:
			var t Task
			t, err = raw.quickAddNow(ctx, op.Text)
			if IsOffline(err) && !notSent(err) {
				// Todoist can have added it. A retry could add it two times.
				q.refuse("no answer from Todoist for " + describeOp(op) + ". Look before you add it again")
				q.remove(op.UUID)
				continue
			}
			if err != nil {
				break
			}
			if op.TempID != "" {
				q.mapIDs(map[string]string{op.TempID: t.ID})
			}
			q.remove(op.UUID) // the add is done. The date below is a separate change.
			if op.Date != "" && t.Due == nil {
				due := map[string]any{"due_date": op.Date}
				if _, derr := raw.UpdateTask(ctx, t.ID, due); derr != nil {
					if IsOffline(derr) || transient(derr) {
						_ = q.addFront(command("item_update", syncFields(t.ID, due)))
						return derr
					}
					q.refuse("set the date of “" + op.Text + "”: " + derr.Error())
				}
			}
			continue
		case OpRename:
			cur, ok := s.task(op.TaskID)
			if !ok {
				err = fmt.Errorf("the task is not in Todoist")
				break
			}
			_, _, err = raw.ApplyName(ctx, cur, op.Text, op.Desc, op.Names)
		default:
			err = fmt.Errorf("unknown change %q", op.Kind)
		}
		if IsOffline(err) || transient(err) {
			return err // try again at the next sync
		}
		if err != nil {
			q.refuse(describeOp(op) + ": " + err.Error())
			s.Token = "" // a full sync removes the refused change from the local data
		}
		q.remove(op.UUID)
	}
}

// transient reports whether Todoist refused a request for a short time: too many requests
// (HTTP 429) or a server error (HTTP 500 or more). The queue keeps the changes then.
func transient(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && (ae.Status == http.StatusTooManyRequests || ae.Status >= 500)
}

// addFront puts op at the start of the queue, so that it goes first.
func (q *Queue) addFront(op Op) error {
	if err := q.add(op); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	last := q.ops[len(q.ops)-1]
	copy(q.ops[1:], q.ops[:len(q.ops)-1])
	q.ops[0] = last
	return q.save()
}

// sendCommands sends a batch of Sync commands and removes them from the queue.
func (c *Client) sendCommands(ctx context.Context, q *Queue, batch []Op, s *SyncState) error {
	list := make([]map[string]any, len(batch))
	for i, op := range batch {
		cmd := map[string]any{"type": op.Type, "uuid": op.UUID, "args": op.Args}
		if op.TempID != "" {
			cmd["temp_id"] = op.TempID
		}
		list[i] = cmd
	}
	j, err := json.Marshal(list)
	if err != nil {
		return err
	}
	var resp struct {
		SyncStatus    map[string]json.RawMessage `json:"sync_status"`
		TempIDMapping map[string]string          `json:"temp_id_mapping"`
	}
	if err := c.do(ctx, http.MethodPost, "/sync", nil, url.Values{"commands": {string(j)}}, &resp); err != nil {
		if IsOffline(err) || transient(err) {
			return err // the commands have UUIDs, so a retry does not apply them two times
		}
		// Todoist refused the whole request. Drop the batch, so that the queue does not stop.
		for _, op := range batch {
			q.refuse(describeOp(op) + ": " + err.Error())
			q.remove(op.UUID)
		}
		s.Token = ""
		return nil
	}
	q.mapIDs(resp.TempIDMapping)
	for _, op := range batch {
		if st := string(resp.SyncStatus[op.UUID]); st != `"ok"` && st != "" {
			q.refuse(describeOp(op) + ": " + st)
			s.Token = ""
		}
		q.remove(op.UUID)
	}
	return nil
}

// describeOp names a change for an error message.
func describeOp(op Op) string {
	switch op.Kind {
	case OpQuick:
		return "add “" + op.Text + "”"
	case OpRename:
		return "rename to “" + op.Text + "”"
	}
	return strings.ReplaceAll(op.Type, "_", " ")
}

// task returns the task with id from the replica.
func (s *SyncState) task(id string) (Task, bool) {
	for _, t := range s.Tasks {
		if t.ID == id {
			return t, true
		}
	}
	return Task{}, false
}

// ApplyName gives task cur a new name that Todoist parses, as quick add does. A date,
// p1–p4, or @labels in text change those fields. The other fields stay. If names is true
// (the text names a project), the task moves to the parsed project and section. desc, if
// not nil, is the new description. ApplyName returns the parsed task and the fields that
// changed.
func (c *Client) ApplyName(ctx context.Context, cur Task, text string, desc *string, names bool) (Task, map[string]any, error) {
	p, err := c.Parse(ctx, text)
	if err != nil {
		return p, nil, err
	}
	fields := map[string]any{"content": p.Content}
	if desc != nil {
		fields["description"] = *desc
	}
	if p.Due != nil {
		fields["due_string"] = p.Due.String
	}
	if p.Priority > 1 && p.Priority != cur.Priority {
		fields["priority"] = p.Priority
	}
	if len(p.Labels) > 0 {
		labels := append([]string(nil), cur.Labels...)
		for _, l := range p.Labels {
			found := false
			for _, x := range labels {
				found = found || x == l
			}
			if !found {
				labels = append(labels, l)
			}
		}
		fields["labels"] = labels
	}
	if _, err := c.UpdateTask(ctx, cur.ID, fields); err != nil {
		return p, fields, err
	}
	// Quick add puts a task without #project in the Inbox. Move only if the text named a project.
	if names && (p.ProjectID != cur.ProjectID || p.Section() != cur.Section()) {
		if err := c.Move(ctx, cur.ID, p.ProjectID, p.Section()); err != nil {
			return p, fields, fmt.Errorf("renamed, but the move failed: %w", err)
		}
	}
	return p, fields, nil
}

// QueueName puts a parsed name change into the queue. The TUI uses it when the parser is
// not available because the computer is offline, or when changes wait.
func (c *Client) QueueName(ctx context.Context, cur Task, text string, desc *string, names bool) error {
	if c.queue == nil {
		return fmt.Errorf("no offline queue")
	}
	if err := c.queue.add(Op{Kind: OpRename, TaskID: cur.ID, Text: text, Desc: desc, Names: names}); err != nil {
		return err
	}
	markQueued(ctx, c.queue.Len() == 1) // the only change: it waits because the parser was not available
	return nil
}

// Queue returns the offline queue of the client, or nil.
func (c *Client) Queue() *Queue { return c.queue }

// SetQueue gives the client an offline queue. The CLI has no queue: its changes go to
// Todoist at once, or fail.
func (c *Client) SetQueue(q *Queue) { c.queue = q }
