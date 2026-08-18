// Package batch implements a write-batch mechanism for bitcask. A batch
// collects multiple Put/Delete operations in memory and applies them
// atomically: either all writes succeed or none do. This reduces per-operation
// overhead (fewer fsyncs, single lock acquisition) and provides a form of
// transaction isolation against concurrent readers.
package batch

import (
	"errors"
	"sync"
)

// ErrCommitted is returned when an operation is attempted on a batch that has
// already been committed.
var ErrCommitted = errors.New("batch: already committed")

// ErrDiscarded is returned when an operation is attempted on a discarded batch.
var ErrDiscarded = errors.New("batch: discarded")

// ErrEmpty is returned when committing an empty batch.
var ErrEmpty = errors.New("batch: empty")

// OpType distinguishes Put from Delete within a batch.
type OpType int

const (
	OpPut    OpType = iota // OpPut is a set operation.
	OpDelete              // OpDelete is a remove operation.
)

// Op is a single buffered write operation.
type Op struct {
	Type  OpType
	Key   []byte
	Value []byte // nil for deletes
}

// Batch buffers writes and applies them atomically.
type Batch struct {
	mu        sync.Mutex
	ops       []Op
	committed bool
	discarded bool
	sizeBytes int64
}

// New creates a new empty batch.
func New() *Batch {
	return &Batch{
		ops: make([]Op, 0, 16),
	}
}

// Put adds a set operation to the batch.
func (b *Batch) Put(key, value []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.committed {
		return ErrCommitted
	}
	if b.discarded {
		return ErrDiscarded
	}
	k := make([]byte, len(key))
	copy(k, key)
	v := make([]byte, len(value))
	copy(v, value)
	b.ops = append(b.ops, Op{Type: OpPut, Key: k, Value: v})
	b.sizeBytes += int64(len(key) + len(value))
	return nil
}

// Delete adds a remove operation to the batch.
func (b *Batch) Delete(key []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.committed {
		return ErrCommitted
	}
	if b.discarded {
		return ErrDiscarded
	}
	k := make([]byte, len(key))
	copy(k, key)
	b.ops = append(b.ops, Op{Type: OpDelete, Key: k})
	b.sizeBytes += int64(len(key))
	return nil
}

// Ops returns the buffered operations. The caller must not modify the slice.
func (b *Batch) Ops() []Op {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ops
}

// Len returns the number of buffered operations.
func (b *Batch) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.ops)
}

// SizeBytes returns the approximate memory footprint of buffered keys/values.
func (b *Batch) SizeBytes() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sizeBytes
}

// MarkCommitted marks the batch as committed so no further ops can be added.
func (b *Batch) MarkCommitted() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.committed = true
}

// Discard discards the batch. Further calls to Put/Delete will fail.
func (b *Batch) Discard() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.discarded = true
	b.ops = nil
}

// IsCommitted reports whether the batch has been committed.
func (b *Batch) IsCommitted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.committed
}

// IsDiscarded reports whether the batch has been discarded.
func (b *Batch) IsDiscarded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.discarded
}

// Reset clears all operations without marking the batch as committed or
// discarded. The batch can be reused.
func (b *Batch) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ops = b.ops[:0]
	b.sizeBytes = 0
	b.committed = false
	b.discarded = false
}

// Validate checks that all operations have valid keys.
func (b *Batch) Validate() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, op := range b.ops {
		if len(op.Key) == 0 {
			return errors.New("batch: empty key")
		}
	}
	return nil
}

// Deduplicate removes earlier operations on the same key, keeping only the
// last operation per key. This reduces the number of writes applied.
func (b *Batch) Deduplicate() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Track the last index for each key.
	last := make(map[string]int, len(b.ops))
	for i, op := range b.ops {
		last[string(op.Key)] = i
	}

	deduped := make([]Op, 0, len(last))
	for i, op := range b.ops {
		if last[string(op.Key)] == i {
			deduped = append(deduped, op)
		}
	}
	b.ops = deduped

	var size int64
	for _, op := range b.ops {
		size += int64(len(op.Key) + len(op.Value))
	}
	b.sizeBytes = size
}

// Keys returns the unique set of keys referenced in the batch, in first-seen
// order.
func (b *Batch) Keys() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	seen := make(map[string]struct{}, len(b.ops))
	var out []string
	for _, op := range b.ops {
		k := string(op.Key)
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}
