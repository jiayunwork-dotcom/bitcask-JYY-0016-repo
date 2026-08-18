// Package config provides structured configuration for the bitcask engine.
// It defines tunable parameters such as max segment size, sync policy, TTL
// defaults, bloom filter sizing, and compaction thresholds.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// configFile is the conventional name for a persisted JSON config.
const configFile = "bitcask.json"

// ErrInvalid is returned when a Config contains illegal values.
var ErrInvalid = errors.New("config: invalid")

// SyncPolicy controls when the active segment is fsynced.
type SyncPolicy int

const (
	// SyncNone never explicitly fsyncs; relies on OS page cache.
	SyncNone SyncPolicy = iota
	// SyncEveryWrite fsyncs after each Put/Delete.
	SyncEveryWrite
	// SyncInterval fsyncs on a timer.
	SyncInterval
)

// String returns a human-readable label for the sync policy.
func (p SyncPolicy) String() string {
	switch p {
	case SyncNone:
		return "none"
	case SyncEveryWrite:
		return "every_write"
	case SyncInterval:
		return "interval"
	default:
		return fmt.Sprintf("SyncPolicy(%d)", int(p))
	}
}

// ParseSyncPolicy parses a string into a SyncPolicy.
func ParseSyncPolicy(s string) (SyncPolicy, error) {
	switch s {
	case "none", "":
		return SyncNone, nil
	case "every_write":
		return SyncEveryWrite, nil
	case "interval":
		return SyncInterval, nil
	default:
		return SyncNone, fmt.Errorf("%w: unknown sync policy %q", ErrInvalid, s)
	}
}

// Config holds all tunable parameters for a bitcask store instance.
type Config struct {
	// MaxSegmentSize is the maximum bytes an active segment may grow to before
	// rotation. Zero means unlimited.
	MaxSegmentSize int64 `json:"max_segment_size"`

	// Sync controls the fsync policy.
	Sync SyncPolicy `json:"sync_policy"`

	// SyncInterval is the fsync period when Sync == SyncInterval.
	SyncInterval time.Duration `json:"sync_interval"`

	// DefaultTTL is the default time-to-live for new keys. Zero means no
	// expiration.
	DefaultTTL time.Duration `json:"default_ttl"`

	// BloomExpectedKeys is the number of expected unique keys for sizing the
	// bloom filter. Zero disables the bloom filter.
	BloomExpectedKeys int `json:"bloom_expected_keys"`

	// BloomFPRate is the desired false-positive rate for the bloom filter.
	BloomFPRate float64 `json:"bloom_fp_rate"`

	// CompactThreshold is the fraction of dead bytes at which auto-compaction
	// triggers. Range [0,1]. Zero disables auto-compaction.
	CompactThreshold float64 `json:"compact_threshold"`

	// CompactMinSegments is the minimum number of segment files before
	// compaction is considered.
	CompactMinSegments int `json:"compact_min_segments"`

	// MaxKeySize is the maximum allowed key length in bytes. Zero means 64KB.
	MaxKeySize int `json:"max_key_size"`

	// MaxValueSize is the maximum allowed value length in bytes. Zero means
	// 1GB.
	MaxValueSize int `json:"max_value_size"`

	// CacheSize is the number of recently-read values to cache in memory.
	// Zero disables caching.
	CacheSize int `json:"cache_size"`

	// ReadOnly opens the store without permitting writes.
	ReadOnly bool `json:"read_only"`
}

// Default returns a Config with reasonable production defaults.
func Default() Config {
	return Config{
		MaxSegmentSize:     256 << 20, // 256 MB
		Sync:               SyncNone,
		SyncInterval:       time.Second,
		DefaultTTL:         0,
		BloomExpectedKeys:  100_000,
		BloomFPRate:        0.01,
		CompactThreshold:   0.5,
		CompactMinSegments: 3,
		MaxKeySize:         64 << 10, // 64 KB
		MaxValueSize:       1 << 30,  // 1 GB
		CacheSize:          1024,
		ReadOnly:           false,
	}
}

