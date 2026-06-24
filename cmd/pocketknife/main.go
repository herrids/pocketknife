// Command pocketknife is the single generic server plus the headless
// migration and build engines. With no subcommand it scans the apps
// directory, validates and materializes each manifest, registers the
// compiled schemas, reconciles build state, and serves the schema-driven API
// plus each app's activated frontend over one origin. The "migrate"
// subcommand evolves one app's schema to a new manifest version without
// losing data. The "build" subcommand (re)builds and activates an app's
// frontend for its current manifest, or — given a new manifest version —
// runs a full second deploy: data migration, frontend rebuild and
// activation landed as one operation with a single rollback contract.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"pocketknife/api"
	"pocketknife/assets"
	"pocketknife/build"
	"pocketknife/cors"
	"pocketknife/migrate"
	"pocketknife/registry"
	"pocketknife/validateapi"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "migrate":
			runMigrate(os.Args[2:])
			return
		case "build":
			runBuild(os.Args[2:])
			return
		}
	}
	runServe(os.Args[1:])
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	appsDir := fs.String("apps", "apps", "directory containing <app_id>/manifest.json")
	addr := fs.String("addr", ":8080", "address to listen on")
	platformDBPath := fs.String("platform-db", "platform.db", "path to the platform build-job database")
	corsEnabled := fs.Bool("cors", false, "allow cross-origin requests (for a frontend served by a separate dev server)")
	_ = fs.Parse(args)

	reg, results, err := registry.Load(*appsDir)
	if err != nil {
		log.Fatalf("boot failed: %v", err)
	}
	defer reg.Close()

	for _, res := range results {
		switch {
		case res.OK:
			log.Printf("registered app %q from %s", res.AppID, res.ManifestPath)
		case len(res.Errors) > 0:
			log.Printf("SKIPPED %s — manifest failed validation:", res.ManifestPath)
			for _, e := range res.Errors {
				log.Printf("    %s", e.String())
			}
		case res.Err != nil:
			log.Printf("SKIPPED %s — %v", res.ManifestPath, res.Err)
		}
	}

	if len(reg.Apps()) == 0 {
		log.Printf("warning: no apps registered; serving an empty runtime")
	}

	bst, err := build.Open(*platformDBPath)
	if err != nil {
		log.Fatalf("open platform db: %v", err)
	}
	defer bst.Close()

	rres, err := build.Reconcile(reg, bst)
	if err != nil {
		log.Fatalf("boot reconciliation failed: %v", err)
	}
	for _, j := range rres.FailedJobs {
		log.Printf("reconciled in-flight build job %s (app %q) to failed: interrupted by restart", j.ID, j.AppID)
	}
	for _, id := range rres.Activated {
		log.Printf("reattached active build for app %q", id)
	}
	for _, id := range rres.Broken {
		log.Printf("warning: app %q has a stale or missing active-build pointer; serving API-only", id)
	}

	mux := http.NewServeMux()
	mux.Handle("/apps/", api.NewServer(reg))
	mux.Handle("/builds/", build.NewStatusServer(bst, reg))
	mux.Handle("/ui/", assets.NewServer(reg))
	mux.Handle("/validate", validateapi.NewServer())

	handler := cors.Middleware(*corsEnabled, mux)
	log.Printf("pocketknife listening on %s (apps dir: %s)", *addr, *appsDir)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// runMigrate drives the apply-changeset flow for one app. The app's current
