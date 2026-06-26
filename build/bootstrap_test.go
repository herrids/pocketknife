package build

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"pocketknife/registry"
	"pocketknife/store"
)

const bootstrapManifest = `{
  "app": { "id": "freshapp", "name": "Fresh App", "version": 1 },
  "entities": [
    { "id": "ent_note", "name": "note", "fields": [
      { "id": "fld_title", "name": "title", "type": "text", "required": true }
    ]}
  ],
  "frontend": { "dist": "dist" }
}`

func TestBootstrapRegistersAndActivatesNewApp(t *testing.T) {
	appsDir := t.TempDir()
	reg := registry.New()
	bst := openTestStore(t)
	bundle := buildTarGz(t, []tarEntry{{name: "index.html", content: "fresh"}})

	res, err := Bootstrap(reg, bst, appsDir, []byte(bootstrapManifest), bytes.NewReader(bundle))
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if res.Job.State != StateReady {
		t.Fatalf("job state = %s, want ready", res.Job.State)
	}

	ra, ok := reg.App("freshapp")
	if !ok {
		t.Fatal("app should be registered")
	}
	if ra.AssetDir == "" {
		t.Fatal("app should be activated with a non-empty asset dir")
	}
	if got := readMarker(t, ra.AssetDir); got != "fresh" {
		t.Fatalf("served bundle = %q, want fresh", got)
	}

	if _, err := os.Stat(filepath.Join(appsDir, "freshapp", "manifest.json")); err != nil {
		t.Fatalf("manifest.json should exist on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(appsDir, "freshapp", "data.db")); err != nil {
		t.Fatalf("data.db should exist on disk: %v", err)
	}

	active, err := bst.ActiveBuildFor("freshapp")
	if err != nil {
		t.Fatal(err)
	}
	if active == nil || active.AssetDir != ra.AssetDir {
		t.Fatalf("active build pointer not durably recorded: %+v", active)
	}

	// A row exists and the API surface works against a freshly materialized db.
	now := store.NowUTC()
	row, err := ra.Store.Insert(ra.Schema.EntityByID("ent_note"), map[string]any{
		"id": store.NewID(), "created_at": now, "updated_at": now, "title": "hello",
	})
	if err != nil {
		t.Fatalf("insert into freshly materialized db: %v", err)
	}
	if row["title"] != "hello" {
		t.Fatalf("inserted row = %+v", row)
	}
}

func TestBootstrapFailureLeavesNoPartialApp(t *testing.T) {
	appsDir := t.TempDir()
	reg := registry.New()
	bst := openTestStore(t)
	// A bundle missing the manifest's declared entry file fails buildFrontend.
	bundle := buildTarGz(t, []tarEntry{{name: "other.html", content: "not the entry"}})

	res, err := Bootstrap(reg, bst, appsDir, []byte(bootstrapManifest), bytes.NewReader(bundle))
	if err == nil {
		t.Fatal("expected a build failure for a missing entry file")
	}
	if res.Job.State != StateFailed {
		t.Fatalf("job state = %s, want failed", res.Job.State)
	}

	if _, ok := reg.App("freshapp"); ok {
		t.Fatal("app must not be registered after a failed bootstrap")
	}
	if _, err := os.Stat(filepath.Join(appsDir, "freshapp")); !os.IsNotExist(err) {
		t.Fatal("no apps/freshapp directory should remain after a failed bootstrap")
	}
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging directory should have been removed, found: %v", entries)
	}
}

func TestBootstrapRefusesExistingAppDirectory(t *testing.T) {
	appsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(appsDir, "freshapp"), 0o755); err != nil {
		t.Fatal(err)
	}
	reg := registry.New()
	bst := openTestStore(t)
	bundle := buildTarGz(t, []tarEntry{{name: "index.html", content: "fresh"}})

	if _, err := Bootstrap(reg, bst, appsDir, []byte(bootstrapManifest), bytes.NewReader(bundle)); err == nil {
		t.Fatal("expected an error when apps/<app_id> already exists on disk")
	}
}

func TestBootstrapRejectsInvalidManifest(t *testing.T) {
	appsDir := t.TempDir()
	reg := registry.New()
	bst := openTestStore(t)

	_, err := Bootstrap(reg, bst, appsDir, []byte(`{"app":{}}`), bytes.NewReader(nil))
	if err == nil {
		t.Fatal("expected a validation error")
	}
	entries, _ := os.ReadDir(appsDir)
	if len(entries) != 0 {
		t.Fatalf("no staging directory should be created for an invalid manifest, found: %v", entries)
	}
}
