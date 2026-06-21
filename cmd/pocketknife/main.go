// Command pocketknife is the single generic server. It scans the apps
// directory, validates and materializes each manifest, registers the compiled
// schemas, and serves one schema-driven API over all of them.
package main

import (
	"flag"
	"log"
	"net/http"

	"pocketknife/api"
	"pocketknife/registry"
)

func main() {
	appsDir := flag.String("apps", "apps", "directory containing <app_id>/manifest.json")
	addr := flag.String("addr", ":8080", "address to listen on")
	flag.Parse()

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
