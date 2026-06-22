package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"pocketknife/registry"
	"pocketknife/store"

	"pgregory.net/rapid"
)

// propV1/propV2 fix one schema shape that exercises every "safe" change kind at
// once: a field rename (label->title), a widening type change (integer->real),
// an added field with a default (note), and a self-reference (parent, pointing
// at the same entity) left untouched across the migration. None of these need a
// confirm or a witness -- Classify rates every one of them ClassSafe.
const propV1 = `{
  "app": { "id": "proptest", "name": "PropTest", "version": 1 },
  "entities": [
    { "id": "ent_item", "name": "item", "fields": [
      { "id": "fld_label", "name": "label", "type": "text" },
      { "id": "fld_amount", "name": "amount", "type": "integer" },
      { "id": "fld_parent", "name": "parent", "type": "reference", "target": "ent_item", "onDelete": "set_null" }
    ]}
  ]
}`

const propV2 = `{
  "app": { "id": "proptest", "name": "PropTest", "version": 2 },
  "entities": [
    { "id": "ent_item", "name": "item", "fields": [
      { "id": "fld_label", "name": "title", "type": "text" },
      { "id": "fld_amount", "name": "amount", "type": "real" },
      { "id": "fld_parent", "name": "parent", "type": "reference", "target": "ent_item", "onDelete": "set_null" },
      { "id": "fld_note", "name": "note", "type": "text", "default": "unseen" }
    ]}
  ]
}`

// bootPropApp boots a fresh registry over a freshly written propV1 manifest in
// its own temp directory and returns the registry plus a cleanup the caller must
// run (rapid.Check calls its property function many times per test, so each
// draw needs its own isolated app directory rather than sharing *testing.T's
// single per-test TempDir).
func bootPropApp(rt *rapid.T) (*registry.Registry, func()) {
	dir, err := os.MkdirTemp("", "prop-preserve-*")
	if err != nil {
		rt.Fatalf("mkdir temp: %v", err)
	}
	cleanup := func() { os.RemoveAll(dir) }

	appDir := filepath.Join(dir, "proptest")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		cleanup()
		rt.Fatalf("mkdir app dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "manifest.json"), []byte(propV1), 0o644); err != nil {
		cleanup()
		rt.Fatalf("write manifest: %v", err)
	}
	reg, results, err := registry.Load(dir)
	if err != nil {
		cleanup()
		rt.Fatalf("boot: %v", err)
	}
	for _, r := range results {
		if !r.OK {
			cleanup()
			rt.Fatalf("app failed to load: errors=%v err=%v", r.Errors, r.Err)
		}
	}
	return reg, cleanup
}

// TestPropertyAdditiveWideningPreservesData fuzzes row count, text content
// (rapid's default string generator draws adversarial unicode, empty strings,
// etc.), the full int64 domain, and a chained/self-referential parent pointer,
// then runs the real Apply over propV1->propV2 -- a changeset Classify rates
// entirely ClassSafe (rename + widen + add-with-default), so it auto-applies
// with no confirm and no snapshot. The property is data preservation: every
// surviving field's value, read back after the migration, must equal what was
// written before it.
//
// This deliberately includes the full int64 range, which will surface (and is
// expected to surface) the same int64->float64 precision-loss defect already
// pinned by the Layer 1 shell suite's tracker fixture: SQLite's REAL column
// affinity converts an inserted integer via a standard int64->float64
// conversion, which is lossy for magnitudes beyond 2^53. Per the stress-suite
// rules this is left as a failing assertion -- a generative reconfirmation of a
// known real defect -- rather than weakened to exclude the lossy range.
func TestPropertyAdditiveWideningPreservesData(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		reg, cleanup := bootPropApp(rt)
		defer cleanup()
		defer reg.Close()

		ra, _ := reg.App("proptest")
		ent := ra.Schema.Entity("item")

		n := rapid.IntRange(1, 12).Draw(rt, "row_count")
		ids := make([]string, n)
		labels := make([]string, n)
		amounts := make([]int64, n)
		parents := make([]string, n) // "" means no parent

		for i := 0; i < n; i++ {
			id := store.NewID()
			labels[i] = rapid.String().Draw(rt, fmt.Sprintf("label_%d", i))
			amounts[i] = rapid.Int64().Draw(rt, fmt.Sprintf("amount_%d", i))

			// -1 = no parent; 0..i = a chain back to any row seen so far, including
			// a true self-reference when the draw equals i itself.
			parentIdx := rapid.IntRange(-1, i).Draw(rt, fmt.Sprintf("parent_idx_%d", i))
			values := map[string]any{
				"id": id, "created_at": store.NowUTC(), "updated_at": store.NowUTC(),
				"label": labels[i], "amount": amounts[i],
			}
			switch {
			case parentIdx == -1:
				parents[i] = ""
			case parentIdx == i:
				values["parent"] = id
				parents[i] = id
			default:
				values["parent"] = ids[parentIdx]
				parents[i] = ids[parentIdx]
			}

			if _, err := ra.Store.Insert(ent, values); err != nil {
				rt.Fatalf("seed insert %d: %v", i, err)
			}
			ids[i] = id
		}

		res, err := Apply(context.Background(), reg, "proptest", []byte(propV2), Options{})
		if err != nil {
			rt.Fatalf("safe additive/widening migration must auto-apply with no confirm: %v", err)
		}
		if res.NoChange {
			rt.Fatal("propV1 -> propV2 changes the schema; NoChange must be false")
		}
		if res.SnapshotPath != "" {
			rt.Fatalf("an all-safe changeset must take no snapshot, got %q", res.SnapshotPath)
		}

		ra2, _ := reg.App("proptest")
		ent2 := ra2.Schema.Entity("item")
		for i := 0; i < n; i++ {
			row, err := ra2.Store.GetByID(ent2, ids[i])
			if err != nil || row == nil {
				rt.Fatalf("row %d (id=%s) missing after a safe migration: err=%v", i, ids[i], err)
			}
			if row["title"] != labels[i] {
				rt.Fatalf("row %d: renamed field not preserved: got %q want %q", i, row["title"], labels[i])
			}
			gotAmount, ok := row["amount"].(float64)
			if !ok {
				rt.Fatalf("row %d: widened amount should read back as float64, got %T (%v)", i, row["amount"], row["amount"])
			}
			if gotAmount != float64(amounts[i]) || int64(gotAmount) != amounts[i] {
				rt.Fatalf("row %d: int64->real widening lost precision: wrote %d, read back %v "+
					"(round-tripped as int64 %d) -- known int64/float64 precision-loss defect",
					i, amounts[i], gotAmount, int64(gotAmount))
			}
			if parents[i] == "" {
				if row["parent"] != nil {
					rt.Fatalf("row %d: expected no parent, got %v", i, row["parent"])
				}
			} else if row["parent"] != parents[i] {
				rt.Fatalf("row %d: self/chained reference not preserved: got %v want %q", i, row["parent"], parents[i])
			}
			if row["note"] != "unseen" {
				rt.Fatalf("row %d: added field's default was not backfilled onto a pre-existing row: note = %v, want %q", i, row["note"], "unseen")
			}
		}
	})
}
