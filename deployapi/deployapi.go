// Package deployapi is the ingest side of the agent-to-backend wire: it
// receives an approved app -- a manifest plus its already-built frontend
// bundle, keyed on the agent's job id -- and lands it as a live, reachable
// app through the existing validate/materialize/build/registry machinery,
// via the one safe public operation for that, build.ApplyDeployment. It
// never decides Bootstrap-vs-Deploy itself and carries no locking of its
// own -- ApplyDeployment owns both, shared with every other in-process
// caller. The endpoint is idempotent on the caller's job id. It does not
// authenticate its caller -- that is a deliberate, separately-tracked gap,
// not an oversight.
package deployapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"pocketknife/build"
	"pocketknife/registry"
	"pocketknife/validate"
)

const (
	maxManifestBytes int64 = 1 << 20  // 1 MiB, same cap as validateapi
	maxBundleBytes   int64 = 64 << 20 // 64 MiB gzipped upload cap
	maxRequestBytes  int64 = maxManifestBytes + maxBundleBytes + (1 << 16)
)

// Server is the POST /deploy handler's state: the live registry and platform
// build-job store it deploys into, and the apps directory new apps are
// created under.
type Server struct {
	reg     *registry.Registry
	bst     *build.Store
	appsDir string
}

// NewServer returns an http.Handler serving POST /deploy against reg and bst.
func NewServer(reg *registry.Registry, bst *build.Store, appsDir string) http.Handler {
	s := &Server{reg: reg, bst: bst, appsDir: appsDir}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /deploy", s.handleDeploy)
	return mux
}

type response struct {
	AppID   string `json:"appId"`
	Version int    `json:"version"`
	JobID   string `json:"jobId"`
	URL     string `json:"url"`
}

func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)

	req, err := parseRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	defer req.Close()

	if rec, err := s.bst.DeployRequestByExternalID(req.JobID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	} else if rec != nil {
		// Idempotent retry: the same jobId already produced a ready,
		// activated build -- return that result, deploy nothing again.
		writeJSON(w, http.StatusOK, response{AppID: rec.AppID, Version: rec.ManifestVersion, JobID: req.JobID, URL: rec.URL})
		return
	}

	manifestBytes, err := ensureFrontendPointer(req.Manifest)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_manifest", err.Error())
		return
	}

	app, verrs := validate.Manifest(manifestBytes)
	if len(verrs) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "manifest_invalid", verrs.Error())
		return
	}

	// ApplyDeployment owns the per-app lock and the Bootstrap-vs-Deploy
	// decision atomically; this package never makes that decision itself.
	res, err := build.ApplyDeployment(context.Background(), s.reg, s.bst, s.appsDir, manifestBytes, req.Bundle, build.DeployOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "deploy_failed", err.Error())
		return
	}
	if req.Source != nil && res.Job != nil {
		if ra, ok := s.reg.App(app.ID); ok {
			_ = build.StoreSource(ra.Dir, res.Job.ID, req.Source)
		}
	}

	url := "/ui/" + app.ID + "/"
	if err := s.bst.RecordDeployRequest(build.DeployRecord{
		ExternalJobID:   req.JobID,
		AppID:           app.ID,
		ManifestVersion: app.Version,
		URL:             url,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Register display metadata so the launcher grid always has a row for this app.
	if err := s.bst.EnsureAppMeta(app.ID, app.Name, app.Emoji, app.Color); err != nil {
		// Non-fatal: the deploy succeeded; the launcher will show a default row.
		log.Printf("warning: ensure app_meta for %q after deploy: %v", app.ID, err)
	}

	writeJSON(w, http.StatusOK, response{AppID: app.ID, Version: app.Version, JobID: req.JobID, URL: url})
}

// The error envelope mirrors the rest of the server's shape
// ({"error":{code,message}}) so a caller sees one consistent failure body
// across the whole binary. Kept as a separate, unexported copy rather than an
// import, matching api's and validateapi's own convention.
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