// Validate checks internal consistency of c and returns ErrInvalid for any
// illegal values.
func (c *Config) Validate() error {
	if c.MaxSegmentSize < 0 {
		return fmt.Errorf("%w: max_segment_size must be non-negative", ErrInvalid)
	}
	if c.Sync == SyncInterval && c.SyncInterval <= 0 {
		return fmt.Errorf("%w: sync_interval must be positive when sync=interval", ErrInvalid)
	}
	if c.DefaultTTL < 0 {
		return fmt.Errorf("%w: default_ttl must be non-negative", ErrInvalid)
	}
	if c.BloomExpectedKeys < 0 {
		return fmt.Errorf("%w: bloom_expected_keys must be non-negative", ErrInvalid)
	}
	if c.BloomFPRate < 0 || c.BloomFPRate > 1 {
		return fmt.Errorf("%w: bloom_fp_rate must be in [0,1]", ErrInvalid)
	}
	if c.CompactThreshold < 0 || c.CompactThreshold > 1 {
		return fmt.Errorf("%w: compact_threshold must be in [0,1]", ErrInvalid)
	}
	if c.CompactMinSegments < 0 {
		return fmt.Errorf("%w: compact_min_segments must be non-negative", ErrInvalid)
	}
	if c.MaxKeySize < 0 {
		return fmt.Errorf("%w: max_key_size must be non-negative", ErrInvalid)
	}
	if c.MaxValueSize < 0 {
		return fmt.Errorf("%w: max_value_size must be non-negative", ErrInvalid)
	}
	if c.CacheSize < 0 {
		return fmt.Errorf("%w: cache_size must be non-negative", ErrInvalid)
	}
	return nil
}

// EffectiveMaxKeySize returns MaxKeySize or 64KB if zero.
func (c *Config) EffectiveMaxKeySize() int {
	if c.MaxKeySize == 0 {
		return 64 << 10
	}
	return c.MaxKeySize
}

// EffectiveMaxValueSize returns MaxValueSize or 1GB if zero.
func (c *Config) EffectiveMaxValueSize() int {
	if c.MaxValueSize == 0 {
		return 1 << 30
	}
	return c.MaxValueSize
}

// Save writes the config as JSON to the given directory.
func (c *Config) Save(dir string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	path := filepath.Join(dir, configFile)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}

// Load reads a Config from a JSON file in dir. If the file does not exist it
// returns Default().
func Load(dir string) (Config, error) {
	path := filepath.Join(dir, configFile)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer f.Close()
	return ReadFrom(f)
}

// ReadFrom decodes a Config from r.
func ReadFrom(r io.Reader) (Config, error) {
	var c Config
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("config: decode: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Merge applies non-zero values from overrides on top of base. Useful for
// layering CLI flags on top of a file-based config.
func Merge(base, overrides Config) Config {
	out := base
	if overrides.MaxSegmentSize != 0 {
		out.MaxSegmentSize = overrides.MaxSegmentSize
	}
	if overrides.Sync != SyncNone {
		out.Sync = overrides.Sync
	}
	if overrides.SyncInterval != 0 {
		out.SyncInterval = overrides.SyncInterval
	}
	if overrides.DefaultTTL != 0 {
		out.DefaultTTL = overrides.DefaultTTL
	}
	if overrides.BloomExpectedKeys != 0 {
		out.BloomExpectedKeys = overrides.BloomExpectedKeys
	}
	if overrides.BloomFPRate != 0 {
		out.BloomFPRate = overrides.BloomFPRate
	}
	if overrides.CompactThreshold != 0 {
		out.CompactThreshold = overrides.CompactThreshold
	}
	if overrides.CompactMinSegments != 0 {
		out.CompactMinSegments = overrides.CompactMinSegments
	}
	if overrides.MaxKeySize != 0 {
		out.MaxKeySize = overrides.MaxKeySize
	}
	if overrides.MaxValueSize != 0 {
		out.MaxValueSize = overrides.MaxValueSize
	}
	if overrides.CacheSize != 0 {
		out.CacheSize = overrides.CacheSize
	}
	if overrides.ReadOnly {
		out.ReadOnly = true
	}
	return out
}
