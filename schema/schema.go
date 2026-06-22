// Package schema is the typed, in-memory representation that a manifest parses
// into. It holds no behaviour beyond parsing and small lookup helpers; the
// validator, materializer, store and API all read from this model.
//
// The model preserves the manifest's stable IDs as the identity of every app,
// entity and field. Names are mutable labels used as SQL identifiers and JSON
// keys; IDs never change. (A future migration engine will diff schemas by ID.)
package schema

// FieldType is the closed set of field types supported in v1. Any other value
// is a validation error — there is no escape hatch.
type FieldType string

const (
	TypeText      FieldType = "text"
	TypeInteger   FieldType = "integer"
	TypeReal      FieldType = "real"
	TypeBoolean   FieldType = "boolean"
	TypeDatetime  FieldType = "datetime"
	TypeEnum      FieldType = "enum"
	TypeReference FieldType = "reference"
)

// Operation names the four CRUD operations an entity may expose.
type Operation string

const (
	OpCreate Operation = "create"
	OpRead   Operation = "read"
	OpUpdate Operation = "update"
	OpDelete Operation = "delete"
)

// AllOperations is the default operation set for an entity that does not
// declare one.
var AllOperations = []Operation{OpCreate, OpRead, OpUpdate, OpDelete}

// OnDelete actions for reference fields.
const (
	OnDeleteSetNull  = "set_null"
	OnDeleteRestrict = "restrict"
	OnDeleteCascade  = "cascade"
)

// ReservedNames are the platform-managed column names. A manifest must never
// declare a field with one of these names.
var ReservedNames = []string{"id", "created_at", "updated_at"}

// App is the root of the schema model.
type App struct {
	ID       string
	Name     string
	Emoji    string
	Version  int
	Entities []*Entity
	// Frontend points at this version's pre-built static bundle, or nil if the
	// app declares no frontend (API-only).
	Frontend *Frontend
}

// Frontend names a pre-built static asset bundle for this manifest version.
// Pocketknife never bundles on-box in this phase: Dist must already contain
// the built HTML/JS/CSS output, relative to the app's directory.
type Frontend struct {
	// Dist is the path, relative to the app directory, of the built static
	// asset directory (e.g. "frontend/dist").
	Dist string
	// Entry is the file within Dist served for the root and for any path that
	// does not match a real asset (SPA fallback). Defaults to "index.html".
	Entry string
}

// Entity is a single table's worth of schema.
type Entity struct {
	ID         string
	Name       string
	Operations []Operation
	Fields     []*Field
}

// Field is one declared column. Constraint members are only meaningful for the
// field types that allow them (see the manifest schema); the validator
// guarantees that invariant before a Field reaches the rest of the system.
type Field struct {
	ID       string
	Name     string
	Type     FieldType
	Required bool
	Unique   bool

	// HasDefault distinguishes "no default" from "default is the zero value".
	HasDefault bool
	// Default holds the normalised default value: string for text/datetime/enum,
	// int64 for integer, float64 for real, bool for boolean.
	Default any

	// Min/Max bound either length (text) or value (integer/real). nil means
	// unconstrained.
	Min *float64
	Max *float64

	// Values is the allowed set for an enum field.
	Values []string

	// Target is the referenced entity's stable ID; OnDelete the FK action.
	Target   string
	OnDelete string
}

// Field returns the field with the given name, or nil.
func (e *Entity) Field(name string) *Field {
	for _, f := range e.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// FieldByID returns the field with the given stable ID, or nil. The migration
// engine matches fields across versions by ID (a rename keeps the ID), so it
// resolves fields this way rather than by mutable name.
func (e *Entity) FieldByID(id string) *Field {
	for _, f := range e.Fields {
		if f.ID == id {
			return f
		}
	}
	return nil
}

// Allows reports whether the entity exposes the given operation.
func (e *Entity) Allows(op Operation) bool {
	for _, o := range e.Operations {
		if o == op {
			return true
		}
	}
	return false
}

// Entity returns the entity with the given name, or nil.
func (a *App) Entity(name string) *Entity {
	for _, e := range a.Entities {
		if e.Name == name {
			return e
		}
	}
	return nil
}

// EntityByID returns the entity with the given stable ID, or nil.
func (a *App) EntityByID(id string) *Entity {
	for _, e := range a.Entities {
		if e.ID == id {
			return e
		}
	}
	return nil
}
