package tui

import (
	"context"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/biomassa/godoist/internal/todoist"
)

// calFocus is the part of the date dialog that gets the keys.
type calFocus int

const (
	calText calFocus = iota // the text field, parsed by Todoist
	calGrid                 // the month calendar
	calTime                 // the time field
)

// calSource is the input that enter saves: the input that the user changed last.
type calSource int

const (
	srcNone calSource = iota
	srcText
	srcCal // the calendar, the quick picks, or the time field
)

// Line numbers inside the date dialog border. The mouse handler uses them too.
const (
	calLineText  = 1
	calLineNote  = 2
	calLinePicks = 3
	calLineMonth = 5
	calLineDays  = 6
	calLineWeek0 = 7 // six week rows follow
	calLineTime  = 14
	calLineHint  = 16 // two lines: a long hint wraps
	calLineSave  = 18
	calLines     = 19
	calWidth     = 54 // outer width
	calGridWidth = 28 // seven cells of four columns
)

// calDialog is the date dialog: a text field that Todoist parses, a month calendar,
// and a time field. Tab from a changed text parses it once and moves the calendar there.
type calDialog struct {
	taskIDs    []string
	rules      map[string][2]string // recurring tasks: ID → {rule, lang}
	title      string
	text       textinput.Model
	origText   string
	lastParsed string // the text of the last parse, so that tab does not parse it again
	timeIn     textinput.Model
	focus      calFocus
	source     calSource
	day        time.Time // selected day, local midnight
	month      time.Time // first day of the shown month
	cleared    bool      // "No date" is selected
	parsing    bool
	saving     bool
	askRecur   bool   // waiting for o or r after enter on a recurring task
	recurring  bool   // at least one task repeats
	note       string // parse result or error, under the text
	noteErr    bool
}

type (
	calParsedMsg struct {
		text string
		due  *todoist.Due
		err  error
	}
	calSavedMsg struct {
		text string
		err  error
	}
)

func dayOf(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, time.Local)
}

func monthOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
}

// openCalendar opens the date dialog for t with text in the text field.
func (m *Model) openCalendar(t *todoist.Task, text string) tea.Cmd {
	return m.openCalendarWith([]*todoist.Task{t}, text)
}

// openCalendarFor opens the date dialog for several tasks. The text field has the due
// string if all tasks have the same one.
func (m *Model) openCalendarFor(ts []*todoist.Task) tea.Cmd {
	text := ""
	if ts[0].Due != nil {
		text = ts[0].Due.String
	}
	for _, t := range ts[1:] {
		if t.Due == nil || t.Due.String != text {
			text = ""
		}
	}
	return m.openCalendarWith(ts, text)
}

// openCalendarWith opens the date dialog for ts. The calendar starts on the first task's day.
func (m *Model) openCalendarWith(ts []*todoist.Task, text string) tea.Cmd {
	t := ts[0]
	title := "Due date · " + plain(t.Content)
	if len(ts) > 1 {
		title = fmt.Sprintf("Due date · %d tasks", len(ts))
	}
	c := &calDialog{taskIDs: joinIDs(ts), rules: map[string][2]string{}, title: title, origText: text, lastParsed: text}
	for _, x := range ts {
		if x.Due != nil && x.Due.IsRecurring {
			c.recurring = true
			c.rules[x.ID] = [2]string{x.Due.String, x.Due.Lang}
		}
	}
	c.text = textinput.New()
	c.text.Prompt = " "
	c.text.Placeholder = "fri 9am · every mon · jan 15 · no date"
	c.text.CharLimit = 200
	c.text.SetWidth(calWidth - 6)
	c.text.SetValue(text)
	c.text.CursorEnd()
	c.timeIn = textinput.New()
	c.timeIn.Prompt = ""
	c.timeIn.Placeholder = "all day"
	c.timeIn.CharLimit = 5
	c.timeIn.SetWidth(8)
	c.day = dayOf(time.Now())
	if due, hasTime, ok := t.Due.Time(); ok {
		c.day = dayOf(due)
		if hasTime {
			c.timeIn.SetValue(due.Format("15:04"))
		}
	}
	c.month = monthOf(c.day)
	m.cal = c
	return c.text.Focus()
}

// setCalFocus moves the keys to part f of the dialog.
func (c *calDialog) setCalFocus(f calFocus) tea.Cmd {
	c.focus = f
	c.text.Blur()
	c.timeIn.Blur()
	switch f {
	case calText:
		return c.text.Focus()
	case calTime:
		return c.timeIn.Focus()
	}
	return nil
}

