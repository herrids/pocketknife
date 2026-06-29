package build

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"pocketknife/schema"
)

// BuildsDirName is the per-app subdirectory holding every versioned build
// artifact. A build is never overwritten in place: each job gets its own new
// directory, named after the job id, so a half-finished copy can never be
// mistaken for a previous good build.
const BuildsDirName = "builds"

// buildFrontend validates that the manifest's declared pre-built bundle
// exists and contains its entry file, then copies it into a new, immutable,
// job-versioned artifact directory under appDir/builds/. Pocketknife does not
// bundle on-box in this phase: fe.Dist must already be a built static bundle
// (HTML/JS/CSS) sitting on disk next to the manifest.
func buildFrontend(appDir, jobID string, fe *schema.Frontend) (string, error) {
	distPath := filepath.Join(appDir, fe.Dist)
	info, err := os.Stat(distPath)
	if err != nil {
		return "", fmt.Errorf("frontend dist %q: %w", fe.Dist, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("frontend dist %q is not a directory", fe.Dist)
	}
	entryInfo, err := os.Stat(filepath.Join(distPath, fe.Entry))
	if err != nil || entryInfo.IsDir() {
		return "", fmt.Errorf("frontend entry %q not found in dist %q", fe.Entry, fe.Dist)
	}

	artifactDir := filepath.Join(appDir, BuildsDirName, jobID)
	if err := copyDir(distPath, artifactDir); err != nil {
		return "", fmt.Errorf("copy frontend assets: %w", err)
	}
	return artifactDir, nil
}

// copyDir recursively copies src's contents into dst, creating dst fresh.
// Every file is fsynced so a completed artifact directory survives a crash;
// a copy that fails partway never gets promoted, since activation only
// happens after copyDir returns successfully.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// pruneOldBuilds removes versioned build artifact directories under
// appDir/builds other than keepJobID, beyond the most recent keep-1 of them —
// it keeps a short tail of recent artifacts (useful while diagnosing a bad
// rollout) without growing the builds directory without bound.
func pruneOldBuilds(appDir, keepJobID string, retain int) error {
	dir := filepath.Join(appDir, BuildsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	type artifact struct {
		name    string
		modTime int64
	}
	var others []artifact
	for _, e := range entries {
		if !e.IsDir() || e.Name() == keepJobID {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		others = append(others, artifact{name: e.Name(), modTime: info.ModTime().UnixNano()})
	}
	// Job ids are random and carry no chronological order, so prune by
	// directory modification time (set once, when copyDir finishes writing it)
	// rather than by name.
	sort.Slice(others, func(i, j int) bool { return others[i].modTime < others[j].modTime })
	if len(others) <= retain {
		return nil
	}
	for _, a := range others[:len(others)-retain] {
		if err := os.RemoveAll(filepath.Join(dir, a.name)); err != nil {
			return err
		}
		// Retire the corresponding source artifact alongside the build so both
		// share exactly the same retention tail without a separate pass.
		_ = os.RemoveAll(filepath.Join(appDir, SourcesDirName, a.name))
	}
	return nil
}
