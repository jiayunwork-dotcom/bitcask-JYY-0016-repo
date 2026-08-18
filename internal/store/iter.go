package store

import (
	"sort"

	"bitcask/internal/index"
)

// Iterator provides sequential access to live key/value pairs in sorted key
// order. It reads each value from disk on demand.
type Iterator struct {
	store   *Store
	entries []index.KeyLocation
	pos     int
	err     error
}

// Iter returns an iterator over all live keys in sorted order. The store must
// remain open for the lifetime of the iterator.
func (s *Store) Iter() *Iterator {
	s.mu.Lock()
	entries := s.index.LiveEntries()
	s.mu.Unlock()
	return &Iterator{store: s, entries: entries}
}

// Valid reports whether the iterator is positioned at a valid entry.
func (it *Iterator) Valid() bool {
	return it.pos < len(it.entries) && it.err == nil
}

// Next advances the iterator to the next entry.
func (it *Iterator) Next() {
	it.pos++
}

// Key returns the current key.
func (it *Iterator) Key() string {
	if !it.Valid() {
		return ""
	}
	return it.entries[it.pos].Key
}

// Value reads and returns the current value from disk.
func (it *Iterator) Value() ([]byte, error) {
	if !it.Valid() {
		return nil, ErrClosed
	}
	kl := it.entries[it.pos]
	v, ok, err := it.store.Get([]byte(kl.Key))
	if err != nil {
		it.err = err
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return v, nil
}

// Err returns any error encountered during iteration.
func (it *Iterator) Err() error {
	return it.err
}

// Rewind resets the iterator to the beginning.
func (it *Iterator) Rewind() {
	it.pos = 0
	it.err = nil
}

// Count returns the total number of entries the iterator will visit.
func (it *Iterator) Count() int {
	return len(it.entries)
}

// Seek positions the iterator at the first key >= target using binary search.
func (it *Iterator) Seek(target string) {
	idx := sort.Search(len(it.entries), func(i int) bool {
		return it.entries[i].Key >= target
	})
	it.pos = idx
	it.err = nil
}

// Prefix returns an iterator that only yields keys with the given prefix.
func (s *Store) Prefix(prefix string) *Iterator {
	s.mu.Lock()
	all := s.index.LiveEntries()
	s.mu.Unlock()

	var filtered []index.KeyLocation
	for _, kl := range all {
		if keyHasPrefix(kl.Key, prefix) {
			filtered = append(filtered, kl)
		}
	}
	return &Iterator{store: s, entries: filtered}
}

// Range returns an iterator that only yields keys in [start, end).
func (s *Store) Range(start, end string) *Iterator {
	s.mu.Lock()
	all := s.index.LiveEntries()
	s.mu.Unlock()

	var filtered []index.KeyLocation
	for _, kl := range all {
		if kl.Key >= start && kl.Key < end {
			filtered = append(filtered, kl)
		}
	}
	return &Iterator{store: s, entries: filtered}
}
