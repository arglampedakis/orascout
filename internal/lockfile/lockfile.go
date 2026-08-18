// Package lockfile provides a best-effort PID lock file so two orascout cycles
// can't run concurrently (e.g. a systemd timer firing while a previous run is
// still pulling a large artifact).
package lockfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ErrLocked indicates another process currently holds the lock.
var ErrLocked = errors.New("lockfile is held by another process")

// Lock represents an acquired lock.
type Lock struct {
	path string
	file *os.File
}

// Acquire takes the lock at path. Returns ErrLocked if another live process
// already holds it (PID listed in the file is checked for liveness).
func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir lock dir: %w", err)
	}

	// O_EXCL + O_CREATE — atomic "create if absent".
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err == nil {
		return writePID(f, path)
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("open lockfile: %w", err)
	}

	// File exists. Inspect the stored PID — if the process is dead, the lock
	// is stale and we can reclaim it.
	stale, sErr := isStale(path)
	if sErr != nil {
		return nil, fmt.Errorf("check stale lock: %w", sErr)
	}
	if !stale {
		return nil, ErrLocked
	}

	// Reclaim stale lock.
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("remove stale lock: %w", err)
	}
	f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lockfile after reclaim: %w", err)
	}
	return writePID(f, path)
}

// Release removes the lockfile. Safe to call multiple times.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	_ = l.file.Close()
	l.file = nil
	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove lockfile: %w", err)
	}
	return nil
}

func writePID(f *os.File, path string) (*Lock, error) {
	if _, err := fmt.Fprintf(f, "%d", os.Getpid()); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write pid: %w", err)
	}
	return &Lock{path: path, file: f}, nil
}

func isStale(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	pidStr := strings.TrimSpace(string(raw))
	if pidStr == "" {
		return true, nil
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return true, nil
	}
	if pid == os.Getpid() {
		return true, nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return true, nil
	}
	// Signal 0 = liveness probe on Unix.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return true, nil
	}
	return false, nil
}
