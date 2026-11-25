package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/biomassa/godoist/internal/config"
)

// --theme NAME with a broken NAME.toml says why the file didn't load, not only that the
// theme is unknown.
func TestThemeFlagBrokenFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("TODOIST_TOKEN", "test")
	themes, err := config.ThemeDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(themes, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themes, "desk.toml"), []byte("background = \"#000000\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := rootCmd()
	root.SetArgs([]string{"--theme", "desk"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	err = root.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown theme "desk"`) || !strings.Contains(err.Error(), "foreground is missing") {
		t.Errorf("err = %v", err)
	}
}