// selectDay selects d in the calendar and makes the calendar the input that enter saves.
func (c *calDialog) selectDay(d time.Time) {
	c.day, c.month, c.cleared, c.source = dayOf(d), monthOf(d), false, srcCal
}

// calTab moves the focus by d. Leaving a changed text parses it once.
func (m Model) calTab(d int) (tea.Model, tea.Cmd) {
	c := m.cal
	leaving := c.focus == calText
	next := c.setCalFocus(calFocus((int(c.focus) + d + 3) % 3))
	text := strings.TrimSpace(c.text.Value())
	if !leaving || text == "" || text == c.lastParsed || strings.EqualFold(text, "no date") {
		return m, next
	}
	c.lastParsed, c.parsing, c.note, c.noteErr = text, true, "parsing…", false
	m.pending++ // the parse adds and deletes a temporary task, so quit waits for it
	client := m.client
	return m, tea.Batch(next, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		due, err := client.ParseDue(ctx, text)
		return calParsedMsg{text: text, due: due, err: err}
	})
}

func (m Model) calParsed(msg calParsedMsg) (tea.Model, tea.Cmd) {
	m.pending = max(0, m.pending-1)
	c := m.cal
	if c == nil || msg.text != c.lastParsed {
		return m, nil
	}
	c.parsing = false
	switch {
	case msg.err != nil:
		c.note, c.noteErr = "parse failed: "+msg.err.Error(), true
	case msg.due == nil:
		c.note, c.noteErr = "Todoist found no date in the text", true
	default:
		t, hasTime, _ := msg.due.Time()
		c.day, c.month = dayOf(t), monthOf(t)
		c.timeIn.SetValue("")
		if hasTime {
			c.timeIn.SetValue(t.Format("15:04"))
		}
		c.note, c.noteErr = "Todoist reads: "+todoist.FormatDue(msg.due, time.Now()), false
		if msg.due.IsRecurring {
			c.note += " ↻ " + msg.due.String
		}
	}
	return m, nil
}

// quickPick is a shortcut in the row above the calendar.
type quickPick struct {
	label string
	day   time.Time
	none  bool // "No date"
}

