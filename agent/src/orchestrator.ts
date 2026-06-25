// Owns the one job's lifecycle end to end: the planner session, the last
// manifest+client pair that validate_manifest ever accepted, the scratch
// directory the builder authors into, and the one call to submit. submit()
// is the only irreversible action in this whole system, and it is plain
// orchestrator code — never a tool either model can call.

import { randomUUID } from "node:crypto";
import { cp, mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { Planner } from "./planner.js";
import { runBuilder } from "./builder.js";
import { buildFrontend } from "./build-frontend.js";
import { selectValidator, selectSubmitter } from "./seams/select.js";
import { withJobTrace } from "./tracing.js";
import type { Submitter } from "./seams/submitter.js";
import type { Validator } from "./seams/validator.js";
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
  private readonly planner: Planner;
  private lastValid: ValidatedManifest | undefined;
  private readyToBuild = false;

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

  async startPlanning(prompt: string, pastedCode?: string): Promise<void> {
    await withJobTrace(this.jobId, "planner", () => this.planner.start(prompt, pastedCode));
  }

  async refinePlan(userText: string): Promise<void> {
    await withJobTrace(this.jobId, "planner", () => this.planner.refine(userText));
  }

  /** True once the planner has reported (via ready_to_build) that the user wants to proceed. */
  isReadyToBuild(): boolean {
    return this.readyToBuild;
  }

  /**
   * The planner -> builder transition. Seeds the React/Vite/Tailwind scaffold
   * into a fresh scratch directory, drops the last validated manifest and its
   * generated client in, runs the builder to author the app, then builds it to
   * a static dist/. Throws if no manifest has validated yet -- this is the
   * approval gate's other half (the CLI decides *when* to call this; this is
   * what makes calling it too early impossible).
   */
  async build(): Promise<void> {
    if (!this.lastValid) {
      throw new Error("no validated manifest yet -- refine the plan until validate_manifest returns valid: true");
    }

    await mkdir(this.scratchDir, { recursive: true });
    // Seed the buildable scaffold (skip any stray install/build artifacts).
    await cp(SCAFFOLD_DIR, this.scratchDir, {
      recursive: true,
      filter: (src) => {
        const base = path.basename(src);
        return base !== "node_modules" && base !== "dist";
      },
    });
    await writeFile(path.join(this.scratchDir, "manifest.json"), JSON.stringify(this.lastValid.manifest, null, 2));
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
    return this.submitter.submit({
      jobId: this.jobId,
      manifest: this.lastValid.manifest,
      scratchDir: this.scratchDir,
    });
  }
}
