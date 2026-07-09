package validate_test

import (
	"fmt"
	"testing"
)

// manifestWithFunctions wraps one or more function declarations in an
// otherwise-valid manifest with a single "note" entity (id ent_note).
func manifestWithFunctions(fns string) string {
	return fmt.Sprintf(`{
      "app": { "id": "a", "name": "A", "version": 1 },
      "entities": [
        { "id": "ent_note", "name": "note", "operations": ["create", "read"], "fields": [
          { "id": "f_text", "name": "text", "type": "text", "required": true }
        ]}
      ],
      "functions": [%s]
    }`, fns)
}

func TestValidPromptFunction(t *testing.T) {
	app := mustValid(t, manifestWithFunctions(`{
      "id": "fn_summarize",
      "name": "summarize",
      "prompt": "Summarize the following note in a {{tone}} tone: {{text}}",
      "description": "Summarizes a note.",
      "capabilities": { "model": true }
    }`))

	fn := app.Function("summarize")
	if fn == nil {
		t.Fatal("prompt function missing from model")
	}
	if !fn.IsPrompt() {
		t.Error("IsPrompt() = false")
	}
	if fn.Description != "Summarizes a note." {
		t.Errorf("Description = %q", fn.Description)
	}
	if got := fn.PromptParams(); len(got) != 2 || got[0] != "tone" || got[1] != "text" {
		t.Errorf("PromptParams() = %v", got)
	}
	if fn.Capabilities == nil || !fn.Capabilities.Model {
		t.Error("prompt function must carry the model capability")
	}
}

func TestValidWasmFunction(t *testing.T) {
	app := mustValid(t, manifestWithFunctions(`{
      "id": "fn_digest",
      "name": "digest",
      "entry": "functions/digest.wasm",
      "capabilities": {
        "data": [ { "entity": "ent_note", "operations": ["read"] } ],
        "network": ["api.example.com"],
        "model": true
      }
    }`))
	fn := app.Function("digest")
	if fn == nil || fn.IsPrompt() {
		t.Fatalf("wasm function missing or misclassified: %+v", fn)
	}
}

func TestRejectsFunctionWithBothEntryAndPrompt(t *testing.T) {
	errs := mustInvalid(t, manifestWithFunctions(`{
      "id": "fn_x", "name": "x",
      "entry": "functions/x.wasm",
      "prompt": "hi",
      "capabilities": { "model": true }
    }`))
	if !hasCode(errs, "structural") {
		t.Fatalf("expected structural error, got %v", errs)
	}
}

func TestRejectsFunctionWithNeitherEntryNorPrompt(t *testing.T) {
	errs := mustInvalid(t, manifestWithFunctions(`{
      "id": "fn_x", "name": "x",
      "capabilities": { "model": true }
    }`))
	if !hasCode(errs, "structural") {
		t.Fatalf("expected structural error, got %v", errs)
	}
}

func TestRejectsPromptFunctionWithoutModelTrue(t *testing.T) {
	for _, caps := range []string{
		`{ "model": false }`,
		`{}`,
		`{ "data": [ { "entity": "ent_note", "operations": ["read"] } ], "model": true }`,
		`{ "network": ["api.example.com"], "model": true }`,
	} {
		errs := mustInvalid(t, manifestWithFunctions(fmt.Sprintf(`{
          "id": "fn_x", "name": "x", "prompt": "hi",
          "capabilities": %s
        }`, caps)))
		if !hasCode(errs, "structural") {
			t.Fatalf("capabilities %s: expected structural error, got %v", caps, errs)
		}
	}
}

func TestRejectsMalformedPlaceholder(t *testing.T) {
	for _, prompt := range []string{
		"hello {{name",
		"hello {{Name}}",
		"hello {{}}",
	} {
		errs := mustInvalid(t, manifestWithFunctions(fmt.Sprintf(`{
          "id": "fn_x", "name": "x", "prompt": %q,
          "capabilities": { "model": true }
        }`, prompt)))
		if !hasCode(errs, "malformed_placeholder") {
			t.Fatalf("prompt %q: expected malformed_placeholder, got %v", prompt, errs)
		}
	}
}

func TestRejectsBadWasmEntryPath(t *testing.T) {
	for _, entry := range []string{
		"/etc/passwd",
		"../outside.wasm",
		"functions/../../outside.wasm",
		"..",
	} {
		errs := mustInvalid(t, manifestWithFunctions(fmt.Sprintf(`{
          "id": "fn_x", "name": "x", "entry": %q,
          "capabilities": { "model": true }
        }`, entry)))
		if !hasCode(errs, "bad_entry") {
			t.Fatalf("entry %q: expected bad_entry, got %v", entry, errs)
		}
	}
}

func TestRejectsDuplicateFunctionIDAndName(t *testing.T) {
	errs := mustInvalid(t, manifestWithFunctions(`
      { "id": "fn_x", "name": "x", "prompt": "a", "capabilities": { "model": true } },
      { "id": "fn_x", "name": "x", "prompt": "b", "capabilities": { "model": true } }
    `))
	if !hasCode(errs, "duplicate_id") || !hasCode(errs, "duplicate_name") {
		t.Fatalf("expected duplicate_id and duplicate_name, got %v", errs)
	}
}

func TestRejectsWasmDataScopeProblems(t *testing.T) {
	errs := mustInvalid(t, manifestWithFunctions(`{
      "id": "fn_x", "name": "x", "entry": "x.wasm",
      "capabilities": { "data": [
        { "entity": "ent_missing", "operations": ["read"] },
        { "entity": "ent_note", "operations": ["delete"] },
        { "entity": "ent_note", "operations": ["read"] }
      ]}
    }`))
	if !hasCode(errs, "unresolved_reference") {
		t.Errorf("expected unresolved_reference, got %v", errs)
	}
	if !hasCode(errs, "scope_exceeds_entity") {
		t.Errorf("expected scope_exceeds_entity (delete not enabled on entity), got %v", errs)
	}
	if !hasCode(errs, "duplicate_data_scope") {
		t.Errorf("expected duplicate_data_scope, got %v", errs)
	}
}

func TestRejectsReservedEntityName(t *testing.T) {
	errs := mustInvalid(t, `{
      "app": { "id": "a", "name": "A", "version": 1 },
      "entities": [
        { "id": "functions", "name": "functions", "fields": [
          { "id": "f_x", "name": "x", "type": "text" }
        ]}
      ]
    }`)
	if !hasCode(errs, "reserved_name") {
		t.Errorf("expected reserved_name for entity named functions, got %v", errs)
	}
	if !hasCode(errs, "reserved_id") {
		t.Errorf("expected reserved_id for entity id functions, got %v", errs)
	}
}
