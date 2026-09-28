package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SaveTheme and SaveToken keep the other setting of the file.
func TestSaveKeepsOtherSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("TODOIST_TOKEN", "")
	if _, err := SaveToken("abc"); err != nil {
		t.Fatal(err)
	}
	if err := SaveTheme("nord"); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil || c.Token != "abc" || c.Theme != "nord" {
		t.Fatalf("after SaveTheme: %+v %v", c, err)
	}
	if _, err := SaveToken("xyz"); err != nil {
		t.Fatal(err)
	}
	c, _ = Load()
	if c.Token != "xyz" || c.Theme != "nord" {
		t.Errorf("after SaveToken: %+v", c)
	}
	if err := SaveListShare(0.4); err != nil {
		t.Fatal(err)
	}
	if c, _ = Load(); c.ListShare != 0.4 || c.Theme != "nord" || c.Token != "xyz" {
		t.Errorf("after SaveListShare: %+v", c)
	}
	p, _ := Path()
	if !strings.HasPrefix(p, dir) {
		t.Fatalf("config path %s is not in the test directory", p)
	}
	if fi, err := os.Stat(filepath.Clean(p)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v err = %v, want 0600", fi.Mode().Perm(), err)
	}
}
