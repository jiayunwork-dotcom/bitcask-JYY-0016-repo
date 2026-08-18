package batch

import (
	"testing"
)

func TestPutAndLen(t *testing.T) {
	b := New()
	if err := b.Put([]byte("k1"), []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if err := b.Put([]byte("k2"), []byte("v2")); err != nil {
		t.Fatal(err)
	}
	if b.Len() != 2 {
		t.Fatalf("expected 2, got %d", b.Len())
	}
}

func TestDeleteOp(t *testing.T) {
	b := New()
	if err := b.Delete([]byte("key")); err != nil {
		t.Fatal(err)
	}
	ops := b.Ops()
	if len(ops) != 1 || ops[0].Type != OpDelete {
		t.Fatal("expected one delete op")
	}
}

func TestCommittedRejectsOps(t *testing.T) {
	b := New()
	b.MarkCommitted()
	if err := b.Put([]byte("x"), []byte("y")); err != ErrCommitted {
		t.Fatalf("expected ErrCommitted, got %v", err)
	}
	if err := b.Delete([]byte("x")); err != ErrCommitted {
		t.Fatalf("expected ErrCommitted, got %v", err)
	}
}

func TestDiscardedRejectsOps(t *testing.T) {
	b := New()
	b.Discard()
	if err := b.Put([]byte("x"), []byte("y")); err != ErrDiscarded {
		t.Fatalf("expected ErrDiscarded, got %v", err)
	}
}

func TestReset(t *testing.T) {
	b := New()
	_ = b.Put([]byte("a"), []byte("b"))
	b.MarkCommitted()
	b.Reset()
	if b.IsCommitted() {
		t.Fatal("should not be committed after reset")
	}
	if b.Len() != 0 {
		t.Fatal("should be empty after reset")
	}
	// Should accept new ops.
	if err := b.Put([]byte("c"), []byte("d")); err != nil {
		t.Fatal(err)
	}
}

func TestDeduplicate(t *testing.T) {
	b := New()
	_ = b.Put([]byte("k"), []byte("v1"))
	_ = b.Put([]byte("k"), []byte("v2"))
	_ = b.Put([]byte("k"), []byte("v3"))
	_ = b.Put([]byte("other"), []byte("x"))

	b.Deduplicate()
	if b.Len() != 2 {
		t.Fatalf("expected 2 after dedup, got %d", b.Len())
	}
	ops := b.Ops()
	// The last op for "k" should have value "v3".
	for _, op := range ops {
		if string(op.Key) == "k" {
			if string(op.Value) != "v3" {
				t.Fatalf("expected v3, got %s", op.Value)
			}
		}
	}
}

func TestSizeBytes(t *testing.T) {
	b := New()
	_ = b.Put([]byte("abc"), []byte("12345"))
	if b.SizeBytes() != 8 { // 3 + 5
		t.Fatalf("expected 8, got %d", b.SizeBytes())
	}
}

func TestValidateEmptyKey(t *testing.T) {
	b := New()
	_ = b.Put([]byte(""), []byte("val"))
	if err := b.Validate(); err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestKeys(t *testing.T) {
	b := New()
	_ = b.Put([]byte("b"), []byte("1"))
	_ = b.Put([]byte("a"), []byte("2"))
	_ = b.Put([]byte("b"), []byte("3"))

	keys := b.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 unique keys, got %d", len(keys))
	}
}
