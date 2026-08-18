package store

import (
	"bytes"
	"fmt"
	"testing"
)

func TestOpenAndClose(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPutGetDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Put([]byte("key"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	v, ok, err := s.Get([]byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || string(v) != "value" {
		t.Fatalf("expected 'value', got %q (ok=%v)", v, ok)
	}

	if err := s.Delete([]byte("key")); err != nil {
		t.Fatal(err)
	}
	_, ok, err = s.Get([]byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected key to be deleted")
	}
}

func TestEmptyKeyRejected(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Put(nil, []byte("v")); err != ErrEmptyKey {
		t.Fatalf("expected ErrEmptyKey, got %v", err)
	}
	if err := s.Delete(nil); err != ErrEmptyKey {
		t.Fatalf("expected ErrEmptyKey, got %v", err)
	}
}

func TestClosedStoreErrors(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	if err := s.Put([]byte("k"), []byte("v")); err != ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
	if _, _, err := s.Get([]byte("k")); err != ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestMerge(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Write many records, overwrite some.
	for i := 0; i < 100; i++ {
		_ = s.Put([]byte(fmt.Sprintf("k%d", i)), []byte(fmt.Sprintf("v%d", i)))
	}
	for i := 0; i < 50; i++ {
		_ = s.Put([]byte(fmt.Sprintf("k%d", i)), []byte(fmt.Sprintf("new%d", i)))
	}
	// Delete some.
	for i := 50; i < 70; i++ {
		_ = s.Delete([]byte(fmt.Sprintf("k%d", i)))
	}

	if err := s.Merge(); err != nil {
		t.Fatal(err)
	}

	// Verify live keys.
	for i := 0; i < 50; i++ {
		v, ok, err := s.Get([]byte(fmt.Sprintf("k%d", i)))
		if err != nil || !ok {
			t.Fatalf("key k%d missing after merge", i)
		}
		want := fmt.Sprintf("new%d", i)
		if string(v) != want {
			t.Fatalf("k%d = %q, want %q", i, v, want)
		}
	}
	// Deleted keys should still be gone.
	for i := 50; i < 70; i++ {
		_, ok, _ := s.Get([]byte(fmt.Sprintf("k%d", i)))
		if ok {
			t.Fatalf("k%d should be deleted after merge", i)
		}
	}
}

func TestReopenRestoresData(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Put([]byte("persist"), []byte("me"))
	s.Close()

	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	v, ok, err := s2.Get([]byte("persist"))
	if err != nil || !ok {
		t.Fatal("expected key to persist across reopen")
	}
	if string(v) != "me" {
		t.Fatalf("expected 'me', got %q", v)
	}
}

func TestIterator(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for i := 0; i < 10; i++ {
		_ = s.Put([]byte(fmt.Sprintf("key_%02d", i)), []byte(fmt.Sprintf("val_%02d", i)))
	}

	it := s.Iter()
	count := 0
	for it.Valid() {
		count++
		it.Next()
	}
	if count != 10 {
		t.Fatalf("expected 10 entries, got %d", count)
	}
}

func TestPrefix(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.Put([]byte("user:1"), []byte("a"))
	_ = s.Put([]byte("user:2"), []byte("b"))
	_ = s.Put([]byte("order:1"), []byte("c"))

	it := s.Prefix("user:")
	count := 0
	for it.Valid() {
		count++
		it.Next()
	}
	if count != 2 {
		t.Fatalf("expected 2 user: keys, got %d", count)
	}
}

func TestBackupAndRestore(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Put([]byte("backup_key"), []byte("backup_val"))

	var buf bytes.Buffer
	if err := s.Backup(&buf); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dstDir := t.TempDir()
	if err := Restore(&buf, dstDir); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(dstDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	v, ok, err := s2.Get([]byte("backup_key"))
	if err != nil || !ok {
		t.Fatal("expected key in restored store")
	}
	if string(v) != "backup_val" {
		t.Fatalf("got %q, want backup_val", v)
	}
}

func TestSnapshot(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.Put([]byte("snap"), []byte("shot"))

	snapDir := t.TempDir()
	if err := s.Snapshot(snapDir); err != nil {
		t.Fatal(err)
	}

	info, err := Inspect(snapDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.SegmentCount == 0 {
		t.Fatal("expected at least 1 segment in snapshot")
	}
}

func TestCount(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.Put([]byte("a"), []byte("1"))
	_ = s.Put([]byte("b"), []byte("2"))
	_ = s.Delete([]byte("a"))

	if s.Count() != 1 {
		t.Fatalf("expected count=1, got %d", s.Count())
	}
}
