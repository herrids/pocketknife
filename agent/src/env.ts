// Side-effect import: loads the repo root's .env into process.env before any
// other module reads it. Must be the first import in cli.ts -- sibling ES
// module imports evaluate in declaration order, so this runs before
// tracing.ts's top-level env reads regardless of how cli.ts was invoked:
// directly via `npm run agent`, or spawned as a subprocess by the Go
// backend's bridge-mode session spawn in platform/plan.go. loadEnvFile never
// overrides a variable already present in process.env. The root .env is
// shared with the Go server, which loads the same file the same way (see
// loadDotEnv in cmd/pocketknife/main.go) -- one config file for the whole app.
import { fileURLToPath } from "node:url";
import path from "node:path";

const envPath = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "..", ".env");

try {
  process.loadEnvFile(envPath);
} catch (err) {
  if ((err as NodeJS.ErrnoException).code !== "ENOENT") throw err;
}
