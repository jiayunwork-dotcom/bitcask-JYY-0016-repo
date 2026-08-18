package bloom

import (
	"bytes"
	"fmt"
	"testing"
)

func TestNewFilterBasic(t *testing.T) {
	f := New(1000, 0.01)
	if f.BitSize() == 0 {
		t.Fatal("expected non-zero bit size")
	}
	if f.HashCount() == 0 {
		t.Fatal("expected non-zero hash count")
	}
	if f.Count() != 0 {
		t.Fatal("expected zero count on new filter")
	}
}

func TestAddAndMayContain(t *testing.T) {
	f := New(100, 0.01)
	keys := []string{"alpha", "bravo", "charlie", "delta"}
	for _, k := range keys {
		f.Add([]byte(k))
	}
	for _, k := range keys {
		if !f.MayContain([]byte(k)) {
			t.Errorf("expected MayContain(%q) = true", k)
		}
	}
	// A key never added should likely not be found (probabilistic).
	fp := 0
	for i := 0; i < 1000; i++ {
		if f.MayContain([]byte(fmt.Sprintf("missing_%d", i))) {
			fp++
		}
	}
	// With 4 items and fp_rate=0.01 the filter should have very few false positives.
	if fp > 50 {
		t.Errorf("too many false positives: %d/1000", fp)
	}
}

func TestReset(t *testing.T) {
	f := New(100, 0.01)
	f.Add([]byte("key"))
	f.Reset()
	if f.Count() != 0 {
		t.Fatal("count should be 0 after reset")
	}
	if f.MayContain([]byte("key")) {
		t.Fatal("should not contain key after reset")
	}
}

func TestSerializeRoundTrip(t *testing.T) {
	f := New(500, 0.01)
	for i := 0; i < 100; i++ {
		f.Add([]byte(fmt.Sprintf("key_%04d", i)))
	}

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}

	f2 := &Filter{}
	if _, err := f2.ReadFrom(&buf); err != nil {
		t.Fatal(err)
	}

	if f2.BitSize() != f.BitSize() {
		t.Fatalf("m mismatch: %d vs %d", f2.BitSize(), f.BitSize())
	}
	if f2.HashCount() != f.HashCount() {
		t.Fatalf("k mismatch: %d vs %d", f2.HashCount(), f.HashCount())
	}
	for i := 0; i < 100; i++ {
		k := []byte(fmt.Sprintf("key_%04d", i))
		if !f2.MayContain(k) {
			t.Errorf("deserialized filter missing key %q", k)
		}
	}
}

func TestFillRatio(t *testing.T) {
	f := New(10, 0.5)
	if f.FillRatio() != 0 {
		t.Fatal("empty filter should have fill ratio 0")
	}
	f.Add([]byte("x"))
	if f.FillRatio() <= 0 {
		t.Fatal("fill ratio should be positive after add")
	}
}

func TestUnion(t *testing.T) {
	f1 := New(100, 0.01)
	f2 := New(100, 0.01)

	f1.Add([]byte("a"))
	f2.Add([]byte("b"))

	if err := f1.Union(f2); err != nil {
		t.Fatal(err)
	}
	if !f1.MayContain([]byte("a")) {
		t.Fatal("expected a in union")
	}
	if !f1.MayContain([]byte("b")) {
		t.Fatal("expected b in union")
	}
}

func TestEstimateFPRate(t *testing.T) {
	f := New(1000, 0.01)
	for i := 0; i < 1000; i++ {
		f.Add([]byte(fmt.Sprintf("k%d", i)))
	}
	rate := f.EstimateFPRate()
	if rate > 0.05 {
		t.Errorf("estimated FP rate too high: %f", rate)
	}
}
