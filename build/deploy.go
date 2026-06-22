package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"pocketknife/migrate"
	"pocketknife/registry"
	"pocketknife/store"
	"pocketknife/validate"
)

// DeployOptions controls one Deploy run. It is migrate.Options verbatim: a
// second deploy's data side is never re-decided, only reused — destructive
// schema changes still need an explicit confirm and any required witnesses.
type DeployOptions = migrate.Options

// Result describes the outcome of a successful Deploy.
type Result struct {
	Job           *Job
	MigrateResult *migrate.Result // nil for a frontend-only install
}

// DefaultBuildRetention is how many superseded build artifact directories are
// kept per app after a successful activation.
const DefaultBuildRetention = 5

// Deploy is the one entry point for both halves of Phase 2: building and
// activating an app's frontend for its current manifest (Kind = install), and
// the "second deploy" — a new manifest version landed as one operation with a
// data migration, a frontend rebuild, and a single rollback contract (Kind =
// deploy). manifestBytes is the manifest to build for: pass the app's own
// on-disk manifest back in to (re)build the current version's frontend, or a
// new version to redeploy.
//
// Ordering for a second deploy is: snapshot the data unconditionally -> run
// the data migration -> build the new frontend -> activate. On any failure
// the deploy rolls back to the prior good manifest, database snapshot and
// asset directory, and the job lands in StateFailed carrying the cause; the
// app is left exactly as openable as it was before Deploy was called. A
// retry never reopens a failed job — it calls Deploy again, which creates a
// new one.
func Deploy(ctx context.Context, reg *registry.Registry, bst *Store, appID string, manifestBytes []byte, opts DeployOptions) (*Result, error) {
	ra, ok := reg.App(appID)
	if !ok {
		return nil, fmt.Errorf("unknown app %q", appID)
	}

	newApp, verrs := validate.Manifest(manifestBytes)
	if len(verrs) > 0 {
		return nil, fmt.Errorf("manifest failed validation: %s", verrs.Error())
	}
	if newApp.ID != appID {
		return nil, fmt.Errorf("manifest app id %q does not match target app %q", newApp.ID, appID)
	}

	kind := KindInstall
	if newApp.Version != ra.Schema.Version {
		kind = KindDeploy
	}

	job, err := bst.CreateJob(appID, kind, newApp.Version)
	if err != nil {
		return nil, fmt.Errorf("create build job: %w", err)
	}
	if _, err := bst.Transition(job.ID, StateBuilding, ""); err != nil {
		return nil, fmt.Errorf("transition to building: %w", err)
	}

	fail := func(cause error) (*Result, error) {
		if _, terr := bst.Transition(job.ID, StateFailed, cause.Error()); terr != nil {
			return nil, fmt.Errorf("%w (additionally failed to record the job as failed: %v)", cause, terr)
		}
		job.State = StateFailed
		job.Error = cause.Error()
		return &Result{Job: job}, cause
	}

	// Snapshot the prior good state before touching anything, so a frontend
	// build failure that happens *after* a successful data migration can still
	// undo the migration: the deploy is one operation, not two independently
	// committed ones.
	oldManifestPath := filepath.Join(ra.Dir, "manifest.json")
	oldManifestBytes, err := os.ReadFile(oldManifestPath)
	if err != nil {
		return fail(fmt.Errorf("read current manifest: %w", err))
	}
	oldSchema, oldStore, oldDir, oldAssetDir := ra.Schema, ra.Store, ra.Dir, ra.AssetDir

	var snapPath string
	if kind == KindDeploy {
		snapDir := filepath.Join(ra.Dir, migrate.SnapshotDirName)
		snapPath, err = migrate.Snapshot(ra.Store, snapDir)
		if err != nil {
			return fail(fmt.Errorf("pre-deploy snapshot: %w", err))
		}
	}

	rollback := func() error {
		dbPath := oldStore.Path()
		cur, _ := reg.App(appID)
		if cur != nil && cur.Store != oldStore {
			// migrate.Apply's own execution-failure path already closed and
			// reopened the store; use whichever handle is live now.
			dbPath = cur.Store.Path()
			if err := cur.Store.Close(); err != nil {
				return fmt.Errorf("close store for rollback: %w", err)
			}
		} else if err := oldStore.Close(); err != nil {
			return fmt.Errorf("close store for rollback: %w", err)
		}
		if err := migrate.Restore(snapPath, dbPath); err != nil {
			return fmt.Errorf("restore snapshot: %w", err)
		}
		st, err := store.Open(dbPath)
		if err != nil {
			return fmt.Errorf("reopen store after rollback: %w", err)
		}
		if err := os.WriteFile(oldManifestPath, oldManifestBytes, 0o644); err != nil {
			return fmt.Errorf("restore manifest.json: %w", err)
		}
		reg.Register(&registry.RegisteredApp{Schema: oldSchema, Store: st, Dir: oldDir, AssetDir: oldAssetDir})
		return nil
	}

	var mres *migrate.Result
	if kind == KindDeploy {
		mres, err = migrate.Apply(ctx, reg, appID, manifestBytes, opts)
		if err != nil {
			// migrate.Apply already restored the db internally on an execution
			// failure, or made no change at all on a pre-execution refusal —
			// either way the data side is already back to the prior version.
			return fail(fmt.Errorf("data migration: %w", err))
		}
		ra, _ = reg.App(appID) // migrate.Apply re-registered the new schema.
	}

	var newAssetDir string
	if newApp.Frontend != nil {
		newAssetDir, err = buildFrontend(ra.Dir, job.ID, newApp.Frontend)
		if err != nil {
			if kind == KindDeploy {
				if rerr := rollback(); rerr != nil {
					return fail(fmt.Errorf("frontend build failed (%v); rollback ALSO failed: %w", err, rerr))
				}
				return fail(fmt.Errorf("frontend build failed; the deploy was rolled back to the prior version: %w", err))
			}
			return fail(fmt.Errorf("frontend build failed: %w", err))
		}
		if err := bst.SetAssetDir(job.ID, newAssetDir); err != nil {
			if kind == KindDeploy {
				_ = rollback()
			}
			return fail(fmt.Errorf("record build artifact: %w", err))
		}
	}

	if _, err := bst.Transition(job.ID, StateActivating, ""); err != nil {
		return nil, fmt.Errorf("transition to activating: %w", err)
	}

	// Activation cutover: the durable pointer (platform db) and the in-memory
	// registry are updated together, after the new artifact is fully on disk
	// and the data migration has committed. The old asset directory is never
	// touched before this point, so a request arriving mid-cutover always sees
	// either the complete old artifact or the complete new one.
	finalAssetDir := ra.AssetDir
	switch {
	case newApp.Frontend != nil:
		finalAssetDir = newAssetDir
	case kind == KindDeploy:
		// The new manifest explicitly dropped its frontend: an intentional
		// spec change, not a failure, so the app becomes API-only.
		finalAssetDir = ""
	}
	if finalAssetDir != "" {
		if err := bst.PromoteActive(appID, job.ID, finalAssetDir, newApp.Version); err != nil {
			return nil, fmt.Errorf("promote active build: %w", err)
		}
	}
	reg.Register(&registry.RegisteredApp{Schema: ra.Schema, Store: ra.Store, Dir: ra.Dir, AssetDir: finalAssetDir})

	if _, err := bst.Transition(job.ID, StateReady, ""); err != nil {
		return nil, fmt.Errorf("transition to ready: %w", err)
	}
	job.State = StateReady

	if snapPath != "" {
		_ = migrate.Prune(filepath.Join(ra.Dir, migrate.SnapshotDirName), migrate.DefaultRetention)
	}
	if newAssetDir != "" {
		_ = pruneOldBuilds(ra.Dir, job.ID, DefaultBuildRetention)
	}
	return &Result{Job: job, MigrateResult: mres}, nil
}
