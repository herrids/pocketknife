// Package assets serves an app's currently activated frontend bundle directly
// from the trusted core, so a production deployment is one static Go binary
// answering both the API and the UI from a single origin (no CORS needed).
// Every request resolves the app's AssetDir fresh from the registry — it is
// never cached or bound at server-start time — so an activation cutover (or a
// rollback) is visible to the very next request with no restart.
package assets

import (
	"net/http"
	"os"
	"path/filepath"

	"pocketknife/registry"
)

// NewServer builds the per-app static asset handler: GET /ui/{app}/{path...}.
// A path that does not match a real file under the app's AssetDir falls back
// to the frontend's declared entry file, so client-side routing in a
// single-page app works without the trusted core knowing any of its routes.
func NewServer(reg *registry.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ui/{app}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /ui/{app}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, reg, r.PathValue("app"), r.PathValue("path"))
	})
	return mux
}

func serve(w http.ResponseWriter, r *http.Request, reg *registry.Registry, appID, reqPath string) {
	ra, ok := reg.App(appID)
	if !ok || ra.AssetDir == "" {
		http.NotFound(w, r)
		return
	}

	entry := "index.html"
	if ra.Schema.Frontend != nil && ra.Schema.Frontend.Entry != "" {
		entry = ra.Schema.Frontend.Entry
	}

	// reqPath is the remainder past /ui/{app}/, already cleaned by ServeMux
	// (which redirects unclean paths before this handler runs). Re-clean
	// defensively and root it before joining, so it can never escape AssetDir.
	full := filepath.Join(ra.AssetDir, filepath.Clean("/"+reqPath))
	if info, err := os.Stat(full); err != nil || info.IsDir() {
		full = filepath.Join(ra.AssetDir, entry)
	}
	http.ServeFile(w, r, full)
}
