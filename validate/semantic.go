package validate

import (
	"fmt"
	"path"
	"strings"

	"pocketknife/schema"
)

// semantic runs the checks that JSON Schema cannot express. It walks the parsed
// model and accumulates structured errors; it never stops at the first problem,
// so a caller sees every issue at once.
func semantic(app *schema.App) Errors {
	var errs Errors

	entityIDs := map[string]bool{}
	entityNames := map[string]bool{}

	for ei, ent := range app.Entities {
		epath := fmt.Sprintf("/entities/%d", ei)

		if entityIDs[ent.ID] {
			errs = append(errs, Error{epath + "/id", "duplicate_id", fmt.Sprintf("entity id %q is not unique", ent.ID)})
		}
		entityIDs[ent.ID] = true

		if entityNames[ent.Name] {
			errs = append(errs, Error{epath + "/name", "duplicate_name", fmt.Sprintf("entity name %q is not unique", ent.Name)})
		}
		entityNames[ent.Name] = true

		// An entity name becomes a URL segment under /apps/{app}/, so it must
		// not shadow a platform route. The id is checked too: a later rename
		// onto the id would otherwise smuggle the collision in.
		if isReservedEntity(ent.Name) {
			errs = append(errs, Error{epath + "/name", "reserved_name", fmt.Sprintf("entity name %q is reserved by the platform", ent.Name)})
		}
		if isReservedEntity(ent.ID) {
			errs = append(errs, Error{epath + "/id", "reserved_id", fmt.Sprintf("entity id %q is reserved by the platform", ent.ID)})
		}

		fieldIDs := map[string]bool{}
		fieldNames := map[string]bool{}

		for fi, f := range ent.Fields {
			fpath := fmt.Sprintf("%s/fields/%d", epath, fi)

			if fieldIDs[f.ID] {
				errs = append(errs, Error{fpath + "/id", "duplicate_id", fmt.Sprintf("field id %q is not unique within entity %q", f.ID, ent.Name)})
			}
			fieldIDs[f.ID] = true

			// A field id becomes a physical column name (D1), so it must not
			// collide with a platform-managed column.
			if isReserved(f.ID) {
				errs = append(errs, Error{fpath + "/id", "reserved_id", fmt.Sprintf("field id %q is reserved by the platform", f.ID)})
			}

			if fieldNames[f.Name] {
				errs = append(errs, Error{fpath + "/name", "duplicate_name", fmt.Sprintf("field name %q is not unique within entity %q", f.Name, ent.Name)})
			}
			fieldNames[f.Name] = true

			if isReserved(f.Name) {
				errs = append(errs, Error{fpath + "/name", "reserved_name", fmt.Sprintf("field name %q is reserved by the platform", f.Name)})
			}

			errs = append(errs, validateField(fpath, app, ent, f)...)
		}
	}

	fnIDs := map[string]bool{}
	fnNames := map[string]bool{}
	for fi, fn := range app.Functions {
		fpath := fmt.Sprintf("/functions/%d", fi)

		if fnIDs[fn.ID] {
			errs = append(errs, Error{fpath + "/id", "duplicate_id", fmt.Sprintf("function id %q is not unique", fn.ID)})
		}
		fnIDs[fn.ID] = true

		if fnNames[fn.Name] {
			errs = append(errs, Error{fpath + "/name", "duplicate_name", fmt.Sprintf("function name %q is not unique", fn.Name)})
		}
		fnNames[fn.Name] = true

		if fn.IsPrompt() {
			_, malformed := schema.ScanPrompt(fn.Prompt)
			for _, off := range malformed {
				errs = append(errs, Error{fpath + "/prompt", "malformed_placeholder", fmt.Sprintf("prompt of function %q contains a '{{' at byte %d that does not form a valid {{placeholder}} (there is no escaping)", fn.Name, off)})
			}
		} else {
			// The entry is joined onto the app directory and read at invoke
			// time, so it must stay inside the app directory.
			if path.IsAbs(fn.Entry) || fn.Entry != path.Clean(fn.Entry) || fn.Entry == ".." || strings.HasPrefix(fn.Entry, "../") {
				errs = append(errs, Error{fpath + "/entry", "bad_entry", fmt.Sprintf("function %q entry %q must be a clean relative path inside the app directory", fn.Name, fn.Entry)})
			}
		}

		errs = append(errs, validateCapabilities(fpath+"/capabilities", app, fn)...)
	}
	return errs
}

