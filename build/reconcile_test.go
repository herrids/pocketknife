package build

import (
	"context"
	"testing"

	"pocketknife/registry"
)

func TestReconcileFailsInFlightJobsWithNoCommittedActivation(t *testing.T) {
	bst := openTestStore(t)
	reg := registry.New()

	j, _ := bst.CreateJob("a", KindInstall, 1)
	if _, err := bst.Transition(j.ID, StateBuilding, ""); err != nil {
		t.Fatal(err)
	}

	res, err := Reconcile(reg, bst)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.FailedJobs) != 1 || res.FailedJobs[0].ID != j.ID {
		t.Fatalf("expected the interrupted job to be failed, got %+v", res.FailedJobs)
	}

	got, err := bst.Get(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateFailed || got.Error == "" {
		t.Fatalf("job not reconciled to a legible failed state: %+v", got)
	}
}

func TestReconcileCompletesADurablyActivatedJobInstead(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootTestApp(t, appsDir, "a", notesV1AsA)
	bst := openTestStore(t)

	j, _ := bst.CreateJob("a", KindInstall, 1)
	bst.Transition(j.ID, StateBuilding, "")
	bst.Transition(j.ID, StateActivating, "")

	artifactDir := t.TempDir()
	if err := bst.PromoteActive("a", j.ID, artifactDir, 1); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash between the durable PromoteActive commit and the final
	// in-process Transition(StateReady) call: the job row is still "activating".

	res, err := Reconcile(reg, bst)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.FailedJobs) != 0 {
		t.Fatalf("a durably-activated job must not be marked failed: %+v", res.FailedJobs)
	}
	if !contains(res.Activated, "a") {
		t.Fatalf("expected app %q to be reattached, got %+v", "a", res.Activated)
	}

	got, err := bst.Get(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateReady {
		t.Fatalf("durably-activated job should be completed to ready, got %s", got.State)
	}

	ra, _ := reg.App("a")
	if ra.AssetDir != artifactDir {
		t.Fatalf("registry AssetDir = %q, want %q", ra.AssetDir, artifactDir)
	}
}

func TestReconcileLeavesVersionMismatchedPointerBroken(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootTestApp(t, appsDir, "a", notesV1AsA) // registered schema version 1
	bst := openTestStore(t)

	staleDir := t.TempDir()
	if err := bst.PromoteActive("a", "ghost-job", staleDir, 99); err != nil {
		t.Fatal(err)
	}

	res, err := Reconcile(reg, bst)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Broken, "a") {
		t.Fatalf("expected app %q to be reported broken, got %+v", "a", res.Broken)
	}
	ra, _ := reg.App("a")
	if ra.AssetDir != "" {
		t.Fatalf("a version-mismatched pointer must never be attached, got AssetDir %q", ra.AssetDir)
	}
}

func TestReconcileLeavesMissingArtifactPointerBroken(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootTestApp(t, appsDir, "a", notesV1AsA)
	bst := openTestStore(t)

	missing := appsDir + "/does-not-exist"
	if err := bst.PromoteActive("a", "ghost-job", missing, 1); err != nil {
		t.Fatal(err)
	}

	res, err := Reconcile(reg, bst)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Broken, "a") {
		t.Fatalf("expected app %q to be reported broken, got %+v", "a", res.Broken)
	}
	ra, _ := reg.App("a")
	if ra.AssetDir != "" {
		t.Fatalf("a pointer to a missing artifact must never be attached, got AssetDir %q", ra.AssetDir)
	}
}

// TestReconcileReattachesAfterSimulatedReboot is the gate scenario end to end:
// a real Deploy activates the app, then a brand new registry (the only thing
// a process restart truly throws away) is booted from the same apps dir, and
// Reconcile against the same durable platform db must bring the app back to
// ready without rebuilding anything.
func TestReconcileReattachesAfterSimulatedReboot(t *testing.T) {
	appsDir := t.TempDir()
	reg := bootTestApp(t, appsDir, "a", notesV1AsA)
	writeDist(t, appsDir, "a", "frontend/dist", "v1")
	bst := openTestStore(t)

	if _, err := Deploy(context.Background(), reg, bst, "a", []byte(notesV1AsA), DeployOptions{}); err != nil {
		t.Fatalf("install: %v", err)
	}
	ra, _ := reg.App("a")
	wantAssetDir := ra.AssetDir
	reg.Close()

	// Simulate a reboot: a fresh registry, loaded the same way main() does,
	// starts every AssetDir empty.
	rebooted := bootTestApp(t, appsDir, "a", notesV1AsA)
	t.Cleanup(func() { rebooted.Close() })
	if ra, _ := rebooted.App("a"); ra.AssetDir != "" {
		t.Fatal("a fresh registry.Load must start with AssetDir empty")
	}

	res, err := Reconcile(rebooted, bst)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Activated, "a") {
		t.Fatalf("expected reboot reconciliation to reattach app %q, got %+v", "a", res.Activated)
	}
	ra2, _ := rebooted.App("a")
	if ra2.AssetDir != wantAssetDir {
		t.Fatalf("AssetDir after reboot = %q, want %q (must never dark out a ready app)", ra2.AssetDir, wantAssetDir)
	}
}

const notesV1AsA = `{
  "app": { "id": "a", "name": "A", "version": 1 },
  "entities": [
    { "id": "ent_note", "name": "note", "fields": [
      { "id": "fld_title", "name": "title", "type": "text", "required": true }
    ]}
  ],
  "frontend": { "dist": "frontend/dist" }
}`
