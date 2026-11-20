package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempState points the state directory to a new temporary directory and returns it. It
// stops the test if the state file is not there, so that a test never writes the real one.
func tempState(t *testing.T) string {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	p, err := Path()
	if err != nil || !strings.HasPrefix(p, dir) {
		t.Fatalf("state path %s (%v) is not in the test directory", p, err)
	}
	return dir
}

// Without a state file, Load gives an empty state with a map that is ready to use.
func TestLoadNoFile(t *testing.T) {
	tempState(t)
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.ProjectModes == nil || len(s.ProjectModes) != 0 {
		t.Fatalf("ProjectModes = %#v, want an empty map", s.ProjectModes)
	}
	s.ProjectModes["p1"] = ModeNotes // must not panic
}

// Load gives back what Save wrote. Only the owner can read the file and its directory.
func TestSaveLoad(t *testing.T) {
	dir := tempState(t)
	in := State{
		ProjectModes: map[string]string{"p1": ModeNotes},
		OverviewOpen: map[string]map[string]bool{"today": {"t1": true}},
	}
	if err := Save(in); err != nil {
		t.Fatal(err)
	}
	out, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if out.ProjectModes["p1"] != ModeNotes || !out.OverviewOpen["today"]["t1"] {
		t.Fatalf("Load = %+v", out)
	}
	fi, err := os.Stat(filepath.Join(dir, "godoist", "state.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 600", perm)
	}
	if di, err := os.Stat(filepath.Join(dir, "godoist")); err != nil || di.Mode().Perm() != 0o700 {
		t.Errorf("directory: %v %v, want permissions 700", di.Mode().Perm(), err)
	}
}

// A broken file is an error, but Load still gives a state that is ready to use.
func TestLoadBrokenFile(t *testing.T) {
	dir := tempState(t)
	p := filepath.Join(dir, "godoist", "state.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("project_modes = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load()
	if err == nil {
		t.Fatal("no error for a broken file")
	}
	if s.ProjectModes == nil {
		t.Fatal("ProjectModes is nil")
	}
}

// Without XDG_STATE_HOME, the file goes to ~/.local/state.
func TestPathDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", home)
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".local", "state", "godoist", "state.toml"); p != want {
		t.Errorf("Path = %s, want %s", p, want)
	}
}
