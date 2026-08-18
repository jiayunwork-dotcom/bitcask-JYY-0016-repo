// Package ttl implements time-based key expiration for bitcask. Each key may
// carry an optional expiry timestamp; the TTL manager lazily marks expired
// keys as tombstones during reads and periodically sweeps for proactive
// cleanup.
package ttl

import (
	"encoding/binary"
	"errors"
	"sort"
	"sync"
	"time"
)

// ErrExpired is returned when a key has exceeded its TTL.
var ErrExpired = errors.New("ttl: key expired")

// Entry records the absolute expiration time for one key.
type Entry struct {
	Key       string
	ExpiresAt time.Time
}

// Manager tracks per-key expiration times and provides lookup/sweep methods.
type Manager struct {
	mu      sync.RWMutex
	entries map[string]time.Time
	nowFn   func() time.Time
}

// New creates a TTL manager using the real clock.
func New() *Manager {
	return &Manager{
		entries: make(map[string]time.Time),
		nowFn:   time.Now,
	}
}

// NewWithClock creates a TTL manager with a custom clock for testing.
func NewWithClock(nowFn func() time.Time) *Manager {
	return &Manager{
		entries: make(map[string]time.Time),
		nowFn:   nowFn,
	}
}

// Set records an absolute expiration for key.
func (m *Manager) Set(key string, expiresAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = expiresAt
}

// SetTTL records a relative TTL for key starting from now.
func (m *Manager) SetTTL(key string, ttl time.Duration) {
	m.Set(key, m.nowFn().Add(ttl))
}

// Remove removes the TTL entry for key (the key no longer expires).
func (m *Manager) Remove(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
}

// IsExpired reports whether key's TTL has been exceeded. Keys without a TTL
// entry are never expired.
func (m *Manager) IsExpired(key string) bool {
	m.mu.RLock()
	exp, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return m.nowFn().After(exp)
}

// ExpiresAt returns the expiry time for key, or the zero time if no TTL is set.
func (m *Manager) ExpiresAt(key string) (time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.entries[key]
	return t, ok
}

// Remaining returns the remaining time until key expires, or zero if already
// expired or not tracked.
func (m *Manager) Remaining(key string) time.Duration {
	m.mu.RLock()
	exp, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok {
		return 0
	}
	rem := exp.Sub(m.nowFn())
	if rem < 0 {
		return 0
	}
	return rem
}

// Sweep returns all keys that have expired as of now and removes them from the
// tracking map.
func (m *Manager) Sweep() []string {
	now := m.nowFn()
	m.mu.Lock()
	defer m.mu.Unlock()

	var expired []string
	for k, exp := range m.entries {
		if now.After(exp) {
			expired = append(expired, k)
		}
	}
	for _, k := range expired {
		delete(m.entries, k)
	}
	sort.Strings(expired)
	return expired
}

// Len returns the number of keys with active TTLs.
func (m *Manager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// AllEntries returns a snapshot of all tracked entries sorted by expiry time.
func (m *Manager) AllEntries() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Entry, 0, len(m.entries))
	for k, exp := range m.entries {
		out = append(out, Entry{Key: k, ExpiresAt: exp})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ExpiresAt.Before(out[j].ExpiresAt)
	})
	return out
}

// NextExpiry returns the earliest expiry time among all tracked keys, or the
// zero time if no keys are tracked.
func (m *Manager) NextExpiry() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var earliest time.Time
	for _, exp := range m.entries {
		if earliest.IsZero() || exp.Before(earliest) {
			earliest = exp
		}
	}
	return earliest
}

// Encode serialises all TTL entries to a binary format:
//
//	[count:4][{keylen:4, key:keylen, unix_nano:8}...]
func (m *Manager) Encode() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	size := 4
	for k := range m.entries {
		size += 4 + len(k) + 8
	}
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(m.entries)))
	off := 4
	for k, exp := range m.entries {
		binary.BigEndian.PutUint32(buf[off:off+4], uint32(len(k)))
		off += 4
		copy(buf[off:], k)
		off += len(k)
		binary.BigEndian.PutUint64(buf[off:off+8], uint64(exp.UnixNano()))
		off += 8
	}
	return buf
}

// Decode restores TTL entries from the binary format produced by Encode.
func (m *Manager) Decode(data []byte) error {
	if len(data) < 4 {
		return errors.New("ttl: data too short")
	}
	count := binary.BigEndian.Uint32(data[0:4])
	off := 4
	entries := make(map[string]time.Time, count)
	for i := uint32(0); i < count; i++ {
		if off+4 > len(data) {
			return errors.New("ttl: truncated entry")
		}
		kl := int(binary.BigEndian.Uint32(data[off : off+4]))
		off += 4
		if off+kl+8 > len(data) {
			return errors.New("ttl: truncated key/time")
		}
		key := string(data[off : off+kl])
		off += kl
		nano := int64(binary.BigEndian.Uint64(data[off : off+8]))
		off += 8
		entries[key] = time.Unix(0, nano)
	}
	m.mu.Lock()
	m.entries = entries
	m.mu.Unlock()
	return nil
}

// Clear removes all TTL entries.
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make(map[string]time.Time)
}

// Keys returns a sorted list of all keys being tracked.
func (m *Manager) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.entries))
	for k := range m.entries {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
