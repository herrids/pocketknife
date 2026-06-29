// exportServer is the read-only complement of the deploy ingest server: it
// exposes an app's current manifest and (when available) the stored editable
// frontend source so an external client can update the app. Both handlers are
// side-effect free — they read from the registry and build store and write
// nothing. They do not authenticate their caller; like POST /deploy that is a
// deliberate, separately-tracked gap.
package deployapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"pocketknife/build"
	"pocketknife/registry"
)

type exportServer struct {
	reg *registry.Registry
	bst *build.Store
}

// NewExportServer returns an http.Handler that serves:
//
//	GET /export/{appId}        — manifest JSON + hasSource bool
//	GET /export/{appId}/source — raw gzipped source tar (404 when absent)
func NewExportServer(reg *registry.Registry, bst *build.Store) http.Handler {
	s := &exportServer{reg: reg, bst: bst}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /export/{appId}", s.handleExport)
	mux.HandleFunc("GET /export/{appId}/source", s.handleSourceDownload)
	return mux
}

type exportResponse struct {
	Manifest  json.RawMessage `json:"manifest"`
	HasSource bool            `json:"hasSource"`
}

// handleExport returns the app's current manifest and whether stored source is
// available for it. It never returns an empty or fabricated source tar.
func (s *exportServer) handleExport(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	ra, ok := s.reg.App(appID)
	if !ok {
		writeError(w, http.StatusNotFound, "app_not_found", fmt.Sprintf("app %q not found", appID))
		return
	}

	manifestBytes, err := os.ReadFile(filepath.Join(ra.Dir, "manifest.json"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	ab, err := s.bst.ActiveBuildFor(appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	hasSource := false
	if ab != nil {
		_, hasSource = build.SourceDir(ra.Dir, ab.JobID)
	}

	writeJSON(w, http.StatusOK, exportResponse{
		Manifest:  json.RawMessage(manifestBytes),
		HasSource: hasSource,
	})
}

// handleSourceDownload streams the gzipped source tar for the app's active
// build, or returns 404 when no source was stored for that build.
func (s *exportServer) handleSourceDownload(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	ra, ok := s.reg.App(appID)
	if !ok {
		writeError(w, http.StatusNotFound, "app_not_found", fmt.Sprintf("app %q not found", appID))
		return
	}

	ab, err := s.bst.ActiveBuildFor(appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if ab == nil {
		writeError(w, http.StatusNotFound, "no_source", "app has no active build")
		return
	}

	sourceDir, exists := build.SourceDir(ra.Dir, ab.JobID)
	if !exists {
		writeError(w, http.StatusNotFound, "no_source", "no stored source for this app")
		return
	}

	data, err := build.PackSource(sourceDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
