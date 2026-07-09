package client_test

import (
	"strings"
	"testing"

	"pocketknife/client"
	"pocketknife/validate"
)

const functionsManifest = `{
  "app": { "id": "smart_notes", "name": "Smart Notes", "version": 1 },
  "entities": [
    { "id": "ent_note", "name": "note", "fields": [
      { "id": "fld_text", "name": "text", "type": "text", "required": true }
    ]}
  ],
  "functions": [
    {
      "id": "fn_summarize",
      "name": "summarize",
      "prompt": "Summarize in a {{tone}} tone: {{text}}",
      "description": "Summarizes a note.",
      "capabilities": { "model": true }
    },
    {
      "id": "fn_greet",
      "name": "greet",
      "prompt": "Say hello.",
      "capabilities": { "model": true }
    },
    {
      "id": "fn_digest",
      "name": "digest",
      "entry": "functions/digest.wasm",
      "capabilities": { "data": [ { "entity": "ent_note", "operations": ["read"] } ] }
    }
  ]
}`

func TestGenerateFunctionsClient(t *testing.T) {
	app, errs := validate.Manifest([]byte(functionsManifest))
	if len(errs) > 0 {
		t.Fatalf("manifest failed validation: %v", errs)
	}
	out := string(client.Generate(app))

	wantSubstrings := []string{
		"// --- functions ---",
		// Params interface from the template's placeholders, first-appearance order.
		"export interface SummarizeParams {",
		"tone: string;",
		"text: string;",
		// Description becomes a doc comment.
		"/** Summarizes a note. */",
		// Parameterised prompt function.
		"async summarize(params: SummarizeParams): Promise<string> {",
		`"POST", "/apps/smart_notes/functions/summarize", params`,
		"return res.output;",
		// Static prompt function takes no arguments.
		"async greet(): Promise<string> {",
		`"POST", "/apps/smart_notes/functions/greet", {}`,
		// Wasm function stays untyped.
		"async digest(input: unknown): Promise<unknown> {",
		`"POST", "/apps/smart_notes/functions/digest", input`,
		// Sub-client wired into the root client.
		"export class SmartNotesFunctionsClient {",
		"readonly functions: SmartNotesFunctionsClient;",
		"this.functions = new SmartNotesFunctionsClient(baseUrl, fetchImpl);",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("generated client missing expected fragment: %s", want)
		}
	}

	// Determinism holds with functions present.
	if string(client.Generate(app)) != out {
		t.Fatal("Generate is not deterministic with functions declared")
	}
}

func TestGenerateWithoutFunctionsHasNoFunctionsClient(t *testing.T) {
	app, errs := validate.Manifest([]byte(tasksManifest))
	if len(errs) > 0 {
		t.Fatalf("manifest failed validation: %v", errs)
	}
	out := string(client.Generate(app))
	if strings.Contains(out, "FunctionsClient") || strings.Contains(out, "readonly functions:") {
		t.Error("an app with no functions must not emit a functions sub-client")
	}
}
