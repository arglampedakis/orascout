// Package state persists the last-deployed digest per repo:tag to disk.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const schemaVersion = 1

// Entry is one record in the state file.
type Entry struct {
	Digest     string    `json:"digest"`
	DeployedAt time.Time `json:"deployedAt"`
	Type       string    `json:"type"`
}

// fileShape is what's written to disk.
type fileShape struct {
	Schema  int              `json:"schema"`
	Entries map[string]Entry `json:"entries"`
}

// Store is a thread-safe digest cache backed by a JSON file.
type Store struct {
	path string

	mu   sync.Mutex
	data fileShape
}

// Open loads the state file at path. If the file does not exist, an empty
// store is returned (and the file will be created on first Save).
func Open(path string) (*Store, error) {
	s := &Store{
		path: path,
		data: fileShape{Schema: schemaVersion, Entries: map[string]Entry{}},
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state %s: %w", path, err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", path, err)
	}
	if s.data.Entries == nil {
		s.data.Entries = map[string]Entry{}
	}
	return s, nil
}

// Get returns the recorded entry for ref (e.g. "docker.io/foo/bar:latest").
// ok is false if no entry exists.
func (s *Store) Get(ref string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data.Entries[ref]
	return e, ok
}

// Set records an entry and persists the whole state file atomically.
func (s *Store) Set(ref string, e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Entries[ref] = e
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	buf, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return fmt.Errorf("write state tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}
