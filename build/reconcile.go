package build

import (
	"fmt"
	"os"

	"pocketknife/registry"
)

// ReconcileResult summarizes what boot reconciliation found, for the caller to
// log; none of it is fatal to boot.
type ReconcileResult struct {
	FailedJobs []*Job   // in-flight jobs that could not have survived the restart
	Activated  []string // app IDs whose durable active-build pointer was reattached
	Broken     []string // app IDs with a stale or missing active-build pointer; left unserved
}

// Reconcile resolves boot-time build state against the registry. It must run
// after registry.Load, which always starts every app's AssetDir empty — Load
// has no notion of builds. Two things are true on every process start:
//
//  1. No queued, building or activating job can have survived the restart, so
//     each is moved to failed — unless its own activation had already durably
//     committed (active_builds points at it and the artifact is still on
//     disk), in which case the cutover is real and the job is completed
//     retroactively instead of being marked as having failed.
//  2. Every app's durable active-build pointer (active_builds), if its
//     artifact still exists on disk and its manifest version still matches
//     what's registered, is reattached to the in-memory registry. This is
//     what keeps a reboot from darkening an app that was already ready.
func Reconcile(reg *registry.Registry, bst *Store) (*ReconcileResult, error) {
	res := &ReconcileResult{}

	inFlight, err := bst.InFlightJobs()
	if err != nil {
		return nil, fmt.Errorf("list in-flight build jobs: %w", err)
	}
	for _, j := range inFlight {
		ab, err := bst.ActiveBuildFor(j.AppID)
		if err != nil {
			return nil, fmt.Errorf("active build for %s: %w", j.AppID, err)
		}
		if ab != nil && ab.JobID == j.ID && assetDirExists(ab.AssetDir) {
			if _, err := bst.Transition(j.ID, StateReady, ""); err != nil {
				return nil, fmt.Errorf("complete durably-activated job %s: %w", j.ID, err)
			}
			continue
		}
		if _, err := bst.Transition(j.ID, StateFailed, "interrupted by restart: no build can survive a process restart"); err != nil {
			return nil, fmt.Errorf("fail interrupted job %s: %w", j.ID, err)
		}
		j.State = StateFailed
		res.FailedJobs = append(res.FailedJobs, j)
	}

	for _, ra := range reg.Apps() {
		ab, err := bst.ActiveBuildFor(ra.Schema.ID)
		if err != nil {
			return nil, fmt.Errorf("active build for %s: %w", ra.Schema.ID, err)
		}
		if ab == nil {
			continue
		}
		if ab.ManifestVersion != ra.Schema.Version || !assetDirExists(ab.AssetDir) {
			// The on-disk manifest no longer matches what was last activated, or
			// the artifact it points to is gone. The pointer cannot be trusted,
			// so the app is left unserved rather than serving something stale or
			// nonexistent.
			res.Broken = append(res.Broken, ra.Schema.ID)
			continue
		}
		reg.Register(&registry.RegisteredApp{Schema: ra.Schema, Store: ra.Store, Dir: ra.Dir, AssetDir: ab.AssetDir})
		res.Activated = append(res.Activated, ra.Schema.ID)
	}

	return res, nil
}

func assetDirExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
