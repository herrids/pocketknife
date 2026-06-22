package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"pocketknife/registry"
	"pocketknife/store"
)

const notesV1 = `{
  "app": { "id": "notes", "name": "Notes", "version": 1 },
  "entities": [
    { "id": "ent_note", "name": "note", "fields": [
      { "id": "fld_title", "name": "title", "type": "text", "required": true }
    ]}
  ],
  "frontend": { "dist": "frontend/dist" }
}`

const notesV2 = `{
  "app": { "id": "notes", "name": "Notes", "version": 2 },
  "entities": [
    { "id": "ent_note", "name": "note", "fields": [
      { "id": "fld_title", "name": "title", "type": "text", "required": true },
      { "id": "fld_body",  "name": "body",  "type": "text" }
    ]}
  ],
  "frontend": { "dist": "frontend/dist" }
}`

const notesV3NoFrontend = `{
  "app": { "id": "notes", "name": "Notes", "version": 3 },
  "entities": [
    { "id": "ent_note", "name": "note", "fields": [
      { "id": "fld_title", "name": "title", "type": "text", "required": true }
    ]}
  ]
}`

func TestDeployInstallBuildsAndActivates(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootTestApp(t, appsDir, "notes", notesV1)
	writeDist(t, appsDir, "notes", "frontend/dist", "v1")
	bst := openTestStore(t)

	res, err := Deploy(context.Background(), reg, bst, "notes", []byte(notesV1), DeployOptions{})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if res.Job.State != StateReady {
		t.Fatalf("job state = %s, want ready", res.Job.State)
	}
	if res.Job.Kind != KindInstall {
		t.Fatalf("kind = %s, want install", res.Job.Kind)
	}

	ra, ok := reg.App("notes")
	if !ok || ra.AssetDir == "" {
		t.Fatal("app should be activated with a non-empty asset dir")
	}
	if got := readMarker(t, ra.AssetDir); got != "v1" {
		t.Fatalf("served bundle = %q, want v1", got)
	}

	active, err := bst.ActiveBuildFor("notes")
	if err != nil {
		t.Fatal(err)
	}
	if active == nil || active.AssetDir != ra.AssetDir || active.ManifestVersion != 1 {
		t.Fatalf("active build pointer not durably recorded: %+v", active)
	}
}

func TestDeployInstallFailureIsLegibleAndRetriable(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootTestApp(t, appsDir, "notes", notesV1)
	// Deliberately do not write the dist directory the manifest declares.
	bst := openTestStore(t)

	res, err := Deploy(context.Background(), reg, bst, "notes", []byte(notesV1), DeployOptions{})
	if err == nil {
		t.Fatal("expected a build failure for a missing frontend dist")
	}
	if res.Job.State != StateFailed {
		t.Fatalf("job state = %s, want failed", res.Job.State)
	}
	if res.Job.Error == "" {
		t.Fatal("a failed job must carry a diagnosable error message")
	}
	ra, _ := reg.App("notes")
	if ra.AssetDir != "" {
		t.Fatal("app must not be activated on a failed build")
	}

	// Fix the dist and retry: the retry must succeed and must create a brand
	// new job rather than reopening the failed one.
	writeDist(t, appsDir, "notes", "frontend/dist", "v1")
	res2, err := Deploy(context.Background(), reg, bst, "notes", []byte(notesV1), DeployOptions{})
	if err != nil {
		t.Fatalf("retry deploy: %v", err)
	}
	if res2.Job.ID == res.Job.ID {
		t.Fatal("a retry must create a new job, not reopen the failed one")
	}
	if res2.Job.State != StateReady {
		t.Fatalf("retry job state = %s, want ready", res2.Job.State)
	}
}

func TestDeploySecondDeployMigratesAndSwapsFrontendWithoutDarkout(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootTestApp(t, appsDir, "notes", notesV1)
	writeDist(t, appsDir, "notes", "frontend/dist", "v1")
	bst := openTestStore(t)

	if _, err := Deploy(context.Background(), reg, bst, "notes", []byte(notesV1), DeployOptions{}); err != nil {
		t.Fatalf("initial install: %v", err)
	}
	ra1, _ := reg.App("notes")
	oldAssetDir := ra1.AssetDir

	id := insertNote(t, ra1, "hello")

	// New version, new frontend content at the same dist path.
	writeDist(t, appsDir, "notes", "frontend/dist", "v2")
	res, err := Deploy(context.Background(), reg, bst, "notes", []byte(notesV2), DeployOptions{})
	if err != nil {
		t.Fatalf("second deploy: %v", err)
	}
	if res.Job.Kind != KindDeploy {
		t.Fatalf("kind = %s, want deploy", res.Job.Kind)
	}
	if res.MigrateResult == nil || res.MigrateResult.Changeset == nil {
		t.Fatal("a second deploy must run the data migration")
	}

	ra2, _ := reg.App("notes")
	if ra2.Schema.Version != 2 {
		t.Fatalf("schema version = %d, want 2", ra2.Schema.Version)
	}
	if ra2.AssetDir == oldAssetDir {
		t.Fatal("activation must cut over to a new artifact directory")
	}
	if got := readMarker(t, ra2.AssetDir); got != "v2" {
		t.Fatalf("served bundle = %q, want v2", got)
	}
	// The old artifact is untouched, proving cutover never mutates the
	// previously-served bundle in place.
	if got := readMarker(t, oldAssetDir); got != "v1" {
		t.Fatalf("old artifact changed after cutover: got %q", got)
	}

	row, err := ra2.Store.GetByID(ra2.Schema.Entity("note"), id)
	if err != nil {
		t.Fatal(err)
	}
	if row == nil || row["title"] != "hello" {
		t.Fatalf("data did not survive the migration: %v", row)
	}
}

