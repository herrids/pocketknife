// The handoff. The agent never writes to any live location, database, or
// registry itself — submit() is the one irreversible action, and it lives in
// orchestrator code, never behind a model-callable tool.
//
// Stub (today): tar/copy the bundle into ./out/<jobId>/.
// HTTP (later): multipart POST to the Go control plane, idempotency-keyed on
// jobId.

import { mkdir, rm, writeFile, readdir, cp, stat } from "node:fs/promises";
import path from "node:path";

export interface SubmitInput {
  jobId: string;
  manifest: unknown;
  scratchDir: string; // contains the authored project; its dist/ is the built bundle
}

export interface SubmitResult {
  appId: string;
}

export interface Submitter {
  submit(input: SubmitInput): Promise<SubmitResult>;
}

export class StubSubmitter implements Submitter {
  constructor(private readonly outDir: string) {}

  async submit(input: SubmitInput): Promise<SubmitResult> {
    const jobDir = path.resolve(this.outDir, input.jobId);

    // Ship the built bundle, not the source: dist/ is the static HTML/JS/CSS
    // the Go backend serves verbatim (entry index.html + hashed assets). Source,
    // node_modules, and build config never leave the scratch directory.
    const distDir = path.join(input.scratchDir, "dist");
    const distStat = await stat(distDir).catch(() => undefined);
    if (!distStat?.isDirectory()) {
      throw new Error(`no built frontend at ${distDir} -- the build step must run before submit`);
    }

    // Idempotent on jobId: wipe any prior run's files before rewriting, so a
    // retry overwrites in place instead of leaving stale frontend files
    // behind from a previous, differently-shaped build.
    await rm(jobDir, { recursive: true, force: true });
    await mkdir(jobDir, { recursive: true });

    await writeFile(path.join(jobDir, "manifest.json"), JSON.stringify(input.manifest, null, 2));

    const frontendDir = path.join(jobDir, "frontend");
    await cp(distDir, frontendDir, { recursive: true });

    const bundleFiles = await listFilesRecursive(frontendDir, frontendDir);
    await writeFile(
      path.join(jobDir, "bundle.json"),
      JSON.stringify({ jobId: input.jobId, files: ["manifest.json", ...bundleFiles.map((f) => `frontend/${f}`)] }, null, 2),
    );

    return { appId: `stub-${input.jobId}` };
  }
}

export class HttpSubmitter implements Submitter {
  constructor(private readonly baseUrl: string) {}

  async submit(_input: SubmitInput): Promise<SubmitResult> {
    throw new Error("HttpSubmitter not implemented yet");
  }
}

async function listFilesRecursive(dir: string, root: string): Promise<string[]> {
  const entries = await readdir(dir, { withFileTypes: true });
  const files: string[] = [];
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await listFilesRecursive(full, root)));
    } else {
      files.push(path.relative(root, full));
    }
  }
  return files;
}
