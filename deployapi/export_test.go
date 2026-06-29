package deployapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"pocketknife/build"
	"pocketknife/deployapi"
	"pocketknife/registry"
	"pocketknife/store"
)

// exportTestEnv wires a deploy server and an export server against the same
// registry and build store, mirroring the main.go setup.
type exportTestEnv struct {
	deploy  *httptest.Server
	export  *httptest.Server
	appsDir string
	reg     *registry.Registry
	bst     *build.Store
}

func newExportTestEnv(t *testing.T) *exportTestEnv {
	t.Helper()
	appsDir := t.TempDir()
	reg := registry.New()
	bst, err := build.Open(filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bst.Close() })

	env := &exportTestEnv{
		deploy:  httptest.NewServer(deployapi.NewServer(reg, bst, appsDir)),
		export:  httptest.NewServer(deployapi.NewExportServer(reg, bst)),
		appsDir: appsDir,
		reg:     reg,
		bst:     bst,
	}
	t.Cleanup(func() {
		env.deploy.Close()
		env.export.Close()
	})
	return env
}

func getExport(t *testing.T, baseURL, appID string) (int, map[string]any) {
	t.Helper()
	res, err := http.Get(baseURL + "/export/" + appID)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return res.StatusCode, out
}

func getExportSource(t *testing.T, baseURL, appID string) (int, []byte) {
	t.Helper()
	res, err := http.Get(baseURL + "/export/" + appID + "/source")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, raw
}

func TestExportReturnsManifestAndSourceFlag(t *testing.T) {
	env := newExportTestEnv(t)

	// Deploy with source.
	sa := buildTarGz(t, []tarEntry{{name: "src/App.tsx", content: "v1-src"}})
	status, resp := postDeploy(t, env.deploy.URL,
		map[string]string{"jobId": "job-export-1"},
		map[string][]byte{
			"manifest": []byte(journalV1),
			"bundle":   goodBundle(t, "v1"),
			"source":   sa,
		},
	)
	if status != http.StatusOK {
		t.Fatalf("deploy: %d %v", status, resp)
	}

	// Export should return manifest + hasSource: true.
	status, resp = getExport(t, env.export.URL, "journal")
	if status != http.StatusOK {
		t.Fatalf("export: %d %v", status, resp)
	}
	if resp["hasSource"] != true {
		t.Fatalf("hasSource = %v, want true", resp["hasSource"])
	}

	// Manifest round-trips correctly: app.id should survive.
	mRaw, _ := json.Marshal(resp["manifest"])
	var manifest map[string]any
	if err := json.Unmarshal(mRaw, &manifest); err != nil {
		t.Fatalf("parse manifest from export: %v", err)
	}
	app, _ := manifest["app"].(map[string]any)
	if app["id"] != "journal" {
		t.Fatalf("exported manifest app.id = %v, want journal", app["id"])
	}
}

