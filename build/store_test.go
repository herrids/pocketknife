package build

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// TestLockAppSerializesSameAppID proves two concurrent holders of the same
// app id's lock never run their critical sections at the same time.
func TestLockAppSerializesSameAppID(t *testing.T) {
	bst := openTestStore(t)

	var mu sync.Mutex // guards the plain counters below, independent of LockApp
	var active, maxActive int
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := bst.LockApp("same-app")
			defer unlock()

			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()

			time.Sleep(2 * time.Millisecond) // widen the window a real race would need

			mu.Lock()
			active--
			mu.Unlock()
		}()
	}
	wg.Wait()

	if maxActive != 1 {
		t.Fatalf("max concurrently active holders of the same app id's lock = %d, want 1", maxActive)
	}
}

// TestLockAppDoesNotSerializeDifferentAppIDs proves the lock is per-app-id,
// not global: two different app ids must be able to make progress at the
// same time, never waiting on each other.
func TestLockAppDoesNotSerializeDifferentAppIDs(t *testing.T) {
	bst := openTestStore(t)

	unlockA := bst.LockApp("app-a")
	defer unlockA()

	done := make(chan struct{})
	go func() {
		unlockB := bst.LockApp("app-b")
		defer unlockB()
		close(done)
	}()

	select {
	case <-done:
		// app-b's lock was acquired without waiting on app-a's — correct.
	case <-time.After(2 * time.Second):
		t.Fatal("a different app id's LockApp call blocked on an unrelated app id's lock")
	}
}

func TestCreateJobStartsQueued(t *testing.T) {
	bst := openTestStore(t)
	j, err := bst.CreateJob("app1", KindInstall, 1)
	if err != nil {
		t.Fatal(err)
	}
	if j.State != StateQueued {
		t.Fatalf("got state %s, want queued", j.State)
	}
	if j.Kind != KindInstall || j.AppID != "app1" || j.ManifestVersion != 1 {
		t.Fatalf("unexpected job: %+v", j)
	}
}

func TestTransitionFollowsTheAllowedPath(t *testing.T) {
	bst := openTestStore(t)
	j, _ := bst.CreateJob("app1", KindInstall, 1)

	if _, err := bst.Transition(j.ID, StateReady, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("queued -> ready directly should be invalid, got %v", err)
	}
	if _, err := bst.Transition(j.ID, StateBuilding, ""); err != nil {
		t.Fatalf("queued -> building: %v", err)
	}
	if _, err := bst.Transition(j.ID, StateActivating, ""); err != nil {
		t.Fatalf("building -> activating: %v", err)
	}
	got, err := bst.Transition(j.ID, StateReady, "")
	if err != nil {
		t.Fatalf("activating -> ready: %v", err)
	}
	if got.State != StateReady {
		t.Fatalf("got %s, want ready", got.State)
	}
	if _, err := bst.Transition(j.ID, StateFailed, "x"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("ready is terminal; transitioning out of it must fail")
	}
}

func TestFailedIsReachableFromEveryWorkingState(t *testing.T) {
	bst := openTestStore(t)

	queued, _ := bst.CreateJob("a", KindInstall, 1)

	building, _ := bst.CreateJob("a", KindInstall, 1)
	if _, err := bst.Transition(building.ID, StateBuilding, ""); err != nil {
		t.Fatal(err)
	}

	activating, _ := bst.CreateJob("a", KindInstall, 1)
	if _, err := bst.Transition(activating.ID, StateBuilding, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := bst.Transition(activating.ID, StateActivating, ""); err != nil {
		t.Fatal(err)
	}

	for _, j := range []*Job{queued, building, activating} {
		got, err := bst.Transition(j.ID, StateFailed, "boom")
		if err != nil {
			t.Fatalf("from %s: %v", j.State, err)
		}
		if got.State != StateFailed || got.Error != "boom" {
			t.Fatalf("got %+v", got)
		}
	}
}

func TestRetryNeverReopensAFailedJobItCreatesANewOne(t *testing.T) {
	bst := openTestStore(t)
	j1, _ := bst.CreateJob("a", KindInstall, 1)
	if _, err := bst.Transition(j1.ID, StateFailed, "boom"); err != nil {
		t.Fatal(err)
	}

	j2, _ := bst.CreateJob("a", KindInstall, 1)
	if j2.ID == j1.ID {
		t.Fatal("retry must create a new job id, not reopen the failed one")
	}
	if j2.State != StateQueued {
		t.Fatalf("new job should start queued, got %s", j2.State)
	}

	jobs, err := bst.ListForApp("a")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs in history, got %d", len(jobs))
	}
	if jobs[0].ID != j2.ID {
		t.Fatal("ListForApp should return the most recent job first")
	}
}

func TestInFlightJobsExcludesTerminalStates(t *testing.T) {
	bst := openTestStore(t)
	queued, _ := bst.CreateJob("a", KindInstall, 1)

	ready, _ := bst.CreateJob("a", KindInstall, 1)
	bst.Transition(ready.ID, StateBuilding, "")
	bst.Transition(ready.ID, StateActivating, "")
	bst.Transition(ready.ID, StateReady, "")

	failed, _ := bst.CreateJob("a", KindInstall, 1)
	bst.Transition(failed.ID, StateFailed, "x")

	building, _ := bst.CreateJob("a", KindInstall, 1)
	bst.Transition(building.ID, StateBuilding, "")

	jobs, err := bst.InFlightJobs()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, j := range jobs {
		ids[j.ID] = true
	}
	if !ids[queued.ID] || !ids[building.ID] {
		t.Fatalf("expected queued and building jobs in-flight, got %+v", jobs)
	}
	if ids[ready.ID] || ids[failed.ID] {
		t.Fatalf("terminal jobs leaked into in-flight: %+v", jobs)
	}
}

func TestPromoteActiveUpsertsOnePointerPerApp(t *testing.T) {
	bst := openTestStore(t)
	j1, _ := bst.CreateJob("a", KindInstall, 1)
	if err := bst.PromoteActive("a", j1.ID, "/dist/v1", 1); err != nil {
		t.Fatal(err)
	}

	ab, err := bst.ActiveBuildFor("a")
	if err != nil {
		t.Fatal(err)
	}
	if ab == nil || ab.JobID != j1.ID || ab.AssetDir != "/dist/v1" || ab.ManifestVersion != 1 {
		t.Fatalf("got %+v", ab)
	}

	j2, _ := bst.CreateJob("a", KindDeploy, 2)
	if err := bst.PromoteActive("a", j2.ID, "/dist/v2", 2); err != nil {
		t.Fatal(err)
	}
	ab2, err := bst.ActiveBuildFor("a")
	if err != nil {
		t.Fatal(err)
	}
	if ab2.JobID != j2.ID || ab2.AssetDir != "/dist/v2" || ab2.ManifestVersion != 2 {
		t.Fatalf("upsert did not replace the pointer: %+v", ab2)
	}
}

func TestActiveBuildForUnknownAppIsNil(t *testing.T) {
	bst := openTestStore(t)
	ab, err := bst.ActiveBuildFor("nope")
	if err != nil {
		t.Fatal(err)
	}
	if ab != nil {
		t.Fatalf("expected nil, got %+v", ab)
	}
}
