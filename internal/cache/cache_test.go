package cache

import (
	"fmt"
	"testing"
)

func TestPutAndGet(t *testing.T) {
	c := New(10)
	c.Put("hello", []byte("world"))
	v, ok := c.Get("hello")
	if !ok {
		t.Fatal("expected hit")
	}
	if string(v) != "world" {
		t.Fatalf("got %q, want %q", v, "world")
	}
}

func TestMiss(t *testing.T) {
	c := New(10)
	_, ok := c.Get("missing")
	if ok {
		t.Fatal("expected miss")
	}
}

func TestEviction(t *testing.T) {
	c := New(3)
	c.Put("a", []byte("1"))
	c.Put("b", []byte("2"))
	c.Put("c", []byte("3"))
	c.Put("d", []byte("4")) // should evict "a"

	if _, ok := c.Get("a"); ok {
		t.Fatal("expected 'a' to be evicted")
	}
	if _, ok := c.Get("d"); !ok {
		t.Fatal("expected 'd' to be present")
	}
}

func TestDelete(t *testing.T) {
	c := New(10)
	c.Put("key", []byte("val"))
	c.Delete("key")
	if _, ok := c.Get("key"); ok {
		t.Fatal("expected key to be deleted")
	}
	if c.Len() != 0 {
		t.Fatal("expected empty cache after delete")
	}
}

func TestLRUOrdering(t *testing.T) {
	c := New(3)
	c.Put("a", []byte("1"))
	c.Put("b", []byte("2"))
	c.Put("c", []byte("3"))

	// Access "a" to promote it.
	c.Get("a")

	// Insert "d" — should evict "b" (now oldest).
	c.Put("d", []byte("4"))

	if _, ok := c.Get("b"); ok {
		t.Fatal("expected 'b' evicted after 'a' was accessed")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected 'a' still present")
	}
}

func TestResize(t *testing.T) {
	c := New(5)
	for i := 0; i < 5; i++ {
		c.Put(fmt.Sprintf("k%d", i), []byte("v"))
	}
	c.Resize(2)
	if c.Len() != 2 {
		t.Fatalf("expected 2 entries after resize, got %d", c.Len())
	}
}

func TestStats(t *testing.T) {
	c := New(10)
	c.Put("x", []byte("y"))
	c.Get("x")       // hit
	c.Get("missing") // miss

	hits, misses, _ := c.Stats()
	if hits != 1 {
		t.Fatalf("expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Fatalf("expected 1 miss, got %d", misses)
	}
}

func TestDisabledCache(t *testing.T) {
	c := New(0)
	c.Put("key", []byte("val"))
	if _, ok := c.Get("key"); ok {
		t.Fatal("disabled cache should never hit")
	}
	if c.Len() != 0 {
		t.Fatal("disabled cache should have 0 length")
	}
}

func TestPeek(t *testing.T) {
	c := New(3)
	c.Put("a", []byte("1"))
	c.Put("b", []byte("2"))
	c.Put("c", []byte("3"))

	// Peek at "a" — should NOT promote it.
	v, ok := c.Peek("a")
	if !ok || string(v) != "1" {
		t.Fatal("peek should return the value")
	}

	// Insert "d" — should evict "a" (still oldest because peek didn't promote).
	c.Put("d", []byte("4"))
	if _, ok := c.Get("a"); ok {
		t.Fatal("expected 'a' evicted since peek doesn't promote")
	}
}

func TestContains(t *testing.T) {
	c := New(10)
	c.Put("x", []byte("y"))
	if !c.Contains("x") {
		t.Fatal("expected contains to be true")
	}
	if c.Contains("z") {
		t.Fatal("expected contains to be false for absent key")
	}
}
