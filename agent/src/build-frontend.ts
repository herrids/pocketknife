// The build stage. Plain orchestrator code — never a tool the model holds —
// that turns the authored scaffold into a static dist/ the Go backend can serve
// verbatim. It installs dependencies and runs `vite build`; on a build or type
// error it runs a bounded self-heal loop: feed the build output back to a
// sandboxed fix session (the same Read/Write/Edit/Glob/Skill config + scratch
// guard as the builder), then rebuild. After the attempt budget is spent it
// throws, so a job never gets submitted with a frontend that doesn't compile.

import { spawn } from "node:child_process";

import { sandboxedBuilderOptions, streamQuery } from "./builder.js";

const MAX_FIX_ATTEMPTS = 2;

export interface BuildCallbacks {
  onText?: (text: string) => void;
}

export async function buildFrontend(scratchDir: string, callbacks: BuildCallbacks = {}): Promise<void> {
  const notify = callbacks.onText ?? (() => {});

  notify("Installing dependencies…");
  const install = await run("npm", ["install", "--no-audit", "--no-fund"], scratchDir);
  if (install.code !== 0) {
    throw new Error(`npm install failed:\n${install.output}`);
  }

  notify("Building frontend (vite)…");
  let result = await run("npm", ["run", "build"], scratchDir);

  for (let attempt = 1; result.code !== 0 && attempt <= MAX_FIX_ATTEMPTS; attempt++) {
    notify(`Build failed — attempting fix ${attempt}/${MAX_FIX_ATTEMPTS}…`);
    await runFixSession(scratchDir, result.output, callbacks);
    result = await run("npm", ["run", "build"], scratchDir);
  }

  if (result.code !== 0) {
    throw new Error(`frontend build still failing after ${MAX_FIX_ATTEMPTS} fix attempts:\n${result.output}`);
  }

  notify("Frontend build succeeded.");
}

/** A one-shot sandboxed session asked to repair the source so the build passes. */
async function runFixSession(scratchDir: string, buildOutput: string, callbacks: BuildCallbacks): Promise<void> {
  const prompt = `The frontend build (\`npm run build\`: tsc --noEmit then vite build) failed with the
output below. Fix the source under src/ so the build passes. Stay faithful to the
"pocketknife-frontend" skill: route all data through the generated client in
src/client.ts, keep using the scaffold's components and design tokens, and do not edit
build config (vite.config.ts, tsconfig.json, tailwind.config.ts) unless the error is
unambiguously there. Do not weaken types with \`any\` to silence errors — fix the real
cause.

Build output:
${truncate(buildOutput, 12_000)}`;

  await streamQuery(prompt, sandboxedBuilderOptions(scratchDir), callbacks.onText);
}

interface RunResult {
  code: number;
  output: string;
}

/** Runs a command in cwd, capturing merged stdout+stderr. Never rejects. */
function run(command: string, args: string[], cwd: string): Promise<RunResult> {
  return new Promise((resolve) => {
    const child = spawn(command, args, { cwd, env: process.env });
    let output = "";
    child.stdout.on("data", (chunk) => {
      output += chunk.toString();
    });
    child.stderr.on("data", (chunk) => {
      output += chunk.toString();
    });
    child.on("error", (err) => resolve({ code: 1, output: `${output}\n${command} failed to start: ${err.message}` }));
    child.on("close", (code) => resolve({ code: code ?? 1, output }));
  });
}

/** Keep the tail of long build output — the actual errors are usually at the end. */
function truncate(text: string, max: number): string {
  if (text.length <= max) return text;
  return `…(truncated)…\n${text.slice(text.length - max)}`;
}
