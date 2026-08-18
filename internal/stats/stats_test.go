package stats

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCountersTake(t *testing.T) {
	c := &Counters{}
	c.Puts.Add(10)
	c.Gets.Add(20)
	c.BytesIn.Add(1024)

	snap := c.Take()
	if snap.Puts != 10 {
		t.Fatalf("expected 10 puts, got %d", snap.Puts)
	}
	if snap.Gets != 20 {
		t.Fatalf("expected 20 gets, got %d", snap.Gets)
	}
	if snap.BytesIn != 1024 {
		t.Fatalf("expected 1024 bytes in, got %d", snap.BytesIn)
	}
}

func TestCountersReset(t *testing.T) {
	c := &Counters{}
	c.Puts.Add(5)
	c.Reset()
	if c.Puts.Load() != 0 {
		t.Fatal("expected 0 after reset")
	}
}

func TestSegmentDeadRatio(t *testing.T) {
	s := SegmentInfo{LiveBytes: 700, DeadBytes: 300}
	ratio := s.DeadRatio()
	if ratio < 0.29 || ratio > 0.31 {
		t.Fatalf("expected ~0.3, got %f", ratio)
	}

	empty := SegmentInfo{}
	if empty.DeadRatio() != 0 {
		t.Fatal("empty segment should have 0 dead ratio")
	}
}

func TestCompactEventDuration(t *testing.T) {
	e := CompactEvent{
		StartedAt:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2024, 1, 1, 0, 0, 5, 0, time.UTC),
	}
	if e.Duration() != 5*time.Second {
		t.Fatalf("expected 5s, got %v", e.Duration())
	}
}

func TestCompactEventSpaceReclaimed(t *testing.T) {
	e := CompactEvent{BytesIn: 1000, BytesOut: 400}
	if e.SpaceReclaimed() != 600 {
		t.Fatalf("expected 600, got %d", e.SpaceReclaimed())
	}
}

func TestHistoryAddAndLen(t *testing.T) {
	h := NewHistory(3)
	h.Add(CompactEvent{SegmentsIn: 1})
	h.Add(CompactEvent{SegmentsIn: 2})
	h.Add(CompactEvent{SegmentsIn: 3})
	h.Add(CompactEvent{SegmentsIn: 4}) // evicts first

	if h.Len() != 3 {
		t.Fatalf("expected 3, got %d", h.Len())
	}
	events := h.Events()
	if events[0].SegmentsIn != 2 {
		t.Fatalf("expected first event segments_in=2, got %d", events[0].SegmentsIn)
	}
}

func TestHistoryLast(t *testing.T) {
	h := NewHistory(10)
	_, ok := h.Last()
	if ok {
		t.Fatal("expected no last event on empty history")
	}
	h.Add(CompactEvent{SegmentsIn: 5})
	e, ok := h.Last()
	if !ok || e.SegmentsIn != 5 {
		t.Fatal("unexpected last event")
	}
}

func TestCollectorTotalDiskBytes(t *testing.T) {
	c := NewCollector()
	c.SetSegments([]SegmentInfo{
		{SizeBytes: 100},
		{SizeBytes: 200},
		{SizeBytes: 300},
	})
	if c.TotalDiskBytes() != 600 {
		t.Fatalf("expected 600, got %d", c.TotalDiskBytes())
	}
}

func TestCollectorOverallDeadRatio(t *testing.T) {
	c := NewCollector()
	c.SetSegments([]SegmentInfo{
		{SizeBytes: 100, DeadBytes: 50},
		{SizeBytes: 100, DeadBytes: 50},
	})
	ratio := c.OverallDeadRatio()
	if ratio != 0.5 {
		t.Fatalf("expected 0.5, got %f", ratio)
	}
}

func TestCollectorJSON(t *testing.T) {
	c := NewCollector()
	c.Counters.Puts.Add(42)
	data, err := c.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	counters := parsed["counters"].(map[string]interface{})
	if counters["puts"].(float64) != 42 {
		t.Fatal("expected puts=42 in JSON")
	}
}
