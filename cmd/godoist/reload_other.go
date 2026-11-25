//go:build !unix

package main

import tea "charm.land/bubbletea/v2"

// reloadThemeOnSignal does nothing: only Unix has SIGUSR2.
func reloadThemeOnSignal(*tea.Program) {}