// on-disk manifest is the "old" side; -to names the proposed next version. A
// destructive migration needs -confirm and, where required, witnesses supplied
// via a -witnesses JSON file (keyed by stable field id).
func runMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	appsDir := fs.String("apps", "apps", "directory containing <app_id>/manifest.json")
	appID := fs.String("app", "", "id of the app to migrate (required)")
	toPath := fs.String("to", "", "path to the new manifest.json (required)")
	confirm := fs.Bool("confirm", false, "confirm destructive operations")
	witnessPath := fs.String("witnesses", "", "path to a JSON file of witnesses keyed by field id")
	_ = fs.Parse(args)

	if *appID == "" || *toPath == "" {
		log.Fatalf("usage: pocketknife migrate -app <id> -to <new_manifest.json> [-confirm] [-witnesses <file.json>]")
	}

	reg, _, err := registry.Load(*appsDir)
	if err != nil {
		log.Fatalf("boot failed: %v", err)
	}
	defer reg.Close()

	newBytes, err := os.ReadFile(*toPath)
	if err != nil {
		log.Fatalf("read new manifest: %v", err)
	}

	opts := migrate.Options{Confirm: *confirm}
	if *witnessPath != "" {
		wb, err := os.ReadFile(*witnessPath)
		if err != nil {
			log.Fatalf("read witnesses: %v", err)
		}
		if err := json.Unmarshal(wb, &opts.Witnesses); err != nil {
			log.Fatalf("parse witnesses %s: %v", *witnessPath, err)
		}
	}

	res, err := migrate.Apply(context.Background(), reg, *appID, newBytes, opts)
	if res != nil && res.Changeset != nil && !res.NoChange {
		printChangeset(res.Changeset)
	}
	if err != nil {
		log.Fatalf("%v", err)
	}
	if res.NoChange {
		log.Printf("no changes: %q is already at the target schema", *appID)
		return
	}
	if res.SnapshotPath != "" {
		log.Printf("snapshot saved at %s", res.SnapshotPath)
	}
	log.Printf("migration applied: %q is now at version %d", *appID, res.Changeset.ToVersion)
}

// runBuild drives one Deploy call for an app: a frontend-only (re)build and
// activation of its current manifest version when -to is omitted, or a full
// second deploy — data migration plus frontend rebuild and activation, with
// rollback to the prior good version on any failure — when -to names a new
// manifest version.
func runBuild(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	appsDir := fs.String("apps", "apps", "directory containing <app_id>/manifest.json")
	platformDBPath := fs.String("platform-db", "platform.db", "path to the platform build-job database")
	appID := fs.String("app", "", "id of the app to build (required)")
	toPath := fs.String("to", "", "path to a new manifest.json (omit to rebuild the current version)")
	confirm := fs.Bool("confirm", false, "confirm destructive data migration operations")
	witnessPath := fs.String("witnesses", "", "path to a JSON file of witnesses keyed by field id")
	_ = fs.Parse(args)

	if *appID == "" {
		log.Fatalf("usage: pocketknife build -app <id> [-to <new_manifest.json>] [-confirm] [-witnesses <file.json>]")
	}

	reg, _, err := registry.Load(*appsDir)
	if err != nil {
		log.Fatalf("boot failed: %v", err)
	}
	defer reg.Close()

	bst, err := build.Open(*platformDBPath)
	if err != nil {
		log.Fatalf("open platform db: %v", err)
	}
	defer bst.Close()

	if _, err := build.Reconcile(reg, bst); err != nil {
		log.Fatalf("boot reconciliation failed: %v", err)
	}

	ra, ok := reg.App(*appID)
	if !ok {
		log.Fatalf("unknown app %q", *appID)
	}

	manifestPath := *toPath
	if manifestPath == "" {
		manifestPath = filepath.Join(ra.Dir, "manifest.json")
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("read manifest: %v", err)
	}

	opts := build.DeployOptions{Confirm: *confirm}
	if *witnessPath != "" {
		wb, err := os.ReadFile(*witnessPath)
		if err != nil {
			log.Fatalf("read witnesses: %v", err)
		}
		if err := json.Unmarshal(wb, &opts.Witnesses); err != nil {
			log.Fatalf("parse witnesses %s: %v", *witnessPath, err)
		}
	}

	res, err := build.Deploy(context.Background(), reg, bst, *appID, manifestBytes, opts)
	if err != nil {
		if res != nil && res.Job != nil {
			log.Fatalf("build failed (job %s, state %s): %v", res.Job.ID, res.Job.State, err)
		}
		log.Fatalf("build failed: %v", err)
	}
	log.Printf("build succeeded: job %s is %s", res.Job.ID, res.Job.State)
}

func printChangeset(cs *migrate.Changeset) {
	log.Printf("changeset for %q (v%d -> v%d): %d operation(s)", cs.AppID, cs.FromVersion, cs.ToVersion, len(cs.Ops))
	for _, op := range cs.Ops {
		log.Printf("    %s", op.String())
	}
}
