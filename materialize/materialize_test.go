package materialize_test

import (
	"strings"
	"testing"

	"pocketknife/materialize"
	"pocketknife/validate"
)

func compile(t *testing.T, body string) []string {
	t.Helper()
	app, errs := validate.Manifest([]byte(body))
	if len(errs) > 0 {
		t.Fatalf("manifest invalid: %v", errs)
	}
	stmts, err := materialize.Statements(app)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	return stmts
}

func joinAll(stmts []string) string { return strings.Join(stmts, "\n") }

func TestPlatformColumnsAlwaysPresent(t *testing.T) {
	ddl := joinAll(compile(t, `{
      "app": { "id": "a", "name": "A", "version": 1 },
      "entities": [{ "id": "e", "name": "thing", "fields": [
        { "id": "f", "name": "title", "type": "text" }
      ]}]
    }`))
	for _, want := range []string{
		"id TEXT PRIMARY KEY",
		"created_at TEXT NOT NULL",
		"updated_at TEXT NOT NULL",
		"CREATE TABLE IF NOT EXISTS thing",
	} {
		if !strings.Contains(ddl, want) {
			t.Fatalf("DDL missing %q:\n%s", want, ddl)
		}
	}
}

func TestTypeMappingAndChecks(t *testing.T) {
	ddl := joinAll(compile(t, `{
      "app": { "id": "a", "name": "A", "version": 1 },
      "entities": [{ "id": "e", "name": "thing", "fields": [
        { "id": "f1", "name": "name",   "type": "text", "required": true, "min": 1, "max": 50, "unique": true },
        { "id": "f2", "name": "score",  "type": "integer", "min": 0, "max": 100 },
        { "id": "f3", "name": "ratio",  "type": "real" },
        { "id": "f4", "name": "active", "type": "boolean" },
        { "id": "f5", "name": "when",   "type": "datetime" },
        { "id": "f6", "name": "kind",   "type": "enum", "values": ["low","high"] }
      ]}]
    }`))

	checks := []string{
		"name TEXT NOT NULL",
		"length(name) >= 1",
		"length(name) <= 50",
		"score INTEGER",
		"score >= 0",
		"score <= 100",
		"ratio REAL",
		"active INTEGER",
		"active IN (0, 1)",
		"when TEXT",
		"kind TEXT",
		"kind IN ('low', 'high')",
		"CREATE UNIQUE INDEX IF NOT EXISTS ux_thing_name ON thing(name)",
	}
	for _, want := range checks {
		if !strings.Contains(ddl, want) {
			t.Fatalf("DDL missing %q:\n%s", want, ddl)
		}
	}
}

func TestReferenceForeignKey(t *testing.T) {
	ddl := joinAll(compile(t, `{
      "app": { "id": "a", "name": "A", "version": 1 },
      "entities": [
        { "id": "ent_project", "name": "project", "fields": [
          { "id": "p1", "name": "name", "type": "text", "required": true }
        ]},
        { "id": "ent_task", "name": "task", "fields": [
          { "id": "t1", "name": "title", "type": "text", "required": true },
          { "id": "t2", "name": "project", "type": "reference", "target": "ent_project", "onDelete": "set_null" }
        ]}
      ]
    }`))

	if !strings.Contains(ddl, "FOREIGN KEY (project) REFERENCES project(id) ON DELETE SET NULL") {
		t.Fatalf("missing FK clause:\n%s", ddl)
	}
}

func TestEnumLiteralsAreQuoteEscaped(t *testing.T) {
	ddl := joinAll(compile(t, `{
      "app": { "id": "a", "name": "A", "version": 1 },
      "entities": [{ "id": "e", "name": "thing", "fields": [
        { "id": "f", "name": "mood", "type": "enum", "values": ["it's fine", "ok"] }
      ]}]
    }`))
	// A value containing a single quote must be doubled so it cannot break out
	// of the literal: it's fine -> 'it''s fine'.
	if !strings.Contains(ddl, "'it''s fine'") {
		t.Fatalf("enum quote not escaped:\n%s", ddl)
	}
}
