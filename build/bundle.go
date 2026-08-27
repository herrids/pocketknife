package build

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MaxBundleEntries and MaxBundleBytes cap an extracted frontend bundle so an
// adversarial upload cannot exhaust disk: the same posture the sandbox takes
// toward untrusted input.
const (
	MaxBundleEntries = 10000
	MaxBundleBytes   = 200 << 20 // 200 MiB
)

// errBundleTooLarge signals that an entry's actual content exceeded its
// remaining share of MaxBundleBytes. It is caught and reworded by
// ExtractBundle; it never escapes this file.
var errBundleTooLarge = errors.New("bundle exceeds byte limit")

// ExtractBundle decompresses and extracts a gzipped tar stream into destDir.
// Every entry's path is resolved and checked to stay strictly inside destDir
// before anything is written: an entry using ".." or an absolute path is
// rejected, and only regular files and directories are accepted -- a symlink,
// hardlink, device file or anything else aborts the whole extraction. Entry
// count is capped against the tar headers; total extracted bytes is capped
// against bytes actually written to disk (not a header's declared size, which
// a crafted entry could understate) — writeBundleFile enforces this by never
// copying more than its remaining share of the cap, regardless of what a
// header claims. No file is written outside destDir under any input, and on
// any failure destDir is removed rather than left holding a partial
// extraction — every caller treats destDir as owned exclusively by this call.
func ExtractBundle(r io.Reader, destDir string) error {
	if err := extractBundle(r, destDir); err != nil {
		_ = os.RemoveAll(destDir)
		return err
	}
	return nil
}

func extractBundle(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("bundle is not gzip-compressed: %w", err)
	}
	defer gz.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create bundle destination: %w", err)
	}

	tr := tar.NewReader(gz)
	var totalBytes int64
	var entries int

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read bundle tar: %w", err)
		}

		entries++
		if entries > MaxBundleEntries {
			return fmt.Errorf("bundle exceeds %d entries", MaxBundleEntries)
		}

		target, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			return fmt.Errorf("bundle entry %q: %w", hdr.Name, err)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create %q: %w", hdr.Name, err)
			}
		case tar.TypeReg:
			remaining := MaxBundleBytes - totalBytes
			if remaining < 0 {
				remaining = 0
			}
			n, err := writeBundleFile(target, tr, remaining)
			totalBytes += n
			if errors.Is(err, errBundleTooLarge) {
				return fmt.Errorf("bundle exceeds %d bytes", MaxBundleBytes)
			}
			if err != nil {
				return fmt.Errorf("write %q: %w", hdr.Name, err)
			}
		default:
			return fmt.Errorf("bundle entry %q: unsupported type %v (only regular files and directories are allowed)", hdr.Name, hdr.Typeflag)
		}
	}
	return nil
}

// writeBundleFile copies r into target, never writing more than maxBytes+1
// bytes: enough to detect an overrun without buffering an unbounded amount of
// attacker-controlled data first. It returns the number of bytes actually
// written. If more than maxBytes bytes were available, it returns
// errBundleTooLarge alongside the (maxBytes+1) bytes it did write — the
// caller's cumulative total still reflects real bytes on disk, and
// ExtractBundle removes the whole destination directory on any error.
func writeBundleFile(target string, r io.Reader, maxBytes int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := io.CopyN(out, r, maxBytes+1)
	if err != nil && err != io.EOF {
		out.Close()
		return n, err
	}
	if cerr := out.Close(); cerr != nil {
		return n, cerr
	}
	if n > maxBytes {
		return n, errBundleTooLarge
	}
	return n, nil
}

// safeJoin resolves name against base the way a tar extractor must: it
// rejects an absolute path outright, then joins and cleans, then verifies the
// cleaned result is still inside base. This catches both a literal ".." entry
// and a deep path that climbs out via several segments.
func safeJoin(base, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute path not allowed")
	}
	baseClean := filepath.Clean(base)
	cleaned := filepath.Clean(filepath.Join(baseClean, name))
	if cleaned != baseClean && !strings.HasPrefix(cleaned, baseClean+string(filepath.Separator)) {
		return "", fmt.Errorf("escapes destination directory")
	}
	return cleaned, nil
}
