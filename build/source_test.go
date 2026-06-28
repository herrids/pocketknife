package build

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSourceWritesUnderJobID(t *testing.T) {
	appDir := t.TempDir()
	archive := buildTarGz(t, []tarEntry{
		{name: "src/", content: ""},
		{name: "src/App.tsx", content: "export default function App() {}"},
		{name: "package.json", content: `{"name":"app"}`},
	})

	if err := StoreSource(appDir, "job-1", bytes.NewReader(archive)); err != nil {
		t.Fatalf("StoreSource: %v", err)
	}

	want := filepath.Join(appDir, SourcesDirName, "job-1", "src", "App.tsx")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected stored file %s: %v", want, err)
	}
}

func TestStoreSourceDoesNotOverwritePriorJob(t *testing.T) {
	appDir := t.TempDir()
	first := buildTarGz(t, []tarEntry{{name: "src/App.tsx", content: "v1"}})
	second := buildTarGz(t, []tarEntry{{name: "src/App.tsx", content: "v2"}})

	if err := StoreSource(appDir, "job-1", bytes.NewReader(first)); err != nil {
		t.Fatal(err)
	}
	if err := StoreSource(appDir, "job-2", bytes.NewReader(second)); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(appDir, SourcesDirName, "job-1", "src", "App.tsx"))
	if err != nil || string(got) != "v1" {
		t.Fatalf("job-1 source = %q, %v; want v1", got, err)
	}
}

func TestStoreSourceRejectsTraversal(t *testing.T) {
	appDir := t.TempDir()
	evil := buildTarGz(t, []tarEntry{{name: "../evil.txt", content: "bad"}})

	if err := StoreSource(appDir, "job-1", bytes.NewReader(evil)); err == nil {
		t.Fatal("expected an error for a path-traversal entry")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(appDir), "evil.txt")); !os.IsNotExist(err) {
		t.Fatal("traversal must not have been written outside appDir")
	}
}

func TestSourceDirExistsOnlyAfterStore(t *testing.T) {
	appDir := t.TempDir()

	_, exists := SourceDir(appDir, "job-1")
	if exists {
		t.Fatal("source should not exist before StoreSource is called")
	}

	archive := buildTarGz(t, []tarEntry{{name: "src/main.ts", content: "x"}})
	if err := StoreSource(appDir, "job-1", bytes.NewReader(archive)); err != nil {
		t.Fatal(err)
	}

	dir, exists := SourceDir(appDir, "job-1")
	if !exists {
		t.Fatal("source should exist after StoreSource")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("SourceDir returned %q which does not stat: %v", dir, err)
	}
}

func TestPruneRetiresBuildAndSourceTogether(t *testing.T) {
	appDir := t.TempDir()
	archive := func() []byte { return buildTarGz(t, []tarEntry{{name: "src/x.ts", content: "x"}}) }

	// Store source and build dirs for three jobs.
	for _, jid := range []string{"job-a", "job-b", "job-c"} {
		if err := StoreSource(appDir, jid, bytes.NewReader(archive())); err != nil {
			t.Fatal(err)
		}
		buildsDir := filepath.Join(appDir, BuildsDirName, jid)
		if err := os.MkdirAll(buildsDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Prune keeping only 1 old build beyond the keep job (job-c).
	if err := pruneOldBuilds(appDir, "job-c", 1); err != nil {
		t.Fatalf("pruneOldBuilds: %v", err)
	}

	// job-a (oldest) should have been pruned — both build and source.
	if _, err := os.Stat(filepath.Join(appDir, BuildsDirName, "job-a")); !os.IsNotExist(err) {
		t.Fatal("job-a build dir should have been pruned")
	}
	if _, exists := SourceDir(appDir, "job-a"); exists {
		t.Fatal("job-a source dir should have been pruned alongside its build")
	}

	// job-b (retained tail) and job-c (keepJobID) should remain.
	if _, exists := SourceDir(appDir, "job-b"); !exists {
		t.Fatal("job-b source dir should be retained")
	}
	if _, exists := SourceDir(appDir, "job-c"); !exists {
		t.Fatal("job-c source dir should be retained (keepJobID)")
	}
}

func TestPackSourceRoundTrips(t *testing.T) {
	// Write a source tree to a temp dir, pack it, extract to another dir,
	// and verify the files are identical.
	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "src", "App.tsx"), []byte("export default function App() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "package.json"), []byte(`{"name":"app"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	packed, err := PackSource(srcDir)
	if err != nil {
		t.Fatalf("PackSource: %v", err)
	}

	destDir := t.TempDir()
	if err := ExtractBundle(bytes.NewReader(packed), destDir); err != nil {
		t.Fatalf("ExtractBundle of packed source: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "src", "App.tsx"))
	if err != nil || string(got) != "export default function App() {}" {
		t.Fatalf("round-tripped App.tsx = %q, %v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(destDir, "package.json"))
	if err != nil || string(got) != `{"name":"app"}` {
		t.Fatalf("round-tripped package.json = %q, %v", got, err)
	}
}
