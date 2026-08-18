package flock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireAndRelease(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()

	// Lock file should exist.
	path := filepath.Join(dir, "LOCK")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}

	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestDoubleAcquireFails(t *testing.T) {
	dir := t.TempDir()
	l1, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Release()

	_, err = Acquire(dir)
	if err == nil {
		t.Fatal("expected second acquire to fail")
	}
}

func TestIsHeld(t *testing.T) {
	dir := t.TempDir()
	if IsHeld(dir) {
		t.Fatal("should not be held initially")
	}
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !IsHeld(dir) {
		t.Fatal("should be held after acquire")
	}
	l.Release()
	if IsHeld(dir) {
		t.Fatal("should not be held after release")
	}
}

func TestOwnerPID(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()

	pid := l.OwnerPID()
	if pid != os.Getpid() {
		t.Fatalf("expected PID %d, got %d", os.Getpid(), pid)
	}
}

func TestReadOwner(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()

	pid, err := ReadOwner(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pid != os.Getpid() {
		t.Fatalf("expected PID %d, got %d", os.Getpid(), pid)
	}
}

func TestReleaseNotLocked(t *testing.T) {
	l := &Lock{}
	err := l.Release()
	if err != ErrNotLocked {
		t.Fatalf("expected ErrNotLocked, got %v", err)
	}
}

func TestLockPath(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()

	want := filepath.Join(dir, "LOCK")
	if l.Path() != want {
		t.Fatalf("got %q, want %q", l.Path(), want)
	}
}
