// Package codec provides encoding and decoding utilities used across the
// bitcask engine. It defines helpers for length-prefixed byte slices, varint
// encoding, and checksumming that are shared between the log, index, and hint
// file implementations.
package codec

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"math"
)

// MaxVarintLen is the maximum byte length of a varint-encoded uint64.
const MaxVarintLen = binary.MaxVarintLen64

// ErrOverflow is returned when a varint exceeds 64 bits.
var ErrOverflow = errors.New("codec: varint overflow")

// ErrTruncated is returned when the input is too short.
var ErrTruncated = errors.New("codec: truncated input")

// ErrTooLarge is returned when a length prefix exceeds a safety bound.
var ErrTooLarge = errors.New("codec: length exceeds limit")

// PutUint32BE writes a big-endian uint32 to b.
func PutUint32BE(b []byte, v uint32) {
	binary.BigEndian.PutUint32(b, v)
}

// Uint32BE reads a big-endian uint32 from b.
func Uint32BE(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}

// PutUint64BE writes a big-endian uint64 to b.
func PutUint64BE(b []byte, v uint64) {
	binary.BigEndian.PutUint64(b, v)
}

// Uint64BE reads a big-endian uint64 from b.
func Uint64BE(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

// AppendVarint appends a varint-encoded uint64 to buf.
func AppendVarint(buf []byte, v uint64) []byte {
	var tmp [MaxVarintLen]byte
	n := binary.PutUvarint(tmp[:], v)
	return append(buf, tmp[:n]...)
}

// ReadVarint reads a varint from r and returns the value and number of bytes
// consumed.
func ReadVarint(r io.ByteReader) (uint64, int, error) {
	var x uint64
	var s uint
	for i := 0; i < MaxVarintLen; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, i, err
		}
		if b < 0x80 {
			if i == MaxVarintLen-1 && b > 1 {
				return 0, i + 1, ErrOverflow
			}
			return x | uint64(b)<<s, i + 1, nil
		}
		x |= uint64(b&0x7f) << s
		s += 7
	}
	return 0, MaxVarintLen, ErrOverflow
}

// DecodeVarint decodes a varint from buf and returns the value and bytes read.
func DecodeVarint(buf []byte) (uint64, int) {
	return binary.Uvarint(buf)
}

// EncodeVarint encodes v as a varint and returns the bytes.
func EncodeVarint(v uint64) []byte {
	var buf [MaxVarintLen]byte
	n := binary.PutUvarint(buf[:], v)
	return buf[:n]
}

// LengthPrefixed prepends a 4-byte big-endian length to data and returns the
// combined slice.
func LengthPrefixed(data []byte) []byte {
	out := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(data)))
	copy(out[4:], data)
	return out
}

// ReadLengthPrefixed reads a 4-byte length prefix and then the payload from r.
// It enforces a maximum payload size of maxLen; pass 0 for a default of 64MB.
func ReadLengthPrefixed(r io.Reader, maxLen int) ([]byte, error) {
	if maxLen <= 0 {
		maxLen = 64 << 20
	}
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint32(hdr[:]))
	if n > maxLen {
		return nil, ErrTooLarge
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// WriteLengthPrefixed writes a 4-byte length prefix followed by data to w.
func WriteLengthPrefixed(w io.Writer, data []byte) (int, error) {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(data)))
	n1, err := w.Write(hdr[:])
	if err != nil {
		return n1, err
	}
	n2, err := w.Write(data)
	return n1 + n2, err
}

// CRC32 computes the IEEE CRC32 of data.
func CRC32(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// CRC32Combine computes CRC32 of two concatenated slices without copying.
func CRC32Combine(a, b []byte) uint32 {
	h := crc32.NewIEEE()
	_, _ = h.Write(a)
	_, _ = h.Write(b)
	return h.Sum32()
}

// VerifyCRC32 checks whether the given crc matches the data.
func VerifyCRC32(data []byte, expected uint32) bool {
	return CRC32(data) == expected
}

// Float64ToBytes converts a float64 to 8 bytes (IEEE 754 big-endian).
func Float64ToBytes(f float64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], math.Float64bits(f))
	return buf[:]
}

// BytesToFloat64 converts 8 bytes to a float64.
func BytesToFloat64(b []byte) float64 {
	bits := binary.BigEndian.Uint64(b)
	return math.Float64frombits(bits)
}

// Int64ToBytes converts an int64 to 8 big-endian bytes.
func Int64ToBytes(v int64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(v))
	return buf[:]
}

// BytesToInt64 converts 8 big-endian bytes to int64.
func BytesToInt64(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}

// ZigZagEncode encodes a signed int64 as an unsigned uint64 using zig-zag encoding.
func ZigZagEncode(v int64) uint64 {
	return uint64((v << 1) ^ (v >> 63))
}

// ZigZagDecode decodes a zig-zag encoded uint64 back to int64.
func ZigZagDecode(v uint64) int64 {
	return int64((v >> 1) ^ -(v & 1))
}
