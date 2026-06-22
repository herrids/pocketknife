package build

import (
	"os"
	"path/filepath"
	"testing"

	"pocketknife/registry"
)

// openTestStore opens a fresh platform db in a temp dir, closed automatically
// at test cleanup.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	bst, err := Open(filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatalf("open platform db: %v", err)
	}
	t.Cleanup(func() { bst.Close() })
	return bst
}

// bootTestApp writes manifest to appsDir/<appID>/manifest.json and boots a
// fresh registry over appsDir — every app starts with AssetDir empty, exactly
// as registry.Load behaves on every real process start.
func bootTestApp(t *testing.T, appsDir, appID, manifest string) *registry.Registry {
	t.Helper()
	dir := filepath.Join(appsDir, appID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, results, err := registry.Load(appsDir)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	for _, r := range results {
		if !r.OK {
			t.Fatalf("app failed to load: %v %v", r.Errors, r.Err)
		}
	}
	return reg
}

// writeDist writes a minimal one-file static bundle at
// appsDir/<appID>/<distRel>/index.html containing marker, so a test can prove
// which version's bundle ended up activated.
func writeDist(t *testing.T, appsDir, appID, distRel, marker string) {
	t.Helper()
	dir := filepath.Join(appsDir, appID, distRel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readMarker(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read marker from %s: %v", dir, err)
	}
	return string(b)
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
