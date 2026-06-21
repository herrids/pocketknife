package api

import (
	"encoding/json"
	"fmt"

	"pocketknife/registry"
	"pocketknife/schema"
	"pocketknife/store"
)

// coerce validates and converts one present body value for a field into its
// canonical storage representation. It returns (value, isNull, issue):
//   - issue != nil  → the value failed field validation (HTTP 400)
//   - isNull == true → the field was explicitly set to JSON null
//
// The same field rules used to validate a manifest's defaults are applied here,
// so the API boundary enforces exactly what the schema promises.
func coerce(ra *registry.RegisteredApp, f *schema.Field, raw json.RawMessage) (any, bool, *fieldIssue) {
	if isJSONNull(raw) {
		return nil, true, nil
	}
	issue := func(msg string) *fieldIssue { return &fieldIssue{Field: f.Name, Message: msg} }

	switch f.Type {
	case schema.TypeText:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false, issue("must be a string")
		}
		n := float64(len([]rune(s)))
		if f.Min != nil && n < *f.Min {
			return nil, false, issue(fmt.Sprintf("must be at least %s characters", num(*f.Min)))
		}
		if f.Max != nil && n > *f.Max {
			return nil, false, issue(fmt.Sprintf("must be at most %s characters", num(*f.Max)))
		}
		return s, false, nil

	case schema.TypeInteger:
		var jn json.Number
		if err := json.Unmarshal(raw, &jn); err != nil {
			return nil, false, issue("must be an integer")
		}
		i, err := jn.Int64()
		if err != nil {
			return nil, false, issue("must be a whole integer")
		}
		if f.Min != nil && float64(i) < *f.Min {
			return nil, false, issue(fmt.Sprintf("must be >= %s", num(*f.Min)))
		}
		if f.Max != nil && float64(i) > *f.Max {
			return nil, false, issue(fmt.Sprintf("must be <= %s", num(*f.Max)))
		}
		return i, false, nil

	case schema.TypeReal:
		var jn json.Number
		if err := json.Unmarshal(raw, &jn); err != nil {
			return nil, false, issue("must be a number")
		}
		fv, err := jn.Float64()
		if err != nil {
			return nil, false, issue("must be a number")
		}
		if f.Min != nil && fv < *f.Min {
			return nil, false, issue(fmt.Sprintf("must be >= %s", num(*f.Min)))
		}
		if f.Max != nil && fv > *f.Max {
			return nil, false, issue(fmt.Sprintf("must be <= %s", num(*f.Max)))
		}
		return fv, false, nil

	case schema.TypeBoolean:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, false, issue("must be a boolean")
		}
		return b, false, nil

	case schema.TypeDatetime:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false, issue("must be an ISO-8601 datetime string")
		}
		canon, err := store.CanonicalDatetime(s)
		if err != nil {
			return nil, false, issue(err.Error())
		}
		return canon, false, nil

	case schema.TypeEnum:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false, issue("must be a string")
		}
		for _, v := range f.Values {
			if v == s {
				return s, false, nil
			}
		}
		return nil, false, issue(fmt.Sprintf("must be one of %v", f.Values))

	case schema.TypeReference:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false, issue("must be a reference id string")
		}
		target := ra.Schema.EntityByID(f.Target)
		if target == nil {
			return nil, false, issue("reference target is not available")
		}
		ok, err := ra.Store.Exists(target, s)
		if err != nil {
			return nil, false, issue("could not verify reference target")
		}
		if !ok {
			return nil, false, issue(fmt.Sprintf("referenced %s %q does not exist", target.Name, s))
		}
		return s, false, nil

	default:
		return nil, false, issue("unsupported field type")
	}
}

func isJSONNull(raw json.RawMessage) bool {
	return len(raw) == 4 && string(raw) == "null"
}

func num(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}
