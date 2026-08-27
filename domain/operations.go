// Package domain holds the runtime operations every installed app's CRUD
// surface reduces to — create, get, list, update, delete one entity's rows —
// as plain Go functions with no dependency on net/http or any other
// transport. It resolves the app and entity through the registry, enforces
// the entity's declared operation set, applies the shared field-coercion
// rules (coerce.go), and calls the store, returning a structured *OpError a
// transport maps to its own wire shape.
//
// This is the seam a future MCP transport needs: today only api/ wraps
// these functions, but sandbox/'s function runtime already proves the
// underlying pattern (call the store directly, no HTTP in sight) works, and
// nothing here changes for MCP to become a second caller alongside HTTP.
package domain

import (
	"encoding/json"
	"errors"
	"net/url"

	"pocketknife/registry"
	"pocketknife/schema"
	"pocketknife/store"
)

// ListResult is the outcome of a successful List: the matching rows, the
// total count ignoring limit/offset, and the limit/offset actually applied
// (defaulted and/or clamped from the request).
type ListResult struct {
	Rows   []map[string]any
	Total  int
	Limit  int
	Offset int
}

// resolveEntity looks up the app and entity and checks that op is enabled,
// the same three checks every operation below needs before touching the
// store.
func resolveEntity(reg *registry.Registry, appID, entityName string, op schema.Operation) (*registry.RegisteredApp, *schema.Entity, *OpError) {
	ra, ok := reg.App(appID)
	if !ok {
		return nil, nil, &OpError{Kind: ErrAppNotFound, Message: "no app with id " + appID}
	}
	ent := ra.Schema.Entity(entityName)
	if ent == nil {
		return nil, nil, &OpError{Kind: ErrEntityNotFound, Message: "no entity " + entityName + " in app " + appID}
	}
	if !ent.Allows(op) {
		return nil, nil, &OpError{Kind: ErrOperationDisabled, Message: "operation " + string(op) + " is not enabled for entity " + ent.Name}
	}
	return ra, ent, nil
}

// storeOpError classifies a store-layer error into the sentinel-backed
// OpError kinds every transport needs to distinguish (a conflict is the
// caller's fault; anything else is unexpected).
func storeOpError(err error) *OpError {
	switch {
	case errors.Is(err, store.ErrUnique):
		return &OpError{Kind: ErrUnique, Message: "a row with this value already exists"}
	case errors.Is(err, store.ErrForeignKey):
		return &OpError{Kind: ErrReferenceConflict, Message: "operation violates a reference constraint"}
	default:
		return &OpError{Kind: ErrInternal, Message: err.Error()}
	}
}

func notFoundRow(ent *schema.Entity, id string) *OpError {
	return &OpError{Kind: ErrRowNotFound, Message: "no " + ent.Name + " with id " + id}
}

// coerceFields runs the shared per-declared-field loop used by both Create
// and Update: default/required handling for Create's absent-field case
// (onCreate), coercion for every present field, and null handling. It always
// collects every issue rather than failing on the first one, so a caller
// with several bad fields sees all of them in one round trip — and so a
// possible future caller with different transport-level policies (e.g. a
// sandboxed function reporting only the first issue) can do so by simply
// taking issues[0] rather than needing a different code path here.
func coerceFields(ra *registry.RegisteredApp, ent *schema.Entity, body map[string]json.RawMessage, onCreate bool) (map[string]any, []FieldError) {
	values := map[string]any{}
	var issues []FieldError
	for _, f := range ent.Fields {
		raw, present := body[f.Name]
		if !present {
			if onCreate {
				if f.HasDefault {
					values[f.Name] = DefaultStoreValue(f)
				} else if f.Required {
					issues = append(issues, FieldError{Field: f.Name, Message: "is required"})
				}
			}
			// On update, an absent field is left untouched — that's the
			// partial-update contract.
			continue
		}
		val, isNull, ferr := CoerceFieldValue(ra.Schema, ra.Store, f, raw)
		if ferr != nil {
			issues = append(issues, *ferr)
			continue
		}
		if isNull {
			if f.Required {
				issues = append(issues, FieldError{Field: f.Name, Message: "is required and cannot be null"})
				continue
			}
			values[f.Name] = nil
			continue
		}
		values[f.Name] = val
	}
	return values, issues
}

