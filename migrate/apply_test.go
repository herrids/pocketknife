package migrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"pocketknife/registry"
	"pocketknife/store"
)

// setupReg writes a manifest to a temp apps dir and boots a registry over it.
func setupReg(t *testing.T, appID, manifest string) (*registry.Registry, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, appID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, results, err := registry.Load(root)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	for _, r := range results {
		if !r.OK {
			t.Fatalf("app failed to load: %v %v", r.Errors, r.Err)
		}
	}
	t.Cleanup(func() { reg.Close() })
	return reg, dir
}

func seedReg(t *testing.T, reg *registry.Registry, appID, entity string, values map[string]any) string {
	t.Helper()
	ra, _ := reg.App(appID)
	return seed(t, ra.Store, ra.Schema.Entity(entity), values)
}

const applyV1 = `{
  "app": { "id": "tracker", "name": "Tracker", "version": 1 },
  "entities": [
    { "id": "ent_item", "name": "item", "fields": [
      { "id": "fld_title", "name": "title", "type": "text", "required": true },
      { "id": "fld_count", "name": "count", "type": "integer" }
    ]}
  ]
}`

func TestApplySafeAutoApplies(t *testing.T) {
	reg, dir := setupReg(t, "tracker", applyV1)
	id := seedReg(t, reg, "tracker", "item", map[string]any{"title": "keep", "count": int64(1)})

	const v2 = `{
      "app": { "id": "tracker", "name": "Tracker", "version": 2 },
      "entities": [
        { "id": "ent_item", "name": "item", "fields": [
          { "id": "fld_title", "name": "title", "type": "text", "required": true },
          { "id": "fld_count", "name": "count", "type": "integer" },
          { "id": "fld_note", "name": "note", "type": "text" }
        ]}
      ]
    }`
	// No confirmation needed for a purely safe migration.
	res, err := Apply(context.Background(), reg, "tracker", []byte(v2), Options{})
	if err != nil {
		t.Fatalf("safe apply: %v", err)
	}
	if res.NoChange || res.SnapshotPath != "" {
		t.Fatalf("safe migration should change schema and take no snapshot: %+v", res)
	}

	// The registry now serves the new schema, and data is intact.
	ra, _ := reg.App("tracker")
	if ra.Schema.Version != 2 || ra.Schema.Entity("item").Field("note") == nil {
		t.Fatal("registry not updated to the new schema")
	}
	row, _ := ra.Store.GetByID(ra.Schema.Entity("item"), id)
	if row["title"] != "keep" || row["note"] != nil {
		t.Fatalf("data not preserved through safe apply: %v", row)
	}
	// manifest.json was promoted on disk.
	onDisk, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if string(onDisk) != v2 {
		t.Fatal("manifest.json was not promoted to the new version")
	}
}

func TestApplyDestructiveRefusedWithoutConfirm(t *testing.T) {
	reg, dir := setupReg(t, "tracker", applyV1)
	id := seedReg(t, reg, "tracker", "item", map[string]any{"title": "keep", "count": int64(7)})

	const v2 = `{
      "app": { "id": "tracker", "name": "Tracker", "version": 2 },
      "entities": [
        { "id": "ent_item", "name": "item", "fields": [
          { "id": "fld_title", "name": "title", "type": "text", "required": true }
        ]}
      ]
    }`
	res, err := Apply(context.Background(), reg, "tracker", []byte(v2), Options{Confirm: false})
	if err == nil {
		t.Fatal("dropping a field without confirmation must be refused")
	}
	// Nothing changed: schema, data, and manifest are untouched.
	ra, _ := reg.App("tracker")
	if ra.Schema.Version != 1 || ra.Schema.Entity("item").Field("count") == nil {
		t.Fatal("refused migration must leave the prior schema registered")
	}
	row, _ := ra.Store.GetByID(ra.Schema.Entity("item"), id)
	if row == nil || row["count"] == nil {
		t.Fatalf("refused migration must not touch data: %v", row)
	}
	if onDisk, _ := os.ReadFile(filepath.Join(dir, "manifest.json")); string(onDisk) != applyV1 {
		t.Fatal("refused migration must not promote the manifest")
	}
	_ = res
}

func TestApplyDestructiveWithConfirmTakesSnapshot(t *testing.T) {
	reg, _ := setupReg(t, "tracker", applyV1)
	id := seedReg(t, reg, "tracker", "item", map[string]any{"title": "keep", "count": int64(7)})

	const v2 = `{
      "app": { "id": "tracker", "name": "Tracker", "version": 2 },
      "entities": [
        { "id": "ent_item", "name": "item", "fields": [
          { "id": "fld_title", "name": "title", "type": "text", "required": true }
        ]}
      ]
    }`
	res, err := Apply(context.Background(), reg, "tracker", []byte(v2), Options{Confirm: true})
	if err != nil {
		t.Fatalf("confirmed destructive apply: %v", err)
	}
	if res.SnapshotPath == "" {
		t.Fatal("a destructive migration must take a snapshot")
	}
	if _, err := os.Stat(res.SnapshotPath); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}
	ra, _ := reg.App("tracker")
	if ra.Schema.Entity("item").Field("count") != nil {
		t.Fatal("dropped field still present after confirmed migration")
	}
	row, _ := ra.Store.GetByID(ra.Schema.Entity("item"), id)
	if row["title"] != "keep" {
		t.Fatalf("surviving data lost: %v", row)
	}
}

