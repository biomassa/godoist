//go:build unix

package main

import (
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/biomassa/godoist/internal/tui"
)

// reloadThemeOnSignal makes SIGUSR2 read the theme files again. A tool that
// writes the theme file, like a desktop shell after a wallpaper change, sends it with
// "pkill -USR2 -x godoist". SIGUSR1 is taken by the prof build.
func reloadThemeOnSignal(p *tea.Program) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR2)
	go func() {
		for range ch {
			p.Send(tui.ThemeReloadMsg{})
		}
	}()
}
