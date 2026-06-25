// The validate_manifest tool is the planner's only way to learn whether a
// candidate manifest is usable. Its Zod input schema is intentionally
// permissive (the manifest shape is whatever the model is iterating on); the
// real check happens inside the handler via the injected Validator. This
// handler is also the capture point: a manifest that comes back valid, and
// the client generated from it, are reported to the orchestrator through
// onValid — the only place that pair is ever recorded as "ready."
//
// ready_to_build is how the model signals user intent ("build it", "let's
// go", "ship it", whatever phrasing) without itself performing anything: the
// handler only flips a flag via onReady. The orchestrator decides what that
// flag means (it still requires a manifest that actually validated), so a
// model miscall can't force a transition on its own.

import { z } from "zod";
import { tool, createSdkMcpServer, type McpSdkServerConfigWithInstance } from "@anthropic-ai/claude-agent-sdk";

import type { Validator } from "../seams/validator.js";

export interface ValidatedManifest {
  manifest: unknown;
  client: string;
}

export function createValidateManifestTool(validator: Validator, onValid: (result: ValidatedManifest) => void) {
  return tool(
    "validate_manifest",
    "Validate a candidate Pocketknife app manifest against the structural schema and the " +
      "semantic rules (stable-id uniqueness, reserved names, reference resolution, default " +
      "bounds). On success, returns the generated TypeScript client surface the frontend will " +
      "be authored against. On failure, returns the specific errors to fix — repair the " +
      "manifest and call this again. A manifest is never final until this returns valid: true.",
    {
      manifest: z.record(z.string(), z.unknown()).describe("The full candidate manifest as a JSON object."),
    },
    async (args) => {
      const result = await validator.validate(args.manifest);

      if (result.valid) {
        onValid({ manifest: args.manifest, client: result.client });
        return {
          content: [
            {
              type: "text" as const,
              text: `Manifest is valid.\n\nGenerated TypeScript client surface:\n\n${result.client}`,
            },
          ],
        };
      }

      const lines = result.errors.map((e) => `- ${e.path || "(root)"}: ${e.message}`);
      return {
        isError: true,
        content: [
          {
            type: "text" as const,
            text: `Manifest is invalid. Fix these and revalidate:\n${lines.join("\n")}`,
          },
        ],
      };
    },
  );
}

export function createReadyToBuildTool(onReady: () => void) {
  return tool(
    "ready_to_build",
    "Call this when the user has indicated, in any phrasing (\"build it\", \"let's go\", " +
      "\"ship it\", \"looks good\", etc.), that they want to move forward with the current " +
      "plan — but ONLY if validate_manifest has already returned valid: true for this exact " +
      "manifest (no unvalidated edits since). If the manifest hasn't validated yet, do not " +
      "call this — tell the user what's still missing instead.",
    {},
    async () => {
      onReady();
      return {
        content: [{ type: "text" as const, text: "Acknowledged — handing off to the build stage." }],
      };
    },
  );
}

export function createPocketknifeMcpServer(
  validator: Validator,
  onValid: (result: ValidatedManifest) => void,
  onReady: () => void,
): McpSdkServerConfigWithInstance {
  return createSdkMcpServer({
    name: "pocketknife",
    version: "0.1.0",
    tools: [createValidateManifestTool(validator, onValid), createReadyToBuildTool(onReady)],
  });
}
