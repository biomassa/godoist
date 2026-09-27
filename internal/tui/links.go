package tui

import (
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// bareURL is a web address in text that is not a markdown link.
var bareURL = regexp.MustCompile(`https?://[^\s<>()\[\]]+`)

// titleSeg is a part of a task title: plain text, or the text of a link with its URL.
type titleSeg struct{ text, url string }

// titleSegments cuts a task title into plain text and links. A markdown link shows its
// text, and a bare URL shows itself. Bold markers and line breaks go, as in plain.
func titleSegments(s string) []titleSeg {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "**", ""), "\n", " ")
	var out []titleSeg
	addText := func(t string) {
		for t != "" {
			loc := bareURL.FindStringIndex(t)
			if loc == nil {
				out = append(out, titleSeg{text: t})
				return
			}
			u := strings.TrimRight(t[loc[0]:loc[1]], ".,;:!?'\"") // end punctuation is not a part of the URL
			if loc[0] > 0 {
				out = append(out, titleSeg{text: t[:loc[0]]})
			}
			out = append(out, titleSeg{text: u, url: u})
			t = t[loc[0]+len(u):]
		}
	}
	for s != "" {
		loc := mdLink.FindStringSubmatchIndex(s)
		if loc == nil {
			addText(s)
			break
		}
		addText(s[:loc[0]])
		out = append(out, titleSeg{text: s[loc[2]:loc[3]], url: s[loc[4]:loc[5]]})
		s = s[loc[1]:]
	}
	return out
}

// taskLinks returns the URLs of a task: the links of the title first, then the links of
// the description.
func taskLinks(title, desc string) []string {
	var urls []string
	for _, sg := range titleSegments(title) {
		if sg.url != "" {
			urls = append(urls, sg.url)
		}
	}
	for _, sg := range titleSegments(desc) {
		if sg.url != "" {
			urls = append(urls, sg.url)
		}
	}
	return urls
}

// linkStyle is the style of a link in a title: underlined, with an OSC 8 hyperlink, so
// that the terminal can open it (for example with ctrl+shift+click in kitty).
func linkStyle(s lipgloss.Style, url string) lipgloss.Style {
	return s.Underline(true).Hyperlink(url)
}

// titleLine draws a task title on one line of width w. Links are hyperlinks. A title that
// is too long ends with "…".
func titleLine(title string, w int, s lipgloss.Style) string {
	if w <= 0 {
		return ""
	}
	var b strings.Builder
	left := w
	for _, sg := range titleSegments(title) {
		text := sg.text
		cut := ansi.StringWidth(text) > left
		if cut {
			text = ansi.Truncate(text, left, "…")
		}
		style := s
		if sg.url != "" {
			style = linkStyle(s, sg.url)
		}
		b.WriteString(style.Render(text))
		left -= ansi.StringWidth(text)
		if cut || left <= 0 {
			break
		}
	}
	return b.String()
}

// titleLines wraps a task title to lines of width w. Links are hyperlinks. Each word is
// drawn by itself, so that a hyperlink never continues into the space at the end of a line.
func titleLines(title string, w int, s lipgloss.Style) []string {
	type word struct{ text, url string }
	var words []word
	for _, sg := range titleSegments(title) {
		for _, f := range strings.Fields(sg.text) {
			words = append(words, word{f, sg.url})
		}
	}
	var lines []string
	var line strings.Builder
	used := 0
	var prev word
	for _, wd := range words {
		ww := ansi.StringWidth(wd.text)
		if used > 0 && used+1+ww > w {
			lines = append(lines, line.String())
			line.Reset()
			used = 0
		}
		style := s
		if wd.url != "" {
			style = linkStyle(s, wd.url)
		}
		if used > 0 {
			sp := s
			if wd.url != "" && wd.url == prev.url { // the space inside a link is a part of it
				sp = style
			}
			line.WriteString(sp.Render(" "))
			used++
		}
		if ww > w { // a word longer than the line
			wd.text = ansi.Truncate(wd.text, w, "…")
			ww = ansi.StringWidth(wd.text)
		}
		line.WriteString(style.Render(wd.text))
		used += ww
		prev = wd
	}
	if used > 0 || len(lines) == 0 {
		lines = append(lines, line.String())
	}
	return lines
}

// openURL opens url in the browser: with open on macOS, and with xdg-open on other systems.
// Tests replace it.
var openURL = func(url string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	c := exec.Command(name, url) // no stdout or stderr: the output would break the screen
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait() //nolint:errcheck // release the process when it ends
	return nil
}

// openLink opens the first link of the task under the cursor in the browser.
func (m Model) openLink() (tea.Model, tea.Cmd) {
	t := m.currentTask()
	if t == nil {
		return m, nil
	}
	urls := taskLinks(t.Content, t.Description)
	if len(urls) == 0 {
		m.setStatus("this task has no link", false)
		return m, nil
	}
	if err := openURL(urls[0]); err != nil {
		m.setStatus("could not open the link: "+err.Error(), true)
		return m, nil
	}
	m.setStatus("opened "+urls[0], false)
	return m, nil
}
