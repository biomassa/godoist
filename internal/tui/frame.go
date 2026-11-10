package tui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// FPS is the frame rate of the renderer. The renderer wakes FPS times each second, also when
// nothing changes, so a lower rate gives less idle load. The completion animation uses the
// same frame time (fxFrame).
const FPS = 30

// frameTime is the time of one frame.
const frameTime = time.Second / FPS

// frameMsg asks for the frame of the wheel events that came after the last frame.
type frameMsg struct{}

// themeGen counts the color changes. The caches of styled lines use it in their keys, so
// a new theme or a new terminal background makes new lines.
var themeGen int

// frameCache keeps the work of the last frames. Bubble Tea copies the Model for each
// message, so the Model keeps a pointer to the cache. A nil cache caches nothing.
type frameCache struct {
	rows lineCache[rowKey]  // the styled rows of the task list
	nav  lineCache[navLine] // the styled rows of the sidebar
	pads lineCache[padKey]  // lines padded to a width

	// legendKey and legendParts are the last styled legend items: they change seldom, and
	// the bottom bar draws them in each frame.
	legendKey   string
	legendParts []string

	// Wheel events less than one frame apart share one frame (see holdWheel).
	held     bool      // View gives last again: the model has wheel events that it did not draw
	frameDue bool      // a frameMsg will come
	built    time.Time // the time of the last frame that View built
	last     tea.View  // the last frame that View built
	noHold   bool      // benchmarks: each wheel event builds its frame
}

func newFrameCache() *frameCache { return &frameCache{} }

// lineCache keeps the lines of the last two frames by key. A scroll shows most lines of
// the frame before, and a map lookup is much faster than the styling of a line. The cache
// holds at most the lines of two frames.
type lineCache[K comparable] struct {
	cur, prev map[K]string
}

// get returns the line of key k. If the last two frames do not have it, make makes it.
func (c *lineCache[K]) get(k K, make func() string) string {
	if c.cur == nil {
		c.cur = map[K]string{}
	}
	if s, ok := c.cur[k]; ok {
		return s
	}
	s, ok := c.prev[k]
	if !ok {
		s = make()
	}
	c.cur[k] = s
	return s
}

// frame starts a new frame: the lines of the frame before stay for one more frame.
func (c *lineCache[K]) frame() {
	c.prev, c.cur = c.cur, make(map[K]string, len(c.cur))
}

// frame starts a new frame in all line caches.
func (fc *frameCache) frame() {
	if fc == nil {
		return
	}
	fc.rows.frame()
	fc.nav.frame()
	fc.pads.frame()
}

// padKey is a line and the width to pad it to.
type padKey struct {
	s string
	w int
}

// padLines is padLines with a plain fill, with the padded lines of the last frames.
func (fc *frameCache) padLines(lines []string, w, h int) []string {
	if fc == nil {
		return padLines(lines, w, h)
	}
	out := make([]string, h)
	for i := range out {
		if i < len(lines) {
			s := lines[i]
			out[i] = fc.pads.get(padKey{s, w}, func() string { return pad(s, w, lipgloss.NewStyle()) })
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

// rowKey is everything that a cached task list row shows. task is a text of the task
// fields that the row shows: some changes edit a task in place.
type rowKey struct {
	r      row // with task nil
	task   string
	w      int
	look   int // 0 plain, 1 the cursor in the focused list, 2 the cursor in a list without focus
	notes  bool
	cross  bool
	minute int64 // due dates show relative to now ("Today", overdue colors)
	theme  int
}

// row returns the task list row of key k. make makes it if the last frames do not have it.
func (fc *frameCache) row(k rowKey, make func() string) string {
	if fc == nil {
		return make()
	}
	return fc.rows.get(k, make)
}

// navLine is everything that a sidebar row shows.
type navLine struct {
	n                navItem
	w                int
	cur, focus, glow bool
	theme            int
}

// navRow returns the sidebar row of key k.
func (fc *frameCache) navRow(k navLine) string {
	if fc == nil {
		return navRow(k)
	}
	return fc.nav.get(k, func() string { return navRow(k) })
}

// legend returns the styled legend items of keys, from the last frame if they did not
// change.
func (fc *frameCache) legend(keys [][2]string) []string {
	var key strings.Builder
	key.WriteString(strconv.Itoa(themeGen))
	for _, k := range keys {
		key.WriteByte(0)
		key.WriteString(k[0])
		key.WriteByte(1)
		key.WriteString(k[1])
	}
	if fc != nil && key.String() == fc.legendKey {
		return fc.legendParts
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = st(fg(hexAccent)).Bold(true).Render(k[0]) + " " + st(hexMuted).Render(k[1])
	}
	if fc != nil {
		fc.legendKey, fc.legendParts = key.String(), parts
	}
	return parts
}

// holdsWheel reports whether msg is a wheel event that comes less than one frame after
// the last built frame. Such an event changes the cursor or the scroll position, but View
// gives the last frame again. A frameMsg at the end of the frame time builds one frame for
// all these events. Bubble Tea calls View for each message, and the terminal shows at most
// FPS frames each second, so the frames between are work that nobody sees.
func (fc *frameCache) holdsWheel(msg tea.Msg) bool {
	_, ok := msg.(tea.MouseWheelMsg)
	return ok && !fc.noHold && !fc.built.IsZero() && time.Since(fc.built) < frameTime
}

// hold marks the last frame as old and returns the command that asks for the next frame.
func (fc *frameCache) hold() tea.Cmd {
	fc.held = true
	if fc.frameDue {
		return nil
	}
	fc.frameDue = true
	return tea.Tick(frameTime-time.Since(fc.built), func(time.Time) tea.Msg { return frameMsg{} })
}