func TestDeploySecondDeployRollsBackOnInjectedFrontendFailure(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootTestApp(t, appsDir, "notes", notesV1)
	writeDist(t, appsDir, "notes", "frontend/dist", "v1")
	bst := openTestStore(t)

	if _, err := Deploy(context.Background(), reg, bst, "notes", []byte(notesV1), DeployOptions{}); err != nil {
		t.Fatalf("initial install: %v", err)
	}
	ra1, _ := reg.App("notes")
	oldAssetDir := ra1.AssetDir
	id := insertNote(t, ra1, "hello")

	// Inject a frontend build failure for the second deploy: remove the dist
	// directory the new manifest still declares, so the migration runs and
	// commits but the subsequent frontend build fails.
	if err := os.RemoveAll(filepath.Join(appsDir, "notes", "frontend", "dist")); err != nil {
		t.Fatal(err)
	}

	res, err := Deploy(context.Background(), reg, bst, "notes", []byte(notesV2), DeployOptions{})
	if err == nil {
		t.Fatal("expected the deploy to fail")
	}
	if res.Job.State != StateFailed {
		t.Fatalf("job state = %s, want failed", res.Job.State)
	}

	ra2, _ := reg.App("notes")
	if ra2.Schema.Version != 1 {
		t.Fatalf("schema version = %d, want rolled back to 1", ra2.Schema.Version)
	}
	if ra2.AssetDir != oldAssetDir {
		t.Fatalf("asset dir = %q, want rolled back to %q", ra2.AssetDir, oldAssetDir)
	}
	if got := readMarker(t, ra2.AssetDir); got != "v1" {
		t.Fatalf("served bundle after rollback = %q, want v1", got)
	}

	row, err := ra2.Store.GetByID(ra2.Schema.Entity("note"), id)
	if err != nil {
		t.Fatal(err)
	}
	if row == nil || row["title"] != "hello" {
		t.Fatalf("data not preserved across rollback: %v", row)
	}

	manifestOnDisk, err := os.ReadFile(filepath.Join(appsDir, "notes", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(manifestOnDisk) != notesV1 {
		t.Fatal("manifest.json on disk must be restored to the prior good version")
	}

	// The app must remain cleanly retriable: fix the dist and redeploy.
	writeDist(t, appsDir, "notes", "frontend/dist", "v2")
	res2, err := Deploy(context.Background(), reg, bst, "notes", []byte(notesV2), DeployOptions{})
	if err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
	if res2.Job.State != StateReady {
		t.Fatalf("retry job state = %s, want ready", res2.Job.State)
	}
}

func TestDeployIntentionalFrontendRemovalBecomesAPIOnly(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootTestApp(t, appsDir, "notes", notesV1)
	writeDist(t, appsDir, "notes", "frontend/dist", "v1")
	bst := openTestStore(t)

	if _, err := Deploy(context.Background(), reg, bst, "notes", []byte(notesV1), DeployOptions{}); err != nil {
		t.Fatalf("initial install: %v", err)
	}

	res, err := Deploy(context.Background(), reg, bst, "notes", []byte(notesV3NoFrontend), DeployOptions{})
	if err != nil {
		t.Fatalf("deploy dropping the frontend: %v", err)
	}
	if res.Job.State != StateReady {
		t.Fatalf("job state = %s, want ready", res.Job.State)
	}

	ra, _ := reg.App("notes")
	if ra.AssetDir != "" {
		t.Fatalf("dropping the frontend block must leave the app API-only, got AssetDir %q", ra.AssetDir)
	}
}

// insertNote inserts a row directly through the store, bypassing the HTTP API
// (which lives in a separate, frozen-contract package this one must not
// depend on).
func insertNote(t *testing.T, ra *registry.RegisteredApp, title string) string {
	t.Helper()
	now := store.NowUTC()
	row, err := ra.Store.Insert(ra.Schema.Entity("note"), map[string]any{
		"id":         store.NewID(),
		"created_at": now,
		"updated_at": now,
		"title":      title,
	})
	if err != nil {
		t.Fatalf("insert note: %v", err)
	}
	return row["id"].(string)
}
