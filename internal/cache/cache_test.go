package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/biomassa/godoist/internal/todoist"
)

// tempCache points the user cache directory to a new temporary directory. It stops the
// test if the cache is not there, so that a test never writes the real cache.
func tempCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir) // Linux
	t.Setenv("HOME", dir)           // macOS: ~/Library/Caches
	p, err := path()
	if err != nil || !strings.HasPrefix(p, dir) {
		t.Fatalf("cache path %s (%v) is not in the test directory", p, err)
	}
}

// Load gives back what Save wrote for the same token.
func TestSaveLoad(t *testing.T) {
	tempCache(t)
	st := todoist.SyncState{Token: "tok", Tasks: []todoist.Task{{ID: "t1", Content: "milk"}}}
	if err := Save("abc", st); err != nil {
		t.Fatal(err)
	}
	got := Load("abc")
	if got.Token != "tok" || len(got.Tasks) != 1 || got.Tasks[0].Content != "milk" {
		t.Fatalf("Load = %+v", got)
	}
}

// A different token gets an empty state, so that one account never sees the tasks
// of another one.
func TestLoadOtherToken(t *testing.T) {
	tempCache(t)
	if err := Save("abc", todoist.SyncState{Token: "tok"}); err != nil {
		t.Fatal(err)
	}
	if got := Load("xyz"); got.Token != "" {
		t.Fatalf("Load with another token = %+v, want an empty state", got)
	}
}

// No file, a broken file, or a file of an older version give an empty state.
func TestLoadBadFile(t *testing.T) {
	tempCache(t)
	if got := Load("abc"); got.Token != "" {
		t.Fatalf("no file: %+v", got)
	}
	p, err := path()
	if err != nil {
		t.Fatal(err)
	}
	if err := Save("abc", todoist.SyncState{Token: "tok"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Replace(string(b), fmt.Sprintf(`"version":%d`, version), fmt.Sprintf(`"version":%d`, version-1), 1)
	if old == string(b) {
		t.Fatalf("no version field in %s", b)
	}
	for name, content := range map[string]string{"old version": old, "broken": "{"} {
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := Load("abc"); got.Token != "" {
			t.Errorf("%s: %+v, want an empty state", name, got)
		}
	}
}

// Only the owner can read the cache and its directory, and the cache does not hold the token.
func TestSaveFile(t *testing.T) {
	tempCache(t)
	if err := Save("secret-token", todoist.SyncState{}); err != nil {
		t.Fatal(err)
	}
	p, _ := path()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 600", perm)
	}
	if di, err := os.Stat(filepath.Dir(p)); err != nil || di.Mode().Perm() != 0o700 {
		t.Errorf("directory: %v %v, want permissions 700", di.Mode().Perm(), err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "secret-token") {
		t.Error("the cache file holds the token")
	}
}

// Each account has its own queue file, and the name does not hold the token.
func TestQueuePath(t *testing.T) {
	tempCache(t)
	a, err := QueuePath("token-a")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := QueuePath("token-b")
	if a == b {
		t.Errorf("two tokens share the queue file %s", a)
	}
	if strings.Contains(a, "token-a") {
		t.Errorf("the queue path %s holds the token", a)
	}
}
