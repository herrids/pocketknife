// acceptance_test.go is the Phase 2 gate: it runs the build/activate pipeline
// against the repo's own three example apps and their real, hand-authored,
// tsc-compiled frontends — not synthetic fixtures — driving them through the
// generic API and static asset servers exactly as a real deployment would.
// It deliberately does not re-prove every state-machine edge case (store_test.go,
// frontend_test.go, deploy_test.go and reconcile_test.go already do that against
// synthetic fixtures); it proves the three real apps hold end to end.
package build

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"pocketknife/api"
	"pocketknife/assets"
	"pocketknife/registry"
)

// copyGateApp copies one of the repo's real example apps — manifest, and its
// hand-authored frontend/dist if it has one — into a fresh apps dir, so the
// gate never builds against the real repo's runtime files.
func copyGateApp(t *testing.T, dstAppsDir, appID string) {
	t.Helper()
	src := filepath.Join("..", "apps", appID)
	if err := copyDir(src, filepath.Join(dstAppsDir, appID)); err != nil {
		t.Fatalf("copy gate app %s: %v", appID, err)
	}
}

func bootGateApp(t *testing.T, appsDir, appID string) *registry.Registry {
	t.Helper()
	copyGateApp(t, appsDir, appID)
	reg, results, err := registry.Load(appsDir)
	if err != nil {
		t.Fatalf("boot %s: %v", appID, err)
	}
	for _, r := range results {
		if !r.OK {
			t.Fatalf("gate app %s failed to load: errors=%v err=%v", r.ManifestPath, r.Errors, r.Err)
		}
	}
	return reg
}

func readGateManifest(t *testing.T, appsDir, appID string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(appsDir, appID, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func httpJSON(t *testing.T, method, url string, body any, wantStatus int) map[string]any {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != wantStatus {
		t.Fatalf("%s %s: status = %d, want %d; body=%s", method, url, res.StatusCode, wantStatus, raw)
	}
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, raw)
		}
	}
	return out
}

func httpGetBody(t *testing.T, url string, wantStatus int) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != wantStatus {
		t.Fatalf("GET %s: status = %d, want %d; body=%s", url, res.StatusCode, wantStatus, raw)
	}
	return string(raw)
}

// --- 1: install, activate, open, and exercise real CRUD for each gate app ---

