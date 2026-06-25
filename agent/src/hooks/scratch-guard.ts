// The builder's hard boundary. The prompt instructs the builder to stay
// inside its scratch directory, but a prompt is not a security boundary —
// this PreToolUse hook is. It denies any Write or Edit whose file_path
// resolves (after following `..` segments and symlinks, including on a
// not-yet-existing file via its parent directory) outside the scratch root.

import fs from "node:fs";
import path from "node:path";
import type { HookCallbackMatcher, PreToolUseHookInput, SyncHookJSONOutput } from "@anthropic-ai/claude-agent-sdk";

export function createScratchGuard(scratchDir: string): HookCallbackMatcher {
  const root = fs.realpathSync(scratchDir);

  return {
    matcher: "Write|Edit",
    hooks: [
      async (input) => {
        const toolInput = (input as PreToolUseHookInput).tool_input as { file_path?: unknown } | undefined;
        const filePath = toolInput?.file_path;

        if (typeof filePath !== "string") {
          return { continue: true };
        }

        if (isInsideScratch(filePath, root)) {
          return { continue: true };
        }

        const denied: SyncHookJSONOutput = {
          hookSpecificOutput: {
            hookEventName: "PreToolUse",
            permissionDecision: "deny",
            permissionDecisionReason: `Refusing to write outside the scratch directory (${root}). Attempted path: ${filePath}`,
          },
        };
        return denied;
      },
    ],
  };
}

/** Resolves `..` segments and symlinks (falling back to the parent directory's
 * real path when the target file doesn't exist yet) before checking containment,
 * so neither a relative escape nor a symlink pointing outside scratch can slip
 * through. */
function isInsideScratch(filePath: string, root: string): boolean {
  const absolute = path.resolve(filePath);
  const real = realpathOrParent(absolute);
  const rel = path.relative(root, real);
  return rel === "" || (!rel.startsWith("..") && !path.isAbsolute(rel));
}

function realpathOrParent(absolute: string): string {
  try {
    return fs.realpathSync(absolute);
  } catch {
    try {
      return path.join(fs.realpathSync(path.dirname(absolute)), path.basename(absolute));
    } catch {
      return absolute;
    }
  }
}