// TestApplyRestoresOnExecutionFailure forces a failure that passes pre-flight but
// fails during execution (adding a uniqueness constraint over duplicate rows) and
// proves the snapshot restores the data and the prior schema stays registered.
func TestApplyRestoresOnExecutionFailure(t *testing.T) {
	const v1 = `{
      "app": { "id": "tracker", "name": "Tracker", "version": 1 },
      "entities": [
        { "id": "ent_item", "name": "item", "fields": [
          { "id": "fld_code", "name": "code", "type": "text", "required": true }
        ]}
      ]
    }`
	reg, dir := setupReg(t, "tracker", v1)
	// Two rows sharing a code: a later UNIQUE index cannot be built.
	seedReg(t, reg, "tracker", "item", map[string]any{"code": "dup"})
	seedReg(t, reg, "tracker", "item", map[string]any{"code": "dup"})

	const v2 = `{
      "app": { "id": "tracker", "name": "Tracker", "version": 2 },
      "entities": [
        { "id": "ent_item", "name": "item", "fields": [
          { "id": "fld_code", "name": "code", "type": "text", "required": true, "unique": true }
        ]}
      ]
    }`
	res, err := Apply(context.Background(), reg, "tracker", []byte(v2), Options{Confirm: true})
	if err == nil {
		t.Fatal("adding a unique constraint over duplicates must fail")
	}
	_ = res

	// Prior schema is still registered (no uniqueness), and both rows survive.
	ra, _ := reg.App("tracker")
	if ra.Schema.Version != 1 || ra.Schema.Entity("item").Field("code").Unique {
		t.Fatal("failed migration must keep the prior (non-unique) schema")
	}
	_, total, err := ra.Store.List(ra.Schema.Entity("item"), store.ListQuery{Limit: 100})
	if err != nil {
		t.Fatalf("list after restore: %v", err)
	}
	if total != 2 {
		t.Fatalf("restore lost data: %d rows, want 2", total)
	}
	if onDisk, _ := os.ReadFile(filepath.Join(dir, "manifest.json")); string(onDisk) != v1 {
		t.Fatal("failed migration must not promote the manifest")
	}
}

func TestApplyNoChange(t *testing.T) {
	reg, _ := setupReg(t, "tracker", applyV1)
	res, err := Apply(context.Background(), reg, "tracker", []byte(applyV1), Options{})
	if err != nil {
		t.Fatalf("no-op apply: %v", err)
	}
	if !res.NoChange {
		t.Fatal("identical manifest should report NoChange")
	}
}

// TestApplyEmptyDiffStillPromotesNewVersion covers a version bump whose only
// change is outside the entity schema (here: adding a prompt function). Diff
// is empty, so no data migration runs — but the new manifest must still be
// promoted on disk and re-registered, or the function is silently dropped and
// lost on restart.
func TestApplyEmptyDiffStillPromotesNewVersion(t *testing.T) {
	const v2WithFunction = `{
      "app": { "id": "tracker", "name": "Tracker", "version": 2 },
      "entities": [
        { "id": "ent_item", "name": "item", "fields": [
          { "id": "fld_title", "name": "title", "type": "text", "required": true },
          { "id": "fld_count", "name": "count", "type": "integer" }
        ]}
      ],
      "functions": [
        {
          "id": "fn_summarize",
          "name": "summarize",
          "prompt": "Summarize: {{text}}",
          "capabilities": { "model": true }
        }
      ]
    }`

	reg, dir := setupReg(t, "tracker", applyV1)
	id := seedReg(t, reg, "tracker", "item", map[string]any{"title": "keep", "count": int64(1)})

	res, err := Apply(context.Background(), reg, "tracker", []byte(v2WithFunction), Options{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.NoChange {
		t.Fatal("an entity-identical manifest should still report NoChange (no data moved)")
	}

	// The registry serves the new schema, including the function.
	ra, _ := reg.App("tracker")
	if ra.Schema.Version != 2 {
		t.Errorf("registered version = %d, want 2", ra.Schema.Version)
	}
	if ra.Schema.Function("summarize") == nil {
		t.Error("prompt function missing from the re-registered schema")
	}

	// The on-disk manifest (source of truth for the next boot) was promoted.
	onDisk, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != v2WithFunction {
		t.Error("manifest.json was not promoted to the new version")
	}

	// Data is untouched and the store handle still works.
	row, err := ra.Store.GetByID(ra.Schema.Entity("item"), id)
	if err != nil || row == nil {
		t.Fatalf("seeded row lost after promotion: %v", err)
	}
}
