// Package config reads and writes the godoist settings. At this time, the only setting is the API token.
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
}

// Path returns the config file location (~/.config/godoist/config.toml).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "godoist", "config.toml"), nil
}

// Load reads the token from $TODOIST_TOKEN, falling back to the config file.
func Load() (Config, error) {
	var c Config
	if t := os.Getenv("TODOIST_TOKEN"); t != "" {
		c.Token = t
		return c, nil
	}
	p, err := Path()
	if err != nil {
		return c, err
	}
	if _, err := toml.DecodeFile(p, &c); err != nil && !errors.Is(err, os.ErrNotExist) {
		return c, fmt.Errorf("reading %s: %w", p, err)
	}
	if c.Token == "" {
		return c, fmt.Errorf("no API token: set TODOIST_TOKEN or run `godoist login`")
	}
	return c, nil
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
