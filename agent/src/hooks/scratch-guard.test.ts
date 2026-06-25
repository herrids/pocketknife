import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, symlink, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import { createScratchGuard } from "./scratch-guard.js";
import type { PreToolUseHookInput } from "@anthropic-ai/claude-agent-sdk";

function preToolUseInput(toolName: string, filePath: string): PreToolUseHookInput {
  return {
    hook_event_name: "PreToolUse",
    session_id: "test",
    transcript_path: "/dev/null",
    cwd: "/",
    tool_name: toolName,
    tool_input: { file_path: filePath },
    tool_use_id: "tool_1",
  };
}

const signal = new AbortController().signal;

test("scratch guard allows a write inside the scratch dir", async () => {
  const scratch = await mkdtemp(path.join(tmpdir(), "guard-ok-"));
  const guard = createScratchGuard(scratch);
  const result = await guard.hooks[0](preToolUseInput("Write", path.join(scratch, "index.html")), "tool_1", {
    signal,
  });
  assert.equal((result as { continue?: boolean }).continue, true);
  await rm(scratch, { recursive: true, force: true });
});

test("scratch guard denies a write outside the scratch dir", async () => {
  const scratch = await mkdtemp(path.join(tmpdir(), "guard-deny-"));
  const guard = createScratchGuard(scratch);
  const result = await guard.hooks[0](preToolUseInput("Write", "/etc/pwned.txt"), "tool_1", { signal });
  const out = result as { hookSpecificOutput?: { permissionDecision?: string } };
  assert.equal(out.hookSpecificOutput?.permissionDecision, "deny");
  await rm(scratch, { recursive: true, force: true });
});

test("scratch guard denies a relative .. escape", async () => {
  const scratch = await mkdtemp(path.join(tmpdir(), "guard-deny-rel-"));
  const guard = createScratchGuard(scratch);
  const result = await guard.hooks[0](
    preToolUseInput("Edit", path.join(scratch, "..", "escaped.txt")),
    "tool_1",
    { signal },
  );
  const out = result as { hookSpecificOutput?: { permissionDecision?: string } };
  assert.equal(out.hookSpecificOutput?.permissionDecision, "deny");
  await rm(scratch, { recursive: true, force: true });
});

test("scratch guard denies a symlink that points outside scratch", async () => {
  const scratch = await mkdtemp(path.join(tmpdir(), "guard-deny-symlink-"));
  const outside = await mkdtemp(path.join(tmpdir(), "guard-outside-"));
  const guard = createScratchGuard(scratch);

  const linkPath = path.join(scratch, "escape-link");
  await symlink(outside, linkPath, "dir");

  const result = await guard.hooks[0](
    preToolUseInput("Write", path.join(linkPath, "file.txt")),
    "tool_1",
    { signal },
  );
  const out = result as { hookSpecificOutput?: { permissionDecision?: string } };
  assert.equal(out.hookSpecificOutput?.permissionDecision, "deny");

  await rm(scratch, { recursive: true, force: true });
  await rm(outside, { recursive: true, force: true });
});

test("scratch guard ignores non-Write/Edit shaped input without a file_path", async () => {
  const scratch = await mkdtemp(path.join(tmpdir(), "guard-other-"));
  const guard = createScratchGuard(scratch);
  const input: PreToolUseHookInput = {
    hook_event_name: "PreToolUse",
    session_id: "test",
    transcript_path: "/dev/null",
    cwd: "/",
    tool_name: "Glob",
    tool_input: { pattern: "**/*" },
    tool_use_id: "tool_1",
  };
  const result = await guard.hooks[0](input, "tool_1", { signal });
  assert.equal((result as { continue?: boolean }).continue, true);
  await rm(scratch, { recursive: true, force: true });
});
