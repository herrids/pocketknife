package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"

	"pocketknife/schema"
	"pocketknife/store"
)

// defaultListLimit and maxListLimit mirror api/query.go's defaultLimit and
// maxLimit. A function's data_call list path deliberately does not support
// filter/sort in this phase — that richer query expressiveness is a scope
// cut, not an oversight.
const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// dataRequest is the wire shape a guest function sends to data_call. It
// mirrors, but does not share Go types with, the manifest's data scope and
// the generic API's body shape — this is a wire protocol between host and
// guest, authored independently on each side.
type dataRequest struct {
	Entity    string                     `json:"entity"`
	Operation string                     `json:"operation"`
	ID        string                     `json:"id,omitempty"`
	Values    map[string]json.RawMessage `json:"values,omitempty"`
	Limit     int                        `json:"limit,omitempty"`
	Offset    int                        `json:"offset,omitempty"`
}

// handleDataCall is the capability-gated entry point for a function's data
// access. The capability check happens before the entity is even resolved,
// let alone before any store call: a function with no granted scope for this
// entity/operation pair never touches the database.
func handleDataCall(inv *invocation, reqBytes []byte) ([]byte, int32) {
	var req dataRequest
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return errorBody(fmt.Sprintf("malformed data request: %v", err)), codeBadRequest
	}

	op := schema.Operation(req.Operation)
	if !inv.fn.Capabilities.Allows(req.Entity, op) {
		return nil, codeDenied
	}

	// Defensive: the validator guarantees every declared data-scope entity
	// resolves to a real entity at manifest-validation time, but the sandbox
	// boundary never trusts that guarantee from the outside — it re-resolves
	// for itself.
	ent := inv.app.EntityByID(req.Entity)
	if ent == nil {
		return errorBody("entity not available"), codeBackendError
	}

	switch op {
	case schema.OpCreate:
		return dataCreate(inv, ent, req)
	case schema.OpRead:
		if req.ID != "" {
			return dataRead(inv, ent, req.ID)
		}
		return dataList(inv, ent, req)
	case schema.OpUpdate:
		return dataUpdate(inv, ent, req)
	case schema.OpDelete:
		return dataDelete(inv, ent, req)
	default:
		return errorBody("unsupported operation"), codeBadRequest
	}
}

func dataCreate(inv *invocation, ent *schema.Entity, req dataRequest) ([]byte, int32) {
	values := map[string]any{}
	for _, f := range ent.Fields {
		raw, present := req.Values[f.Name]
		if !present {
			if f.HasDefault {
				values[f.Name] = defaultStoreValue(f)
			} else if f.Required {
				return errorBody(f.Name + " is required"), codeBadRequest
			}
			continue
		}
		val, isNull, err := coerceValue(inv.app, inv.store, f, raw)
		if err != nil {
			return errorBody(err.Error()), codeBadRequest
		}
		if isNull {
			if f.Required {
				return errorBody(f.Name + " is required and cannot be null"), codeBadRequest
			}
			values[f.Name] = nil
			continue
		}
		values[f.Name] = val
	}

	now := store.NowUTC()
	values["id"] = store.NewID()
	values["created_at"] = now
	values["updated_at"] = now

	row, err := inv.store.Insert(ent, values)
	if err != nil {
		return storeErrorBody(err)
	}
	return successBody(map[string]any{"row": row})
}

func dataRead(inv *invocation, ent *schema.Entity, id string) ([]byte, int32) {
	row, err := inv.store.GetByID(ent, id)
	if err != nil {
		return storeErrorBody(err)
	}
	return successBody(map[string]any{"row": row})
}

func dataList(inv *invocation, ent *schema.Entity, req dataRequest) ([]byte, int32) {
	limit := req.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	rows, total, err := inv.store.List(ent, store.ListQuery{Limit: limit, Offset: offset})
	if err != nil {
		return storeErrorBody(err)
	}
	return successBody(map[string]any{"rows": rows, "total": total})
}

func dataUpdate(inv *invocation, ent *schema.Entity, req dataRequest) ([]byte, int32) {
	if req.ID == "" {
		return errorBody("id is required for update"), codeBadRequest
	}
	values := map[string]any{}
	for _, f := range ent.Fields {
		raw, present := req.Values[f.Name]
		if !present {
			continue // partial update: untouched fields are left as-is
		}
		val, isNull, err := coerceValue(inv.app, inv.store, f, raw)
		if err != nil {
			return errorBody(err.Error()), codeBadRequest
		}
		if isNull {
			if f.Required {
				return errorBody(f.Name + " is required and cannot be null"), codeBadRequest
			}
			values[f.Name] = nil
			continue
		}
		values[f.Name] = val
	}
	values["updated_at"] = store.NowUTC()

	row, err := inv.store.Update(ent, req.ID, values)
	if err != nil {
		return storeErrorBody(err)
	}
	return successBody(map[string]any{"row": row})
}

func dataDelete(inv *invocation, ent *schema.Entity, req dataRequest) ([]byte, int32) {
	if req.ID == "" {
		return errorBody("id is required for delete"), codeBadRequest
	}
	deleted, err := inv.store.Delete(ent, req.ID)
	if err != nil {
		return storeErrorBody(err)
	}
	return successBody(map[string]any{"deleted": deleted})
}

