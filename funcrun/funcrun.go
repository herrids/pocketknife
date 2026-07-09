// Package funcrun executes one declared app function and is the only bridge
// between the HTTP surface and the two function runtimes. A prompt function
// never runs code: its request parameters are validated against the template's
// placeholders, rendered by verbatim substitution, and sent through the model
// broker — the broker (not this package, not the manifest) is what holds the
// provider credential, so nothing here ever sees a token. A wasm function is
// compiled once (cached by the sandbox) and invoked inside the sandbox's
// capability-checked boundary; funcrun adds no power beyond resolving the
// module path inside the app's own directory, which the validator has already
// constrained to a clean relative path.
package funcrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"pocketknife/broker"
	"pocketknife/registry"
	"pocketknife/sandbox"
	"pocketknife/schema"
)

// ErrLoad wraps a failure to read or compile a wasm function's module — a
// server-side deployment problem, distinct from anything the caller sent.
var ErrLoad = errors.New("funcrun: function module failed to load")

// ParamIssue is one problem with a prompt function's request parameters.
type ParamIssue struct {
	Param   string `json:"param"`
	Message string `json:"message"`
}

// BadInputError reports every parameter problem with a prompt-function
// request at once, mirroring how the generic API accumulates field issues.
type BadInputError struct {
	Issues []ParamIssue
}

func (e *BadInputError) Error() string {
	return fmt.Sprintf("funcrun: request parameters failed validation (%d issue(s))", len(e.Issues))
}

// Outcome is the result of a completed invocation. For a prompt function,
// Output is the model's text and Failed is always false (a broker failure is
// an error, not an outcome). For a wasm function, Output is whatever bytes
// the guest wrote and Failed mirrors the guest's own nonzero exit code.
type Outcome struct {
	Output []byte
	Failed bool
}

// Runner executes functions against a shared sandbox and broker. A nil
// broker is valid: prompt functions (and wasm model_call) then fail with
// broker.ErrNotConfigured, which the API maps to 503.
type Runner struct {
	sb  *sandbox.Sandbox
	brk *broker.Broker
}

// New builds a Runner over the given sandbox and broker.
func New(sb *sandbox.Sandbox, brk *broker.Broker) *Runner {
	return &Runner{sb: sb, brk: brk}
}

// Run executes fn — declared by ra's manifest — with the raw request body as
// input. Prompt functions expect a JSON object of string parameters matching
// the template's placeholders; wasm functions receive the body verbatim.
func (r *Runner) Run(ctx context.Context, ra *registry.RegisteredApp, fn *schema.Function, input []byte) (*Outcome, error) {
	if fn.IsPrompt() {
		return r.runPrompt(ctx, fn, input)
	}
	return r.runWasm(ctx, ra, fn, input)
}

func (r *Runner) runPrompt(ctx context.Context, fn *schema.Function, input []byte) (*Outcome, error) {
	values, err := decodePromptParams(fn, input)
	if err != nil {
		return nil, err
	}

	prompt := schema.RenderPrompt(fn.Prompt, values)
	text, err := r.brk.Call(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return &Outcome{Output: []byte(text)}, nil
}

// decodePromptParams checks the request body against the template's declared
// placeholders and accumulates every issue — missing, unknown and non-string
// parameters — so a caller sees all problems at once.
func decodePromptParams(fn *schema.Function, input []byte) (map[string]string, error) {
	params := fn.PromptParams()

	var raw map[string]json.RawMessage
	if len(input) == 0 {
		raw = map[string]json.RawMessage{}
	} else if err := json.Unmarshal(input, &raw); err != nil {
		return nil, &BadInputError{Issues: []ParamIssue{{Param: "", Message: "request body must be a JSON object of string parameters"}}}
	}

	declared := make(map[string]bool, len(params))
	for _, p := range params {
		declared[p] = true
	}

	var issues []ParamIssue
	values := make(map[string]string, len(params))

	for _, p := range params {
		rv, ok := raw[p]
		if !ok {
			issues = append(issues, ParamIssue{Param: p, Message: "is required"})
			continue
		}
		var s string
		if err := json.Unmarshal(rv, &s); err != nil {
			issues = append(issues, ParamIssue{Param: p, Message: "must be a string"})
			continue
		}
		values[p] = s
	}

	var unknown []string
	for k := range raw {
		if !declared[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		issues = append(issues, ParamIssue{Param: k, Message: "unknown parameter"})
	}

	if len(issues) > 0 {
		return nil, &BadInputError{Issues: issues}
	}
	return values, nil
}

func (r *Runner) runWasm(ctx context.Context, ra *registry.RegisteredApp, fn *schema.Function, input []byte) (*Outcome, error) {
	cm, err := r.sb.Compile(ctx, filepath.Join(ra.Dir, fn.Entry))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoad, err)
	}
	res, err := r.sb.Invoke(ctx, cm, fn, ra.Store, ra.Schema, r.brk, input)
	if err != nil {
		return nil, err
	}
	return &Outcome{Output: res.Output, Failed: res.Failed}, nil
}
