package ttl

import (
	"testing"
	"time"
)

func TestSetAndIsExpired(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m := NewWithClock(func() time.Time { return now })

	m.Set("k1", now.Add(time.Hour))
	if m.IsExpired("k1") {
		t.Fatal("k1 should not be expired yet")
	}

	// Advance clock past expiry.
	now = now.Add(2 * time.Hour)
	if !m.IsExpired("k1") {
		t.Fatal("k1 should be expired")
	}
}

func TestSetTTL(t *testing.T) {
	now := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	m := NewWithClock(func() time.Time { return now })

	m.SetTTL("x", 5*time.Minute)
	exp, ok := m.ExpiresAt("x")
	if !ok {
		t.Fatal("expected expiry entry")
	}
	want := now.Add(5 * time.Minute)
	if !exp.Equal(want) {
		t.Fatalf("expires at %v, want %v", exp, want)
	}
}

func TestRemove(t *testing.T) {
	m := New()
	m.SetTTL("a", time.Second)
	m.Remove("a")
	if m.IsExpired("a") {
		t.Fatal("removed key should not be expired")
	}
	if m.Len() != 0 {
		t.Fatal("expected empty manager after remove")
	}
}

func TestSweep(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewWithClock(func() time.Time { return now })

	m.Set("a", now.Add(-time.Second)) // already expired
	m.Set("b", now.Add(time.Hour))    // not expired
	m.Set("c", now.Add(-time.Minute)) // already expired

	expired := m.Sweep()
	if len(expired) != 2 {
		t.Fatalf("expected 2 expired, got %d", len(expired))
	}
	if m.Len() != 1 {
		t.Fatalf("expected 1 remaining, got %d", m.Len())
	}
}

func TestRemaining(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewWithClock(func() time.Time { return now })

	m.Set("k", now.Add(10*time.Minute))
	rem := m.Remaining("k")
	if rem != 10*time.Minute {
		t.Fatalf("expected 10m remaining, got %v", rem)
	}

	// After expiry, remaining is 0.
	now = now.Add(20 * time.Minute)
	if m.Remaining("k") != 0 {
		t.Fatal("expected 0 remaining after expiry")
	}
}

func TestEncodeDecodeCycle(t *testing.T) {
	m := New()
	m.SetTTL("alpha", time.Hour)
	m.SetTTL("beta", 2*time.Hour)

	data := m.Encode()
	m2 := New()
	if err := m2.Decode(data); err != nil {
		t.Fatal(err)
	}
	if m2.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", m2.Len())
	}
	// Check keys are present.
	for _, k := range []string{"alpha", "beta"} {
		if _, ok := m2.ExpiresAt(k); !ok {
			t.Errorf("key %q not found after decode", k)
		}
	}
}

func TestNextExpiry(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewWithClock(func() time.Time { return now })

	m.Set("a", now.Add(10*time.Minute))
	m.Set("b", now.Add(5*time.Minute))
	m.Set("c", now.Add(15*time.Minute))

	next := m.NextExpiry()
	want := now.Add(5 * time.Minute)
	if !next.Equal(want) {
		t.Fatalf("next expiry %v, want %v", next, want)
	}
}

func TestClear(t *testing.T) {
	m := New()
	m.SetTTL("x", time.Second)
	m.SetTTL("y", time.Second)
	m.Clear()
	if m.Len() != 0 {
		t.Fatal("expected empty after clear")
	}
}
