// Package flock provides advisory file locking for bitcask. Only one process
// at a time should open a bitcask directory for writing; the lock file prevents
// concurrent access that would corrupt the data log.
package flock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// lockFile is the conventional lock file name inside a bitcask directory.
const lockFile = "LOCK"

// ErrLocked is returned when another process holds the lock.
var ErrLocked = errors.New("flock: database locked by another process")

// ErrNotLocked is returned when Unlock is called but no lock is held.
var ErrNotLocked = errors.New("flock: not locked")

// Lock represents an advisory file lock on a bitcask directory.
type Lock struct {
	path string
	f    *os.File
}

// Acquire attempts to take an exclusive lock on dir. If another process holds
// the lock, ErrLocked is returned immediately (non-blocking).
func Acquire(dir string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("flock: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, lockFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("flock: open %s: %w", path, err)
	}

	if err := tryLock(f); err != nil {
		_ = f.Close()
		return nil, err
	}

	// Write PID + timestamp for diagnostic purposes.
	content := fmt.Sprintf("pid=%d\ntime=%s\n", os.Getpid(), time.Now().Format(time.RFC3339))
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(content), 0)
	}

	return &Lock{path: path, f: f}, nil
}

// AcquireWithRetry retries lock acquisition up to timeout with the given
// interval between attempts.
func AcquireWithRetry(dir string, timeout, interval time.Duration) (*Lock, error) {
	deadline := time.Now().Add(timeout)
	for {
		l, err := Acquire(dir)
		if err == nil {
			return l, nil
		}
		if !errors.Is(err, ErrLocked) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("flock: timed out after %v: %w", timeout, ErrLocked)
		}
		time.Sleep(interval)
	}
}

// Release releases the lock and removes the lock file.
func (l *Lock) Release() error {
	if l.f == nil {
		return ErrNotLocked
	}
	if err := unlock(l.f); err != nil {
		_ = l.f.Close()
		return err
	}
	_ = l.f.Close()
	l.f = nil
	_ = os.Remove(l.path)
	return nil
}

// Path returns the lock file path.
func (l *Lock) Path() string { return l.path }

// OwnerPID reads the PID from the lock file content (best-effort).
func (l *Lock) OwnerPID() int {
	if l.f == nil {
		return 0
	}
	buf := make([]byte, 128)
	n, _ := l.f.ReadAt(buf, 0)
	content := string(buf[:n])
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "pid=") {
			v, _ := strconv.Atoi(strings.TrimPrefix(line, "pid="))
			return v
		}
	}
	return 0
}

// IsHeld reports whether the lock file appears to be held by some process.
// It does not guarantee the holding process is still alive.
func IsHeld(dir string) bool {
	path := filepath.Join(dir, lockFile)
	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return false
	}
	defer f.Close()
	if err := tryLock(f); err != nil {
		return true
	}
	_ = unlock(f)
	return false
}

// ReadOwner reads the lock file and returns the PID stored in it.
func ReadOwner(dir string) (int, error) {
	path := filepath.Join(dir, lockFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "pid=") {
			v, e := strconv.Atoi(strings.TrimPrefix(line, "pid="))
			return v, e
		}
	}
	return 0, fmt.Errorf("flock: no pid in lock file")
}

// tryLock attempts a non-blocking exclusive advisory lock via flock(2).
func tryLock(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return ErrLocked
		}
		return fmt.Errorf("flock: flock: %w", err)
	}
	return nil
}

// unlock releases the advisory lock.
func unlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
