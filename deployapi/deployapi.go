// Package deployapi is the ingest side of the agent-to-backend wire: it
// receives an approved app -- a manifest plus its already-built frontend
// bundle, keyed on the agent's job id -- and lands it as a live, reachable
// app through the existing validate/materialize/build/registry machinery.
// It introduces exactly one new capability the rest of the runtime lacked:
// bootstrapping a brand-new app id that the registry has never seen
// (build.Bootstrap); an already-known app id is redeployed through the
// existing build.Deploy. The endpoint is idempotent on the caller's job id
// and serializes concurrent requests for the same app id, but does not
// authenticate its caller -- that is a deliberate, separately-tracked gap,
// not an oversight.
package deployapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sync"

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
// build-job store it deploys into, the apps directory new apps are created
// under, and a per-app-id lock so two requests for the same app never race.
type Server struct {
	reg     *registry.Registry
	bst     *build.Store
	appsDir string

	mu       sync.Mutex
	appLocks map[string]*sync.Mutex
}

// NewServer returns an http.Handler serving POST /deploy against reg and bst.
func NewServer(reg *registry.Registry, bst *build.Store, appsDir string) http.Handler {
	s := &Server{reg: reg, bst: bst, appsDir: appsDir, appLocks: map[string]*sync.Mutex{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /deploy", s.handleDeploy)
	return mux
}

func (s *Server) lockFor(appID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.appLocks[appID]
	if !ok {
		l = &sync.Mutex{}
		s.appLocks[appID] = l
	}
	return l
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

	lock := s.lockFor(app.ID)
	lock.Lock()
	defer lock.Unlock()

	if ra, ok := s.reg.App(app.ID); ok {
		err = s.redeploy(ra, app.Frontend.Dist, manifestBytes, req.Bundle, req.Source)
	} else {
		err = s.firstInstall(app.ID, manifestBytes, req.Bundle, req.Source)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "deploy_failed", err.Error())
		return
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

// firstInstall routes an unknown app id through build.Bootstrap, which owns
// staging, materializing and registering the brand-new app. If source is
// non-nil it is stored as an opaque artifact keyed on the resulting job id.
func (s *Server) firstInstall(appID string, manifestBytes []byte, bundle io.Reader, source multipart.File) error {
	res, err := build.Bootstrap(s.reg, s.bst, s.appsDir, manifestBytes, bundle)
	if err != nil || source == nil || res == nil || res.Job == nil {
		return err
	}
	// Bootstrap registered the app; resolve its dir from the live registry.
	if ra, ok := s.reg.App(appID); ok {
		_ = build.StoreSource(ra.Dir, res.Job.ID, source)
	}
	return nil
}

// redeploy writes the new bundle into the already-registered app's directory
// and routes the deploy through build.Deploy, which decides install-vs-data-
// migration by manifest version and owns the single rollback contract. If
// source is non-nil it is stored after a successful deploy.
func (s *Server) redeploy(ra *registry.RegisteredApp, distRel string, manifestBytes []byte, bundle io.Reader, source multipart.File) error {
	distDir := filepath.Join(ra.Dir, distRel)
	if err := os.RemoveAll(distDir); err != nil {
		return fmt.Errorf("clear previous bundle: %w", err)
	}
	if err := build.ExtractBundle(bundle, distDir); err != nil {
		return fmt.Errorf("extract frontend bundle: %w", err)
	}
	res, err := build.Deploy(context.Background(), s.reg, s.bst, ra.Schema.ID, manifestBytes, build.DeployOptions{})
	if err != nil || source == nil || res == nil || res.Job == nil {
		return err
	}
	_ = build.StoreSource(ra.Dir, res.Job.ID, source)
	return nil
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
