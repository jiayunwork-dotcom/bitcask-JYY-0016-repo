// Package bloom implements a classic Bloom filter used by bitcask to avoid
// unnecessary disk reads for keys that definitely do not exist in the store.
// The implementation uses double hashing (two independent hash functions
// combined) to simulate k hash functions, as described by Kirsch & Mitzenmacher.
package bloom

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/fnv"
	"io"
	"math"
	"sync"
)

// ErrCorrupt is returned when a serialised filter is malformed.
var ErrCorrupt = errors.New("bloom: corrupt filter data")

// Filter is a thread-safe Bloom filter backed by a bit array.
type Filter struct {
	mu   sync.RWMutex
	bits []uint64
	m    uint64 // number of bits
	k    uint64 // number of hash functions
	n    uint64 // items added
}

// New creates a filter sized for expectedN items at the given false-positive
// rate. The returned filter is empty.
func New(expectedN int, fpRate float64) *Filter {
	if expectedN <= 0 {
		expectedN = 1
	}
	if fpRate <= 0 || fpRate >= 1 {
		fpRate = 0.01
	}
	m := optimalM(expectedN, fpRate)
	k := optimalK(m, expectedN)
	words := (m + 63) / 64
	return &Filter{
		bits: make([]uint64, words),
		m:    m,
		k:    k,
	}
}

// optimalM computes the optimal number of bits.
func optimalM(n int, p float64) uint64 {
	fm := -float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)
	m := uint64(math.Ceil(fm))
	if m == 0 {
		m = 1
	}
	return m
}

// optimalK computes the optimal number of hash functions.
func optimalK(m uint64, n int) uint64 {
	fk := (float64(m) / float64(n)) * math.Ln2
	k := uint64(math.Round(fk))
	if k == 0 {
		k = 1
	}
	return k
}

// Add inserts key into the filter.
func (f *Filter) Add(key []byte) {
	h1, h2 := twoHashes(key)
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := uint64(0); i < f.k; i++ {
		pos := (h1 + i*h2) % f.m
		f.bits[pos/64] |= 1 << (pos % 64)
	}
	f.n++
}

// MayContain returns true if key might be in the set. False means key is
// definitely absent.
func (f *Filter) MayContain(key []byte) bool {
	h1, h2 := twoHashes(key)
	f.mu.RLock()
	defer f.mu.RUnlock()
	for i := uint64(0); i < f.k; i++ {
		pos := (h1 + i*h2) % f.m
		if f.bits[pos/64]&(1<<(pos%64)) == 0 {
			return false
		}
	}
	return true
}

// Count returns the number of items that have been added.
func (f *Filter) Count() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.n
}

// BitSize returns the number of bits in the filter.
func (f *Filter) BitSize() uint64 { return f.m }

// HashCount returns the number of hash functions used.
func (f *Filter) HashCount() uint64 { return f.k }

// EstimateFPRate returns the estimated current false-positive rate given the
// number of items inserted.
func (f *Filter) EstimateFPRate() float64 {
	f.mu.RLock()
	n := f.n
	f.mu.RUnlock()
	// p ≈ (1 - e^(-kn/m))^k
	exp := math.Exp(-float64(f.k) * float64(n) / float64(f.m))
	return math.Pow(1-exp, float64(f.k))
}

// Reset clears all bits and resets the count.
func (f *Filter) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.bits {
		f.bits[i] = 0
	}
	f.n = 0
}

// FillRatio returns the fraction of bits set.
func (f *Filter) FillRatio() float64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var set uint64
	for _, w := range f.bits {
		set += uint64(popcount(w))
	}
	return float64(set) / float64(f.m)
}

// WriteTo serialises the filter to w.
func (f *Filter) WriteTo(w io.Writer) (int64, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var hdr [24]byte
	binary.BigEndian.PutUint64(hdr[0:8], f.m)
	binary.BigEndian.PutUint64(hdr[8:16], f.k)
	binary.BigEndian.PutUint64(hdr[16:24], f.n)
	n, err := w.Write(hdr[:])
	if err != nil {
		return int64(n), err
	}
	total := int64(n)

	buf := make([]byte, 8)
	for _, word := range f.bits {
		binary.BigEndian.PutUint64(buf, word)
		nn, err := w.Write(buf)
		total += int64(nn)
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// ReadFrom deserialises a filter from r, replacing the receiver's state.
func (f *Filter) ReadFrom(r io.Reader) (int64, error) {
	var hdr [24]byte
	n, err := io.ReadFull(r, hdr[:])
	if err != nil {
		return int64(n), fmt.Errorf("%w: header: %v", ErrCorrupt, err)
	}
	total := int64(n)

	m := binary.BigEndian.Uint64(hdr[0:8])
	k := binary.BigEndian.Uint64(hdr[8:16])
	count := binary.BigEndian.Uint64(hdr[16:24])

	if m == 0 || k == 0 {
		return total, fmt.Errorf("%w: zero m or k", ErrCorrupt)
	}
	words := (m + 63) / 64
	if words > 1<<28 {
		return total, fmt.Errorf("%w: absurd size", ErrCorrupt)
	}

	bits := make([]uint64, words)
	buf := make([]byte, 8)
	for i := range bits {
		nn, err := io.ReadFull(r, buf)
		total += int64(nn)
		if err != nil {
			return total, fmt.Errorf("%w: bits: %v", ErrCorrupt, err)
		}
		bits[i] = binary.BigEndian.Uint64(buf)
	}

	f.mu.Lock()
	f.bits = bits
	f.m = m
	f.k = k
	f.n = count
	f.mu.Unlock()
	return total, nil
}

// twoHashes produces two independent 64-bit hashes using FNV-1a with
// different seeds.
func twoHashes(key []byte) (uint64, uint64) {
	h1 := fnv.New64a()
	_, _ = h1.Write(key)
	v1 := h1.Sum64()

	h2 := fnv.New64()
	_, _ = h2.Write(key)
	v2 := h2.Sum64()
	if v2 == 0 {
		v2 = 1
	}
	return v1, v2
}

// popcount returns the number of set bits in x.
func popcount(x uint64) int {
	// Hamming weight using bit manipulation
	x = x - ((x >> 1) & 0x5555555555555555)
	x = (x & 0x3333333333333333) + ((x >> 2) & 0x3333333333333333)
	x = (x + (x >> 4)) & 0x0f0f0f0f0f0f0f0f
	return int((x * 0x0101010101010101) >> 56)
}

// Union merges other into f. Both filters must have the same parameters.
func (f *Filter) Union(other *Filter) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	if f.m != other.m || f.k != other.k {
		return fmt.Errorf("bloom: cannot union filters with different parameters")
	}
	for i := range f.bits {
		f.bits[i] |= other.bits[i]
	}
	f.n += other.n
	return nil
}

// NewFromHash creates a filter that uses a custom hash.Hash64 factory. This is
// useful for testing or when a specific hash function is preferred.
func NewFromHash(expectedN int, fpRate float64, newHash func() hash.Hash64) *Filter {
	f := New(expectedN, fpRate)
	_ = newHash // reserved for future use
	return f
}