func TestGateTasksInstallActivatesAndServesRealFrontend(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootGateApp(t, appsDir, "tasks")
	t.Cleanup(func() { reg.Close() })
	bst := openTestStore(t)

	res, err := Deploy(context.Background(), reg, bst, "tasks", readGateManifest(t, appsDir, "tasks"), DeployOptions{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if res.Job.State != StateReady {
		t.Fatalf("job state = %s, want ready", res.Job.State)
	}

	apiSrv := httptest.NewServer(api.NewServer(reg))
	t.Cleanup(apiSrv.Close)
	assetsSrv := httptest.NewServer(assets.NewServer(reg))
	t.Cleanup(assetsSrv.Close)

	if body := httpGetBody(t, assetsSrv.URL+"/ui/tasks/", http.StatusOK); body == "" {
		t.Fatal("expected the real hand-authored index.html to be served")
	}

	proj := httpJSON(t, "POST", apiSrv.URL+"/apps/tasks/project", map[string]any{"name": "Gate"}, http.StatusCreated)
	task := httpJSON(t, "POST", apiSrv.URL+"/apps/tasks/task", map[string]any{"title": "Ship it", "project": proj["id"]}, http.StatusCreated)
	if task["priority"] != "medium" {
		t.Fatalf("default priority = %v, want medium", task["priority"])
	}
	listed := httpJSON(t, "GET", apiSrv.URL+"/apps/tasks/task?filter=project:eq:"+proj["id"].(string), nil, http.StatusOK)
	if listed["total"].(float64) != 1 {
		t.Fatalf("queried task list total = %v, want 1", listed["total"])
	}
}

func TestGateReadingTrackerInstallActivatesAndServesRealFrontend(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootGateApp(t, appsDir, "reading_tracker")
	t.Cleanup(func() { reg.Close() })
	bst := openTestStore(t)

	res, err := Deploy(context.Background(), reg, bst, "reading_tracker", readGateManifest(t, appsDir, "reading_tracker"), DeployOptions{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if res.Job.State != StateReady {
		t.Fatalf("job state = %s, want ready", res.Job.State)
	}

	apiSrv := httptest.NewServer(api.NewServer(reg))
	t.Cleanup(apiSrv.Close)
	assetsSrv := httptest.NewServer(assets.NewServer(reg))
	t.Cleanup(assetsSrv.Close)

	httpGetBody(t, assetsSrv.URL+"/ui/reading_tracker/", http.StatusOK)

	book := httpJSON(t, "POST", apiSrv.URL+"/apps/reading_tracker/book",
		map[string]any{"title": "The Go Programming Language", "rating": 5}, http.StatusCreated)
	httpJSON(t, "PATCH", apiSrv.URL+"/apps/reading_tracker/book/"+book["id"].(string),
		map[string]any{"done": true}, http.StatusOK)
	got := httpJSON(t, "GET", apiSrv.URL+"/apps/reading_tracker/book/"+book["id"].(string), nil, http.StatusOK)
	if got["done"] != true {
		t.Fatalf("update via real API did not stick: %v", got)
	}
}

func TestGateGratitudeLogInstallActivatesAndServesRealFrontend(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootGateApp(t, appsDir, "gratitude_log")
	t.Cleanup(func() { reg.Close() })
	bst := openTestStore(t)

	res, err := Deploy(context.Background(), reg, bst, "gratitude_log", readGateManifest(t, appsDir, "gratitude_log"), DeployOptions{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if res.Job.State != StateReady {
		t.Fatalf("job state = %s, want ready", res.Job.State)
	}

	apiSrv := httptest.NewServer(api.NewServer(reg))
	t.Cleanup(apiSrv.Close)
	assetsSrv := httptest.NewServer(assets.NewServer(reg))
	t.Cleanup(assetsSrv.Close)

	httpGetBody(t, assetsSrv.URL+"/ui/gratitude_log/", http.StatusOK)

	httpJSON(t, "POST", apiSrv.URL+"/apps/gratitude_log/entry", map[string]any{"text": "grateful for a working gate"}, http.StatusCreated)
	listed := httpJSON(t, "GET", apiSrv.URL+"/apps/gratitude_log/entry", nil, http.StatusOK)
	if listed["total"].(float64) != 1 {
		t.Fatalf("entry list total = %v, want 1", listed["total"])
	}
}

// --- 2: a deliberately broken build must land in a legible, retriable failed
// state, never a hang or a silent happy-path ring ---

func TestGateGratitudeLogBrokenBuildFailsLegiblyAndRetries(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootGateApp(t, appsDir, "gratitude_log")
	t.Cleanup(func() { reg.Close() })
	bst := openTestStore(t)

	// Break the build the manifest already declares: delete the dist directory
	// it points to before the install ever runs.
	if err := os.RemoveAll(filepath.Join(appsDir, "gratitude_log", "frontend", "dist")); err != nil {
		t.Fatal(err)
	}

	manifest := readGateManifest(t, appsDir, "gratitude_log")
	res, err := Deploy(context.Background(), reg, bst, "gratitude_log", manifest, DeployOptions{})
	if err == nil {
		t.Fatal("expected the broken build to fail")
	}
	if res.Job.State != StateFailed {
		t.Fatalf("job state = %s, want failed", res.Job.State)
	}
	if res.Job.Error == "" {
		t.Fatal("a failed job must carry a diagnosable error message")
	}
	if ra, _ := reg.App("gratitude_log"); ra.AssetDir != "" {
		t.Fatal("app must not be activated on a failed build")
	}

	// Fix the dist and retry: must succeed, and via a brand new job rather than
	// reopening the failed one.
	writeDist(t, appsDir, "gratitude_log", "frontend/dist", "fixed")
	res2, err := Deploy(context.Background(), reg, bst, "gratitude_log", manifest, DeployOptions{})
	if err != nil {
		t.Fatalf("retry after fixing the build: %v", err)
	}
	if res2.Job.ID == res.Job.ID {
		t.Fatal("a retry must create a new job, not reopen the failed one")
	}
	if res2.Job.State != StateReady {
		t.Fatalf("retry job state = %s, want ready", res2.Job.State)
	}
}

// --- 3: editing a gate app (new manifest version) is one migrate+rebuild
// operation; an injected rebuild failure rolls back without darkout, and the
// app is cleanly retriable afterwards ---

const readingTrackerV2Frontend = `{
  "app": { "id": "reading_tracker", "name": "Reading Tracker", "emoji": "📚", "version": 2 },
  "entities": [
    {
      "id": "ent_book",
      "name": "book",
      "operations": ["create", "read", "update", "delete"],
      "fields": [
        { "id": "fld_title",       "name": "title",       "type": "text",     "required": true, "max": 200 },
        { "id": "fld_author",      "name": "author",      "type": "text" },
        { "id": "fld_rating",      "name": "rating",      "type": "integer",  "min": 1, "max": 5 },
        { "id": "fld_done",        "name": "done",        "type": "boolean",  "default": false },
        { "id": "fld_finished_at", "name": "finished_at", "type": "datetime" },
        { "id": "fld_notes",       "name": "notes",       "type": "text" }
      ]
    }
  ],
  "frontend": { "dist": "frontend/dist" }
}`

func TestGateReadingTrackerSecondDeployRollsBackThenSucceeds(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootGateApp(t, appsDir, "reading_tracker")
	t.Cleanup(func() { reg.Close() })
	bst := openTestStore(t)

	v1 := readGateManifest(t, appsDir, "reading_tracker")
	if _, err := Deploy(context.Background(), reg, bst, "reading_tracker", v1, DeployOptions{}); err != nil {
		t.Fatalf("install: %v", err)
	}
	ra1, _ := reg.App("reading_tracker")
	oldAssetDir := ra1.AssetDir

	apiSrv := httptest.NewServer(api.NewServer(reg))
	t.Cleanup(apiSrv.Close)
	assetsSrv := httptest.NewServer(assets.NewServer(reg))
	t.Cleanup(assetsSrv.Close)

	book := httpJSON(t, "POST", apiSrv.URL+"/apps/reading_tracker/book", map[string]any{"title": "Gate Book"}, http.StatusCreated)
	bookID := book["id"].(string)

	// Inject a frontend build failure for the second deploy: the new manifest
	// still declares the dist, but it has gone missing.
	if err := os.RemoveAll(filepath.Join(appsDir, "reading_tracker", "frontend", "dist")); err != nil {
		t.Fatal(err)
	}
	res, err := Deploy(context.Background(), reg, bst, "reading_tracker", []byte(readingTrackerV2Frontend), DeployOptions{})
	if err == nil {
		t.Fatal("expected the second deploy to fail")
	}
	if res.Job.State != StateFailed {
		t.Fatalf("job state = %s, want failed", res.Job.State)
	}

	ra, _ := reg.App("reading_tracker")
	if ra.Schema.Version != 1 {
		t.Fatalf("schema version = %d, want rolled back to 1", ra.Schema.Version)
	}
	if ra.AssetDir != oldAssetDir {
		t.Fatalf("asset dir = %q, want rolled back to %q", ra.AssetDir, oldAssetDir)
	}
	// The app must still be fully openable on the old version: serve + API.
	httpGetBody(t, assetsSrv.URL+"/ui/reading_tracker/", http.StatusOK)
	got := httpJSON(t, "GET", apiSrv.URL+"/apps/reading_tracker/book/"+bookID, nil, http.StatusOK)
	if got["title"] != "Gate Book" {
		t.Fatalf("data lost across rollback: %v", got)
	}

	// Fix the dist and retry: must succeed, swap the served bundle, keep the data.
	writeDist(t, appsDir, "reading_tracker", "frontend/dist", "v2")
	res2, err := Deploy(context.Background(), reg, bst, "reading_tracker", []byte(readingTrackerV2Frontend), DeployOptions{})
	if err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
	if res2.Job.State != StateReady {
		t.Fatalf("retry job state = %s, want ready", res2.Job.State)
	}

	ra2, _ := reg.App("reading_tracker")
	if ra2.Schema.Version != 2 {
		t.Fatalf("schema version = %d, want 2", ra2.Schema.Version)
	}
	if ra2.AssetDir == oldAssetDir {
		t.Fatal("activation must cut over to a new artifact directory")
	}
	if got := readMarker(t, ra2.AssetDir); got != "v2" {
		t.Fatalf("served bundle = %q, want v2", got)
	}

	got2 := httpJSON(t, "GET", apiSrv.URL+"/apps/reading_tracker/book/"+bookID, nil, http.StatusOK)
	if got2["title"] != "Gate Book" {
		t.Fatalf("data lost across successful redeploy: %v", got2)
	}
	if _, ok := got2["notes"]; !ok {
		t.Fatalf("new v2 field missing from response: %v", got2)
	}
}

// --- 4: a reboot mid-build must reconcile the interrupted job to a legible
// failed state and must never dark out the previously-ready app ---

func TestGateTasksRebootMidBuildReconciles(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootGateApp(t, appsDir, "tasks")
	bst := openTestStore(t)

	if _, err := Deploy(context.Background(), reg, bst, "tasks", readGateManifest(t, appsDir, "tasks"), DeployOptions{}); err != nil {
		t.Fatalf("install: %v", err)
	}
	ra, _ := reg.App("tasks")
	wantAssetDir := ra.AssetDir

	apiSrv := httptest.NewServer(api.NewServer(reg))
	t.Cleanup(apiSrv.Close)
	created := httpJSON(t, "POST", apiSrv.URL+"/apps/tasks/project", map[string]any{"name": "Gate"}, http.StatusCreated)
	projID := created["id"].(string)

	// Simulate a crash mid-rebuild: a second job for the same install is left
	// "building", with no activation ever committed for it.
	j, err := bst.CreateJob("tasks", KindInstall, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bst.Transition(j.ID, StateBuilding, ""); err != nil {
		t.Fatal(err)
	}
	reg.Close()

	// Reboot: a fresh registry loaded the same way main() does. Every AssetDir
	// starts empty until reconciliation runs.
	rebooted, results, err := registry.Load(appsDir)
	if err != nil {
		t.Fatalf("reboot: %v", err)
	}
	for _, r := range results {
		if !r.OK {
			t.Fatalf("app failed to reload after reboot: %v %v", r.Errors, r.Err)
		}
	}
	t.Cleanup(func() { rebooted.Close() })
	if ra, _ := rebooted.App("tasks"); ra.AssetDir != "" {
		t.Fatal("a freshly booted registry must start with AssetDir empty")
	}

	res, err := Reconcile(rebooted, bst)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.FailedJobs) != 1 || res.FailedJobs[0].ID != j.ID {
		t.Fatalf("expected the interrupted job to be failed, got %+v", res.FailedJobs)
	}
	if !contains(res.Activated, "tasks") {
		t.Fatalf("expected tasks to be reattached after reboot, got %+v", res.Activated)
	}

	ra2, _ := rebooted.App("tasks")
	if ra2.AssetDir != wantAssetDir {
		t.Fatalf("AssetDir after reboot = %q, want %q (a reboot must never dark out a ready app)", ra2.AssetDir, wantAssetDir)
	}

	// The app must be fully usable again, with its pre-reboot data intact.
	rebootedAPISrv := httptest.NewServer(api.NewServer(rebooted))
	t.Cleanup(rebootedAPISrv.Close)
	rebootedAssetsSrv := httptest.NewServer(assets.NewServer(rebooted))
	t.Cleanup(rebootedAssetsSrv.Close)

	httpGetBody(t, rebootedAssetsSrv.URL+"/ui/tasks/", http.StatusOK)
	httpJSON(t, "GET", rebootedAPISrv.URL+"/apps/tasks/project/"+projID, nil, http.StatusOK)
}
