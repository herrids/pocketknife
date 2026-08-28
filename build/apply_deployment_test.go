package build

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"pocketknife/registry"
)

// assertSecondCallerBlocksOnSameAppID deterministically proves a second
// concurrent ApplyDeployment call for appID cannot proceed until the first
// one releases the per-app lock. It uses build.Store's test-only
// testHoldLock hook to pause the first call while it holds the lock, so
// this needs no wall-clock head-start assumption about goroutine scheduling
// order at all — unlike a "give the first call a few ms head start"
// approach, which proved unreliable under -race's added scheduling
// overhead. first and second are otherwise plain ApplyDeployment calls.
func assertSecondCallerBlocksOnSameAppID(t *testing.T, bst *Store, appID string, first, second func() error) {
	t.Helper()
	release := make(chan struct{})
	entered := make(chan struct{})
	var once sync.Once

	bst.testHoldLock = func(gotID string) {
		if gotID != appID {
			return
		}
		once.Do(func() { close(entered) })
		<-release
	}
	defer func() { bst.testHoldLock = nil }()

	firstDone := make(chan error, 1)
	go func() { firstDone <- first() }()

	select {
	case <-entered:
		// first now holds appID's lock and is paused inside testHoldLock.
	case <-time.After(5 * time.Second):
		t.Fatal("first call never reached the lock-held checkpoint")
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- second() }()

	select {
	case err := <-secondDone:
		t.Fatalf("second call completed (err=%v) while the first still held appID's lock", err)
	case <-time.After(50 * time.Millisecond):
		// Expected: second is genuinely blocked on the same lock.
	}

	close(release) // let first proceed and release the lock.

	if err := <-firstDone; err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second call: %v", err)
	}
}

// TestApplyDeploymentSerializesConcurrentInstallsOfTheSameNewApp proves that
// two concurrent ApplyDeployment calls for an app id the registry has never
// seen serialize: the second call's Bootstrap-vs-Deploy decision is made
// only after the first call has completed and registered the app, so it
// correctly becomes a redeploy rather than racing a second Bootstrap.
func TestApplyDeploymentSerializesConcurrentInstallsOfTheSameNewApp(t *testing.T) {
	appsDir := t.TempDir()
	reg := registry.New()
	bst := openTestStore(t)

	call := func(marker string) func() error {
		return func() error {
			bundle := buildTarGz(t, []tarEntry{{name: "index.html", content: marker}})
			_, err := ApplyDeployment(context.Background(), reg, bst, appsDir, []byte(bootstrapManifest), bytes.NewReader(bundle), DeployOptions{})
			return err
		}
	}
	assertSecondCallerBlocksOnSameAppID(t, bst, "freshapp", call("first"), call("second"))

	ra, ok := reg.App("freshapp")
	if !ok {
		t.Fatal("app must be registered exactly once after both concurrent installs")
	}
	if ra.AssetDir == "" {
		t.Fatal("app must be activated")
	}
}

// TestApplyDeploymentSerializesConcurrentDeploysOfAnExistingApp proves two
// concurrent ApplyDeployment calls for an already-installed app never run
// their mutation paths (frontend swap, activation) at the same time.
func TestApplyDeploymentSerializesConcurrentDeploysOfAnExistingApp(t *testing.T) {
	appsDir := t.TempDir()
	reg := registry.New()
	bst := openTestStore(t)
	bundle := buildTarGz(t, []tarEntry{{name: "index.html", content: "v1"}})

	if _, err := ApplyDeployment(context.Background(), reg, bst, appsDir, []byte(bootstrapManifest), bytes.NewReader(bundle), DeployOptions{}); err != nil {
		t.Fatalf("initial install: %v", err)
	}

	call := func(marker string) func() error {
		return func() error {
			redeployBundle := buildTarGz(t, []tarEntry{{name: "index.html", content: marker}})
			_, err := ApplyDeployment(context.Background(), reg, bst, appsDir, []byte(bootstrapManifest), bytes.NewReader(redeployBundle), DeployOptions{})
			return err
		}
	}
	assertSecondCallerBlocksOnSameAppID(t, bst, "freshapp", call("v2"), call("v3"))

	ra, ok := reg.App("freshapp")
	if !ok || ra.AssetDir == "" {
		t.Fatal("app must remain registered and activated after both concurrent redeploys")
	}
}

// TestApplyDeploymentDoesNotSerializeDifferentAppIDs proves ApplyDeployment
// calls for different app ids never wait on each other: app_a's lock is
// deterministically held open (via the same testHoldLock checkpoint as
// above) while app_b's whole ApplyDeployment call is required to complete
// promptly — a shared, global lock would instead make app_b wait for app_a
// to release it.
func TestApplyDeploymentDoesNotSerializeDifferentAppIDs(t *testing.T) {
	appsDir := t.TempDir()
	reg := registry.New()
	bst := openTestStore(t)

	manifestFor := func(id string) string {
		return `{
          "app": { "id": "` + id + `", "name": "App", "version": 1 },
          "entities": [{ "id": "ent_note", "name": "note", "fields": [
            { "id": "fld_title", "name": "title", "type": "text", "required": true }
          ]}],
          "frontend": { "dist": "dist" }
        }`
	}

	release := make(chan struct{})
	entered := make(chan struct{})
	var once sync.Once
	bst.testHoldLock = func(gotID string) {
		if gotID != "app_a" {
			return
		}
		once.Do(func() { close(entered) })
		<-release
	}
	defer func() { bst.testHoldLock = nil }()

	aDone := make(chan error, 1)
	go func() {
		bundle := buildTarGz(t, []tarEntry{{name: "index.html", content: "a"}})
		_, err := ApplyDeployment(context.Background(), reg, bst, appsDir, []byte(manifestFor("app_a")), bytes.NewReader(bundle), DeployOptions{})
		aDone <- err
	}()

	select {
	case <-entered:
		// app_a's lock is held and paused.
	case <-time.After(5 * time.Second):
		t.Fatal("app_a call never reached the lock-held checkpoint")
	}

	bDone := make(chan error, 1)
	go func() {
		bundle := buildTarGz(t, []tarEntry{{name: "index.html", content: "b"}})
		_, err := ApplyDeployment(context.Background(), reg, bst, appsDir, []byte(manifestFor("app_b")), bytes.NewReader(bundle), DeployOptions{})
		bDone <- err
	}()

	select {
	case err := <-bDone:
		if err != nil {
			t.Fatalf("app_b: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("app_b did not complete while app_a's unrelated lock was held — app ids are not independent")
	}

	close(release)
	if err := <-aDone; err != nil {
		t.Fatalf("app_a: %v", err)
	}

	if _, ok := reg.App("app_a"); !ok {
		t.Fatal("app_a must be registered")
	}
	if _, ok := reg.App("app_b"); !ok {
		t.Fatal("app_b must be registered")
	}
}
