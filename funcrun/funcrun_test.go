package funcrun_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"pocketknife/broker"
	"pocketknife/funcrun"
	"pocketknife/registry"
	"pocketknife/sandbox"
	"pocketknife/schema"
)

// guestWasmPath is set by TestMain. The wasm tests reuse the sandbox
// package's guest fixture — the wire protocol between host and guest is the
// sandbox's contract, and funcrun sits strictly above it.
var guestWasmPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pocketknife-funcrun-test-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "funcrun_test: make temp dir:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	guestWasmPath = filepath.Join(dir, "driver.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", guestWasmPath, "./driver")
	cmd.Dir = "../sandbox/testdata/guestsrc"
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "funcrun_test: build guest fixture: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// stubCaller returns canned text and records the rendered prompt it was
// handed, so tests can assert on the substitution result.
type stubCaller struct {
	prompt string
	text   string
	err    error
}

func (s *stubCaller) Call(ctx context.Context, prompt string) (string, error) {
	s.prompt = prompt
	return s.text, s.err
}

func promptFn() *schema.Function {
	return &schema.Function{
		ID:           "fn_summarize",
		Name:         "summarize",
		Prompt:       "Summarize in a {{tone}} tone: {{text}}",
		Capabilities: &schema.Capabilities{Model: true},
	}
}

func promptApp(fn *schema.Function) *registry.RegisteredApp {
	return &registry.RegisteredApp{
		Schema: &schema.App{ID: "a", Name: "A", Version: 1, Functions: []*schema.Function{fn}},
	}
}

func TestPromptFunctionRendersAndCallsBroker(t *testing.T) {
	stub := &stubCaller{text: "a fine summary"}
	r := funcrun.New(nil, broker.New(stub))

	fn := promptFn()
	out, err := r.Run(context.Background(), promptApp(fn), fn,
		[]byte(`{"tone": "cheerful", "text": "long day"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Failed {
		t.Error("prompt outcome must not be Failed")
	}
	if string(out.Output) != "a fine summary" {
		t.Errorf("output = %q", out.Output)
	}
	if stub.prompt != "Summarize in a cheerful tone: long day" {
		t.Errorf("rendered prompt = %q", stub.prompt)
	}
}

func TestPromptFunctionAccumulatesAllParamIssues(t *testing.T) {
	r := funcrun.New(nil, broker.New(&stubCaller{text: "x"}))
	fn := promptFn()

	_, err := r.Run(context.Background(), promptApp(fn), fn,
		[]byte(`{"tone": 3, "extra": "nope"}`))
	var bie *funcrun.BadInputError
	if !errors.As(err, &bie) {
		t.Fatalf("expected BadInputError, got %v", err)
	}
	// tone must be a string, text is required, extra is unknown — all at once.
	if len(bie.Issues) != 3 {
		t.Fatalf("expected 3 issues, got %+v", bie.Issues)
	}
	got := map[string]string{}
	for _, is := range bie.Issues {
		got[is.Param] = is.Message
	}
	if got["tone"] != "must be a string" || got["text"] != "is required" || got["extra"] != "unknown parameter" {
		t.Errorf("issues = %+v", bie.Issues)
	}
}

func TestPromptFunctionNonObjectBodyIsBadInput(t *testing.T) {
	r := funcrun.New(nil, broker.New(&stubCaller{text: "x"}))
	fn := promptFn()

	var bie *funcrun.BadInputError
	_, err := r.Run(context.Background(), promptApp(fn), fn, []byte(`"just a string"`))
	if !errors.As(err, &bie) {
		t.Fatalf("expected BadInputError for a non-object body, got %v", err)
	}
}

func TestPromptFunctionWithoutBrokerIsNotConfigured(t *testing.T) {
	r := funcrun.New(nil, broker.New(nil))
	fn := &schema.Function{
		ID: "fn_static", Name: "static", Prompt: "Say hello.",
		Capabilities: &schema.Capabilities{Model: true},
	}

	_, err := r.Run(context.Background(), promptApp(fn), fn, []byte(`{}`))
	if !errors.Is(err, broker.ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestPromptFunctionEmptyBodyAllowedForStaticPrompt(t *testing.T) {
	stub := &stubCaller{text: "hello"}
	r := funcrun.New(nil, broker.New(stub))
	fn := &schema.Function{
		ID: "fn_static", Name: "static", Prompt: "Say hello.",
		Capabilities: &schema.Capabilities{Model: true},
	}

	out, err := r.Run(context.Background(), promptApp(fn), fn, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out.Output) != "hello" || stub.prompt != "Say hello." {
		t.Errorf("output = %q, prompt = %q", out.Output, stub.prompt)
	}
}

// wasmApp stages the compiled guest fixture into a temp app dir so Runner
// resolves the entry relative to RegisteredApp.Dir, exactly as in production.
func wasmApp(t *testing.T) (*registry.RegisteredApp, *schema.Function) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "functions"), 0o755); err != nil {
		t.Fatal(err)
	}
	wasm, err := os.ReadFile(guestWasmPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "functions", "driver.wasm"), wasm, 0o644); err != nil {
		t.Fatal(err)
	}

	fn := &schema.Function{
		ID: "fn_driver", Name: "driver", Entry: "functions/driver.wasm",
		Capabilities: &schema.Capabilities{},
	}
	app := &schema.App{ID: "a", Name: "A", Version: 1, Functions: []*schema.Function{fn}}
	return &registry.RegisteredApp{Schema: app, Dir: dir}, fn
}

func newSandbox(t *testing.T) *sandbox.Sandbox {
	t.Helper()
	sb, err := sandbox.New(sandbox.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sb.Close(context.Background()) })
	return sb
}

func TestWasmFunctionEchoRoundTrip(t *testing.T) {
	r := funcrun.New(newSandbox(t), broker.New(nil))
	ra, fn := wasmApp(t)

	input, _ := json.Marshal(map[string]any{"action": "echo", "request": json.RawMessage(`{"hi":"there"}`)})
	out, err := r.Run(context.Background(), ra, fn, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Failed {
		t.Error("echo must not fail")
	}
	if string(out.Output) != `{"hi":"there"}` {
		t.Errorf("output = %q", out.Output)
	}
}

func TestWasmFunctionGuestFailureIsAnOutcome(t *testing.T) {
	r := funcrun.New(newSandbox(t), broker.New(nil))
	ra, fn := wasmApp(t)

	out, err := r.Run(context.Background(), ra, fn, []byte(`{"action": "no_such_action"}`))
	if err != nil {
		t.Fatalf("guest failure must be an outcome, not an error: %v", err)
	}
	if !out.Failed {
		t.Error("expected Failed for a guest-reported failure")
	}
}

func TestWasmFunctionMissingModuleIsErrLoad(t *testing.T) {
	r := funcrun.New(newSandbox(t), broker.New(nil))
	ra, fn := wasmApp(t)
	fn.Entry = "functions/absent.wasm"

	_, err := r.Run(context.Background(), ra, fn, []byte(`{}`))
	if !errors.Is(err, funcrun.ErrLoad) {
		t.Fatalf("expected ErrLoad, got %v", err)
	}
}
