package migrate

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"pocketknife/materialize"
	"pocketknife/registry"
	"pocketknife/validate"
)

// TestCrashDuringMigrationLeavesConsistentState hunts the crash window identified
// in apply.go: Execute() commits the rebuild transaction, and only afterwards does
// Apply write the promoted manifest.json (a plain os.WriteFile, not a
// write-temp-then-rename) and re-register the new schema in memory. A process
// killed between those two steps could leave data.db physically at the new schema
// while manifest.json on disk still names the old version -- an inconsistency a
// later boot (which trusts manifest.json and only ever runs idempotent CREATE
// statements, never reconciling drift; see TestBootDoesNotReconcileDriftedManifest)
// would not detect.
//
// This drives the real `pocketknife migrate` binary as a subprocess against a
// sizable dataset (so the rebuild's row copy takes measurable wall-clock time),
// SIGKILLs it after a swept range of delays spanning the whole operation -- with
// extra density near the tail, where the manifest-promotion gap actually is -- and
// asserts after every kill that: (a) data.db is never corrupt (PRAGMA
// integrity_check), (b) no row is ever lost or duplicated, and (c) manifest.json's
// declared version always matches the physical column type of fld_count, never one
// version while the table already reflects the other.
func TestCrashDuringMigrationLeavesConsistentState(t *testing.T) {
	if testing.Short() {
		t.Skip("crash-loop test is slow; skipped with -short")
	}

	const crashRows = 50000
	const appID = "crashapp"

	bin := buildPocketknifeBinary(t)

	v1 := crashAppManifest(1, "integer")
	v2 := crashAppManifest(2, "real")
	v2Path := filepath.Join(t.TempDir(), "v2.manifest.json")
	if err := os.WriteFile(v2Path, []byte(v2), 0o644); err != nil {
		t.Fatalf("write v2 manifest: %v", err)
	}

	goldenRoot := t.TempDir()
	seedCrashApp(t, goldenRoot, appID, v1, crashRows)

	// Calibration: run the migration once, uninterrupted, to learn how long a
	// full run takes on this machine, so the kill-delay sweep is scaled to the
	// real operation instead of guessing a fixed duration.
	calibRoot := t.TempDir()
	copyAppDir(t, goldenRoot, calibRoot, appID)
	calibStart := time.Now()
	if out, err := exec.Command(bin, "migrate", "-apps", calibRoot, "-app", appID, "-to", v2Path).CombinedOutput(); err != nil {
		t.Fatalf("calibration migration failed: %v\n%s", err, out)
	}
	full := time.Since(calibStart)
	t.Logf("uninterrupted migration of %d rows took %s", crashRows, full)

	for i, delay := range killDelays(full) {
		delay := delay
		t.Run(fmt.Sprintf("kill_%02d_after_%s", i, delay), func(t *testing.T) {
			iterRoot := t.TempDir()
			copyAppDir(t, goldenRoot, iterRoot, appID)

			cmd := exec.Command(bin, "migrate", "-apps", iterRoot, "-app", appID, "-to", v2Path)
			var out fakeSyncBuffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			if err := cmd.Start(); err != nil {
				t.Fatalf("start migrate subprocess: %v", err)
			}
			time.Sleep(delay)
			_ = cmd.Process.Kill() // SIGKILL; no-op if the process already exited
			_ = cmd.Wait()

			assertConsistentPostCrashState(t, iterRoot, appID, crashRows)
		})
	}
}

// killDelays returns a swept set of durations to wait before SIGKILL-ing the
// migration subprocess: a coarse pass across the whole operation, plus a fine,
// tail-weighted cluster approaching and just past the measured full duration --
// where the post-commit manifest-promotion gap actually lives.
func killDelays(full time.Duration) []time.Duration {
	var ds []time.Duration
	// Coarse sweep across the whole run.
	for frac := 0.0; frac <= 1.0; frac += 0.1 {
		ds = append(ds, time.Duration(float64(full)*frac))
	}
	// Fine, tail-weighted cluster around the end of the run, where Execute's
	// commit and the subsequent os.WriteFile both happen.
	tailOffsets := []time.Duration{
		20 * time.Millisecond, 10 * time.Millisecond, 5 * time.Millisecond,
		2 * time.Millisecond, 1 * time.Millisecond, 500 * time.Microsecond,
		200 * time.Microsecond, 100 * time.Microsecond, 50 * time.Microsecond,
		20 * time.Microsecond, 10 * time.Microsecond, 0,
		-10 * time.Microsecond, -50 * time.Microsecond, -100 * time.Microsecond,
		-500 * time.Microsecond, -1 * time.Millisecond, -5 * time.Millisecond,
	}
	for _, off := range tailOffsets {
		d := full + off
		if d < 0 {
			d = 0
		}
		ds = append(ds, d)
	}
	return ds
}

// fakeSyncBuffer is a minimal io.Writer sink for a subprocess's combined output;
// the crash test only needs it for failure diagnostics, never assertions.
type fakeSyncBuffer struct{ buf []byte }

func (b *fakeSyncBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

// buildPocketknifeBinary compiles the real CLI once per test run.
func buildPocketknifeBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "pocketknife-crash-test")
	cmd := exec.Command("go", "build", "-o", bin, "pocketknife/cmd/pocketknife")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build pocketknife: %v\n%s", err, out)
	}
	return bin
}

