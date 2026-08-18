// Package stats tracks runtime statistics for the bitcask engine: byte
// counters, operation counts, compaction history, and segment metrics. These
// are useful for monitoring and for informing auto-compaction decisions.
package stats

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// Counters holds monotonically increasing operation counters.
type Counters struct {
	Puts      atomic.Int64
	Gets      atomic.Int64
	Deletes   atomic.Int64
	Merges    atomic.Int64
	BytesIn   atomic.Int64
	BytesOut  atomic.Int64
	Misses    atomic.Int64
	CacheHits atomic.Int64
}

// Snapshot is a point-in-time copy of all counters.
type Snapshot struct {
	Puts      int64     `json:"puts"`
	Gets      int64     `json:"gets"`
	Deletes   int64     `json:"deletes"`
	Merges    int64     `json:"merges"`
	BytesIn   int64     `json:"bytes_in"`
	BytesOut  int64     `json:"bytes_out"`
	Misses    int64     `json:"misses"`
	CacheHits int64     `json:"cache_hits"`
	TakenAt   time.Time `json:"taken_at"`
}

// Take captures the current counter values.
func (c *Counters) Take() Snapshot {
	return Snapshot{
		Puts:      c.Puts.Load(),
		Gets:      c.Gets.Load(),
		Deletes:   c.Deletes.Load(),
		Merges:    c.Merges.Load(),
		BytesIn:   c.BytesIn.Load(),
		BytesOut:  c.BytesOut.Load(),
		Misses:    c.Misses.Load(),
		CacheHits: c.CacheHits.Load(),
		TakenAt:   time.Now(),
	}
}

// Reset zeroes all counters.
func (c *Counters) Reset() {
	c.Puts.Store(0)
	c.Gets.Store(0)
	c.Deletes.Store(0)
	c.Merges.Store(0)
	c.BytesIn.Store(0)
	c.BytesOut.Store(0)
	c.Misses.Store(0)
	c.CacheHits.Store(0)
}

// SegmentInfo holds metadata about a single segment file.
type SegmentInfo struct {
	FileID     int64 `json:"file_id"`
	SizeBytes  int64 `json:"size_bytes"`
	LiveKeys   int   `json:"live_keys"`
	DeadKeys   int   `json:"dead_keys"`
	LiveBytes  int64 `json:"live_bytes"`
	DeadBytes  int64 `json:"dead_bytes"`
	RecordCount int  `json:"record_count"`
}

// DeadRatio returns the fraction of dead bytes in this segment (0-1).
func (s *SegmentInfo) DeadRatio() float64 {
	total := s.LiveBytes + s.DeadBytes
	if total == 0 {
		return 0
	}
	return float64(s.DeadBytes) / float64(total)
}

// CompactEvent records a compaction that happened.
type CompactEvent struct {
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	SegmentsIn   int       `json:"segments_in"`
	SegmentsOut  int       `json:"segments_out"`
	BytesIn      int64     `json:"bytes_in"`
	BytesOut     int64     `json:"bytes_out"`
	KeysRetained int       `json:"keys_retained"`
	KeysDropped  int       `json:"keys_dropped"`
}

// Duration returns how long the compaction took.
func (e *CompactEvent) Duration() time.Duration {
	return e.FinishedAt.Sub(e.StartedAt)
}

// SpaceReclaimed returns the bytes freed by compaction.
func (e *CompactEvent) SpaceReclaimed() int64 {
	return e.BytesIn - e.BytesOut
}

// History stores the last N compaction events.
type History struct {
	mu     sync.Mutex
	events []CompactEvent
	cap    int
}

// NewHistory creates a history that keeps the last cap events.
func NewHistory(cap int) *History {
	if cap <= 0 {
		cap = 32
	}
	return &History{
		events: make([]CompactEvent, 0, cap),
		cap:    cap,
	}
}

// Add appends a new event, evicting the oldest if at capacity.
func (h *History) Add(e CompactEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.events) >= h.cap {
		copy(h.events, h.events[1:])
		h.events = h.events[:len(h.events)-1]
	}
	h.events = append(h.events, e)
}

// Events returns a copy of the stored events in chronological order.
func (h *History) Events() []CompactEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]CompactEvent, len(h.events))
	copy(out, h.events)
	return out
}

// Last returns the most recent event and whether one exists.
func (h *History) Last() (CompactEvent, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.events) == 0 {
		return CompactEvent{}, false
	}
	return h.events[len(h.events)-1], true
}

// Len returns the number of stored events.
func (h *History) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.events)
}

// Clear removes all events.
func (h *History) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = h.events[:0]
}

// Collector aggregates counters, segment info, and compaction history into a
// single stats endpoint.
type Collector struct {
	Counters *Counters
	History  *History
	mu       sync.RWMutex
	segments []SegmentInfo
}

// NewCollector creates a Collector with default history capacity.
func NewCollector() *Collector {
	return &Collector{
		Counters: &Counters{},
		History:  NewHistory(64),
	}
}

// SetSegments replaces the segment info list.
func (c *Collector) SetSegments(infos []SegmentInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.segments = infos
}

// Segments returns the current segment info list.
func (c *Collector) Segments() []SegmentInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]SegmentInfo, len(c.segments))
	copy(out, c.segments)
	return out
}

// TotalDiskBytes sums the size of all segments.
func (c *Collector) TotalDiskBytes() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var total int64
	for _, s := range c.segments {
		total += s.SizeBytes
	}
	return total
}

// TotalDeadBytes sums dead bytes across all segments.
func (c *Collector) TotalDeadBytes() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var total int64
	for _, s := range c.segments {
		total += s.DeadBytes
	}
	return total
}

// OverallDeadRatio returns the aggregate dead ratio.
func (c *Collector) OverallDeadRatio() float64 {
	total := c.TotalDiskBytes()
	if total == 0 {
		return 0
	}
	return float64(c.TotalDeadBytes()) / float64(total)
}

// JSON returns the full stats payload as indented JSON.
func (c *Collector) JSON() ([]byte, error) {
	snap := c.Counters.Take()
	segs := c.Segments()
	hist := c.History.Events()
	payload := struct {
		Counters Snapshot       `json:"counters"`
		Segments []SegmentInfo  `json:"segments"`
		History  []CompactEvent `json:"history"`
	}{
		Counters: snap,
		Segments: segs,
		History:  hist,
	}
	return json.MarshalIndent(payload, "", "  ")
}
