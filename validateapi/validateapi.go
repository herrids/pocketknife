// Package validateapi exposes a single stateless HTTP endpoint that validates a
// manifest and, on success, returns the typed TypeScript client generated from
// it. It is the dry-run authoring loop: an agent or tool iterating on a manifest
// can POST a candidate, get back either the structured validation errors to fix
// or the client.ts it can immediately build a frontend against — without
// touching any registry, database, build job or disk.
//
// The endpoint runs the exact same gate the rest of the runtime uses
// (validate.Manifest) and, on a valid manifest, the same generator the build
// pipeline would use (client.Generate). "Valid here" therefore means "will
// register, materialize and serve there", and the returned client is
// byte-identical to what a deploy of that manifest would produce.
package validateapi

import (
	"encoding/json"
	"io"
	"net/http"

	"pocketknife/client"
	"pocketknife/validate"
)

// maxManifestBytes caps the request body. Manifests are small declarative
// documents; anything larger is rejected rather than read.
const maxManifestBytes = 1 << 20

// NewServer returns an http.Handler serving POST /validate. It holds no state.
func NewServer() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /validate", handleValidate)
	return mux
}

// response is the success/validation-failure body. On a valid manifest Client
// holds the generated TypeScript source and Errors is empty; on an invalid
// manifest Errors holds the structured failures and Client is empty.
type response struct {
	Valid  bool            `json:"valid"`
	Client string          `json:"client,omitempty"`
	Errors validate.Errors `json:"errors,omitempty"`
}

func handleValidate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, maxManifestBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not read request body")
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is empty; expected a manifest JSON object")
		return
	}

	app, verrs := validate.Manifest(data)
	if len(verrs) > 0 {
		// The request succeeded; the manifest did not. 422 distinguishes a
		// well-formed request carrying an invalid manifest from a malformed one.
		writeJSON(w, http.StatusUnprocessableEntity, response{Valid: false, Errors: verrs})
		return
	}

	writeJSON(w, http.StatusOK, response{Valid: true, Client: string(client.Generate(app))})
}

// The error envelope mirrors the generic API's shape ({"error":{code,message}})
// so a caller sees one consistent failure body across the whole server.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: apiError{Code: code, Message: message}})
}
