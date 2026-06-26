package build

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"pocketknife/materialize"
	"pocketknife/registry"
	"pocketknife/store"
	"pocketknife/validate"
)

// Bootstrap brings a brand-new, not-yet-registered app from a manifest plus a
// built frontend bundle to a fully registered, activated app -- the missing
// first half of Deploy for an app id the registry has never seen. It mirrors
// registry.Load's per-app bootstrap (validate -> materialize -> open store ->
// register) and then runs the same build-and-activate tail Deploy's install
// path runs, under one platform build job.
//
// Every step happens inside a temp-named staging directory under appsDir;
// only once the database is built and the frontend artifact is copied does
// the staging directory get renamed to its real apps/<app_id> path and the
// app registered. On any failure before that point, the staging directory is
// removed and nothing is left on disk or in the registry -- the caller is
// free to retry under the same app id.
func Bootstrap(reg *registry.Registry, bst *Store, appsDir string, manifestBytes []byte, bundle io.Reader) (*Result, error) {
	app, verrs := validate.Manifest(manifestBytes)
	if len(verrs) > 0 {
		return nil, fmt.Errorf("manifest failed validation: %s", verrs.Error())
	}

	finalDir := filepath.Join(appsDir, app.ID)
	if _, err := os.Stat(finalDir); err == nil {
		return nil, fmt.Errorf("app directory %q already exists on disk", finalDir)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat %q: %w", finalDir, err)
	}

	tmpDir := filepath.Join(appsDir, ".staging-"+store.NewID())
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "manifest.json"), manifestBytes, 0o644); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("write staged manifest: %w", err)
	}

	job, err := bst.CreateJob(app.ID, KindInstall, app.Version)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("create build job: %w", err)
	}
	if _, err := bst.Transition(job.ID, StateBuilding, ""); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("transition to building: %w", err)
	}

	// cleanupDir tracks whatever directory currently holds the in-progress
	// app -- the staging dir until the rename below succeeds, the final
	// app-id dir afterward -- so fail() always removes the right thing and
	// never leaves a half-built app reachable under its real id.
	cleanupDir := tmpDir
	fail := func(cause error) (*Result, error) {
		os.RemoveAll(cleanupDir)
		if _, terr := bst.Transition(job.ID, StateFailed, cause.Error()); terr != nil {
			return nil, fmt.Errorf("%w (additionally failed to record the job as failed: %v)", cause, terr)
		}
		job.State = StateFailed
		job.Error = cause.Error()
		return &Result{Job: job}, cause
	}

	var assetDir string
	if app.Frontend != nil {
		distDir := filepath.Join(tmpDir, app.Frontend.Dist)
		if err := ExtractBundle(bundle, distDir); err != nil {
			return fail(fmt.Errorf("extract frontend bundle: %w", err))
		}
		if _, err := buildFrontend(tmpDir, job.ID, app.Frontend); err != nil {
			return fail(err)
		}
		// buildFrontend's artifact naming is purely a function of (appDir,
		// jobID); the rename below moves the whole tree, so this path is
		// exactly where the artifact will live once finalDir is real.
		assetDir = filepath.Join(finalDir, BuildsDirName, job.ID)
	}

	stmts, err := materialize.Statements(app)
	if err != nil {
		return fail(fmt.Errorf("materialize: %w", err))
	}

	st, err := store.Open(filepath.Join(tmpDir, "data.db"))
	if err != nil {
		return fail(fmt.Errorf("open store: %w", err))
	}
	if err := st.ApplyDDL(stmts); err != nil {
		st.Close()
		return fail(fmt.Errorf("apply ddl: %w", err))
	}
	if err := st.Close(); err != nil {
		return fail(fmt.Errorf("close staged store: %w", err))
	}

	if err := os.Rename(tmpDir, finalDir); err != nil {
		return fail(fmt.Errorf("activate app directory: %w", err))
	}
	cleanupDir = finalDir

	st, err = store.Open(filepath.Join(finalDir, "data.db"))
	if err != nil {
		return fail(fmt.Errorf("reopen store after activation: %w", err))
	}

	if assetDir != "" {
		if err := bst.SetAssetDir(job.ID, assetDir); err != nil {
			st.Close()
			return fail(fmt.Errorf("record build artifact: %w", err))
		}
	}

	if _, err := bst.Transition(job.ID, StateActivating, ""); err != nil {
		st.Close()
		return fail(fmt.Errorf("transition to activating: %w", err))
	}

	if assetDir != "" {
		if err := bst.PromoteActive(app.ID, job.ID, assetDir, app.Version); err != nil {
			st.Close()
			return fail(fmt.Errorf("promote active build: %w", err))
		}
	}

	// Activation cutover: the app becomes live in the registry only after
	// every disk and platform-db write it depends on has already succeeded.
	reg.Register(&registry.RegisteredApp{Schema: app, Store: st, Dir: finalDir, AssetDir: assetDir})

	if _, err := bst.Transition(job.ID, StateReady, ""); err != nil {
		// The app is already live; a failure to record StateReady is a
		// bookkeeping problem, not a serving one, so it is reported but does
		// not roll back an app that is genuinely up.
		return nil, fmt.Errorf("transition to ready: %w", err)
	}
	job.State = StateReady

	return &Result{Job: job}, nil
}
