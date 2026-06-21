package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"pocketknife/materialize"
	"pocketknife/store"
	"pocketknife/validate"
)

// LoadResult records the outcome of processing one manifest during boot. It lets
// the caller log skipped (invalid) manifests without aborting the whole boot.
type LoadResult struct {
	Dir          string
	ManifestPath string
	AppID        string
	OK           bool
	Errors       validate.Errors
	Err          error
}

// Load scans appsDir for */manifest.json, then for each: validates (the hard
// gate), materializes its database idempotently, and registers the compiled
// schema. An invalid or unprocessable manifest is recorded in the returned
// results and skipped — never served — but does not stop the others.
//
// Load is the natural seam for a future migration engine: between materialize
// and register, a migrate(stored, new) step would reconcile a changed manifest
// against an existing data.db. v1 assumes manifest and data.db are consistent
// and only ever runs idempotent CREATE statements.
func Load(appsDir string) (*Registry, []LoadResult, error) {
	matches, err := filepath.Glob(filepath.Join(appsDir, "*", "manifest.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("scan apps dir: %w", err)
	}
	sort.Strings(matches)

	reg := New()
	var results []LoadResult

	for _, manifestPath := range matches {
		dir := filepath.Dir(manifestPath)
		res := LoadResult{Dir: dir, ManifestPath: manifestPath}

		data, err := os.ReadFile(manifestPath)
		if err != nil {
			res.Err = fmt.Errorf("read manifest: %w", err)
			results = append(results, res)
			continue
		}

		app, verrs := validate.Manifest(data)
		if len(verrs) > 0 {
			res.Errors = verrs
			results = append(results, res)
			continue
		}
		res.AppID = app.ID

		stmts, err := materialize.Statements(app)
		if err != nil {
			res.Err = fmt.Errorf("materialize: %w", err)
			results = append(results, res)
			continue
		}

		st, err := store.Open(filepath.Join(dir, "data.db"))
		if err != nil {
			res.Err = fmt.Errorf("open store: %w", err)
			results = append(results, res)
			continue
		}
		if err := st.ApplyDDL(stmts); err != nil {
			st.Close()
			res.Err = fmt.Errorf("apply ddl: %w", err)
			results = append(results, res)
			continue
		}

		// Seam: migrate(storedManifest, app) would go here before serving.

		reg.Register(&RegisteredApp{Schema: app, Store: st, Dir: dir})
		res.OK = true
		results = append(results, res)
	}

	return reg, results, nil
}
