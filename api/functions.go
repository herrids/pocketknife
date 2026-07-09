package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"pocketknife/broker"
	"pocketknife/funcrun"
	"pocketknife/sandbox"
)

// functionResult is the uniform success envelope for an invocation: the
// model's text for a prompt function (a JSON string), or the guest's bytes
// for a wasm function (embedded as JSON when they are valid JSON, as a
// string otherwise).
type functionResult struct {
	Output any `json:"output"`
}

// handleInvoke serves POST /apps/{app}/functions/{name}. The literal
// "functions" segment wins over the {entity} wildcard on the same mux, which
// is why the validator reserves "functions" as an entity name.
func (s *Server) handleInvoke(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("app")
	fnName := r.PathValue("name")

	ra, ok := s.reg.App(appID)
	if !ok {
		writeError(w, http.StatusNotFound, "app_not_found", "no app with id "+appID)
		return
	}
	fn := ra.Schema.Function(fnName)
	if fn == nil {
		writeError(w, http.StatusNotFound, "function_not_found", "no function "+fnName+" in app "+appID)
		return
	}
	if s.fns == nil {
		writeError(w, http.StatusServiceUnavailable, "functions_unavailable", "this server is not configured to run functions")
		return
	}

	input, err := io.ReadAll(io.LimitReader(r.Body, sandbox.DefaultMaxInputBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not read request body")
		return
	}
	if len(input) > sandbox.DefaultMaxInputBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "input_too_large", "request body exceeds the function input limit")
		return
	}

	out, err := s.fns.Run(r.Context(), ra, fn, input)
	if err != nil {
		writeInvokeError(w, fn.IsPrompt(), err)
		return
	}
	if out.Failed {
		writeError(w, http.StatusUnprocessableEntity, "function_failed",
			"function "+fnName+" reported a failure", string(out.Output))
		return
	}

	var body functionResult
	if fn.IsPrompt() {
		body.Output = string(out.Output)
	} else if json.Valid(out.Output) {
		body.Output = json.RawMessage(out.Output)
	} else {
		body.Output = string(out.Output)
	}
	writeJSON(w, http.StatusOK, body)
}

// writeInvokeError maps a Runner error onto the API's error envelope. The
// mapping never leaks server-side detail beyond the class of failure: the
// broker's provider errors, sandbox trap reasons and module paths stay in
// the server log domain, not the response.
func writeInvokeError(w http.ResponseWriter, isPrompt bool, err error) {
	var bie *funcrun.BadInputError
	switch {
	case errors.As(err, &bie):
		if len(bie.Issues) == 1 && bie.Issues[0].Param == "" {
			writeError(w, http.StatusBadRequest, "invalid_body", bie.Issues[0].Message)
			return
		}
		details := make([]any, len(bie.Issues))
		for i, is := range bie.Issues {
			details[i] = is
		}
		writeError(w, http.StatusBadRequest, "invalid_params", "request parameters failed validation", details...)
	case errors.Is(err, broker.ErrNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "model_not_configured", "no model provider is configured on this server")
	case errors.Is(err, funcrun.ErrLoad):
		writeError(w, http.StatusInternalServerError, "function_load_failed", "the function's module could not be loaded")
	case errors.Is(err, sandbox.ErrTimeout):
		writeError(w, http.StatusGatewayTimeout, "function_timeout", "the function ran past its time budget")
	case errors.Is(err, sandbox.ErrResourceExhausted), errors.Is(err, sandbox.ErrTrapped):
		writeError(w, http.StatusInternalServerError, "function_crashed", "the function crashed")
	case isPrompt:
		writeError(w, http.StatusBadGateway, "model_call_failed", "the model provider call failed")
	default:
		writeError(w, http.StatusInternalServerError, "function_crashed", "the function could not be run")
	}
}
