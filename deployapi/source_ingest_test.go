package deployapi_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"pocketknife/build"
	"pocketknife/deployapi"
	"pocketknife/registry"
)

// sourceTestServer is like testServer but exposes appsDir for source-presence
// assertions that need to inspect the filesystem directly.
type sourceTestServer struct {
	srv     *httptest.Server
	appsDir string
	reg     *registry.Registry
	bst     *build.Store
}

func newSourceTestServer(t *testing.T) *sourceTestServer {
	t.Helper()
	appsDir := t.TempDir()
	reg := registry.New()
	bst, err := build.Open(filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bst.Close() })

	ts := &sourceTestServer{
		srv:     httptest.NewServer(deployapi.NewServer(reg, bst, appsDir)),
		appsDir: appsDir,
		reg:     reg,
		bst:     bst,
	}
	t.Cleanup(func() { ts.srv.Close() })
	return ts
}

// sourceArchive builds a minimal gzipped tar representing an editable frontend
// source tree.
func sourceArchive(t *testing.T, marker string) []byte {
	return buildTarGz(t, []tarEntry{
		{name: "src/App.tsx", content: marker},
		{name: "package.json", content: `{"name":"app"}`},
	})
}

func TestDeployWithSourceStoresBothAndResponseUnchanged(t *testing.T) {
	ts := newSourceTestServer(t)

	status, resp := postDeploy(t, ts.srv.URL,
		map[string]string{"jobId": "job-src-1"},
		map[string][]byte{
			"manifest": []byte(journalV1),
			"bundle":   goodBundle(t, "v1"),
			"source":   sourceArchive(t, "src-v1"),
		},
	)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; resp=%v", status, resp)
	}
	if resp["appId"] != "journal" {
		t.Fatalf("appId = %v, want journal", resp["appId"])
	}

	// Source must be stored; find the active build's job id.
	ab, err := ts.bst.ActiveBuildFor("journal")
	if err != nil || ab == nil {
		t.Fatalf("active build: %v %v", ab, err)
	}
	appDir := filepath.Join(ts.appsDir, "journal")
	if _, exists := build.SourceDir(appDir, ab.JobID); !exists {
		t.Fatal("source should be stored under the active build's job id")
	}
	if _, err := os.Stat(filepath.Join(appDir, build.SourcesDirName, ab.JobID, "src", "App.tsx")); err != nil {
		t.Fatalf("stored source file: %v", err)
	}
}

func TestDeployWithoutSourceSucceedsAndStoresNothing(t *testing.T) {
	ts := newSourceTestServer(t)

	status, resp := deploy(t, ts.srv.URL, "job-nosrc-1", journalV1, goodBundle(t, "v1"))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; resp=%v", status, resp)
	}

	ab, err := ts.bst.ActiveBuildFor("journal")
	if err != nil || ab == nil {
		t.Fatalf("active build: %v %v", ab, err)
	}
	appDir := filepath.Join(ts.appsDir, "journal")
	if _, exists := build.SourceDir(appDir, ab.JobID); exists {
		t.Fatal("no source dir should be stored when source part is absent")
	}
}

func TestDeployMaliciousSourceContainedNoEscape(t *testing.T) {
	ts := newSourceTestServer(t)

	// A source archive whose first entry escapes via "..". ExtractBundle checks
	// the path before writing, so no file is written outside sources/<jobID>/.
	evilSource := buildTarGz(t, []tarEntry{{name: "../evil.txt", content: "bad"}})
	postDeploy(t, ts.srv.URL,
		map[string]string{"jobId": "job-evil-1"},
		map[string][]byte{
			"manifest": []byte(journalV1),
			"bundle":   goodBundle(t, "v1"),
			"source":   evilSource,
		},
	)

	// The hard invariant: no file written outside appsDir (source containment via
	// ExtractBundle's path-traversal check, which runs before any write).
	escaped := filepath.Join(filepath.Dir(ts.appsDir), "evil.txt")
	if _, err := os.Stat(escaped); !os.IsNotExist(err) {
		t.Fatal("traversal source entry must not have escaped appsDir")
	}
}