// nextWeekday returns the first day after from that is wd.
func nextWeekday(from time.Time, wd time.Weekday) time.Time {
	d := dayOf(from).AddDate(0, 0, 1)
	for d.Weekday() != wd {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// quickPicks uses the "next week" and "weekend" days from the Todoist settings.
func (m Model) quickPicks() []quickPick {
	now := time.Now()
	u := m.snap.User
	return []quickPick{
		{label: "Today", day: dayOf(now)},
		{label: "Tomorrow", day: dayOf(now).AddDate(0, 0, 1)},
		{label: "Next week", day: nextWeekday(now, todoist.Weekday(u.NextWeek, time.Monday))},
		{label: "Weekend", day: nextWeekday(now, todoist.Weekday(u.WeekendStartDay, time.Saturday))},
		{label: "No date", none: true},
	}
}

func (m Model) applyPick(i int) {
	picks := m.quickPicks()
	if i < 0 || i >= len(picks) {
		return
	}
	c := m.cal
	if picks[i].none {
		c.cleared, c.source = true, srcCal
		return
	}
	c.selectDay(picks[i].day)
}

// updateCalendar handles keys while the date dialog is open.
func (m Model) updateCalendar(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	c := m.cal
	key := msg.String()
	if c.saving {
		return m, nil
	}
	if c.askRecur {
		switch key {
		case "o":
			c.askRecur = false
			return m.saveCalDate(true)
		case "r":
			c.askRecur = false
			return m.saveCalDate(false)
		case "esc":
			c.askRecur = false
		}
		return m, nil
	}
	switch key {
	case "esc":
		m.cal = nil
		m.setStatus("cancelled", false)
		return m, nil
	case "enter":
		if c.parsing {
			return m, nil
		}
		return m.saveCalendar()
	case "tab":
		return m.calTab(1)
	case "shift+tab":
		return m.calTab(-1)
	}
	var cmd tea.Cmd
	switch c.focus {
	case calText:
		before := c.text.Value()
		c.text, cmd = c.text.Update(msg)
		if c.text.Value() != before {
			c.source = srcText
		}
	case calTime:
		before := c.timeIn.Value()
		c.timeIn, cmd = c.timeIn.Update(msg)
		if c.timeIn.Value() != before {
			c.source, c.cleared = srcCal, false
		}
	case calGrid:
		switch key {
		case "left":
			c.selectDay(c.day.AddDate(0, 0, -1))
		case "right":
			c.selectDay(c.day.AddDate(0, 0, 1))
		case "up":
			c.selectDay(c.day.AddDate(0, 0, -7))
		case "down":
			c.selectDay(c.day.AddDate(0, 0, 7))
		case "pgup":
			c.selectDay(c.day.AddDate(0, -1, 0))
		case "pgdown":
			c.selectDay(c.day.AddDate(0, 1, 0))
		case "home":
			c.selectDay(time.Now())
		case "1", "2", "3", "4", "5":
			m.applyPick(int(key[0] - '1'))
		}
	}
	return m, cmd
}

// parseClock reads "9", "9:30", "0930", or "21:05". An empty string means all day.
func parseClock(s string) (h, min int, allDay, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, true, true
	}
	hs, ms, found := strings.Cut(s, ":")
	if !found {
		switch len(s) {
		case 1, 2:
			hs, ms = s, "0"
		case 3, 4:
			hs, ms = s[:len(s)-2], s[len(s)-2:]
		default:
			return 0, 0, false, false
		}
	}
	h, err1 := strconv.Atoi(hs)
	min, err2 := strconv.Atoi(ms)
	if err1 != nil || err2 != nil || h < 0 || h > 23 || min < 0 || min > 59 {
		return 0, 0, false, false
	}
	return h, min, false, true
}

// saveCalendar saves the input that the user changed last.
func (m Model) saveCalendar() (tea.Model, tea.Cmd) {
	c := m.cal
	switch {
	case c.source == srcNone:
		m.cal = nil
		m.setStatus("no changes", false)
		return m, nil
	case c.source == srcText:
		text := strings.TrimSpace(c.text.Value())
		if text == "" {
			text = "no date"
		}
		return m.calSave(func(ctx context.Context) (string, error) { return dueByTextAll(ctx, m.client, c.taskIDs, text) })
	case c.cleared:
		return m.calSave(func(ctx context.Context) (string, error) { return dueByTextAll(ctx, m.client, c.taskIDs, "no date") })
	}
	if _, _, _, ok := parseClock(c.timeIn.Value()); !ok {
		c.note, c.noteErr = "the time must look like 9:30 or 21:00, or be empty for all day", true
		return m, nil
	}
	if c.recurring {
		c.askRecur = true
		return m, nil
	}
	return m.saveCalDate(false)
}

// saveCalDate saves the calendar day and time. If keepRepeat is true, only the current
// occurrence moves. If not, the task gets this date and loses its repeat.
func (m Model) saveCalDate(keepRepeat bool) (tea.Model, tea.Cmd) {
	c := m.cal
	h, mi, allDay, _ := parseClock(c.timeIn.Value())
	date := c.day.Format("2006-01-02")
	label := c.day.Format("Mon 2 Jan")
	fields := map[string]any{"due_date": date}
	if !allDay {
		date = time.Date(c.day.Year(), c.day.Month(), c.day.Day(), h, mi, 0, 0, time.Local).Format("2006-01-02T15:04:05")
		label += fmt.Sprintf(" %02d:%02d", h, mi)
		fields = map[string]any{"due_datetime": date}
	}
	client, ids, rules := m.client, c.taskIDs, c.rules
	return m.calSave(func(ctx context.Context) (string, error) {
		for _, id := range ids {
			if r, ok := rules[id]; ok && keepRepeat {
				if err := client.MoveOccurrence(ctx, id, date, r[0], r[1]); err != nil {
					return "", err
				}
				continue
			}
			if _, err := client.UpdateTask(ctx, id, fields); err != nil {
				return "", err
			}
		}
		switch {
		case len(ids) > 1:
			return fmt.Sprintf("Due → %s for %d task(s)", label, len(ids)), nil
		case keepRepeat:
			return "Moved this occurrence to " + label + " · repeats " + rules[ids[0]][0], nil
		}
		return "Due → " + label, nil
	})
}

// dueByTextAll sends one due string for all tasks.
func dueByTextAll(ctx context.Context, client *todoist.Client, ids []string, text string) (string, error) {
	var msg string
	for _, id := range ids {
		var err error
		if msg, err = dueByText(ctx, client, id, text); err != nil {
			return "", err
		}
	}
	if len(ids) > 1 {
		msg = fmt.Sprintf("%s for %d task(s)", msg, len(ids))
	}
	return msg, nil
}

// dueByText sends a due string for Todoist to parse. "no date" removes the date.
func dueByText(ctx context.Context, client *todoist.Client, id, text string) (string, error) {
	t, err := client.UpdateTask(ctx, id, map[string]any{"due_string": text})
	if err != nil {
		return "", err
	}
	if t.Due == nil {
		return "Due date removed", nil
	}
	msg := "Due → " + todoist.FormatDue(t.Due, time.Now())
	if t.Due.IsRecurring {
		msg += " ↻ " + t.Due.String
	}
	return msg, nil
}

// calSave runs a save. The dialog stays open until the save succeeds.
func (m Model) calSave(f func(context.Context) (string, error)) (tea.Model, tea.Cmd) {
	m.cal.saving = true
	m.cal.note, m.cal.noteErr = "saving…", false
	m.pending++
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		text, err := f(ctx)
		return calSavedMsg{text: text, err: err}
	}
}

