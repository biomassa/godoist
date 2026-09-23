// Package state persists local UI state (per-project view modes).
package state

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// ModeNotes marks a project that opens in notebook view.
const ModeNotes = "notes"

// State is the content of the state file.
type State struct {
	ProjectModes map[string]string `toml:"project_modes"` // project ID → "notes"
}

// Path is $XDG_STATE_HOME/godoist/state.toml, defaulting to ~/.local/state.
func Path() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "godoist", "state.toml"), nil
}

// Load reads the state file. A missing file gives an empty state.
func Load() (State, error) {
	s := State{ProjectModes: map[string]string{}}
	p, err := Path()
	if err != nil {
		return s, err
	}
	if _, err := toml.DecodeFile(p, &s); err != nil && !errors.Is(err, os.ErrNotExist) {
		return s, err
	}
	if s.ProjectModes == nil {
		s.ProjectModes = map[string]string{}
	}
	return s, nil
}

// Save writes the state file with owner-only permissions.
func Save(s State) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(s)
}
