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
      "bounds). On success, confirms the manifest is valid — do not call ready_to_build until " +
      "this returns valid. On failure, returns the specific errors to fix — repair and revalidate.",
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
              text: "Manifest is valid. The client surface is stored and will be used by the builder.",
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

export function createApproveConceptTool(onConceptApproved: () => void) {
  return tool(
    "approve_concept",
    "Call this once the user has confirmed they are happy with the concept description " +
      "you presented — the screens, user actions, and key features, in plain language " +
      "without any schema detail. Do not call this before presenting a concept, and do " +
      "not begin drafting a manifest until this tool has been called successfully.",
    {},
    async () => {
      onConceptApproved();
      return {
        content: [{ type: "text" as const, text: "Concept approved — you may now draft the manifest." }],
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
  onConceptApproved: () => void,
  onReady: () => void,
): McpSdkServerConfigWithInstance {
  return createSdkMcpServer({
    name: "pocketknife",
    version: "0.1.0",
    tools: [
      createApproveConceptTool(onConceptApproved),
      createValidateManifestTool(validator, onValid),
      createReadyToBuildTool(onReady),
    ],
  });
}
