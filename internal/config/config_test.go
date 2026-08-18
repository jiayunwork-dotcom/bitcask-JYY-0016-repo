package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultIsValid(t *testing.T) {
	c := Default()
	if err := c.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestValidateRejectsNegativeSegmentSize(t *testing.T) {
	c := Default()
	c.MaxSegmentSize = -1
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateRejectsBadBloomFP(t *testing.T) {
	c := Default()
	c.BloomFPRate = 1.5
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error for bloom_fp_rate > 1")
	}
}

func TestValidateRejectsSyncIntervalZero(t *testing.T) {
	c := Default()
	c.Sync = SyncInterval
	c.SyncInterval = 0
	if err := c.Validate(); err == nil {
		t.Fatal("expected error when sync=interval but interval=0")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	c := Default()
	c.MaxSegmentSize = 999
	c.CacheSize = 42

	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.MaxSegmentSize != 999 {
		t.Fatalf("expected 999, got %d", loaded.MaxSegmentSize)
	}
	if loaded.CacheSize != 42 {
		t.Fatalf("expected 42, got %d", loaded.CacheSize)
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Should return default.
	if c.MaxSegmentSize != Default().MaxSegmentSize {
		t.Fatal("expected default config when file missing")
	}
}

func TestReadFromInvalid(t *testing.T) {
	r := strings.NewReader(`{"max_segment_size": -5}`)
	_, err := ReadFrom(r)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestMerge(t *testing.T) {
	base := Default()
	over := Config{MaxSegmentSize: 1024, ReadOnly: true}
	merged := Merge(base, over)
	if merged.MaxSegmentSize != 1024 {
		t.Fatalf("expected 1024, got %d", merged.MaxSegmentSize)
	}
	if !merged.ReadOnly {
		t.Fatal("expected ReadOnly=true")
	}
	if merged.CacheSize != base.CacheSize {
		t.Fatal("CacheSize should come from base")
	}
}

func TestParseSyncPolicy(t *testing.T) {
	tests := []struct {
		in   string
		want SyncPolicy
	}{
		{"none", SyncNone},
		{"", SyncNone},
		{"every_write", SyncEveryWrite},
		{"interval", SyncInterval},
	}
	for _, tt := range tests {
		got, err := ParseSyncPolicy(tt.in)
		if err != nil {
			t.Fatalf("ParseSyncPolicy(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("ParseSyncPolicy(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
	_, err := ParseSyncPolicy("bogus")
	if err == nil {
		t.Fatal("expected error for unknown policy")
	}
}

func TestEffectiveDefaults(t *testing.T) {
	c := Config{}
	if c.EffectiveMaxKeySize() != 64<<10 {
		t.Fatal("expected 64KB default")
	}
	if c.EffectiveMaxValueSize() != 1<<30 {
		t.Fatal("expected 1GB default")
	}
}

func TestSyncPolicyString(t *testing.T) {
	if SyncNone.String() != "none" {
		t.Fatal("bad string")
	}
	if SyncEveryWrite.String() != "every_write" {
		t.Fatal("bad string")
	}
	if SyncInterval.String() != "interval" {
		t.Fatal("bad string")
	}
}

func TestSaveCreatesFile(t *testing.T) {
	dir := t.TempDir()
	c := Default()
	c.SyncInterval = 5 * time.Second
	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "bitcask.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}
