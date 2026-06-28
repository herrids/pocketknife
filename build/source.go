// source stores and retrieves each deploy's editable frontend source. Source
// artifacts are immutable, job-versioned directories at apps/<id>/sources/<jobID>/,
// parallel to the build artifacts at apps/<id>/builds/<jobID>/. They share the
// active_builds activation pointer: the current source is always the one stored
// under the currently active build's job id. Rollback repoints activation and
// therefore repoints source at the same time, so source can never drift from the
// live build.
//
// The backend never installs or executes stored source; it is extracted and
// returned opaquely using the same path-traversal/symlink/size guards as the
// frontend bundle (ExtractBundle).
package build

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SourcesDirName is the per-app subdirectory holding versioned frontend source
// artifacts, parallel to BuildsDirName.
const SourcesDirName = "sources"

// StoreSource extracts the gzipped tar r into apps/<appDir>/sources/<jobID>/
// using the same containment guards as ExtractBundle. The directory is written
// once and never modified; a prior job's source artifact is unaffected.
func StoreSource(appDir, jobID string, r io.Reader) error {
	destDir := filepath.Join(appDir, SourcesDirName, jobID)
	if err := ExtractBundle(r, destDir); err != nil {
		return fmt.Errorf("store source for job %s: %w", jobID, err)
	}
	return nil
}

// SourceDir returns the path of the stored source artifact for the given job
// and whether it exists on disk. Existence of the directory is the sole signal
// for whether source was stored — no separate database column is required.
func SourceDir(appDir, jobID string) (dir string, exists bool) {
	dir = filepath.Join(appDir, SourcesDirName, jobID)
	info, err := os.Stat(dir)
	return dir, err == nil && info.IsDir()
}

// PackSource creates a gzipped tar of sourceDir's contents. Entries are paths
// relative to sourceDir itself, matching the layout StoreSource expects on
// re-ingest. Symlinks and non-regular files are silently skipped.
func PackSource(sourceDir string) ([]byte, error) {
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		gz := gzip.NewWriter(pw)
		tw := tar.NewWriter(gz)
		err := tarDir(tw, sourceDir, sourceDir)
		if err == nil {
			err = tw.Close()
		}
		if err == nil {
			err = gz.Close()
		}
		_ = pw.CloseWithError(err)
		errCh <- err
	}()
	data, readErr := io.ReadAll(pr)
	writeErr := <-errCh
	if writeErr != nil {
		return nil, fmt.Errorf("pack source: %w", writeErr)
	}
	if readErr != nil {
		return nil, fmt.Errorf("pack source: read: %w", readErr)
	}
	return data, nil
}

// tarDir recursively adds the regular-file contents of dir to tw, using paths
// relative to root. Symlinks and other non-regular entries are skipped.
func tarDir(tw *tar.Writer, dir, root string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(dir, e.Name())
		relPath, err := filepath.Rel(root, srcPath)
		if err != nil {
			return err
		}
		if e.IsDir() {
			if err := tw.WriteHeader(&tar.Header{
				Typeflag: tar.TypeDir,
				Name:     relPath + "/",
				Mode:     0o755,
			}); err != nil {
				return err
			}
			if err := tarDir(tw, srcPath, root); err != nil {
				return err
			}
			continue
		}
		if !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     relPath,
			Size:     info.Size(),
			Mode:     0o644,
		}); err != nil {
			return err
		}
		f, err := os.Open(srcPath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, f)
		f.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}