// crashAppManifest renders the one-entity manifest used by the crash test, with
// fld_count at the given type -- "integer" for v1, "real" for the widening v2.
func crashAppManifest(version int, countType string) string {
	return fmt.Sprintf(`{
  "app": { "id": "crashapp", "name": "Crash App", "version": %d },
  "entities": [
    { "id": "ent_item", "name": "item", "fields": [
      { "id": "fld_count", "name": "count", "type": "%s" }
    ]}
  ]
}`, version, countType)
}

// seedCrashApp writes <root>/<appID>/manifest.json (the given v1 manifest) and a
// data.db with `rows` rows, bypassing the Store's one-statement-per-call Insert
// (too slow at this row count) in favor of a single bulk transaction over a raw
// connection -- this is test fixture setup, not the code under test.
func seedCrashApp(t *testing.T, root, appID, manifestJSON string, rows int) {
	t.Helper()
	dir := filepath.Join(root, appID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifestJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	app, errs := validate.Manifest([]byte(manifestJSON))
	if len(errs) > 0 {
		t.Fatalf("seed manifest invalid: %v", errs)
	}
	stmts, err := materialize.Statements(app)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	dbPath := filepath.Join(dir, "data.db")
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer db.Close()

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed ddl %q: %v", stmt, err)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("seed begin: %v", err)
	}
	stmt, err := tx.Prepare("INSERT INTO ent_item (id, created_at, updated_at, fld_count) VALUES (?, ?, ?, ?);")
	if err != nil {
		t.Fatalf("seed prepare: %v", err)
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	for i := 0; i < rows; i++ {
		id := fmt.Sprintf("crash-row-%08d", i)
		if _, err := stmt.Exec(id, now, now, i); err != nil {
			t.Fatalf("seed insert %d: %v", i, err)
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatalf("seed commit: %v", err)
	}

	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE);"); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}
}

// copyAppDir copies just manifest.json and data.db from src's <appID> dir into a
// freshly created <appID> dir under dst. The golden source is always clean-closed
// (checkpointed, no sidecars), so this is the whole on-disk state.
func copyAppDir(t *testing.T, srcRoot, dstRoot, appID string) {
	t.Helper()
	srcDir := filepath.Join(srcRoot, appID)
	dstDir := filepath.Join(dstRoot, appID)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"manifest.json", "data.db"} {
		if err := copyTestFile(filepath.Join(srcDir, name), filepath.Join(dstDir, name)); err != nil {
			t.Fatalf("copy %s: %v", name, err)
		}
	}
}

func copyTestFile(src, dst string) error {
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
	return out.Close()
}

// assertConsistentPostCrashState is the one assertion this whole test exists to
// run: after an arbitrary kill point, manifest.json must be valid JSON, its
// declared app version must agree with the physical shape of the table it
// describes, every row must still be present, and the database file itself must
// not be corrupt.
func assertConsistentPostCrashState(t *testing.T, appsRoot, appID string, wantRows int) {
	t.Helper()
	dir := filepath.Join(appsRoot, appID)
	manifestPath := filepath.Join(dir, "manifest.json")
	dbPath := filepath.Join(dir, "data.db")

	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest.json after crash: %v", err)
	}
	app, errs := validate.Manifest(manifestBytes)
	if len(errs) > 0 {
		t.Fatalf("CRASH-INDUCED DEFECT: manifest.json is not a valid manifest after a kill "+
			"(non-atomic os.WriteFile in apply.go can be torn by a kill mid-write): %v\nraw: %s",
			errs, manifestBytes)
		return
	}
	version := app.Version

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open data.db after crash: %v", err)
	}
	defer db.Close()

	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check;").Scan(&integrity); err != nil {
		t.Fatalf("integrity_check after crash: %v", err)
	}
	if integrity != "ok" {
		t.Fatalf("CRASH-INDUCED DEFECT: data.db failed integrity_check after a kill: %s", integrity)
	}

	countType := physicalFieldType(t, db, "ent_item", "fld_count")
	switch version {
	case 1:
		if countType != "INTEGER" {
			t.Fatalf("CRASH-INDUCED DEFECT: manifest.json declares version 1 (fld_count integer) "+
				"but the physical column type is %q -- manifest/physical schema mismatch after a kill "+
				"between Execute()'s commit and manifest.json promotion in apply.go", countType)
		}
	case 2:
		if countType != "REAL" {
			t.Fatalf("CRASH-INDUCED DEFECT: manifest.json declares version 2 (fld_count real) "+
				"but the physical column type is %q -- manifest/physical schema mismatch after a kill "+
				"between Execute()'s commit and manifest.json promotion in apply.go", countType)
		}
	default:
		t.Fatalf("unexpected manifest version %d after crash", version)
	}

	var rowCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM ent_item;").Scan(&rowCount); err != nil {
		t.Fatalf("count rows after crash: %v", err)
	}
	if rowCount != wantRows {
		t.Fatalf("CRASH-INDUCED DEFECT: row count after crash = %d, want %d (a kill must never lose or duplicate rows)", rowCount, wantRows)
	}

	// The real boot path must also be able to come up cleanly against whatever
	// state was left behind, whichever side of the promotion the kill landed on.
	db.Close()
	reg, results, err := registry.Load(appsRoot)
	if err != nil {
		t.Fatalf("registry.Load after crash: %v", err)
	}
	defer reg.Close()
	for _, r := range results {
		if !r.OK {
			t.Fatalf("app failed to (re)load after crash: errors=%v err=%v", r.Errors, r.Err)
		}
	}
}

// physicalFieldType reads a column's declared SQL type via PRAGMA table_info.
func physicalFieldType(t *testing.T, db *sql.DB, table, column string) string {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ");")
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == column {
			return typ
		}
	}
	t.Fatalf("column %s not found on %s", column, table)
	return ""
}