func (m Model) calSaved(msg calSavedMsg) (tea.Model, tea.Cmd) {
	m.pending = max(0, m.pending-1)
	if msg.err != nil {
		if m.cal != nil {
			m.cal.saving = false
			m.cal.note, m.cal.noteErr = "save failed: "+msg.err.Error(), true
		}
		return m, nil
	}
	m.cal = nil
	m.setStatus(msg.text, false)
	if m.quitting && m.pending == 0 {
		return m, tea.Quit
	}
	next := m.startSync()
	return m, next
}

// ---- drawing and mouse ----

func (m Model) calRect() rect {
	w, h := min(calWidth, m.width), calLines+2
	return rect{max(0, (m.width-w)/2), max(0, (m.height-h)/3), w, h}
}

// calGridLeft is the inner x of the first calendar column.
func calGridLeft() int { return (calWidth - 2 - calGridWidth) / 2 }

// calGridStart is the first day in the first calendar row, from the Todoist week start.
func (m Model) calGridStart() time.Time {
	start := todoist.Weekday(m.snap.User.StartDay, time.Monday)
	first := m.cal.month
	off := (int(first.Weekday()) - int(start) + 7) % 7
	return first.AddDate(0, 0, -off)
}

// pickRanges returns the inner x range of each quick pick in the picks row.
func (m Model) pickRanges() [][2]int {
	var out [][2]int
	x := 1
	for _, p := range m.quickPicks() {
		w := lipgloss.Width(p.label)
		out = append(out, [2]int{x, x + w})
		x += w + 3
	}
	return out
}

// busyDays counts the tasks due on each day ("2006-01-02").
func (m Model) busyDays() map[string]int {
	busy := map[string]int{}
	for _, t := range m.snap.Tasks {
		if d, _, ok := t.Due.Time(); ok {
			busy[d.Format("2006-01-02")]++
		}
	}
	return busy
}

