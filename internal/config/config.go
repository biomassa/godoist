// Package config reads and writes the godoist settings: the API token and the color theme.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the content of the config file.
type Config struct {
	Token string `toml:"token"`
	Theme string `toml:"theme,omitempty"` // the theme kept in the theme picker (T)
	// ListShare is the share of the task list in the space next to the details pane
	// ({ and } in the TUI). 0 gives half.
	ListShare float64 `toml:"list_share,omitempty"`
}

// Path returns the config file location (~/.config/godoist/config.toml).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "godoist", "config.toml"), nil
}

// Load reads the config file. $TODOIST_TOKEN, if set, replaces the token of the file.
func Load() (Config, error) {
	c, err := loadFile()
	if err != nil {
		return c, err
	}
	if t := os.Getenv("TODOIST_TOKEN"); t != "" {
		c.Token = t
	}
	if c.Token == "" {
		return c, fmt.Errorf("no API token: set TODOIST_TOKEN or run `godoist login`")
	}
	return c, nil
}

// loadFile reads the config file as it is. A missing file gives an empty Config.
func loadFile() (Config, error) {
	var c Config
	p, err := Path()
	if err != nil {
		return c, err
	}
	if _, err := toml.DecodeFile(p, &c); err != nil && !errors.Is(err, os.ErrNotExist) {
		return c, fmt.Errorf("reading %s: %w", p, err)
	}
	return c, nil
}

// SaveTheme writes the theme into the config file and keeps the other settings. A token
// from $TODOIST_TOKEN does not go into the file.
func SaveTheme(name string) error {
	c, err := loadFile()
	if err != nil {
		return err
	}
	c.Theme = name
	_, err = Save(c)
	return err
}

// SaveListShare writes the share of the task list into the config file and keeps the
// other settings.
func SaveListShare(share float64) error {
	c, err := loadFile()
	if err != nil {
		return err
	}
	c.ListShare = share
	_, err = Save(c)
	return err
}

// SaveToken writes the API token into the config file and keeps the other settings.
// It returns the file path.
func SaveToken(token string) (string, error) {
	c, err := loadFile()
	if err != nil {
		return "", err
	}
	c.Token = token
	return Save(c)
}

// Save writes the config file with owner-only permissions.
func Save(c Config) (string, error) {
	p, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return p, toml.NewEncoder(f).Encode(c)
}
