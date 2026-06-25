// The frontend-authoring stage. Runs once, after the planner has reached a
// validated manifest and the orchestrator has seeded the React/Vite/Tailwind
// scaffold into a fresh scratch directory and written manifest.json and
// src/client.ts into it. The builder's cwd is pinned to that scratch directory
// and its tool set is Read/Write/Edit/Glob/Skill — nothing that reaches outside
// it on its own — and the scratch guard hook enforces that boundary even if the
// prompt or the model's judgment fails. The actual `vite build` runs afterward
// as plain orchestrator code (build-frontend.ts), never as a tool the model holds.

import type { Options } from "@anthropic-ai/claude-agent-sdk";

import { query } from "./tracing.js";
import { createScratchGuard } from "./hooks/scratch-guard.js";

const PROMPT = `Your current directory holds a complete Vite + React + TypeScript + Tailwind +
shadcn/ui scaffold that already builds, plus two files describing the app to build:
manifest.json and src/client.ts.

Author the app's frontend following the "pocketknife-frontend" skill exactly: read the
manifest, the generated client, and the scaffold's components; then build a polished,
phone-quality React app — an app shell, one considered view per read-enabled entity, and
designed loading/empty/error/success states — composing the scaffold's shadcn/ui
components and design tokens. Route every read and write through the generated client
(never a raw fetch), and write all output under the current directory. The project must
still pass tsc and vite build when you're done.

When you're done, give a one-paragraph summary of what you built.`;

export interface BuilderCallbacks {
  onText?: (text: string) => void;
}

/**
 * The sandboxed SDK configuration shared by the builder and any follow-up fix
 * session: cwd pinned to the scratch dir, a tool set that can touch disk but
 * not run commands or reach the network, and the scratch-guard hook as the
 * hard boundary. Reused by build-frontend.ts so a self-heal pass runs under the
 * exact same constraints as the original authoring pass.
 */
export function sandboxedBuilderOptions(scratchDir: string): Options {
  return {
    cwd: scratchDir,
    tools: ["Read", "Write", "Edit", "Glob", "Skill"],
    permissionMode: "acceptEdits",
    settingSources: ["project"],
    skills: ["pocketknife-frontend"],
    hooks: {
      PreToolUse: [createScratchGuard(scratchDir)],
    },
  };
}

/** Streams an SDK query's assistant text to the callback. */
export async function streamQuery(
  prompt: string,
  options: Options,
  onText?: (text: string) => void,
): Promise<void> {
  for await (const message of query({ prompt, options })) {
    if (message.type === "assistant") {
      for (const block of message.message.content) {
        if (block.type === "text") {
          onText?.(block.text);
        }
      }
    }
  }
}

export async function runBuilder(scratchDir: string, callbacks: BuilderCallbacks = {}): Promise<void> {
  await streamQuery(PROMPT, sandboxedBuilderOptions(scratchDir), callbacks.onText);
}
