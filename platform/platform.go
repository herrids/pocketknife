// Package platform serves the shell's backend: app registry metadata, session
// authentication, and the agent bridge. All routes live under /platform/.
// Auth is required on all /platform/ routes except /platform/auth/login and
// /platform/auth/logout. App API and UI routes (/apps/, /ui/, etc.) are
// intentionally left open in this phase (single-user localhost).
package platform

import (
	"encoding/json"
	"io"
	"net/http"

	"pocketknife/build"
	"pocketknife/registry"
)

// NewServer returns an http.Handler that serves all /platform/ routes. It
// initialises the admin password (from env or generated) at call time, so any
// generated password is printed before the server starts accepting connections.
// addr is the address this server itself listens on (e.g. ":8080"); it is
// passed to spawned agent subprocesses as GO_BASE_URL so they can call back
// into this server without needing that value configured separately.
func NewServer(bst *build.Store, reg *registry.Registry, agentBin, addr string) (http.Handler, error) {
	auth, err := newAuthState()
	if err != nil {
		return nil, err
	}

	inner := http.NewServeMux()

	// Auth endpoints (exempt from the auth guard).
	inner.HandleFunc("/platform/auth/login", auth.handleLogin)
	inner.HandleFunc("/platform/auth/logout", auth.handleLogout)

	// Registry API.
	rs := &registryServer{bst: bst, reg: reg}
	rs.route(inner)

	// Agent bridge.
	ps := newPlanServer(agentBin, addr)
	ps.route(inner)

	// Wrap the whole mux with the auth guard.
	return auth.authMiddleware(inner), nil
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a standard {"error":{"code":"...","message":"..."}} body.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// decodeJSON decodes r.Body into dst (max 1 MiB).
func decodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst)
}
