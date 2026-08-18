// Package keydir implements the "key directory" snapshot mechanism for
// bitcask. A keydir is a serialisable point-in-time snapshot of the in-memory
// index that can be written to disk (as a hint file) and reloaded on startup
// to avoid a full log scan. Unlike the live index, a keydir is immutable once
// created and can be shared across goroutines without locking.
package keydir

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

// ErrFormat is returned when a keydir file is malformed.
var ErrFormat = errors.New("keydir: invalid format")

// magic is the 4-byte header identifying a keydir file.
var magic = [4]byte{'K', 'D', 'I', 'R'}

// version is the current on-disk format version.
const version uint16 = 1

// Record is a single entry in the key directory.
type Record struct {
	Key       string
	FileID    int64
	Offset    int64
	Size      int64
	Tombstone bool
	Timestamp int64 // unix nano when written
}

// Snapshot is an immutable key directory.
type Snapshot struct {
	records   []Record
	byKey     map[string]int // key -> index in records
	createdAt time.Time
}

// NewSnapshot builds a snapshot from a slice of records. Duplicate keys are
// resolved by keeping the last one (highest index).
func NewSnapshot(recs []Record) *Snapshot {
	byKey := make(map[string]int, len(recs))
	for i, r := range recs {
		byKey[r.Key] = i
	}
	return &Snapshot{
		records:   recs,
		byKey:     byKey,
		createdAt: time.Now(),
	}
}

// Len returns the total number of unique keys in the snapshot.
func (s *Snapshot) Len() int { return len(s.byKey) }

// Get looks up a key and returns the record and whether it was found.
func (s *Snapshot) Get(key string) (Record, bool) {
	idx, ok := s.byKey[key]
	if !ok {
		return Record{}, false
	}
	return s.records[idx], true
}

// LiveCount returns the number of non-tombstoned keys.
func (s *Snapshot) LiveCount() int {
	count := 0
	for _, idx := range s.byKey {
		if !s.records[idx].Tombstone {
			count++
		}
	}
	return count
}

// Keys returns all keys in sorted order.
func (s *Snapshot) Keys() []string {
	out := make([]string, 0, len(s.byKey))
	for k := range s.byKey {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// LiveKeys returns sorted keys that are not tombstoned.
func (s *Snapshot) LiveKeys() []string {
	var out []string
	for k, idx := range s.byKey {
		if !s.records[idx].Tombstone {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Records returns a copy of all records.
func (s *Snapshot) Records() []Record {
	out := make([]Record, len(s.records))
	copy(out, s.records)
	return out
}

// CreatedAt returns when the snapshot was created.
func (s *Snapshot) CreatedAt() time.Time { return s.createdAt }

// FileIDs returns the unique set of file IDs referenced, sorted ascending.
func (s *Snapshot) FileIDs() []int64 {
	seen := make(map[int64]struct{})
	for _, r := range s.records {
		seen[r.FileID] = struct{}{}
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// WriteTo serialises the snapshot to w in a compact binary format:
//
//	[magic:4][version:2][count:4][records...]
//
// Each record:
//
//	[keylen:4][key:keylen][fileid:8][offset:8][size:8][tombstone:1][timestamp:8]
func (s *Snapshot) WriteTo(w io.Writer) (int64, error) {
	var hdr [10]byte
	copy(hdr[0:4], magic[:])
	binary.BigEndian.PutUint16(hdr[4:6], version)
	binary.BigEndian.PutUint32(hdr[6:10], uint32(len(s.records)))
	n, err := w.Write(hdr[:])
	if err != nil {
		return int64(n), err
	}
	total := int64(n)

	for _, r := range s.records {
		nn, err := writeRecord(w, r)
		total += nn
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// ReadFrom loads a snapshot from r.
func ReadFrom(r io.Reader) (*Snapshot, error) {
	var hdr [10]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("%w: header: %v", ErrFormat, err)
	}
	if hdr[0] != magic[0] || hdr[1] != magic[1] || hdr[2] != magic[2] || hdr[3] != magic[3] {
		return nil, fmt.Errorf("%w: bad magic", ErrFormat)
	}
	ver := binary.BigEndian.Uint16(hdr[4:6])
	if ver != version {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrFormat, ver)
	}
	count := binary.BigEndian.Uint32(hdr[6:10])
	if count > 1<<28 {
		return nil, fmt.Errorf("%w: absurd record count %d", ErrFormat, count)
	}

	recs := make([]Record, 0, count)
	for i := uint32(0); i < count; i++ {
		rec, err := readRecord(r)
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	snap := NewSnapshot(recs)
	return snap, nil
}

// SaveToFile writes the snapshot to a file.
func (s *Snapshot) SaveToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := s.WriteTo(f); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// LoadFromFile reads a snapshot from a file.
func LoadFromFile(path string) (*Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadFrom(f)
}

// recordSize is the fixed overhead per record: keylen(4)+fileid(8)+offset(8)+size(8)+tomb(1)+ts(8)=37.
const recordFixed = 4 + 8 + 8 + 8 + 1 + 8

func writeRecord(w io.Writer, r Record) (int64, error) {
	buf := make([]byte, recordFixed+len(r.Key))
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(r.Key)))
	copy(buf[4:4+len(r.Key)], r.Key)
	off := 4 + len(r.Key)
	binary.BigEndian.PutUint64(buf[off:off+8], uint64(r.FileID))
	off += 8
	binary.BigEndian.PutUint64(buf[off:off+8], uint64(r.Offset))
	off += 8
	binary.BigEndian.PutUint64(buf[off:off+8], uint64(r.Size))
	off += 8
	if r.Tombstone {
		buf[off] = 1
	}
	off++
	binary.BigEndian.PutUint64(buf[off:off+8], uint64(r.Timestamp))

	n, err := w.Write(buf)
	return int64(n), err
}

func readRecord(r io.Reader) (Record, error) {
	var kl [4]byte
	if _, err := io.ReadFull(r, kl[:]); err != nil {
		return Record{}, fmt.Errorf("%w: keylen: %v", ErrFormat, err)
	}
	keyLen := int(binary.BigEndian.Uint32(kl[:]))
	if keyLen > 1<<24 {
		return Record{}, fmt.Errorf("%w: absurd key length %d", ErrFormat, keyLen)
	}

	buf := make([]byte, keyLen+8+8+8+1+8)
	if _, err := io.ReadFull(r, buf); err != nil {
		return Record{}, fmt.Errorf("%w: record body: %v", ErrFormat, err)
	}

	key := string(buf[:keyLen])
	off := keyLen
	fileID := int64(binary.BigEndian.Uint64(buf[off : off+8]))
	off += 8
	offset := int64(binary.BigEndian.Uint64(buf[off : off+8]))
	off += 8
	size := int64(binary.BigEndian.Uint64(buf[off : off+8]))
	off += 8
	tomb := buf[off] == 1
	off++
	ts := int64(binary.BigEndian.Uint64(buf[off : off+8]))

	return Record{
		Key:       key,
		FileID:    fileID,
		Offset:    offset,
		Size:      size,
		Tombstone: tomb,
		Timestamp: ts,
	}, nil
}
