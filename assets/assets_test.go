package assets_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"pocketknife/assets"
	"pocketknife/registry"
	"pocketknife/schema"
)

func newTestServer(t *testing.T, reg *registry.Registry) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(assets.NewServer(reg))
	t.Cleanup(srv.Close)
	return srv
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

func TestServesKnownFileAndFallsBackToEntryForUnknownPath(t *testing.T) {
	dist := t.TempDir()
	writeFile(t, dist, "index.html", "<html>app</html>")
	writeFile(t, dist, "app.js", "console.log(1)")

	reg := registry.New()
	reg.Register(&registry.RegisteredApp{
		Schema:   &schema.App{ID: "app", Name: "App", Version: 1, Frontend: &schema.Frontend{Dist: "dist", Entry: "index.html"}},
		AssetDir: dist,
	})
	srv := newTestServer(t, reg)

	resp, body := get(t, srv.URL+"/ui/app/app.js")
	if resp.StatusCode != http.StatusOK || body != "console.log(1)" {
		t.Fatalf("known file: status %d body %q", resp.StatusCode, body)
	}

	resp, body = get(t, srv.URL+"/ui/app/some/spa/route")
	if resp.StatusCode != http.StatusOK || body != "<html>app</html>" {
		t.Fatalf("spa fallback: status %d body %q", resp.StatusCode, body)
	}
}

func TestCustomEntryFileUsedForFallback(t *testing.T) {
	dist := t.TempDir()
	writeFile(t, dist, "shell.html", "<html>shell</html>")

	reg := registry.New()
	reg.Register(&registry.RegisteredApp{
		Schema:   &schema.App{ID: "app", Name: "App", Version: 1, Frontend: &schema.Frontend{Dist: "dist", Entry: "shell.html"}},
		AssetDir: dist,
	})
	srv := newTestServer(t, reg)

	resp, body := get(t, srv.URL+"/ui/app/anything")
	if resp.StatusCode != http.StatusOK || body != "<html>shell</html>" {
		t.Fatalf("custom entry fallback: status %d body %q", resp.StatusCode, body)
	}
}

func TestUnactivatedAppIs404(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.RegisteredApp{
		Schema:   &schema.App{ID: "app", Name: "App", Version: 1},
		AssetDir: "",
	})
	srv := newTestServer(t, reg)

	resp, _ := get(t, srv.URL+"/ui/app/")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an unactivated app", resp.StatusCode)
	}
}

func TestUnknownAppIs404(t *testing.T) {
	reg := registry.New()
	srv := newTestServer(t, reg)

	resp, _ := get(t, srv.URL+"/ui/nope/")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an unknown app", resp.StatusCode)
	}
}

func TestBareAppPathRedirectsToTrailingSlash(t *testing.T) {
	dist := t.TempDir()
	writeFile(t, dist, "index.html", "<html>app</html>")

	reg := registry.New()
	reg.Register(&registry.RegisteredApp{
		Schema:   &schema.App{ID: "app", Name: "App", Version: 1},
		AssetDir: dist,
	})

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	srv := newTestServer(t, reg)
	resp, err := client.Get(srv.URL + "/ui/app")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/ui/app/" {
		t.Fatalf("Location = %q, want /ui/app/", loc)
	}
}

func TestPathTraversalCannotEscapeAssetDir(t *testing.T) {
	dist := t.TempDir()
	writeFile(t, dist, "index.html", "<html>app</html>")
	secretDir := t.TempDir()
	writeFile(t, secretDir, "secret.txt", "top secret")

	reg := registry.New()
	reg.Register(&registry.RegisteredApp{
		Schema:   &schema.App{ID: "app", Name: "App", Version: 1},
		AssetDir: dist,
	})
	srv := newTestServer(t, reg)

	resp, body := get(t, srv.URL+"/ui/app/../"+filepath.Base(secretDir)+"/secret.txt")
	// The stdlib mux cleans ".." out of the path before routing, so this either
	// 404s or falls back to the SPA entry — it must never return the secret.
	if body == "top secret" {
		t.Fatalf("path traversal escaped AssetDir: status %d body %q", resp.StatusCode, body)
	}
}
