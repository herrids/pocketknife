// Package store owns each app's SQLite database: one data.db file per app, no
// shared database ever. It exposes a generic, schema-driven CRUD/query surface.
// Every value crosses the SQL boundary through a bound parameter; identifiers
// (table/column names) come only from validated, SQL-safe schema names.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"pocketknife/schema"
)

// Sentinel errors the API layer maps to HTTP status codes.
var (
	// ErrUnique is returned when a write violates a UNIQUE constraint (409).
	ErrUnique = errors.New("unique constraint violation")
	// ErrForeignKey is returned when a write violates a FOREIGN KEY
	// constraint, e.g. deleting a row referenced under ON DELETE RESTRICT (409).
	ErrForeignKey = errors.New("foreign key constraint violation")
)

// Store is one app's database handle.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (creating if needed) the SQLite database at path with
// foreign-key enforcement enabled on every connection. A single underlying
// connection keeps writes serialised, which is the simplest correct choice for
// SQLite and avoids "database is locked" under concurrent requests.
func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}
	return &Store{db: db, path: path}, nil
}

// Path returns the database file path (used for isolation assertions/tests).
func (s *Store) Path() string { return s.path }

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// ApplyDDL runs schema DDL statements in order. Statements are idempotent
// (CREATE TABLE/INDEX IF NOT EXISTS), so this is safe to run on every boot.
//
// Seam: a future migration engine would compare the stored manifest against the
// new one here and emit ALTER statements; v1 only ever creates.
func (s *Store) ApplyDDL(stmts []string) error {
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("apply DDL %q: %w", firstLine(stmt), err)
		}
	}
	return nil
}

