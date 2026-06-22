// Command pocketknife is the single generic server plus the headless migration
// engine. With no subcommand it scans the apps directory, validates and
// materializes each manifest, registers the compiled schemas, and serves one
// schema-driven API over all of them. The "migrate" subcommand evolves one app's
// schema to a new manifest version without losing data.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"

	"pocketknife/api"
	"pocketknife/migrate"
	"pocketknife/registry"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		runMigrate(os.Args[2:])
		return
	}
	runServe(os.Args[1:])
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	appsDir := fs.String("apps", "apps", "directory containing <app_id>/manifest.json")
	addr := fs.String("addr", ":8080", "address to listen on")
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

	handler := api.NewServer(reg)
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

func printChangeset(cs *migrate.Changeset) {
	log.Printf("changeset for %q (v%d -> v%d): %d operation(s)", cs.AppID, cs.FromVersion, cs.ToVersion, len(cs.Ops))
	for _, op := range cs.Ops {
		log.Printf("    %s", op.String())
	}
}