// Create validates body against ent's declared fields and inserts a new row,
// with the platform columns (id, created_at, updated_at) set automatically.
func Create(reg *registry.Registry, appID, entityName string, body map[string]json.RawMessage) (map[string]any, *OpError) {
	ra, ent, operr := resolveEntity(reg, appID, entityName, schema.OpCreate)
	if operr != nil {
		return nil, operr
	}

	values, issues := coerceFields(ra, ent, body, true)
	if len(issues) > 0 {
		return nil, &OpError{Kind: ErrValidation, Message: "request body failed validation", Issues: issues}
	}

	now := store.NowUTC()
	values["id"] = store.NewID()
	values["created_at"] = now
	values["updated_at"] = now

	row, err := ra.Store.Insert(ent, values)
	if err != nil {
		return nil, storeOpError(err)
	}
	return row, nil
}

// Get returns one row by id.
func Get(reg *registry.Registry, appID, entityName, id string) (map[string]any, *OpError) {
	ra, ent, operr := resolveEntity(reg, appID, entityName, schema.OpRead)
	if operr != nil {
		return nil, operr
	}
	row, err := ra.Store.GetByID(ent, id)
	if err != nil {
		return nil, storeOpError(err)
	}
	if row == nil {
		return nil, notFoundRow(ent, id)
	}
	return row, nil
}

// List returns matching rows for the query-string-encoded filter/sort/
// pagination terms described in domain/query.go.
func List(reg *registry.Registry, appID, entityName string, query url.Values) (*ListResult, *OpError) {
	ra, ent, operr := resolveEntity(reg, appID, entityName, schema.OpRead)
	if operr != nil {
		return nil, operr
	}
	q, ferr := parseListQuery(ent, query)
	if ferr != nil {
		return nil, &OpError{Kind: ErrInvalidQuery, Message: ferr.Message, Issues: []FieldError{*ferr}}
	}
	rows, total, err := ra.Store.List(ent, q)
	if err != nil {
		return nil, storeOpError(err)
	}
	return &ListResult{Rows: rows, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

// Update applies a partial change: only the fields present in body are
// touched, everything else is left as-is.
func Update(reg *registry.Registry, appID, entityName, id string, body map[string]json.RawMessage) (map[string]any, *OpError) {
	ra, ent, operr := resolveEntity(reg, appID, entityName, schema.OpUpdate)
	if operr != nil {
		return nil, operr
	}

	existing, err := ra.Store.GetByID(ent, id)
	if err != nil {
		return nil, storeOpError(err)
	}
	if existing == nil {
		return nil, notFoundRow(ent, id)
	}

	values, issues := coerceFields(ra, ent, body, false)
	if len(issues) > 0 {
		return nil, &OpError{Kind: ErrValidation, Message: "request body failed validation", Issues: issues}
	}
	values["updated_at"] = store.NowUTC()

	row, err := ra.Store.Update(ent, id, values)
	if err != nil {
		return nil, storeOpError(err)
	}
	if row == nil {
		return nil, notFoundRow(ent, id)
	}
	return row, nil
}

// Delete removes a row, reporting whether one existed.
func Delete(reg *registry.Registry, appID, entityName, id string) (bool, *OpError) {
	ra, ent, operr := resolveEntity(reg, appID, entityName, schema.OpDelete)
	if operr != nil {
		return false, operr
	}
	deleted, err := ra.Store.Delete(ent, id)
	if err != nil {
		return false, storeOpError(err)
	}
	if !deleted {
		return false, notFoundRow(ent, id)
	}
	return true, nil
}