// defaultStoreValue converts a field's declared default into its
// storage-ready value. Mirrors api/api.go's defaultStoreValue; duplicated
// rather than shared because sandbox must not import api (api will import
// sandbox to wire the function-invocation HTTP endpoint).
func defaultStoreValue(f *schema.Field) any {
	if f.Type == schema.TypeDatetime {
		if s, ok := f.Default.(string); ok {
			if canon, err := store.CanonicalDatetime(s); err == nil {
				return canon
			}
		}
	}
	return f.Default
}

// coerceValue validates and converts one field's raw JSON value into its
// canonical storage representation. It mirrors api/coerce.go's coerce, with
// two differences: it depends only on schema and store (not registry or
// api, to avoid an import cycle), and it returns a plain error instead of a
// *fieldIssue, since a function's data_call has no HTTP body to attach field
// issues to.
func coerceValue(app *schema.App, st *store.Store, f *schema.Field, raw json.RawMessage) (any, bool, error) {
	if isJSONNull(raw) {
		return nil, true, nil
	}

	switch f.Type {
	case schema.TypeText:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false, fmt.Errorf("%s: must be a string", f.Name)
		}
		n := float64(len([]rune(s)))
		if f.Min != nil && n < *f.Min {
			return nil, false, fmt.Errorf("%s: must be at least %s characters", f.Name, numStr(*f.Min))
		}
		if f.Max != nil && n > *f.Max {
			return nil, false, fmt.Errorf("%s: must be at most %s characters", f.Name, numStr(*f.Max))
		}
		return s, false, nil

	case schema.TypeInteger:
		var jn json.Number
		if err := json.Unmarshal(raw, &jn); err != nil {
			return nil, false, fmt.Errorf("%s: must be an integer", f.Name)
		}
		i, err := jn.Int64()
		if err != nil {
			return nil, false, fmt.Errorf("%s: must be a whole integer", f.Name)
		}
		if f.Min != nil && float64(i) < *f.Min {
			return nil, false, fmt.Errorf("%s: must be >= %s", f.Name, numStr(*f.Min))
		}
		if f.Max != nil && float64(i) > *f.Max {
			return nil, false, fmt.Errorf("%s: must be <= %s", f.Name, numStr(*f.Max))
		}
		return i, false, nil

	case schema.TypeReal:
		var jn json.Number
		if err := json.Unmarshal(raw, &jn); err != nil {
			return nil, false, fmt.Errorf("%s: must be a number", f.Name)
		}
		fv, err := jn.Float64()
		if err != nil {
			return nil, false, fmt.Errorf("%s: must be a number", f.Name)
		}
		if f.Min != nil && fv < *f.Min {
			return nil, false, fmt.Errorf("%s: must be >= %s", f.Name, numStr(*f.Min))
		}
		if f.Max != nil && fv > *f.Max {
			return nil, false, fmt.Errorf("%s: must be <= %s", f.Name, numStr(*f.Max))
		}
		return fv, false, nil

	case schema.TypeBoolean:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, false, fmt.Errorf("%s: must be a boolean", f.Name)
		}
		return b, false, nil

	case schema.TypeDatetime:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false, fmt.Errorf("%s: must be an ISO-8601 datetime string", f.Name)
		}
		canon, err := store.CanonicalDatetime(s)
		if err != nil {
			return nil, false, fmt.Errorf("%s: %s", f.Name, err.Error())
		}
		return canon, false, nil

	case schema.TypeEnum:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false, fmt.Errorf("%s: must be a string", f.Name)
		}
		for _, v := range f.Values {
			if v == s {
				return s, false, nil
			}
		}
		return nil, false, fmt.Errorf("%s: must be one of %v", f.Name, f.Values)

	case schema.TypeReference:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false, fmt.Errorf("%s: must be a reference id string", f.Name)
		}
		target := app.EntityByID(f.Target)
		if target == nil {
			return nil, false, fmt.Errorf("%s: reference target is not available", f.Name)
		}
		ok, err := st.Exists(target, s)
		if err != nil {
			return nil, false, fmt.Errorf("%s: could not verify reference target", f.Name)
		}
		if !ok {
			return nil, false, fmt.Errorf("%s: referenced %s %q does not exist", f.Name, target.Name, s)
		}
		return s, false, nil

	default:
		return nil, false, fmt.Errorf("%s: unsupported field type", f.Name)
	}
}

func isJSONNull(raw json.RawMessage) bool {
	return len(raw) == 4 && string(raw) == "null"
}

func numStr(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}

// successBody marshals v as the body of a successful gated call (code 0,
// meaning "use len(body)").
func successBody(v any) ([]byte, int32) {
	b, err := json.Marshal(v)
	if err != nil {
		return errorBody(err.Error()), codeBackendError
	}
	return b, 0
}

// errorBody renders a {"error": msg} detail payload, used for the
// codeBadRequest/codeBackendError paths where response content is not
// security-sensitive.
func errorBody(msg string) []byte {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return b
}

// storeErrorBody classifies a store-layer error into the bad-request vs.
// backend-error split: a constraint violation is the function's own doing
// (bad request), anything else is an unexpected host-side failure.
func storeErrorBody(err error) ([]byte, int32) {
	if errors.Is(err, store.ErrUnique) || errors.Is(err, store.ErrForeignKey) {
		return errorBody(err.Error()), codeBadRequest
	}
	return errorBody(err.Error()), codeBackendError
}
