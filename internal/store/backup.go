package store

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"bitcask/internal/log"
)

// Backup writes a gzipped tar archive of the database to w. The archive
// contains every segment file and the hint file (if present). The store is
// synced before the backup begins.
func (s *Store) Backup(w io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}

	if err := s.active.Sync(); err != nil {
		return fmt.Errorf("store: backup sync: %w", err)
	}

	gw := gzip.NewWriter(w)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for id, seg := range s.segments {
		if err := addFileToTar(tw, seg.Path(), log.SegmentName(id)); err != nil {
			return fmt.Errorf("store: backup segment %d: %w", id, err)
		}
	}

	hintPath := filepath.Join(s.dir, hintFile)
	if _, err := os.Stat(hintPath); err == nil {
		if err := addFileToTar(tw, hintPath, hintFile); err != nil {
			return fmt.Errorf("store: backup hint: %w", err)
		}
	}

	return nil
}

// Restore extracts a gzipped tar backup into dstDir. Any existing content in
// dstDir is overwritten.
func Restore(r io.Reader, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("store: restore mkdir: %w", err)
	}

	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("store: restore gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("store: restore tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		dst := filepath.Join(dstDir, filepath.Base(hdr.Name))
		f, err := os.Create(dst)
		if err != nil {
			return fmt.Errorf("store: restore create %s: %w", dst, err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			_ = f.Close()
			return fmt.Errorf("store: restore write %s: %w", dst, err)
		}
		_ = f.Close()
	}
	return nil
}

func addFileToTar(tw *tar.Writer, path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name: name,
		Mode: 0o644,
		Size: info.Size(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.Copy(tw, f); err != nil {
		return err
	}
	return nil
}
