package build

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pocketknife/schema"
)

func TestBuildFrontendCopiesDistIntoVersionedArtifact(t *testing.T) {
	appDir := t.TempDir()
	distDir := filepath.Join(appDir, "frontend", "dist")
	if err := os.MkdirAll(filepath.Join(distDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html>v1</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}

	fe := &schema.Frontend{Dist: "frontend/dist", Entry: "index.html"}
	artifactDir, err := buildFrontend(appDir, "job1", fe)
	if err != nil {
		t.Fatalf("buildFrontend: %v", err)
	}
	if artifactDir != filepath.Join(appDir, BuildsDirName, "job1") {
		t.Fatalf("unexpected artifact dir: %s", artifactDir)
	}

	got, err := os.ReadFile(filepath.Join(artifactDir, "index.html"))
	if err != nil || string(got) != "<html>v1</html>" {
		t.Fatalf("entry not copied correctly: %v %q", err, got)
	}
	if _, err := os.Stat(filepath.Join(artifactDir, "assets", "app.js")); err != nil {
		t.Fatalf("nested asset not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err != nil {
		t.Fatal("source dist should be untouched by the copy")
	}
}

func TestBuildFrontendMissingDistFails(t *testing.T) {
	appDir := t.TempDir()
	fe := &schema.Frontend{Dist: "frontend/dist", Entry: "index.html"}
	if _, err := buildFrontend(appDir, "job1", fe); err == nil {
		t.Fatal("expected an error for a missing dist directory")
	}
}

func TestBuildFrontendMissingEntryFails(t *testing.T) {
	appDir := t.TempDir()
	distDir := filepath.Join(appDir, "frontend", "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fe := &schema.Frontend{Dist: "frontend/dist", Entry: "index.html"}
	if _, err := buildFrontend(appDir, "job1", fe); err == nil {
		t.Fatal("expected an error for a missing entry file")
	}
}

func TestPruneOldBuildsKeepsRetainMostRecentByModTime(t *testing.T) {
	appDir := t.TempDir()
	buildsDir := filepath.Join(appDir, BuildsDirName)
	var names []string
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("job%d", i)
		if err := os.MkdirAll(filepath.Join(buildsDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
		time.Sleep(10 * time.Millisecond) // distinct mtimes; job ids carry no order
	}
	keep := names[len(names)-1]

	if err := pruneOldBuilds(appDir, keep, 1); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(buildsDir)
	if err != nil {
		t.Fatal(err)
	}
	var remaining []string
	for _, e := range entries {
		remaining = append(remaining, e.Name())
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 directories remaining (keep + 1 retained), got %v", remaining)
	}
	if !contains(remaining, keep) {
		t.Fatalf("keepJobID must never be pruned: %v", remaining)
	}
	if !contains(remaining, names[2]) {
		t.Fatalf("the most recent non-kept build should survive: %v", remaining)
	}
	if contains(remaining, names[0]) || contains(remaining, names[1]) {
		t.Fatalf("oldest builds should have been pruned: %v", remaining)
	}
}

func TestPruneOldBuildsOnMissingDirIsNoop(t *testing.T) {
	appDir := t.TempDir()
	if err := pruneOldBuilds(appDir, "whatever", 5); err != nil {
		t.Fatalf("missing builds dir should not error: %v", err)
	}
}
