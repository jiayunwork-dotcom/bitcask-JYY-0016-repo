package codec

import (
	"bytes"
	"testing"
)

func TestUint32BE(t *testing.T) {
	var buf [4]byte
	PutUint32BE(buf[:], 0xDEADBEEF)
	got := Uint32BE(buf[:])
	if got != 0xDEADBEEF {
		t.Fatalf("expected 0xDEADBEEF, got 0x%X", got)
	}
}

func TestUint64BE(t *testing.T) {
	var buf [8]byte
	PutUint64BE(buf[:], 0x0102030405060708)
	got := Uint64BE(buf[:])
	if got != 0x0102030405060708 {
		t.Fatalf("expected 0x0102030405060708, got 0x%X", got)
	}
}

func TestVarintRoundTrip(t *testing.T) {
	values := []uint64{0, 1, 127, 128, 255, 256, 16383, 16384, 1<<63 - 1}
	for _, v := range values {
		encoded := EncodeVarint(v)
		decoded, n := DecodeVarint(encoded)
		if n <= 0 {
			t.Fatalf("DecodeVarint returned n=%d for %d", n, v)
		}
		if decoded != v {
			t.Fatalf("roundtrip failed: %d -> %d", v, decoded)
		}
	}
}

func TestAppendVarint(t *testing.T) {
	buf := AppendVarint(nil, 300)
	decoded, _ := DecodeVarint(buf)
	if decoded != 300 {
		t.Fatalf("expected 300, got %d", decoded)
	}
}

func TestLengthPrefixed(t *testing.T) {
	data := []byte("hello, world")
	framed := LengthPrefixed(data)
	r := bytes.NewReader(framed)
	got, err := ReadLengthPrefixed(r, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestWriteLengthPrefixed(t *testing.T) {
	var buf bytes.Buffer
	data := []byte("test payload")
	_, err := WriteLengthPrefixed(&buf, data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadLengthPrefixed(&buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestCRC32(t *testing.T) {
	data := []byte("The quick brown fox")
	c := CRC32(data)
	if c == 0 {
		t.Fatal("CRC32 should not be zero for non-empty data")
	}
	if !VerifyCRC32(data, c) {
		t.Fatal("VerifyCRC32 should pass")
	}
	if VerifyCRC32(data, c+1) {
		t.Fatal("VerifyCRC32 should fail for wrong checksum")
	}
}

func TestCRC32Combine(t *testing.T) {
	a := []byte("hello")
	b := []byte("world")
	combined := CRC32Combine(a, b)
	full := CRC32(append(a, b...))
	if combined != full {
		t.Fatalf("CRC32Combine %d != CRC32(concat) %d", combined, full)
	}
}

func TestFloat64Conversion(t *testing.T) {
	vals := []float64{0, 1.5, -3.14, 1e100}
	for _, v := range vals {
		b := Float64ToBytes(v)
		got := BytesToFloat64(b)
		if got != v {
			t.Fatalf("roundtrip failed for %f: got %f", v, got)
		}
	}
}

func TestInt64Conversion(t *testing.T) {
	vals := []int64{0, 1, -1, 1<<62, -(1 << 62)}
	for _, v := range vals {
		b := Int64ToBytes(v)
		got := BytesToInt64(b)
		if got != v {
			t.Fatalf("roundtrip failed for %d: got %d", v, got)
		}
	}
}

func TestZigZag(t *testing.T) {
	vals := []int64{0, 1, -1, 2, -2, 100, -100, 1<<62, -(1 << 62)}
	for _, v := range vals {
		enc := ZigZagEncode(v)
		dec := ZigZagDecode(enc)
		if dec != v {
			t.Fatalf("zigzag roundtrip failed for %d", v)
		}
	}
}

func TestReadLengthPrefixedTooLarge(t *testing.T) {
	// Construct a frame claiming 1GB payload.
	var buf bytes.Buffer
	var hdr [4]byte
	PutUint32BE(hdr[:], 1<<30)
	buf.Write(hdr[:])
	_, err := ReadLengthPrefixed(&buf, 1024)
	if err != ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}