func TestExportReturnsSourceBytes(t *testing.T) {
	env := newExportTestEnv(t)

	sa := buildTarGz(t, []tarEntry{{name: "src/App.tsx", content: "hellosrc"}})
	if status, resp := postDeploy(t, env.deploy.URL,
		map[string]string{"jobId": "job-exportsrc-1"},
		map[string][]byte{
			"manifest": []byte(journalV1),
			"bundle":   goodBundle(t, "v1"),
			"source":   sa,
		},
	); status != http.StatusOK {
		t.Fatalf("deploy: %d %v", status, resp)
	}

	status, data := getExportSource(t, env.export.URL, "journal")
	if status != http.StatusOK {
		t.Fatalf("export source: %d %s", status, data)
	}
	if len(data) == 0 {
		t.Fatal("export source returned empty body")
	}

	// Re-ingest the returned source bytes to verify they round-trip.
	destDir := t.TempDir()
	if err := build.ExtractBundle(bytes.NewReader(data), destDir); err != nil {
		t.Fatalf("ExtractBundle of exported source: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(destDir, "src", "App.tsx"))
	if err != nil || string(got) != "hellosrc" {
		t.Fatalf("App.tsx after round-trip = %q, %v", got, err)
	}
}

func TestExportSourcelessAppSignalsNoSource(t *testing.T) {
	env := newExportTestEnv(t)

	// Deploy without a source part.
	if status, resp := deploy(t, env.deploy.URL, "job-nosrc", journalV1, goodBundle(t, "v1")); status != http.StatusOK {
		t.Fatalf("deploy: %d %v", status, resp)
	}

	status, resp := getExport(t, env.export.URL, "journal")
	if status != http.StatusOK {
		t.Fatalf("export: %d %v", status, resp)
	}
	if resp["hasSource"] != false {
		t.Fatalf("hasSource = %v, want false for a sourceless deploy", resp["hasSource"])
	}

	// /export/{id}/source returns 404 (not empty tar, not fabricated content).
	status, body := getExportSource(t, env.export.URL, "journal")
	if status != http.StatusNotFound {
		t.Fatalf("export source status = %d, want 404; body=%s", status, body)
	}
}

func TestExportUnknownAppReturnsNotFound(t *testing.T) {
	env := newExportTestEnv(t)

	status, resp := getExport(t, env.export.URL, "nonexistent")
	if status != http.StatusNotFound {
		t.Fatalf("export: %d %v, want 404", status, resp)
	}
	errBody, ok := resp["error"].(map[string]any)
	if !ok || errBody["code"] != "app_not_found" {
		t.Fatalf("response = %+v, want error envelope with code app_not_found", resp)
	}

	status, body := getExportSource(t, env.export.URL, "nonexistent")
	if status != http.StatusNotFound {
		t.Fatalf("export source: %d, want 404; body=%s", status, body)
	}
}

func TestExportIsSideEffectFree(t *testing.T) {
	env := newExportTestEnv(t)

	if status, resp := deploy(t, env.deploy.URL, "job-se-1", journalV1, goodBundle(t, "v1")); status != http.StatusOK {
		t.Fatalf("deploy: %d %v", status, resp)
	}

	ra, ok := env.reg.App("journal")
	if !ok {
		t.Fatal("app not registered")
	}
	beforeAssetDir := ra.AssetDir

	// Call export twice.
	for range 2 {
		getExport(t, env.export.URL, "journal")
	}

	// Registry and asset dir must be unchanged.
	ra2, ok := env.reg.App("journal")
	if !ok || ra2.AssetDir != beforeAssetDir {
		t.Fatalf("export mutated registry: before=%s after=%s", beforeAssetDir, ra2.AssetDir)
	}
}

func TestExportRoundTripThroughDeploy(t *testing.T) {
	env := newExportTestEnv(t)

	// Initial deploy with source.
	sa := buildTarGz(t, []tarEntry{{name: "src/App.tsx", content: "original"}})
	if status, resp := postDeploy(t, env.deploy.URL,
		map[string]string{"jobId": "job-rt-1"},
		map[string][]byte{
			"manifest": []byte(journalV1),
			"bundle":   goodBundle(t, "v1"),
			"source":   sa,
		},
	); status != http.StatusOK {
		t.Fatalf("initial deploy: %d %v", status, resp)
	}

	// Insert data to verify preservation across redeploy.
	ra, _ := env.reg.App("journal")
	now := store.NowUTC()
	rowID := store.NewID()
	_, err := ra.Store.Insert(ra.Schema.EntityByID("ent_entry"), map[string]any{
		"id": rowID, "created_at": now, "updated_at": now, "title": "preserved",
	})
	if err != nil {
		t.Fatalf("insert test row: %v", err)
	}

	// Export the manifest.
	exportStatus, exportResp := getExport(t, env.export.URL, "journal")
	if exportStatus != http.StatusOK {
		t.Fatalf("export: %d %v", exportStatus, exportResp)
	}
	exportedManifest, _ := json.Marshal(exportResp["manifest"])

	// Re-deploy the exported manifest under a new jobId (simulating a no-op redeploy).
	if status, resp := postDeploy(t, env.deploy.URL,
		map[string]string{"jobId": "job-rt-2"},
		map[string][]byte{
			"manifest": exportedManifest,
			"bundle":   goodBundle(t, "v1-redeployed"),
		},
	); status != http.StatusOK {
		t.Fatalf("redeploy of exported manifest: %d %v", status, resp)
	}

	// Data must be preserved: retrieve the row by id.
	ra2, _ := env.reg.App("journal")
	row, err := ra2.Store.GetByID(ra2.Schema.EntityByID("ent_entry"), rowID)
	if err != nil {
		t.Fatalf("GetByID after redeploy: %v", err)
	}
	if row["title"] != "preserved" {
		t.Fatalf("data not preserved across exported-manifest redeploy: %+v", row)
	}
}