// validateCapabilities checks that every declared data scope resolves to a
// real entity in this manifest, is not repeated, and never requests an
// operation the entity itself does not allow — a scope the sandbox could
// never actually exercise is a manifest authoring mistake, not a runtime
// concern.
func validateCapabilities(path string, app *schema.App, fn *schema.Function) Errors {
	var errs Errors
	seenEntities := map[string]bool{}

	for di, ds := range fn.Capabilities.Data {
		dpath := fmt.Sprintf("%s/data/%d", path, di)

		if seenEntities[ds.Entity] {
			errs = append(errs, Error{dpath + "/entity", "duplicate_data_scope", fmt.Sprintf("entity %q is scoped more than once in function %q", ds.Entity, fn.Name)})
		}
		seenEntities[ds.Entity] = true

		ent := app.EntityByID(ds.Entity)
		if ent == nil {
			errs = append(errs, Error{dpath + "/entity", "unresolved_reference", fmt.Sprintf("data scope entity %q does not resolve to an entity in this manifest", ds.Entity)})
			continue
		}
		for _, op := range ds.Operations {
			if !ent.Allows(op) {
				errs = append(errs, Error{dpath + "/operations", "scope_exceeds_entity", fmt.Sprintf("function %q requests %q on entity %q, which the entity itself does not allow", fn.Name, op, ent.Name)})
			}
		}
	}

	seenDomains := map[string]bool{}
	for ni, d := range fn.Capabilities.Network {
		if seenDomains[d] {
			errs = append(errs, Error{fmt.Sprintf("%s/network/%d", path, ni), "duplicate_domain", fmt.Sprintf("domain %q is repeated", d)})
		}
		seenDomains[d] = true
	}
	return errs
}

func isReserved(name string) bool {
	for _, r := range schema.ReservedNames {
		if name == r {
			return true
		}
	}
	return false
}

func isReservedEntity(name string) bool {
	for _, r := range schema.ReservedEntityNames {
		if name == r {
			return true
		}
	}
	return false
}

// validateField checks min/max ordering, reference resolution, enum value
// integrity, and that any default satisfies the field's own constraints.
func validateField(path string, app *schema.App, ent *schema.Entity, f *schema.Field) Errors {
	var errs Errors

	if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
		errs = append(errs, Error{path, "bad_bounds", fmt.Sprintf("min (%v) is greater than max (%v)", *f.Min, *f.Max)})
	}

	switch f.Type {
	case schema.TypeReference:
		if app.EntityByID(f.Target) == nil {
			errs = append(errs, Error{path + "/target", "unresolved_reference", fmt.Sprintf("reference target %q does not resolve to an entity in this manifest", f.Target)})
		}
	case schema.TypeEnum:
		seen := map[string]bool{}
		for _, v := range f.Values {
			if seen[v] {
				errs = append(errs, Error{path + "/values", "duplicate_enum_value", fmt.Sprintf("enum value %q is repeated", v)})
			}
			seen[v] = true
		}
	}

	if f.HasDefault {
		errs = append(errs, validateDefault(path+"/default", f)...)
	}
	return errs
}

// validateDefault confirms a declared default would itself pass field
// validation: enum membership, length bounds (text) and value bounds
// (integer/real).
func validateDefault(path string, f *schema.Field) Errors {
	var errs Errors
	switch f.Type {
	case schema.TypeText:
		s, ok := f.Default.(string)
		if !ok {
			return Errors{{path, "bad_default", "default must be a string"}}
		}
		n := float64(len([]rune(s)))
		if f.Min != nil && n < *f.Min {
			errs = append(errs, Error{path, "bad_default", fmt.Sprintf("default length %d is below min %v", len([]rune(s)), *f.Min)})
		}
		if f.Max != nil && n > *f.Max {
			errs = append(errs, Error{path, "bad_default", fmt.Sprintf("default length %d exceeds max %v", len([]rune(s)), *f.Max)})
		}
	case schema.TypeInteger:
		n, ok := f.Default.(int64)
		if !ok {
			return Errors{{path, "bad_default", "default must be an integer"}}
		}
		if f.Min != nil && float64(n) < *f.Min {
			errs = append(errs, Error{path, "bad_default", fmt.Sprintf("default %d is below min %v", n, *f.Min)})
		}
		if f.Max != nil && float64(n) > *f.Max {
			errs = append(errs, Error{path, "bad_default", fmt.Sprintf("default %d exceeds max %v", n, *f.Max)})
		}
	case schema.TypeReal:
		n, ok := f.Default.(float64)
		if !ok {
			return Errors{{path, "bad_default", "default must be a number"}}
		}
		if f.Min != nil && n < *f.Min {
			errs = append(errs, Error{path, "bad_default", fmt.Sprintf("default %v is below min %v", n, *f.Min)})
		}
		if f.Max != nil && n > *f.Max {
			errs = append(errs, Error{path, "bad_default", fmt.Sprintf("default %v exceeds max %v", n, *f.Max)})
		}
	case schema.TypeEnum:
		s, ok := f.Default.(string)
		if !ok {
			return Errors{{path, "bad_default", "default must be a string"}}
		}
		found := false
		for _, v := range f.Values {
			if v == s {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, Error{path, "bad_default", fmt.Sprintf("enum default %q is not one of the declared values", s)})
		}
	}
	return errs
}