// calBox draws the date dialog.
func (m Model) calBox() string {
	cd := m.cal
	r := m.calRect()
	inner := r.w - 2
	lines := make([]string, calLines)
	lines[calLineText] = cd.text.View()
	if cd.note != "" {
		hex := hexMuted
		if cd.noteErr {
			hex = hexOverdue
		}
		lines[calLineNote] = " " + st(hex).Render(trunc(cd.note, inner-2))
	}

	var picks []string
	for _, p := range m.quickPicks() {
		hex := hexText
		if p.none {
			hex = hexOverdue
		}
		label := st(hex).Render(p.label)
		if (p.none && cd.cleared) || (!p.none && !cd.cleared && cd.source == srcCal && p.day.Equal(cd.day)) {
			label = lipgloss.NewStyle().Background(cd.pickBg()).Foreground(c(fg(hexAccent))).Bold(true).Render(p.label)
		}
		picks = append(picks, label)
	}
	lines[calLinePicks] = " " + strings.Join(picks, st(hexDim).Render(" · "))

	gl := calGridLeft()
	name := cd.month.Format("January 2006")
	gap := calGridWidth - 2 - lipgloss.Width(name)
	lines[calLineMonth] = strings.Repeat(" ", gl) + st(fg(hexAccent)).Bold(true).Render("‹") +
		strings.Repeat(" ", gap/2) + st(hexText).Bold(true).Render(name) + strings.Repeat(" ", gap-gap/2) +
		st(fg(hexAccent)).Bold(true).Render("›")

	start := m.calGridStart()
	var head strings.Builder
	for i := 0; i < 7; i++ {
		head.WriteString(fmt.Sprintf("%-4s", start.AddDate(0, 0, i).Format("Mon")[:2]))
	}
	lines[calLineDays] = strings.Repeat(" ", gl) + st(hexMuted).Render(head.String())

	busy := m.busyDays()
	today := dayOf(time.Now())
	for w := 0; w < 6; w++ {
		var row strings.Builder
		row.WriteString(strings.Repeat(" ", gl))
		for col := 0; col < 7; col++ {
			d := start.AddDate(0, 0, w*7+col)
			if d.Month() != cd.month.Month() {
				row.WriteString("    ")
				continue
			}
			dot := " "
			if busy[d.Format("2006-01-02")] > 0 {
				dot = "•"
			}
			cell := fmt.Sprintf("%2d%s", d.Day(), dot)
			style := lipgloss.NewStyle().Foreground(c(hexText))
			switch {
			case d.Equal(today):
				style = style.Foreground(c(hexToday)).Bold(true)
			case d.Before(today):
				style = style.Foreground(c(hexDim))
			}
			if d.Equal(cd.day) && !cd.cleared {
				style = style.Background(cd.pickBg()).Foreground(c(fg(hexAccent))).Bold(true)
			}
			row.WriteString(style.Render(cell) + " ")
		}
		lines[calLineWeek0+w] = row.String()
	}

	lines[calLineTime] = " " + st(hexMuted).Render("time › ") + cd.timeIn.View()

	var hint string
	switch cd.focus {
	case calText:
		hint = "tab parses the text and moves to the calendar · enter save · esc cancel"
	case calGrid:
		hint = "arrows day · PgUp/PgDn month · Home today · 1–5 picks · enter save"
	case calTime:
		hint = "HH:MM, empty = all day · enter save · tab next"
	}
	hintHex := hexDim
	if cd.askRecur {
		hint, hintHex = "recurring task · o moves this occurrence and keeps the repeat · r replaces the repeat · esc cancel", hexOverdue
		if len(cd.taskIDs) > 1 {
			hint = "recurring tasks · o moves their occurrences and keeps the repeats · r replaces the repeats · esc cancel"
		}
	}
	for i, l := range wrapHint(hint, hintHex, inner-1) {
		if i < 2 {
			lines[calLineHint+i] = l
		}
	}
	switch {
	case cd.askRecur:
		lines[calLineSave] = " " + st(hexOverdue).Bold(true).Render("choose o or r")
	case cd.source == srcText:
		lines[calLineSave] = " " + st(hexMuted).Render("enter saves the text")
	case cd.cleared:
		lines[calLineSave] = " " + st(hexMuted).Render("enter removes the date")
	case cd.source == srcCal:
		lines[calLineSave] = " " + st(hexMuted).Render("enter saves the calendar day and time")
	}
	return box(st(fg(hexAccent)).Bold(true).Render(cd.title), padLines(lines, inner, calLines), r.w, fg(hexAccent))
}

// pickBg is the background of the selected day and quick pick. It is stronger while the
// calendar has the focus.
func (cd *calDialog) pickBg() color.Color {
	if cd.focus == calGrid {
		return c(tint(hexAccent))
	}
	return c(hexSelBg)
}

// calClick handles a click in the date dialog. A click outside cancels the dialog.
func (m Model) calClick(x, y int, dbl bool) (tea.Model, tea.Cmd) {
	c := m.cal
	r := m.calRect()
	if !r.has(x, y) {
		m.cal = nil
		m.setStatus("cancelled", false)
		return m, nil
	}
	if c.saving || c.askRecur {
		return m, nil
	}
	line, ix := y-r.y-1, x-r.x-1
	gl := calGridLeft()
	switch {
	case line == calLineText:
		next := c.setCalFocus(calText)
		return m, next
	case line == calLineTime:
		next := c.setCalFocus(calTime)
		return m, next
	case line == calLinePicks:
		for i, pr := range m.pickRanges() {
			if ix >= pr[0] && ix < pr[1] {
				m.applyPick(i)
			}
		}
	case line == calLineMonth:
		if ix >= gl && ix <= gl+1 {
			c.month = c.month.AddDate(0, -1, 0)
		} else if ix >= gl+calGridWidth-3 && ix <= gl+calGridWidth-1 {
			c.month = c.month.AddDate(0, 1, 0)
		}
	case line >= calLineWeek0 && line < calLineWeek0+6:
		col := (ix - gl) / 4
		if ix < gl || col > 6 {
			return m, nil
		}
		d := m.calGridStart().AddDate(0, 0, (line-calLineWeek0)*7+col)
		if d.Month() != c.month.Month() {
			return m, nil
		}
		c.selectDay(d)
		c.setCalFocus(calGrid)
		if dbl {
			return m.saveCalendar()
		}
	}
	return m, nil
}
