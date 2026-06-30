// Package shellserve serves the compiled shell SPA at the root path with SPA
// fallback: any path that doesn't match a file on disk returns index.html so
// the React Router can handle client-side routing. If distDir does not exist
// (e.g. the shell hasn't been built yet) requests to / return a 503 with a
// short human-readable message instead of crashing.
package shellserve

import (
	"net/http"
	"os"
	"path/filepath"
)

// NewServer returns an http.Handler that serves distDir at /.
func NewServer(distDir string) http.Handler {
	return &spaServer{dir: distDir}
}

type spaServer struct {
	dir string
}

func (s *spaServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if the dist directory exists at all.
	if _, err := os.Stat(s.dir); os.IsNotExist(err) {
		http.Error(w, "shell not built — run make shell-build", http.StatusServiceUnavailable)
		return
	}

	path := filepath.Join(s.dir, filepath.Clean("/"+r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	// SPA fallback: serve index.html for all unmatched paths.
	http.ServeFile(w, r, filepath.Join(s.dir, "index.html"))
}
