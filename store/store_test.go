package store

import (
	"path/filepath"
	"testing"
)

// TestForeignKeysPragmaEnabled asserts that every store connection has
// foreign-key enforcement turned on. Reference integrity in Pocketknife is the
// database's job, not the application's; this pins that guarantee at its source.
func TestForeignKeysPragmaEnabled(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	var fk int
	if err := s.db.QueryRow("PRAGMA foreign_keys;").Scan(&fk); err != nil {
		t.Fatalf("query pragma: %v", err)
	}
	if fk != 1 {
		t.Fatalf("PRAGMA foreign_keys = %d, want 1 (FK enforcement must be on)", fk)
	}
}
