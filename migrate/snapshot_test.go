package migrate

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"pocketknife/materialize"
	"pocketknife/schema"
	"pocketknife/store"
	"pocketknife/validate"
)

const noteManifest = `{
  "app": { "id": "notes", "name": "Notes", "version": 1 },
  "entities": [
    { "id": "ent_note", "name": "note", "fields": [
      { "id": "fld_body", "name": "body", "type": "text", "required": true }
    ]}
  ]
}`

// openNotes opens a notes store at path, materializing its schema.
func openNotes(t *testing.T, path string) (*store.Store, *schema.Entity) {
	t.Helper()
	app, errs := validate.Manifest([]byte(noteManifest))
	if len(errs) > 0 {
		t.Fatalf("manifest invalid: %v", errs)
	}
	stmts, err := materialize.Statements(app)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.ApplyDDL(stmts); err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return st, app.Entity("note")
}

func addNote(t *testing.T, st *store.Store, ent *schema.Entity, body string) {
	t.Helper()
	now := store.NowUTC()
	_, err := st.Insert(ent, map[string]any{
		"id": store.NewID(), "created_at": now, "updated_at": now, "body": body,
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func countNotes(t *testing.T, st *store.Store, ent *schema.Entity) int {
	t.Helper()
	_, total, err := st.List(ent, store.ListQuery{Limit: 100})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return total
}

// TestSnapshotRestoreUnderWAL proves a snapshot taken under WAL restores
// byte-for-byte and recovers the exact data of the snapshot moment.
func TestSnapshotRestoreUnderWAL(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	snapDir := filepath.Join(dir, SnapshotDirName)

	st, ent := openNotes(t, dbPath)
	addNote(t, st, ent, "one")
	addNote(t, st, ent, "two")

	// WAL must actually be active: writes go to the -wal sidecar.
	if _, err := os.Stat(dbPath + "-wal"); err != nil {
		t.Fatalf("expected a -wal sidecar under WAL mode: %v", err)
	}

	// Snapshot the two-note state.
	snapPath, err := Snapshot(st, snapDir)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	snapBytes, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	// Mutate after the snapshot: add a third note, then close so the WAL is
	// folded into the main file.
	addNote(t, st, ent, "three")
	if countNotes(t, st, ent) != 3 {
		t.Fatal("expected 3 notes before restore")
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Restore the snapshot over the live database.
	if err := Restore(snapPath, dbPath); err != nil {
		t.Fatalf("restore: %v", err)
	}

	// The restored file is byte-identical to the snapshot.
	restored, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if !bytes.Equal(snapBytes, restored) {
		t.Fatalf("restore is not byte-exact: snapshot %d bytes, restored %d bytes", len(snapBytes), len(restored))
	}

	// And the data is exactly the snapshot moment: two notes, no "three".
	st2, ent2 := openNotes(t, dbPath)
	defer st2.Close()
	if n := countNotes(t, st2, ent2); n != 2 {
		t.Fatalf("restored note count = %d, want 2 (post-snapshot insert must be gone)", n)
	}
	rows, _, _ := st2.List(ent2, store.ListQuery{Limit: 100})
	for _, r := range rows {
		if r["body"] == "three" {
			t.Fatal("post-snapshot note survived a restore")
		}
	}
}

// refSnapManifest gives the restore/reopen FK test a reference field to probe;
// the notes schema above has none.
const refSnapManifest = `{
  "app": { "id": "refsnap", "name": "RefSnap", "version": 1 },
  "entities": [
    { "id": "ent_parent", "name": "parent", "fields": [
      { "id": "fld_label", "name": "label", "type": "text" }
    ]},
    { "id": "ent_child", "name": "child", "fields": [
      { "id": "fld_ref", "name": "ref", "type": "reference", "target": "ent_parent", "onDelete": "restrict" }
    ]}
  ]
}`

// TestForeignKeysPragmaEnabledAfterRestore extends store.TestForeignKeysPragmaEnabled
// (which only covers a fresh store.Open) across the exact close/restore/reopen
// cycle restoreInPlace performs in apply.go: a destructive migration's failure
// path closes the live store, restores a snapshot file over it, and reopens via
// store.Open. Store keeps its *sql.DB private, so this proves enforcement through
// the public surface instead of a raw PRAGMA query: a dangling reference must
// still be rejected (and a valid one still accepted) on the reopened connection.
func TestForeignKeysPragmaEnabledAfterRestore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	snapDir := filepath.Join(dir, SnapshotDirName)

	st, app := openApp(t, dir, refSnapManifest)
	parentID := seed(t, st, app.Entity("parent"), map[string]any{"label": "p"})

	snapPath, err := Snapshot(st, snapDir)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := Restore(snapPath, dbPath); err != nil {
		t.Fatalf("restore: %v", err)
	}

	st2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen after restore: %v", err)
	}
	defer st2.Close()

	// A valid reference is still accepted.
	if _, err := st2.Insert(app.Entity("child"), map[string]any{
		"id": store.NewID(), "created_at": store.NowUTC(), "updated_at": store.NowUTC(),
		"ref": parentID,
	}); err != nil {
		t.Fatalf("insert with a valid reference after restore+reopen: %v", err)
	}
	// A dangling reference must still be rejected. If the FK pragma were somehow
	// not in effect on the reopened connection, this insert would silently
	// succeed instead of returning store.ErrForeignKey.
	_, err = st2.Insert(app.Entity("child"), map[string]any{
		"id": store.NewID(), "created_at": store.NowUTC(), "updated_at": store.NowUTC(),
		"ref": "does-not-exist",
	})
	if !errors.Is(err, store.ErrForeignKey) {
		t.Fatalf("insert with a dangling reference after restore+reopen: err = %v, want store.ErrForeignKey (FK enforcement must survive the restore cycle)", err)
	}
}

func TestPruneKeepsLastN(t *testing.T) {
	dir := t.TempDir()
	// Create six snapshot-named files with sortable timestamps.
	names := []string{
		"data-20260101T000000.000000000Z.db",
		"data-20260102T000000.000000000Z.db",
		"data-20260103T000000.000000000Z.db",
		"data-20260104T000000.000000000Z.db",
		"data-20260105T000000.000000000Z.db",
		"data-20260106T000000.000000000Z.db",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A non-snapshot file must be left untouched.
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Prune(dir, 2); err != nil {
		t.Fatalf("prune: %v", err)
	}
	remaining, err := listSnapshots(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{names[4], names[5]} // the two most recent
	if len(remaining) != 2 || remaining[0] != want[0] || remaining[1] != want[1] {
		t.Fatalf("prune kept %v, want %v", remaining, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatalf("prune removed a non-snapshot file: %v", err)
	}
}
