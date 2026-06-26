package deployapi_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"pocketknife/api"
	"pocketknife/assets"
	"pocketknife/build"
	"pocketknife/deployapi"
	"pocketknife/registry"
)

const journalV1 = `{
  "app": { "id": "journal", "name": "Journal", "version": 1 },
  "entities": [
    { "id": "ent_entry", "name": "entry", "fields": [
      { "id": "fld_title", "name": "title", "type": "text", "required": true }
    ]}
  ]
}`

const journalV2 = `{
  "app": { "id": "journal", "name": "Journal", "version": 2 },
  "entities": [
    { "id": "ent_entry", "name": "entry", "fields": [
      { "id": "fld_title", "name": "title", "type": "text", "required": true },
      { "id": "fld_body",  "name": "body",  "type": "text" }
    ]}
  ]
}`

const invalidManifest = `{"app":{}}`

type tarEntry struct {
	name    string
	content string
}

func buildTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(e.content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func goodBundle(t *testing.T, marker string) []byte {
	return buildTarGz(t, []tarEntry{{name: "index.html", content: marker}})
}

func brokenBundle(t *testing.T) []byte {
	return buildTarGz(t, []tarEntry{{name: "other.html", content: "not the entry"}})
}

// testServer wires a deployapi.Server, plus assets and api servers, to one
// shared registry -- mirroring how main.go mounts all three against the same
// reg in the real binary, so a deploy through the ingest endpoint is visible
// to the very next request against either of the others.
type testServer struct {
	deploy *httptest.Server
	assets *httptest.Server
	api    *httptest.Server
	reg    *registry.Registry
	bst    *build.Store
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	appsDir := t.TempDir()
	reg := registry.New()
	bst, err := build.Open(filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bst.Close() })

	ts := &testServer{
		deploy: httptest.NewServer(deployapi.NewServer(reg, bst, appsDir)),
		assets: httptest.NewServer(assets.NewServer(reg)),
		api:    httptest.NewServer(api.NewServer(reg)),
		reg:    reg,
		bst:    bst,
	}
	t.Cleanup(func() {
		ts.deploy.Close()
		ts.assets.Close()
		ts.api.Close()
	})
	return ts
}

func buildMultipartBody(t *testing.T, fields map[string]string, files map[string][]byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range files {
		fw, err := w.CreateFormFile(name, name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func postDeploy(t *testing.T, baseURL string, fields map[string]string, files map[string][]byte) (int, map[string]any) {
	t.Helper()
	body, ct := buildMultipartBody(t, fields, files)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/deploy", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", ct)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, raw)
		}
	}
	return res.StatusCode, out
}

func deploy(t *testing.T, baseURL, jobID, manifest string, bundle []byte) (int, map[string]any) {
	return postDeploy(t, baseURL,
		map[string]string{"jobId": jobID},
		map[string][]byte{"manifest": []byte(manifest), "bundle": bundle},
	)
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

func TestDeployFirstInstallServesNewApp(t *testing.T) {
	ts := newTestServer(t)

	status, resp := deploy(t, ts.deploy.URL, "job-1", journalV1, goodBundle(t, "v1"))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; resp=%v", status, resp)
	}
	if resp["appId"] != "journal" {
		t.Fatalf("appId = %v, want journal", resp["appId"])
	}
	if resp["url"] != "/ui/journal/" {
		t.Fatalf("url = %v, want /ui/journal/", resp["url"])
	}

	if got := httpGetBody(t, ts.assets.URL+"/ui/journal/", http.StatusOK); got != "v1" {
		t.Fatalf("served bundle = %q, want v1", got)
	}

	row := httpJSON(t, "POST", ts.api.URL+"/apps/journal/entry", map[string]string{"title": "hello"}, http.StatusCreated)
	if row["title"] != "hello" {
		t.Fatalf("created row = %+v", row)
	}
}

func TestDeployRedeployPreservesDataAndActivatesNewBundle(t *testing.T) {
	ts := newTestServer(t)

	if status, resp := deploy(t, ts.deploy.URL, "job-1", journalV1, goodBundle(t, "v1")); status != http.StatusOK {
		t.Fatalf("initial install: status = %d, resp=%v", status, resp)
	}
	row := httpJSON(t, "POST", ts.api.URL+"/apps/journal/entry", map[string]string{"title": "hello"}, http.StatusCreated)
	id := row["id"].(string)

	status, resp := deploy(t, ts.deploy.URL, "job-2", journalV2, goodBundle(t, "v2"))
	if status != http.StatusOK {
		t.Fatalf("redeploy: status = %d, resp=%v", status, resp)
	}

	if got := httpGetBody(t, ts.assets.URL+"/ui/journal/", http.StatusOK); got != "v2" {
		t.Fatalf("served bundle after redeploy = %q, want v2", got)
	}
	got := httpJSON(t, "GET", ts.api.URL+"/apps/journal/entry/"+id, nil, http.StatusOK)
	if got["title"] != "hello" {
		t.Fatalf("data not preserved across redeploy: %+v", got)
	}
}

