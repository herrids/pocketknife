package build

import (
	"archive/tar"
	"compress/gzip"
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

// ExtractBundle decompresses and extracts a gzipped tar stream into destDir.
// Every entry's path is resolved and checked to stay strictly inside destDir
// before anything is written: an entry using ".." or an absolute path is
// rejected, and only regular files and directories are accepted -- a symlink,
// hardlink, device file or anything else aborts the whole extraction. Total
// extracted bytes and entry count are capped. No file is written outside
// destDir under any input.
func ExtractBundle(r io.Reader, destDir string) error {
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
			totalBytes += hdr.Size
			if totalBytes > MaxBundleBytes {
				return fmt.Errorf("bundle exceeds %d bytes", MaxBundleBytes)
			}
			if err := writeBundleFile(target, tr); err != nil {
				return fmt.Errorf("write %q: %w", hdr.Name, err)
			}
		default:
			return fmt.Errorf("bundle entry %q: unsupported type %v (only regular files and directories are allowed)", hdr.Name, hdr.Typeflag)
		}
	}
	return nil
}

func writeBundleFile(target string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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
