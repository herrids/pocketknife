// Owns the one job's lifecycle end to end: the planner session, the last
// manifest+client pair that validate_manifest ever accepted, the scratch
// directory the builder authors into, and the one call to submit. submit()
// is the only irreversible action in this whole system, and it is plain
// orchestrator code — never a tool either model can call.
//
// Update mode: when loadExistingApp(appId) is called before startPlanning,
// the orchestrator fetches the app's current manifest + source from the
// backend, seeds the planner with the real manifest as context, and seeds
// the builder's scratch dir with the real source tree rather than the blank
// scaffold. The app id and stable ids are preserved throughout; a submit-time
// guard asserts this before the irreversible deploy.

import { randomUUID } from "node:crypto";
import { cp, mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import * as tar from "tar";

import { Planner } from "./planner.js";
import { runBuilder } from "./builder.js";
import { buildFrontend } from "./build-frontend.js";
import { selectValidator, selectSubmitter, selectFetcher } from "./seams/select.js";
import { withJobTrace } from "./tracing.js";
import type { Submitter } from "./seams/submitter.js";
import type { Validator } from "./seams/validator.js";
import type { AppSourceFetcher } from "./seams/fetcher.js";
import type { ValidatedManifest } from "./tools/validate-tool.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PROJECT_ROOT = path.resolve(__dirname, "..");
const SCRATCH_ROOT = path.join(PROJECT_ROOT, ".scratch");
const OUT_ROOT = path.join(PROJECT_ROOT, "out");
const SCAFFOLD_DIR = path.join(PROJECT_ROOT, "templates", "frontend");

export interface OrchestratorCallbacks {
  onPlannerText?: (text: string) => void;
  onBuilderText?: (text: string) => void;
}

export class Orchestrator {
  readonly jobId = randomUUID();
  readonly scratchDir = path.join(SCRATCH_ROOT, this.jobId);

  private readonly validator: Validator = selectValidator();
  private readonly submitter: Submitter = selectSubmitter(OUT_ROOT);
  private readonly fetcher: AppSourceFetcher = selectFetcher();
  private readonly planner: Planner;
  private lastValid: ValidatedManifest | undefined;
  private readyToBuild = false;

  // Update-mode state. Set by loadExistingApp() before startPlanning().
  private updateAppId: string | undefined;
  private fetchedManifest: unknown | undefined;
  private fetchedSourceBuffer: Buffer | undefined;
  private fetchedHasSource = false;

  constructor(private readonly callbacks: OrchestratorCallbacks = {}) {
    this.planner = new Planner(this.validator, {
      onText: callbacks.onPlannerText,
      onValidManifest: (result) => {
        this.lastValid = result;
      },
      onReadyToBuild: () => {
        // Trust the model's intent-reading, not its judgment on validity --
        // this is the actual gate. A stale or mistaken tool call can't force
        // a transition without a manifest that genuinely validated.
        if (this.lastValid) this.readyToBuild = true;
      },
    });
  }

  /**
   * Loads an existing app from the backend before planning begins. Must be
   * called before startPlanning(). Fails fast if the app is not found.
   */
  async loadExistingApp(appId: string): Promise<void> {
    const result = await this.fetcher.fetch(appId);
    this.updateAppId = appId;
    this.fetchedManifest = result.manifest;
    this.fetchedHasSource = result.hasSource;
    this.fetchedSourceBuffer = result.sourceBuffer;
  }

  async startPlanning(prompt: string, pastedCode?: string): Promise<void> {
    await withJobTrace(this.jobId, "planner", () =>
      this.planner.start(prompt, pastedCode, this.fetchedManifest),
    );
  }

  async refinePlan(userText: string): Promise<void> {
    await withJobTrace(this.jobId, "planner", () => this.planner.refine(userText));
  }

  /** True once the planner has reported (via ready_to_build) that the user wants to proceed. */
  isReadyToBuild(): boolean {
    return this.readyToBuild;
  }

  /**
   * The planner -> builder transition. In new-app mode, seeds the blank
   * React/Vite/Tailwind scaffold. In update mode, seeds either the fetched
   * source tree (when source was stored) or the blank scaffold (graceful
   * fallback for legacy apps). Always writes the latest validated manifest
   * and regenerates src/client.ts before running the builder.
   */
  async build(): Promise<void> {
    if (!this.lastValid) {
      throw new Error(
        "no validated manifest yet -- refine the plan until validate_manifest returns valid: true",
      );
    }

    await mkdir(this.scratchDir, { recursive: true });

    if (this.updateAppId && this.fetchedHasSource && this.fetchedSourceBuffer) {
      // Update mode with stored source: extract the real source tree into the
      // scratch dir instead of copying the blank scaffold.
      await extractTarGz(this.fetchedSourceBuffer, this.scratchDir);
    } else {
      // New-app mode, or update mode for a legacy app without stored source:
      // seed from the blank scaffold (skip stray install/build artifacts).
      await cp(SCAFFOLD_DIR, this.scratchDir, {
        recursive: true,
        filter: (src) => {
          const base = path.basename(src);
          return base !== "node_modules" && base !== "dist";
        },
      });
    }

    await writeFile(
      path.join(this.scratchDir, "manifest.json"),
      JSON.stringify(this.lastValid.manifest, null, 2),
    );
    await writeFile(path.join(this.scratchDir, "src", "client.ts"), this.lastValid.client);

    await withJobTrace(this.jobId, "builder", async () => {
      await runBuilder(this.scratchDir, { onText: this.callbacks.onBuilderText });
      await buildFrontend(this.scratchDir, { onText: this.callbacks.onBuilderText });
    });
  }

  /** The one irreversible action. Never call this from inside the planner or builder loop. */
  async submit(): Promise<{ appId: string }> {
    if (!this.lastValid) {
      throw new Error("no validated manifest to submit");
    }

    // Submit-time guard: in update mode assert the manifest's app.id was not
    // reminted. A reminted id would route the deploy to firstInstall instead
    // of redeploy, orphaning the original app's data.
    if (this.updateAppId) {
      const manifest = this.lastValid.manifest as { app?: { id?: string } };
      const outgoingId = manifest?.app?.id;
      if (outgoingId !== this.updateAppId) {
        throw new Error(
          `app id mismatch: fetched app "${this.updateAppId}" but manifest has app.id "${outgoingId}". ` +
            "The planner must not change or remint app.id when updating an existing app.",
        );
      }
    }

    return this.submitter.submit({
      jobId: this.jobId,
      manifest: this.lastValid.manifest,
      scratchDir: this.scratchDir,
    });
  }
}

/** Extracts a gzipped tar Buffer into destDir (mirrors what StoreSource does on the backend). */
async function extractTarGz(buf: Buffer, destDir: string): Promise<void> {
  const { Readable } = await import("node:stream");
  const { pipeline } = await import("node:stream/promises");
  const readable = Readable.from(buf);
  // tar.extract() returns a Writable stream; pipeline handles backpressure and errors.
  await pipeline(readable, tar.extract({ cwd: destDir, gzip: true }) as unknown as NodeJS.WritableStream);
}