func TestDeployRedeployFailureRollsBack(t *testing.T) {
	ts := newTestServer(t)

	if status, resp := deploy(t, ts.deploy.URL, "job-1", journalV1, goodBundle(t, "v1")); status != http.StatusOK {
		t.Fatalf("initial install: status = %d, resp=%v", status, resp)
	}
	row := httpJSON(t, "POST", ts.api.URL+"/apps/journal/entry", map[string]string{"title": "hello"}, http.StatusCreated)
	id := row["id"].(string)

	status, resp := deploy(t, ts.deploy.URL, "job-2", journalV2, brokenBundle(t))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; resp=%v", status, resp)
	}

	if got := httpGetBody(t, ts.assets.URL+"/ui/journal/", http.StatusOK); got != "v1" {
		t.Fatalf("served bundle after rollback = %q, want v1", got)
	}
	got := httpJSON(t, "GET", ts.api.URL+"/apps/journal/entry/"+id, nil, http.StatusOK)
	if got["title"] != "hello" {
		t.Fatalf("data not preserved across rollback: %+v", got)
	}

	ra, ok := ts.reg.App("journal")
	if !ok || ra.Schema.Version != 1 {
		t.Fatalf("schema version = %v, want rolled back to 1", ra)
	}

	// The job must remain cleanly retriable under the same jobId since the
	// failed attempt was never recorded as a successful deploy request.
	if rec, err := ts.bst.DeployRequestByExternalID("job-2"); err != nil || rec != nil {
		t.Fatalf("a failed deploy must not be recorded as idempotent: rec=%v err=%v", rec, err)
	}
}

func TestDeployIdempotentRetryShortCircuits(t *testing.T) {
	ts := newTestServer(t)

	status1, resp1 := deploy(t, ts.deploy.URL, "job-1", journalV1, goodBundle(t, "v1"))
	if status1 != http.StatusOK {
		t.Fatalf("first deploy: status = %d, resp=%v", status1, resp1)
	}

	// Retried under the same jobId with a payload that would otherwise fail
	// validation -- the idempotency short-circuit must happen before any of
	// that is even looked at, returning the original cached result.
	status2, resp2 := deploy(t, ts.deploy.URL, "job-1", invalidManifest, nil)
	if status2 != http.StatusOK {
		t.Fatalf("retried deploy: status = %d, resp=%v", status2, resp2)
	}
	if resp2["appId"] != resp1["appId"] || resp2["url"] != resp1["url"] {
		t.Fatalf("retry result = %+v, want identical to first result %+v", resp2, resp1)
	}
}

func TestDeployInvalidManifestRejected(t *testing.T) {
	ts := newTestServer(t)

	status, resp := deploy(t, ts.deploy.URL, "job-1", invalidManifest, goodBundle(t, "v1"))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; resp=%v", status, resp)
	}
	errBody, ok := resp["error"].(map[string]any)
	if !ok || errBody["code"] != "manifest_invalid" {
		t.Fatalf("response = %+v, want error envelope with code manifest_invalid", resp)
	}

	if _, ok := ts.reg.App("journal"); ok {
		t.Fatal("no app should be registered for an invalid manifest")
	}
}

func TestDeployMissingPartsRejected(t *testing.T) {
	ts := newTestServer(t)

	cases := []struct {
		name   string
		fields map[string]string
		files  map[string][]byte
	}{
		{"missing jobId", map[string]string{}, map[string][]byte{"manifest": []byte(journalV1), "bundle": goodBundle(t, "v1")}},
		{"missing manifest", map[string]string{"jobId": "job-1"}, map[string][]byte{"bundle": goodBundle(t, "v1")}},
		{"missing bundle", map[string]string{"jobId": "job-1"}, map[string][]byte{"manifest": []byte(journalV1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, resp := postDeploy(t, ts.deploy.URL, c.fields, c.files)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; resp=%v", status, resp)
			}
			errBody, ok := resp["error"].(map[string]any)
			if !ok || errBody["code"] != "invalid_request" {
				t.Fatalf("response = %+v, want error envelope with code invalid_request", resp)
			}
		})
	}
}

func TestDeployConcurrentRequestsForSameNewAppSerialize(t *testing.T) {
	ts := newTestServer(t)

	var wg sync.WaitGroup
	results := make([]int, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status, _ := deploy(t, ts.deploy.URL, "job-concurrent-"+string(rune('a'+i)), journalV1, goodBundle(t, "v1"))
			results[i] = status
		}(i)
	}
	wg.Wait()

	for _, s := range results {
		if s != http.StatusOK {
			t.Fatalf("concurrent deploy statuses = %v, want both 200", results)
		}
	}
	if _, ok := ts.reg.App("journal"); !ok {
		t.Fatal("app should end up registered")
	}
}
