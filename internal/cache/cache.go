// Package cache stores the Sync API replica on disk so startup is instant.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/biomassa/godoist/internal/todoist"
)

// version changes when SyncState gets new resource types. An older cache has no data
// for them, and an incremental sync does not send them, so a version change forces a full sync.
const version = 3

// file is the format of the cache file.
type file struct {
	Version int               `json:"version"`
	Account string            `json:"account"` // token fingerprint. A different token discards the cache.
	State   todoist.SyncState `json:"state"`
}

// path is ~/.cache/godoist/sync.json on Linux.
func path() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "godoist", "sync.json"), nil
}

// fingerprint identifies the account without storing the token.
func fingerprint(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:8])
}

// Load returns the cached state for this token, or a zero state (forcing a full sync).
func Load(token string) todoist.SyncState {
	p, err := path()
	if err != nil {
		return todoist.SyncState{}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return todoist.SyncState{}
	}
	var f file
	if json.Unmarshal(b, &f) != nil || f.Account != fingerprint(token) || f.Version != version {
		return todoist.SyncState{}
	}
	return f.State
}

// Save writes the state atomically with owner-only permissions.
func Save(token string, s todoist.SyncState) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(file{Version: version, Account: fingerprint(token), State: s})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "sync-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
