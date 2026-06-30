#!/usr/bin/env node
// Standalone entrypoint: npm run agent -- "<prompt>" [--paste <file>] [--app <app_id>]
// Drives the orchestrator from the terminal. The planner reads the user's
// intent to proceed (any phrasing) and reports it via ready_to_build; this
// loop just polls that flag after each turn. The actual transition to build,
// and the final submit confirmation, still happen here deterministically --
// the model can signal intent but never triggers either itself.
//
// --app <app_id>     Update an existing app: fetch its current manifest + source
//                    from the backend and seed the planning session from them.
//                    Requires SUBMIT_MODE=http and a running backend.
// --bridge-mode      Newline-delimited JSON stdio protocol for the web shell's
//                    agent bridge. Reads {"type":"message","text":"..."} from
//                    stdin; emits turn/plan/ready/error/done events to stdout.
// --prompt <text>    Provide initial prompt as a flag (used by --bridge-mode).

import { createInterface } from "node:readline/promises";
import { readFile } from "node:fs/promises";

import { shutdownTracing } from "./tracing.js";
import { Orchestrator } from "./orchestrator.js";
import { emitEvent, BridgeInput, extractChecklist } from "./bridge.js";

function parseArgs(argv: string[]): {
  prompt: string;
  pasteFile?: string;
  updateAppId?: string;
  bridgeMode: boolean;
} {
  const rest: string[] = [];
  let pasteFile: string | undefined;
  let updateAppId: string | undefined;
  let bridgeMode = false;
  let promptFlag: string | undefined;
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--paste") {
      pasteFile = argv[++i];
    } else if (argv[i] === "--app") {
      updateAppId = argv[++i];
    } else if (argv[i] === "--bridge-mode") {
      bridgeMode = true;
    } else if (argv[i] === "--prompt") {
      promptFlag = argv[++i];
    } else {
      rest.push(argv[i]);
    }
  }
  const prompt = promptFlag ?? rest.join(" ");
  return { prompt, pasteFile, updateAppId, bridgeMode };
}

async function main(): Promise<void> {
  const { prompt, pasteFile, updateAppId, bridgeMode } = parseArgs(process.argv.slice(2));
  if (!prompt) {
    if (bridgeMode) {
      emitEvent({ type: "error", reason: "prompt is required" });
    } else {
      console.error('usage: npm run agent -- "<prompt>" [--paste <file>] [--app <app_id>]');
    }
    process.exitCode = 1;
    return;
  }

  const pastedCode = pasteFile ? await readFile(pasteFile, "utf8") : undefined;

  // ── Bridge mode ──────────────────────────────────────────────────────────
  if (bridgeMode) {
    const bridge = new BridgeInput();
    let lastManifest: unknown;
    let lastClient = "";
    let lastManifestVersion = 0;
    let pendingAppId = "";

    const orchestrator = new Orchestrator({
      onPlannerText: (text) => emitEvent({ type: "turn", role: "assistant", text }),
      onBuilderText: () => { /* suppress in bridge mode */ },
    });

    // Hook into validate_manifest success via the orchestrator's onValidManifest
    // callback — we need to emit a plan event when a manifest validates.
    const origOnValid = (orchestrator as unknown as {
      planner: { _onValidManifest?: (r: { manifest: unknown; client: string }) => void }
    }).planner;
    // We'll capture this via the orchestrator callbacks below by wrapping.

    try {
      if (updateAppId) {
        await orchestrator.loadExistingApp(updateAppId);
        pendingAppId = updateAppId;
      }

      // Start planning. plannerText events are already emitted via callback.
      await orchestrator.startPlanning(prompt, pastedCode);

      // After the first turn, if the manifest validated emit a plan event.
      // We piggyback on the orchestrator's conceptApproved/readyToBuild state
      // by polling: emit plan if we have a valid manifest.

      // Refinement loop: read user messages from stdin.
      while (!orchestrator.isReadyToBuild()) {
        const userText = await bridge.nextMessage();
        await orchestrator.refinePlan(userText);
      }

      // Extract manifest details for the ready event.
      // The orchestrator's lastValid is private, so we infer from the
      // manifest it has through submit's check. We call a no-op to get the
      // version: peek at the manifest version indirectly.
      const manifestProxy = (orchestrator as unknown as {
        lastValid?: { manifest: { app?: { version?: number; id?: string } }; client: string }
      }).lastValid;

      if (manifestProxy) {
        lastManifest = manifestProxy.manifest;
        lastClient = manifestProxy.client;
        lastManifestVersion = manifestProxy.manifest?.app?.version ?? 0;
        pendingAppId = manifestProxy.manifest?.app?.id ?? pendingAppId;

        emitEvent({
          type: "plan",
          checklist: extractChecklist(lastClient, lastManifest),
        });
      }

      emitEvent({ type: "ready", manifestVersion: lastManifestVersion, appId: pendingAppId });

      // Wait for the shell to send {"type":"approve"}.
      await bridge.waitForApprove();

      // Build and submit.
      await orchestrator.build();
      const result = await orchestrator.submit();
      pendingAppId = result.appId;

      emitEvent({ type: "done", appId: pendingAppId });
    } catch (err) {
      const reason = err instanceof Error ? err.message : String(err);
      emitEvent({ type: "error", reason });
      process.exitCode = 1;
    } finally {
      bridge.close();
    }
    return;
  }

  // ── Interactive CLI mode (unchanged) ─────────────────────────────────────
  const orchestrator = new Orchestrator({
    onPlannerText: (text) => console.log(`\n${text}\n`),
    onBuilderText: (text) => console.log(`\n[builder] ${text}\n`),
  });

  // In update mode, fetch the existing app's manifest + source before planning.
  if (updateAppId) {
    console.log(`Fetching existing app "${updateAppId}"...`);
    await orchestrator.loadExistingApp(updateAppId);
    console.log(`Loaded app "${updateAppId}". Planning updates...\n`);
  }

  console.log(`pocketknife-agent — job ${orchestrator.jobId}`);
  if (!updateAppId) {
    console.log("Describe refinements in plain language. Say when you're ready to build.\n");
  }

  await orchestrator.startPlanning(prompt, pastedCode);

  const rl = createInterface({ input: process.stdin, output: process.stdout });
  try {
    while (!orchestrator.isReadyToBuild()) {
      const line = (await rl.question("> ")).trim();
      if (line.length === 0) continue;

      await orchestrator.refinePlan(line);
    }

    console.log("\nBuilding frontend...\n");
    await orchestrator.build();

    const confirm = (await rl.question("\nSubmit this app? [y/N] ")).trim().toLowerCase();
    if (confirm !== "y" && confirm !== "yes") {
      console.log("Not submitted.");
      return;
    }
  } finally {
    rl.close();
  }

  const result = await orchestrator.submit();
  console.log(`\nSubmitted. appId: ${result.appId}`);
}

main()
  .catch((err) => {
    console.error(err);
    process.exitCode = 1;
  })
  .finally(() => shutdownTracing());
