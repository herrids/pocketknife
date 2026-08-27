package store

import (
	"path/filepath"
	"strings"
	"testing"

	"pocketknife/materialize"
	"pocketknife/schema"
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

// testApp is a minimal one-entity, one-field app used by the VerifySchema
// tests below.
func testApp() *schema.App {
	return &schema.App{
		ID: "app", Name: "App", Version: 1,
		Entities: []*schema.Entity{
			{
				ID: "ent_item", Name: "item", Operations: schema.AllOperations,
				Fields: []*schema.Field{
					{ID: "fld_title", Name: "title", Type: schema.TypeText},
				},
			},
		},
	}
}

func materializeInto(t *testing.T, s *Store, app *schema.App) {
	t.Helper()
	stmts, err := materialize.Statements(app)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if err := s.ApplyDDL(stmts); err != nil {
		t.Fatalf("apply ddl: %v", err)
	}
}

func TestVerifySchemaPassesForAFreshlyMaterializedApp(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	app := testApp()
	materializeInto(t, s, app)

	if err := s.VerifySchema(app); err != nil {
		t.Fatalf("VerifySchema on a freshly materialized app: %v", err)
	}
}

func TestVerifySchemaFailsOnMissingTable(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	// No ApplyDDL call at all: the app's table was never created.
	if err := s.VerifySchema(testApp()); err == nil {
		t.Fatal("expected VerifySchema to fail when the entity's table does not exist")
	}
}

func TestVerifySchemaFailsOnMissingColumn(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	app := testApp()
	materializeInto(t, s, app)

	// Simulate the manifest having grown a field the database never got a
	// column for (the exact scenario the boot-time check exists to catch).
	app.Entities[0].Fields = append(app.Entities[0].Fields, &schema.Field{
		ID: "fld_missing", Name: "missing", Type: schema.TypeText,
	})

	err = s.VerifySchema(app)
	if err == nil {
		t.Fatal("expected VerifySchema to fail when a declared field's column is missing")
	}
	if !strings.Contains(err.Error(), "fld_missing") {
		t.Fatalf("error = %v, want it to name the missing column fld_missing", err)
	}
}

func TestVerifySchemaFailsOnExtraColumn(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	app := testApp()
	materializeInto(t, s, app)

	if _, err := s.db.Exec("ALTER TABLE ent_item ADD COLUMN fld_stray TEXT;"); err != nil {
		t.Fatalf("add stray column: %v", err)
	}

	err = s.VerifySchema(app)
	if err == nil {
		t.Fatal("expected VerifySchema to fail when the table has a column the manifest does not declare")
	}
	if !strings.Contains(err.Error(), "fld_stray") {
		t.Fatalf("error = %v, want it to name the extra column fld_stray", err)
	}
}
