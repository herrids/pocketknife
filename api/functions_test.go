package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pocketknife/api"
	"pocketknife/broker"
	"pocketknife/funcrun"
	"pocketknife/registry"
)

// functionAppManifest declares one entity and two prompt functions, one with
// parameters and one static.
const functionAppManifest = `{
  "app": { "id": "smartnotes", "name": "Smart Notes", "version": 1 },
  "entities": [
    { "id": "ent_note", "name": "note", "fields": [
      { "id": "f_text", "name": "text", "type": "text", "required": true }
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
    }
  ]
}`

// recordingCaller is a broker.Caller stub that records the rendered prompt.
type recordingCaller struct {
	prompt string
	text   string
	err    error
}

func (c *recordingCaller) Call(ctx context.Context, prompt string) (string, error) {
	c.prompt = prompt
	return c.text, c.err
}

// bootFunctionApp loads the prompt-function manifest into a fresh registry
// and serves it with a real Runner over the given caller.
func bootFunctionApp(t *testing.T, caller broker.Caller) (*httptest.Server, *registry.Registry) {
	t.Helper()
	dir := t.TempDir()
	appDir := filepath.Join(dir, "smartnotes")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "manifest.json"), []byte(functionAppManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, results, err := registry.Load(dir)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	for _, r := range results {
		if !r.OK {
			t.Fatalf("app failed to load: errors=%v err=%v", r.Errors, r.Err)
		}
	}

	runner := funcrun.New(nil, broker.New(caller))
	srv := httptest.NewServer(api.NewServer(reg, runner))
	t.Cleanup(func() {
		srv.Close()
		reg.Close()
	})
	return srv, reg
}

func errCode(t *testing.T, r resp) string {
	t.Helper()
	e, ok := r.body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected an error envelope, got %s", r.raw)
	}
	code, _ := e["code"].(string)
	return code
}

func TestInvokePromptFunctionRoundTrip(t *testing.T) {
	caller := &recordingCaller{text: "a fine summary"}
	srv, _ := bootFunctionApp(t, caller)

	r := do(t, srv, http.MethodPost, "/apps/smartnotes/functions/summarize",
		map[string]any{"tone": "cheerful", "text": "long day"})
	if r.status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", r.status, r.raw)
	}
	if r.body["output"] != "a fine summary" {
		t.Errorf("output = %v", r.body["output"])
	}
	if caller.prompt != "Summarize in a cheerful tone: long day" {
		t.Errorf("rendered prompt = %q", caller.prompt)
	}
}

func TestInvokeErrorTable(t *testing.T) {
	t.Run("unknown app", func(t *testing.T) {
		srv, _ := bootFunctionApp(t, &recordingCaller{text: "x"})
		r := do(t, srv, http.MethodPost, "/apps/ghost/functions/summarize", map[string]any{})
		if r.status != http.StatusNotFound || errCode(t, r) != "app_not_found" {
			t.Fatalf("got %d %s", r.status, r.raw)
		}
	})

	t.Run("unknown function", func(t *testing.T) {
		srv, _ := bootFunctionApp(t, &recordingCaller{text: "x"})
		r := do(t, srv, http.MethodPost, "/apps/smartnotes/functions/ghost", map[string]any{})
		if r.status != http.StatusNotFound || errCode(t, r) != "function_not_found" {
			t.Fatalf("got %d %s", r.status, r.raw)
		}
	})

	t.Run("missing and unknown params", func(t *testing.T) {
		srv, _ := bootFunctionApp(t, &recordingCaller{text: "x"})
		r := do(t, srv, http.MethodPost, "/apps/smartnotes/functions/summarize",
			map[string]any{"tone": "flat", "surprise": true})
		if r.status != http.StatusBadRequest || errCode(t, r) != "invalid_params" {
			t.Fatalf("got %d %s", r.status, r.raw)
		}
		e := r.body["error"].(map[string]any)
		details, _ := e["details"].([]any)
		if len(details) != 2 {
			t.Fatalf("expected 2 issues (missing text, unknown surprise), got %s", r.raw)
		}
	})

	t.Run("non-object body", func(t *testing.T) {
		srv, _ := bootFunctionApp(t, &recordingCaller{text: "x"})
		r := do(t, srv, http.MethodPost, "/apps/smartnotes/functions/summarize", "not an object")
		if r.status != http.StatusBadRequest || errCode(t, r) != "invalid_body" {
			t.Fatalf("got %d %s", r.status, r.raw)
		}
	})

	t.Run("broker not configured", func(t *testing.T) {
		srv, _ := bootFunctionApp(t, nil)
		r := do(t, srv, http.MethodPost, "/apps/smartnotes/functions/greet", map[string]any{})
		if r.status != http.StatusServiceUnavailable || errCode(t, r) != "model_not_configured" {
			t.Fatalf("got %d %s", r.status, r.raw)
		}
	})

	t.Run("provider failure", func(t *testing.T) {
		srv, _ := bootFunctionApp(t, &recordingCaller{err: errors.New("provider exploded: key sk-secret")})
		r := do(t, srv, http.MethodPost, "/apps/smartnotes/functions/greet", map[string]any{})
		if r.status != http.StatusBadGateway || errCode(t, r) != "model_call_failed" {
			t.Fatalf("got %d %s", r.status, r.raw)
		}
		// The provider's error text (which may carry sensitive detail) must
		// not reach the response.
		if strings.Contains(string(r.raw), "sk-secret") || strings.Contains(string(r.raw), "exploded") {
			t.Fatalf("response leaked provider error detail: %s", r.raw)
		}
	})

	t.Run("nil runner answers 503 for a declared function", func(t *testing.T) {
		dir := t.TempDir()
		appDir := filepath.Join(dir, "smartnotes")
		if err := os.MkdirAll(appDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(appDir, "manifest.json"), []byte(functionAppManifest), 0o644); err != nil {
			t.Fatal(err)
		}
		reg, _, err := registry.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(api.NewServer(reg, nil))
		t.Cleanup(func() {
			srv.Close()
			reg.Close()
		})

		r := do(t, srv, http.MethodPost, "/apps/smartnotes/functions/greet", map[string]any{})
		if r.status != http.StatusServiceUnavailable || errCode(t, r) != "functions_unavailable" {
			t.Fatalf("got %d %s", r.status, r.raw)
		}
	})
}

// TestInvokeRoutePrecedence proves the literal "functions" segment does not
// swallow entity routes and vice versa: entity CRUD still works, and the
// invoke route is not interpreted as create-on-entity-"functions".
func TestInvokeRoutePrecedence(t *testing.T) {
	srv, _ := bootFunctionApp(t, &recordingCaller{text: "ok"})

	// Entity create still routes to the CRUD handler.
	r := do(t, srv, http.MethodPost, "/apps/smartnotes/note", map[string]any{"text": "hello"})
	if r.status != http.StatusCreated {
		t.Fatalf("entity create: got %d %s", r.status, r.raw)
	}

	// The invoke path routes to the function handler, not entity create.
	r = do(t, srv, http.MethodPost, "/apps/smartnotes/functions/greet", map[string]any{})
	if r.status != http.StatusOK || r.body["output"] != "ok" {
		t.Fatalf("invoke: got %d %s", r.status, r.raw)
	}
}
