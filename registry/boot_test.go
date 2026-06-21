package registry_test

import (
	"os"
	"path/filepath"
	"testing"

	"pocketknife/registry"
	"pocketknife/store"
)

func writeManifest(t *testing.T, root, appID, body string) {
	t.Helper()
	dir := filepath.Join(root, appID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const readingManifest = `{
  "app": { "id": "reading_tracker", "name": "Reading Tracker", "version": 1 },
  "entities": [
    { "id": "ent_book", "name": "book", "fields": [
      { "id": "fld_title", "name": "title", "type": "text", "required": true, "max": 200 }
    ]}
  ]
}`

const tasksManifest = `{
  "app": { "id": "tasks", "name": "Tasks", "version": 1 },
  "entities": [
    { "id": "ent_project", "name": "project", "fields": [
      { "id": "fld_name", "name": "name", "type": "text", "required": true, "unique": true }
    ]}
  ]
}`

// insertRow is a tiny direct-store helper for the runtime tests.
func insertRow(t *testing.T, ra *registry.RegisteredApp, entity string, values map[string]any) string {
	t.Helper()
	ent := ra.Schema.Entity(entity)
	values["id"] = store.NewID()
	now := store.NowUTC()
	values["created_at"] = now
	values["updated_at"] = now
	row, err := ra.Store.Insert(ent, values)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return row["id"].(string)
}

func TestRestartPersistsDataAndRederivesRegistry(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "reading_tracker", readingManifest)
	writeManifest(t, root, "tasks", tasksManifest)

	// First boot: register and write a row.
	reg, results, err := registry.Load(root)
	if err != nil {
		t.Fatalf("first boot: %v", err)
	}
	for _, r := range results {
		if !r.OK {
			t.Fatalf("app %s did not load: %v %v", r.ManifestPath, r.Errors, r.Err)
		}
	}
	ra, ok := reg.App("reading_tracker")
	if !ok {
		t.Fatal("reading_tracker not registered")
	}
	bookID := insertRow(t, ra, "book", map[string]any{"title": "Persisted"})
	reg.Close()

	// Simulated restart: delete the in-memory registry entirely and re-derive
	// from the same on-disk manifests + data.db files.
	reg2, _, err := registry.Load(root)
	if err != nil {
		t.Fatalf("second boot: %v", err)
	}
	defer reg2.Close()

	if _, ok := reg2.App("reading_tracker"); !ok {
		t.Fatal("registry did not re-derive reading_tracker from disk")
	}
	if _, ok := reg2.App("tasks"); !ok {
		t.Fatal("registry did not re-derive tasks from disk")
	}

	ra2, _ := reg2.App("reading_tracker")
	row, err := ra2.Store.GetByID(ra2.Schema.Entity("book"), bookID)
	if err != nil {
		t.Fatalf("get after restart: %v", err)
	}
	if row == nil || row["title"] != "Persisted" {
		t.Fatalf("data did not persist across restart: %v", row)
	}
}

func TestAppsHavePhysicallySeparateDatabases(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "reading_tracker", readingManifest)
	writeManifest(t, root, "tasks", tasksManifest)

	reg, _, err := registry.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	reading, _ := reg.App("reading_tracker")
	tasks, _ := reg.App("tasks")

	if reading.Store.Path() == tasks.Store.Path() {
		t.Fatal("apps share a database file")
	}
	for _, p := range []string{reading.Store.Path(), tasks.Store.Path()} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected db file %s to exist: %v", p, err)
		}
	}
	if filepath.Dir(reading.Store.Path()) == filepath.Dir(tasks.Store.Path()) {
		t.Fatal("apps' databases live in the same directory")
	}
}

func TestBootIsIdempotentOnUnchangedManifests(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "reading_tracker", readingManifest)

	reg, _, err := registry.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	ra, _ := reg.App("reading_tracker")
	bookID := insertRow(t, ra, "book", map[string]any{"title": "Once"})
	reg.Close()

	// Re-boot on the unchanged manifest: must not error and must not disturb data.
	reg2, results, err := registry.Load(root)
	if err != nil {
		t.Fatalf("re-boot: %v", err)
	}
	for _, r := range results {
		if !r.OK {
			t.Fatalf("re-boot app %s failed: %v %v", r.ManifestPath, r.Errors, r.Err)
		}
	}
	defer reg2.Close()

	ra2, _ := reg2.App("reading_tracker")
	rows, total, err := ra2.Store.List(ra2.Schema.Entity("book"), store.ListQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 || rows[0]["id"] != bookID {
		t.Fatalf("idempotent re-boot changed data: total=%d rows=%v", total, rows)
	}
}

func TestInvalidManifestIsSkippedNotServed(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "good", readingManifest)
	// declares a reserved field name -> must fail validation and be skipped.
	writeManifest(t, root, "bad", `{
      "app": { "id": "bad", "name": "Bad", "version": 1 },
      "entities": [ { "id": "ent_x", "name": "x", "fields": [
        { "id": "fld_id", "name": "id", "type": "text" }
      ]}]
    }`)

	reg, results, err := registry.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	var badResult *registry.LoadResult
	for i := range results {
		if filepath.Base(filepath.Dir(results[i].ManifestPath)) == "bad" {
			badResult = &results[i]
		}
	}
	if badResult == nil || badResult.OK {
		t.Fatal("invalid manifest was not reported as failed")
	}
	if len(badResult.Errors) == 0 {
		t.Fatal("expected structured validation errors for invalid manifest")
	}
	if _, ok := reg.App("bad"); ok {
		t.Fatal("invalid app must never be registered/served")
	}
	if _, ok := reg.App("reading_tracker"); !ok {
		t.Fatal("a sibling invalid manifest must not stop a valid one")
	}
}