// Insert writes a new row. values holds JSON-typed Go values keyed by field
// name; the platform columns (id, created_at, updated_at) are supplied by the
// caller via reserved keys. It returns the stored row, decoded back to JSON
// types.
func (s *Store) Insert(ent *schema.Entity, values map[string]any) (map[string]any, error) {
	cols := make([]string, 0, len(values))
	placeholders := make([]string, 0, len(values))
	args := make([]any, 0, len(values))

	for col, v := range values {
		cols = append(cols, col)
		placeholders = append(placeholders, "?")
		args = append(args, encode(ent, col, v))
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		ent.Name, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
	if _, err := s.db.Exec(query, args...); err != nil {
		return nil, classify(err)
	}
	return s.GetByID(ent, values["id"].(string))
}

// GetByID returns a single row by primary key, or (nil, nil) if absent.
func (s *Store) GetByID(ent *schema.Entity, id string) (map[string]any, error) {
	cols := selectColumns(ent)
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = ? LIMIT 1;",
		strings.Join(cols, ", "), ent.Name)
	rows, err := s.db.Query(query, id)
	if err != nil {
		return nil, classify(err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	return scanRow(ent, cols, rows)
}

// Exists reports whether a row with the given id exists. Used to validate
// reference targets before a write.
func (s *Store) Exists(ent *schema.Entity, id string) (bool, error) {
	var one int
	err := s.db.QueryRow(fmt.Sprintf("SELECT 1 FROM %s WHERE id = ? LIMIT 1;", ent.Name), id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, classify(err)
	}
	return true, nil
}

// Filter is one AND-combined list filter.
type Filter struct {
	Column   string // validated field/platform column name
	Operator string // SQL operator (=, !=, >, >=, <, <=, LIKE)
	Value    any    // JSON-typed, coerced to the field type
}

// Sort is one ORDER BY term.
type Sort struct {
	Column string
	Desc   bool
}

// ListQuery describes a list request.
type ListQuery struct {
	Filters []Filter
	Sorts   []Sort
	Limit   int
	Offset  int
}

// List returns matching rows plus the total count of rows matching the filters
// (ignoring limit/offset).
func (s *Store) List(ent *schema.Entity, q ListQuery) ([]map[string]any, int, error) {
	where, args := buildWhere(ent, q.Filters)

	var total int
	countQ := fmt.Sprintf("SELECT COUNT(*) FROM %s%s;", ent.Name, where)
	if err := s.db.QueryRow(countQ, args...).Scan(&total); err != nil {
		return nil, 0, classify(err)
	}

	cols := selectColumns(ent)
	var sb strings.Builder
	fmt.Fprintf(&sb, "SELECT %s FROM %s%s", strings.Join(cols, ", "), ent.Name, where)
	if order := buildOrderBy(q.Sorts); order != "" {
		sb.WriteString(order)
	}
	sb.WriteString(" LIMIT ? OFFSET ?")
	listArgs := append(append([]any{}, args...), q.Limit, q.Offset)

	rows, err := s.db.Query(sb.String()+";", listArgs...)
	if err != nil {
		return nil, 0, classify(err)
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		row, err := scanRow(ent, cols, rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

// Update applies a partial change. values holds only the columns to change
// (including updated_at). It returns the updated row, or (nil, nil) if the row
// does not exist.
func (s *Store) Update(ent *schema.Entity, id string, values map[string]any) (map[string]any, error) {
	if len(values) == 0 {
		return s.GetByID(ent, id)
	}
	sets := make([]string, 0, len(values))
	args := make([]any, 0, len(values)+1)
	for col, v := range values {
		sets = append(sets, col+" = ?")
		args = append(args, encode(ent, col, v))
	}
	args = append(args, id)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?;", ent.Name, strings.Join(sets, ", "))
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return nil, classify(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Either the row is absent, or no values changed. Disambiguate by read.
		return s.GetByID(ent, id)
	}
	return s.GetByID(ent, id)
}

// Delete removes a row, reporting whether one was deleted.
func (s *Store) Delete(ent *schema.Entity, id string) (bool, error) {
	res, err := s.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = ?;", ent.Name), id)
	if err != nil {
		return false, classify(err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// --- helpers ---

func selectColumns(ent *schema.Entity) []string {
	cols := make([]string, 0, len(ent.Fields)+3)
	cols = append(cols, "id")
	for _, f := range ent.Fields {
		cols = append(cols, f.Name)
	}
	cols = append(cols, "created_at", "updated_at")
	return cols
}

func buildWhere(ent *schema.Entity, filters []Filter) (string, []any) {
	if len(filters) == 0 {
		return "", nil
	}
	clauses := make([]string, 0, len(filters))
	args := make([]any, 0, len(filters))
	for _, f := range filters {
		clauses = append(clauses, fmt.Sprintf("%s %s ?", f.Column, f.Operator))
		args = append(args, encode(ent, f.Column, f.Value))
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func buildOrderBy(sorts []Sort) string {
	if len(sorts) == 0 {
		return ""
	}
	terms := make([]string, len(sorts))
	for i, s := range sorts {
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		terms[i] = s.Column + " " + dir
	}
	return " ORDER BY " + strings.Join(terms, ", ")
}

// encode converts a JSON-typed value into its SQLite storage representation.
// The only translation needed is boolean → 0/1; everything else binds directly.
func encode(ent *schema.Entity, col string, v any) any {
	if v == nil {
		return nil
	}
	if f := ent.Field(col); f != nil && f.Type == schema.TypeBoolean {
		if b, ok := v.(bool); ok {
			if b {
				return int64(1)
			}
			return int64(0)
		}
	}
	return v
}

// scanRow reads one row into a JSON-ready map, decoding storage types back to
// JSON types (notably INTEGER 0/1 → boolean) and []byte → string.
func scanRow(ent *schema.Entity, cols []string, rows *sql.Rows) (map[string]any, error) {
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	row := make(map[string]any, len(cols))
	for i, col := range cols {
		row[col] = decode(ent, col, vals[i])
	}
	return row, nil
}

func decode(ent *schema.Entity, col string, v any) any {
	if v == nil {
		return nil
	}
	if b, ok := v.([]byte); ok {
		v = string(b)
	}
	if f := ent.Field(col); f != nil && f.Type == schema.TypeBoolean {
		switch n := v.(type) {
		case int64:
			return n != 0
		case float64:
			return n != 0
		}
	}
	return v
}

// classify maps SQLite constraint errors to sentinel errors the API recognises.
func classify(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "UNIQUE constraint failed"):
		return fmt.Errorf("%w: %s", ErrUnique, msg)
	case strings.Contains(msg, "FOREIGN KEY constraint failed"):
		return fmt.Errorf("%w: %s", ErrForeignKey, msg)
	default:
		return err
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
