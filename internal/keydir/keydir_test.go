package keydir

import (
	"bytes"
	"testing"
)

func TestSnapshotBasic(t *testing.T) {
	recs := []Record{
		{Key: "a", FileID: 1, Offset: 0, Size: 50, Tombstone: false, Timestamp: 100},
		{Key: "b", FileID: 1, Offset: 50, Size: 60, Tombstone: false, Timestamp: 200},
		{Key: "c", FileID: 2, Offset: 0, Size: 40, Tombstone: true, Timestamp: 300},
	}
	snap := NewSnapshot(recs)
	if snap.Len() != 3 {
		t.Fatalf("expected 3, got %d", snap.Len())
	}
	if snap.LiveCount() != 2 {
		t.Fatalf("expected 2 live, got %d", snap.LiveCount())
	}
}

func TestSnapshotGet(t *testing.T) {
	recs := []Record{
		{Key: "x", FileID: 5, Offset: 100, Size: 30},
	}
	snap := NewSnapshot(recs)
	r, ok := snap.Get("x")
	if !ok {
		t.Fatal("expected to find key")
	}
	if r.FileID != 5 || r.Offset != 100 {
		t.Fatalf("unexpected record: %+v", r)
	}
	_, ok = snap.Get("missing")
	if ok {
		t.Fatal("should not find missing key")
	}
}

func TestSnapshotKeys(t *testing.T) {
	recs := []Record{
		{Key: "charlie"},
		{Key: "alpha"},
		{Key: "bravo"},
	}
	snap := NewSnapshot(recs)
	keys := snap.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "alpha" || keys[1] != "bravo" || keys[2] != "charlie" {
		t.Fatalf("keys not sorted: %v", keys)
	}
}

func TestSnapshotFileIDs(t *testing.T) {
	recs := []Record{
		{Key: "a", FileID: 3},
		{Key: "b", FileID: 1},
		{Key: "c", FileID: 3},
		{Key: "d", FileID: 5},
	}
	snap := NewSnapshot(recs)
	ids := snap.FileIDs()
	if len(ids) != 3 {
		t.Fatalf("expected 3 unique file IDs, got %d", len(ids))
	}
	if ids[0] != 1 || ids[1] != 3 || ids[2] != 5 {
		t.Fatalf("unexpected file IDs: %v", ids)
	}
}

func TestWriteAndReadSnapshot(t *testing.T) {
	recs := []Record{
		{Key: "hello", FileID: 1, Offset: 0, Size: 100, Tombstone: false, Timestamp: 1000},
		{Key: "world", FileID: 2, Offset: 50, Size: 80, Tombstone: true, Timestamp: 2000},
	}
	snap := NewSnapshot(recs)

	var buf bytes.Buffer
	if _, err := snap.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}

	snap2, err := ReadFrom(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if snap2.Len() != 2 {
		t.Fatalf("expected 2, got %d", snap2.Len())
	}
	r, ok := snap2.Get("hello")
	if !ok || r.FileID != 1 || r.Offset != 0 || r.Size != 100 {
		t.Fatalf("bad record: %+v", r)
	}
	r2, ok := snap2.Get("world")
	if !ok || !r2.Tombstone || r2.Timestamp != 2000 {
		t.Fatalf("bad tombstone record: %+v", r2)
	}
}

func TestLiveKeys(t *testing.T) {
	recs := []Record{
		{Key: "a", Tombstone: false},
		{Key: "b", Tombstone: true},
		{Key: "c", Tombstone: false},
	}
	snap := NewSnapshot(recs)
	live := snap.LiveKeys()
	if len(live) != 2 {
		t.Fatalf("expected 2 live keys, got %d", len(live))
	}
}

func TestDuplicateKeysLastWins(t *testing.T) {
	recs := []Record{
		{Key: "k", FileID: 1, Offset: 0, Size: 10},
		{Key: "k", FileID: 2, Offset: 100, Size: 20},
	}
	snap := NewSnapshot(recs)
	r, _ := snap.Get("k")
	if r.FileID != 2 {
		t.Fatalf("expected last record to win, got FileID=%d", r.FileID)
	}
}
