package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open empty: %v", err)
	}
	if _, ok := s.Get("foo:bar"); ok {
		t.Fatal("fresh store should not have entries")
	}

	want := Entry{Digest: "sha256:abc", DeployedAt: time.Now().UTC().Truncate(time.Second), Type: "jar"}
	if err := s.Set("foo:bar", want); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Reopen and check persistence.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open existing: %v", err)
	}
	got, ok := s2.Get("foo:bar")
	if !ok {
		t.Fatal("entry missing after reopen")
	}
	if got.Digest != want.Digest || got.Type != want.Type {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestStore_AtomicWrite(t *testing.T) {
	// After Set returns, no .tmp file should remain.
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("a:b", Entry{Digest: "sha256:1"}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover tmp file: %s", e.Name())
		}
	}
}

func TestStore_OpenMissingFile(t *testing.T) {
	// Missing file is not an error; we get an empty store.
	s, err := Open(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("Open missing: %v", err)
	}
	if _, ok := s.Get("anything"); ok {
		t.Fatal("missing-file store should be empty")
	}
}
