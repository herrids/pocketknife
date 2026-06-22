package build

import (
	"encoding/json"
	"net/http"

	"pocketknife/registry"
)

// apiError and errorEnvelope mirror api's error body shape byte-for-byte
// (api.apiError / api.errorEnvelope) so a client sees one consistent envelope
// across every route this binary serves. They are kept as a separate,
// unexported copy rather than an import: api's surface is its own frozen
// contract, and build's status routes are a distinct concern with their own
// evolution.
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
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: apiError{Code: code, Message: message}})
}

// NewStatusServer builds the read-only build-status HTTP handler: every build
// job for an app, and the durable activation pointer (if any). This is
// observability only — nothing here starts, cancels or streams a build; a
// future shell polls it.
func NewStatusServer(bst *Store, reg *registry.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /builds/job/{id}", func(w http.ResponseWriter, r *http.Request) {
		handleGetJob(w, r, bst)
	})
	mux.HandleFunc("GET /builds/{app}", func(w http.ResponseWriter, r *http.Request) {
		handleListForApp(w, r, bst, reg)
	})
	return mux
}

func handleListForApp(w http.ResponseWriter, r *http.Request, bst *Store, reg *registry.Registry) {
	appID := r.PathValue("app")
	if _, ok := reg.App(appID); !ok {
		writeError(w, http.StatusNotFound, "app_not_found", "no app with id "+appID)
		return
	}
	jobs, err := bst.ListForApp(appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	active, err := bst.ActiveBuildFor(appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "active": active})
}

func handleGetJob(w http.ResponseWriter, r *http.Request, bst *Store) {
	id := r.PathValue("id")
	job, err := bst.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "job_not_found", "no build job with id "+id)
		return
	}
	writeJSON(w, http.StatusOK, job)
}
