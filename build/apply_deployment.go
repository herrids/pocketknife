package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"pocketknife/registry"
	"pocketknife/validate"
)

// ApplyDeployment is the one safe public entry point for "install or deploy
// this app": every ordinary caller (the HTTP POST /deploy ingest endpoint,
// the CLI, and any future caller — an MCP deploy tool, say) should call this
// rather than Deploy or Bootstrap directly. It owns the whole protocol a
// caller would otherwise have to get right itself:
//
//	acquire the per-app lock (bst.LockApp)
//	  -> re-check whether the app is already registered
//	  -> Bootstrap (unknown app id) or Deploy (known app id)
//	  -> extract bundle first, for Deploy, since only Bootstrap does that itself
//	  -> release the lock
//
// The lock is held across the existence check and the call, so a second
// concurrent request for the same app id can never decide Bootstrap-vs-Deploy
// against information that's already gone stale — it waits, then re-decides
// once it actually has the lock. manifestBytes is validated once here (to
// learn the app id to lock on); Deploy and Bootstrap each validate it again
// themselves, independent of this call, exactly as they always have.
//
// bundle is the frontend's built dist tree, or nil if this deployment carries
// no new frontend bundle (e.g. the CLI, which reads an already-on-disk
// dist directory and never has a bundle reader to extract). appsDir is only
// used for the Bootstrap path (a brand-new app id); pass the same apps
// directory Deploy's caller would have used for an existing one.
//
// Deploy and Bootstrap remain exported: this package's own tests call them
// directly to exercise fine-grained failure/rollback behavior that would be
// awkward to reach any other way, and a caller that already holds appID's
// lock and definitively knows which path applies (there are none outside
// this package today) may still call them directly. Every other caller
// should prefer ApplyDeployment.
func ApplyDeployment(ctx context.Context, reg *registry.Registry, bst *Store, appsDir string, manifestBytes []byte, bundle io.Reader, opts DeployOptions) (*Result, error) {
	app, verrs := validate.Manifest(manifestBytes)
	if len(verrs) > 0 {
		return nil, fmt.Errorf("manifest failed validation: %s", verrs.Error())
	}

	unlock := bst.LockApp(app.ID)
	defer unlock()

	ra, exists := reg.App(app.ID)
	if !exists {
		return Bootstrap(reg, bst, appsDir, manifestBytes, bundle)
	}

	// Deploy assumes the frontend dist directory is already on disk —
	// unlike Bootstrap, which extracts its own bundle internally. Hide that
	// asymmetry here: extract before handing off, exactly as if Bootstrap
	// had done it, so the caller never needs to know the two paths differ.
	if app.Frontend != nil && bundle != nil {
		distDir := filepath.Join(ra.Dir, app.Frontend.Dist)
		if err := os.RemoveAll(distDir); err != nil {
			return nil, fmt.Errorf("clear previous bundle: %w", err)
		}
		if err := ExtractBundle(bundle, distDir); err != nil {
			return nil, fmt.Errorf("extract frontend bundle: %w", err)
		}
	}
	return Deploy(ctx, reg, bst, app.ID, manifestBytes, opts)
}
