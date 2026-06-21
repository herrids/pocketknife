// Package materialize turns a validated schema into SQLite DDL. It is the only
// place that knows the type → column mapping. The DDL it produces is
// idempotent (CREATE TABLE IF NOT EXISTS), so re-running boot on an unchanged
// manifest is a no-op.
//
// Identifiers (table and column names) come only from names the validator has
// already proven SQL-safe (^[a-z][a-z0-9_]*$). Values are never present in DDL
// except enum CHECK literals, which are schema-definition constants, not
// request data; those are single-quote-escaped defensively.
package materialize

import (
	"fmt"
	"strings"

	"pocketknife/schema"
)

// Platform-managed columns, added to every table. The manifest must not declare
// these (the validator rejects them as reserved names).
const platformColumns = "id TEXT PRIMARY KEY, created_at TEXT NOT NULL, updated_at TEXT NOT NULL"

// Statements returns the ordered list of DDL statements for an app: one CREATE
// TABLE per entity, followed by any UNIQUE indexes. Entities are emitted in
// manifest order; references rely on SQLite allowing forward/self FK
// declarations, which hold once foreign_keys is enabled per connection.
func Statements(app *schema.App) ([]string, error) {
	var stmts []string
	var indexes []string

	for _, ent := range app.Entities {
		create, idx, err := tableStatements(app, ent)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, create)
		indexes = append(indexes, idx...)
	}
	return append(stmts, indexes...), nil
}

func tableStatements(app *schema.App, ent *schema.Entity) (string, []string, error) {
	var cols []string
	var constraints []string
	var indexes []string

	for _, f := range ent.Fields {
		colDef, fk, err := columnDef(app, ent, f)
		if err != nil {
			return "", nil, err
		}
		cols = append(cols, colDef)
		if fk != "" {
			constraints = append(constraints, fk)
		}
		if f.Unique {
			indexes = append(indexes, fmt.Sprintf(
				"CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s(%s);",
				uniqueIndexName(ent, f), ent.Name, f.Name))
		}
	}

	body := []string{platformColumns}
	body = append(body, cols...)
	body = append(body, constraints...)

	create := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n  %s\n);",
		ent.Name, strings.Join(body, ",\n  "))
	return create, indexes, nil
}

// columnDef returns the column definition and, for references, the table-level
// FOREIGN KEY constraint clause (empty otherwise).
func columnDef(app *schema.App, ent *schema.Entity, f *schema.Field) (string, string, error) {
	var b strings.Builder
	b.WriteString(f.Name)
	b.WriteByte(' ')

	var checks []string
	fk := ""

	switch f.Type {
	case schema.TypeText:
		b.WriteString("TEXT")
		checks = append(checks, lengthChecks(f)...)
	case schema.TypeInteger:
		b.WriteString("INTEGER")
		checks = append(checks, rangeChecks(f)...)
	case schema.TypeReal:
		b.WriteString("REAL")
		checks = append(checks, rangeChecks(f)...)
	case schema.TypeBoolean:
		b.WriteString("INTEGER")
		checks = append(checks, fmt.Sprintf("%s IN (0, 1)", f.Name))
	case schema.TypeDatetime:
		b.WriteString("TEXT")
	case schema.TypeEnum:
		b.WriteString("TEXT")
		checks = append(checks, fmt.Sprintf("%s IN (%s)", f.Name, enumLiterals(f.Values)))
	case schema.TypeReference:
		b.WriteString("TEXT")
		target := app.EntityByID(f.Target)
		if target == nil {
			// Should be unreachable: the validator guarantees resolution.
			return "", "", fmt.Errorf("entity %q field %q references unknown target %q", ent.Name, f.Name, f.Target)
		}
		fk = fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s(id) ON DELETE %s",
			f.Name, target.Name, onDeleteSQL(f.OnDelete))
	default:
		return "", "", fmt.Errorf("entity %q field %q has unknown type %q", ent.Name, f.Name, f.Type)
	}

	if f.Required {
		b.WriteString(" NOT NULL")
	}
	for _, c := range checks {
		b.WriteString(" CHECK (")
		b.WriteString(c)
		b.WriteString(")")
	}
	return b.String(), fk, nil
}

func lengthChecks(f *schema.Field) []string {
	var checks []string
	if f.Min != nil {
		checks = append(checks, fmt.Sprintf("length(%s) >= %s", f.Name, formatNum(*f.Min)))
	}
	if f.Max != nil {
		checks = append(checks, fmt.Sprintf("length(%s) <= %s", f.Name, formatNum(*f.Max)))
	}
	return checks
}

func rangeChecks(f *schema.Field) []string {
	var checks []string
	if f.Min != nil {
		checks = append(checks, fmt.Sprintf("%s >= %s", f.Name, formatNum(*f.Min)))
	}
	if f.Max != nil {
		checks = append(checks, fmt.Sprintf("%s <= %s", f.Name, formatNum(*f.Max)))
	}
	return checks
}

// formatNum renders a bound without a trailing ".0" for whole numbers, so
// integer bounds read naturally in the DDL.
func formatNum(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}

func onDeleteSQL(action string) string {
	switch action {
	case schema.OnDeleteCascade:
		return "CASCADE"
	case schema.OnDeleteRestrict:
		return "RESTRICT"
	default:
		return "SET NULL"
	}
}

// enumLiterals renders the allowed enum values as SQL string literals. Enum
// values are schema constants (not request data); single quotes are doubled per
// SQL rules so a value containing a quote cannot break out of the literal.
func enumLiterals(values []string) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	return strings.Join(parts, ", ")
}

func uniqueIndexName(ent *schema.Entity, f *schema.Field) string {
	return fmt.Sprintf("ux_%s_%s", ent.Name, f.Name)
}
