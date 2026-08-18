package store

import (
	"fmt"
	"os"
	"path/filepath"

	"bitcask/internal/index"
	"bitcask/internal/log"
)

// Snapshot creates a consistent read-only copy of the database in dstDir. It
// syncs the active segment, then hard-links (or copies if hard-link fails) all
// segment files and the hint file into dstDir. The snapshot is crash-consistent
// because segments are append-only: any partial write at the tail of the active
// segment is benign on reload.
func (s *Store) Snapshot(dstDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}

	if err := s.active.Sync(); err != nil {
		return fmt.Errorf("store: snapshot sync: %w", err)
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("store: snapshot mkdir: %w", err)
	}

	for id, seg := range s.segments {
		src := seg.Path()
		dst := filepath.Join(dstDir, log.SegmentName(id))
		if err := linkOrCopy(src, dst); err != nil {
			return fmt.Errorf("store: snapshot segment %d: %w", id, err)
		}
	}

	// Copy hint file if it exists.
	hintSrc := filepath.Join(s.dir, hintFile)
	if _, err := os.Stat(hintSrc); err == nil {
		hintDst := filepath.Join(dstDir, hintFile)
		if err := linkOrCopy(hintSrc, hintDst); err != nil {
			return fmt.Errorf("store: snapshot hint: %w", err)
		}
	}

	return nil
}

// SnapshotInfo contains metadata about a snapshot.
type SnapshotInfo struct {
	Dir          string
	SegmentCount int
	TotalBytes   int64
	LiveKeys     int
}

// Inspect returns metadata about an existing snapshot directory without
// opening it fully.
func Inspect(dir string) (*SnapshotInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("store: inspect %s: %w", dir, err)
	}

	info := &SnapshotInfo{Dir: dir}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, ok := log.ParseSegmentID(e.Name()); ok {
			info.SegmentCount++
			fi, err := e.Info()
			if err == nil {
				info.TotalBytes += fi.Size()
			}
		}
	}

	// Try loading the hint file for live key count.
	hintPath := filepath.Join(dir, hintFile)
	f, err := os.Open(hintPath)
	if err == nil {
		idx := index.New()
		if err := idx.LoadHint(f); err == nil {
			info.LiveKeys = len(idx.LiveEntries())
		}
		_ = f.Close()
	}

	return info, nil
}

// linkOrCopy attempts a hard link; if that fails (cross-device) it falls back
// to a full copy.
func linkOrCopy(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return fileCopy(src, dst)
}

func fileCopy(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
