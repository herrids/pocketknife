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
// After materializing, Load verifies the resulting database actually matches
// the manifest (Store.VerifySchema) — CREATE TABLE IF NOT EXISTS is a no-op
// against an existing, differently-shaped table, so an app whose manifest.json
// changed without ever being migrated is detected and skipped here, rather
// than silently served against a schema its database no longer has. This is
// detection only: Load never migrates data on an app's behalf.
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

		if err := st.VerifySchema(app); err != nil {
			st.Close()
			res.Err = fmt.Errorf("manifest/database consistency check failed: %w", err)
			results = append(results, res)
			continue
		}

		reg.Register(&RegisteredApp{Schema: app, Store: st, Dir: dir})
		res.OK = true
		results = append(results, res)
	}

	return reg, results, nil
}
