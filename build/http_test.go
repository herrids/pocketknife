package build

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pocketknife/registry"
	"pocketknife/schema"
)

func TestStatusServerListForApp(t *testing.T) {
	bst := openTestStore(t)
	j, _ := bst.CreateJob("app", KindInstall, 1)

	reg := registry.New()
	reg.Register(&registry.RegisteredApp{Schema: &schema.App{ID: "app", Name: "App", Version: 1}})

	srv := httptest.NewServer(NewStatusServer(bst, reg))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/builds/app")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Jobs []struct {
			ID string `json:"ID"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Jobs) != 1 || body.Jobs[0].ID != j.ID {
		t.Fatalf("unexpected jobs in response: %+v", body.Jobs)
	}
}

func TestStatusServerListForUnknownAppIs404(t *testing.T) {
	bst := openTestStore(t)
	reg := registry.New()
	srv := httptest.NewServer(NewStatusServer(bst, reg))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/builds/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestStatusServerGetJob(t *testing.T) {
	bst := openTestStore(t)
	j, _ := bst.CreateJob("app", KindInstall, 1)
	reg := registry.New()
	srv := httptest.NewServer(NewStatusServer(bst, reg))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/builds/job/" + j.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got Job
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != j.ID || got.State != StateQueued {
		t.Fatalf("unexpected job in response: %+v", got)
	}
}

func TestStatusServerGetUnknownJobIs404(t *testing.T) {
	bst := openTestStore(t)
	reg := registry.New()
	srv := httptest.NewServer(NewStatusServer(bst, reg))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/builds/job/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
