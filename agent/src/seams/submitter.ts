// The handoff. The agent never writes to any live location, database, or
// registry itself — submit() is the one irreversible action, and it lives in
// orchestrator code, never behind a model-callable tool.
//
// Stub (today): tar/copy the bundle into ./out/<jobId>/.
// HTTP (today): multipart POST to the Go control plane's POST /deploy,
// idempotency-keyed on jobId -- see deployapi.NewServer() in the Go backend.
//
// Both submitters now also pack the editable frontend source (everything in
// the scratch dir except node_modules and dist) as a "source" part so the
// backend can store it for future updates.

import { mkdir, rm, writeFile, readdir, cp, stat } from "node:fs/promises";
import path from "node:path";
import { gzipSync } from "node:zlib";
import * as tar from "tar";

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

  async submit(input: SubmitInput): Promise<SubmitResult> {
    // Ship the built bundle, not the source -- same contract as StubSubmitter.
    const distDir = path.join(input.scratchDir, "dist");
    const distStat = await stat(distDir).catch(() => undefined);
    if (!distStat?.isDirectory()) {
      throw new Error(`no built frontend at ${distDir} -- the build step must run before submit`);
    }

    const bundle = await packBundle(distDir);
    const source = await packSource(input.scratchDir);

    const form = new FormData();
    form.set("jobId", input.jobId);
    form.set("manifest", new Blob([JSON.stringify(input.manifest)], { type: "application/json" }), "manifest.json");
    form.set("bundle", new Blob([bundle], { type: "application/gzip" }), "bundle.tar.gz");
    form.set("source", new Blob([source], { type: "application/gzip" }), "source.tar.gz");

    const url = `${this.baseUrl}/deploy`;
    let res: Response;
    try {
      res = await fetch(url, { method: "POST", body: form });
    } catch (err) {
      throw new Error(`deploy request to ${url} failed: ${err instanceof Error ? err.message : String(err)}`);
    }

    const body = await res.json().catch(() => undefined);
    if (!res.ok) {
      const message = (body as { error?: { message?: string } } | undefined)?.error?.message;
      throw new Error(`deploy to ${url} failed (${res.status}): ${message ?? "no error detail returned"}`);
    }

    const appId = (body as { appId?: unknown } | undefined)?.appId;
    if (typeof appId !== "string" || appId.length === 0) {
      throw new Error(`deploy to ${url} succeeded but returned no appId: ${JSON.stringify(body)}`);
    }
    return { appId };
  }
}

// packBundle tars and gzips distDir's contents in place -- entries are paths
// relative to distDir itself (e.g. "index.html", "assets/app.js"), matching
// what the backend's bundle extractor expects to land directly under an
// app's served dist/ directory.
async function packBundle(distDir: string): Promise<Buffer> {
  const stream = tar.create({ cwd: distDir, gzip: true }, ["."]);
  const chunks: Buffer[] = [];
  for await (const chunk of stream) {
    chunks.push(chunk as Buffer);
  }
  return Buffer.concat(chunks);
}

// packSource tars and gzips the editable frontend scaffold in scratchDir,
// excluding node_modules and dist (which are build artifacts, not source).
// The source archive is what the backend stores for future updates.
async function packSource(scratchDir: string): Promise<Buffer> {
  const entries = await readdir(scratchDir);
  const include = entries.filter((e) => e !== "node_modules" && e !== "dist");
  if (include.length === 0) {
    // Empty source — valid tar = two 512-byte EOF blocks gzip'd.
    return gzipSync(Buffer.alloc(1024, 0));
  }
  const stream = tar.create({ cwd: scratchDir, gzip: true }, include);
  const chunks: Buffer[] = [];
  for await (const chunk of stream) {
    chunks.push(chunk as Buffer);
  }
  return Buffer.concat(chunks);
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
