package platform_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"pocketknife/build"
	"pocketknife/platform"
	"pocketknife/registry"
)

func openTestStore(t *testing.T) *build.Store {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "platform-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	st, err := build.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// ── Store tests ──────────────────────────────────────────────────────────────

func TestEnsureAppMeta_Defaults(t *testing.T) {
	st := openTestStore(t)
	if err := st.EnsureAppMeta("app1", "My App"); err != nil {
		t.Fatalf("EnsureAppMeta: %v", err)
	}
	m, err := st.GetAppMeta("app1")
	if err != nil {
		t.Fatalf("GetAppMeta: %v", err)
	}
	if m == nil {
		t.Fatal("expected row, got nil")
	}
	if m.Emoji != "📦" {
		t.Errorf("emoji = %q, want 📦", m.Emoji)
	}
	if m.DisplayName != "My App" {
		t.Errorf("displayName = %q, want 'My App'", m.DisplayName)
	}
}

func TestEnsureAppMeta_Idempotent(t *testing.T) {
	st := openTestStore(t)
	_ = st.EnsureAppMeta("app1", "My App")
	_ = st.EnsureAppMeta("app1", "My App Changed") // second call should be no-op
	m, _ := st.GetAppMeta("app1")
	if m.DisplayName != "My App" {
		t.Errorf("expected first value preserved, got %q", m.DisplayName)
	}
}

func TestUpsertAndListAppMeta(t *testing.T) {
	st := openTestStore(t)
	for i, name := range []string{"Alpha", "Beta", "Gamma"} {
		_ = st.EnsureAppMeta("app"+name, name)
		_ = st.UpsertAppMeta(build.AppMeta{AppID: "app" + name, Emoji: "🎯", Color: "#123456", DisplayName: name, GridOrder: i})
	}

	list, err := st.ListAppMeta()
	if err != nil {
		t.Fatalf("ListAppMeta: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(list))
	}
	// Should be sorted by grid_order ascending.
	if list[0].DisplayName != "Alpha" {
		t.Errorf("expected Alpha first, got %s", list[0].DisplayName)
	}
}

func TestReorderApps(t *testing.T) {
	st := openTestStore(t)
	_ = st.EnsureAppMeta("a", "A")
	_ = st.EnsureAppMeta("b", "B")
	_ = st.EnsureAppMeta("c", "C")

	if err := st.ReorderApps([]string{"c", "a", "b"}); err != nil {
		t.Fatalf("ReorderApps: %v", err)
	}
	list, _ := st.ListAppMeta()
	if list[0].AppID != "c" || list[1].AppID != "a" || list[2].AppID != "b" {
		t.Errorf("unexpected order: %v %v %v", list[0].AppID, list[1].AppID, list[2].AppID)
	}
}

// ── Auth + registry handler tests ───────────────────────────────────────────

func newTestServer(t *testing.T) (http.Handler, *build.Store) {
	t.Helper()
	st := openTestStore(t)
	reg := registry.New()
	t.Setenv("POCKETKNIFE_ADMIN_PASSWORD", "testpass123")
	srv, err := platform.NewServer(st, reg, "")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv, st
}

func TestLogin_Success(t *testing.T) {
	srv, _ := newTestServer(t)

	body := `{"password":"testpass123"}`
	req := httptest.NewRequest(http.MethodPost, "/platform/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("login status = %d, want 200", rr.Code)
	}
	cookie := rr.Result().Header.Get("Set-Cookie")
	if cookie == "" {
		t.Error("expected Set-Cookie header")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	srv, _ := newTestServer(t)

	start := time.Now()
	body := `{"password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/platform/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Error("expected brute-force delay on wrong password")
	}
}

func TestAuthGuard_NoToken(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/platform/registry", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func login(t *testing.T, srv http.Handler) string {
	t.Helper()
	body := `{"password":"testpass123"}`
	req := httptest.NewRequest(http.MethodPost, "/platform/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	for _, c := range rr.Result().Cookies() {
		if c.Name == "pk_session" {
			return c.Value
		}
	}
	t.Fatal("no pk_session cookie")
	return ""
}

func TestRegistryList(t *testing.T) {
	srv, st := newTestServer(t)
	_ = st.EnsureAppMeta("myapp", "My App")
	tok := login(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/platform/registry", nil)
	req.AddCookie(&http.Cookie{Name: "pk_session", Value: tok})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var entries []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&entries)
	if len(entries) != 1 || entries[0]["appId"] != "myapp" {
		t.Errorf("unexpected registry response: %v", entries)
	}
}

func TestRegistryPatch(t *testing.T) {
	srv, st := newTestServer(t)
	_ = st.EnsureAppMeta("myapp", "My App")
	tok := login(t, srv)

	body := `{"emoji":"🌱","color":"#A8D5A2"}`
	req := httptest.NewRequest(http.MethodPatch, "/platform/registry/myapp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "pk_session", Value: tok})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	var out map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&out)
	if out["emoji"] != "🌱" {
		t.Errorf("emoji = %v, want 🌱", out["emoji"])
	}
}

func TestRegistryReorder(t *testing.T) {
	srv, st := newTestServer(t)
	_ = st.EnsureAppMeta("a", "A")
	_ = st.EnsureAppMeta("b", "B")
	_ = st.EnsureAppMeta("c", "C")
	tok := login(t, srv)

	body := `{"order":["c","a","b"]}`
	req := httptest.NewRequest(http.MethodPost, "/platform/registry/reorder", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "pk_session", Value: tok})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}
