import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import http from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";
import { after, test } from "node:test";

import { HttpSubmitter, StubSubmitter } from "./submitter.js";

const tmps: string[] = [];

async function tmp(): Promise<string> {
  const dir = await mkdtemp(path.join(tmpdir(), "pk-submit-"));
  tmps.push(dir);
  return dir;
}

after(async () => {
  for (const dir of tmps) await rm(dir, { recursive: true, force: true });
});

test("ships the built dist/, not the source tree", async () => {
  const scratchDir = await tmp();
  // Authored source that must NOT be shipped.
  await mkdir(path.join(scratchDir, "src"), { recursive: true });
  await writeFile(path.join(scratchDir, "src", "client.ts"), "export {};");
  await mkdir(path.join(scratchDir, "node_modules", "react"), { recursive: true });
  await writeFile(path.join(scratchDir, "package.json"), "{}");
  // The built bundle that SHOULD be shipped.
  await mkdir(path.join(scratchDir, "dist", "assets"), { recursive: true });
  await writeFile(path.join(scratchDir, "dist", "index.html"), "<!doctype html>");
  await writeFile(path.join(scratchDir, "dist", "assets", "index-abc.js"), "console.log(1)");

  const outDir = await tmp();
  const submitter = new StubSubmitter(outDir);
  const { appId } = await submitter.submit({ jobId: "job1", manifest: { app: { id: "x" } }, scratchDir });

  assert.equal(appId, "stub-job1");
  const frontendDir = path.join(outDir, "job1", "frontend");
  assert.ok(existsSync(path.join(frontendDir, "index.html")), "ships dist/index.html");
  assert.ok(existsSync(path.join(frontendDir, "assets", "index-abc.js")), "ships hashed asset");
  // Source artifacts stay behind.
  assert.ok(!existsSync(path.join(frontendDir, "src")), "does not ship src/");
  assert.ok(!existsSync(path.join(frontendDir, "node_modules")), "does not ship node_modules/");
  assert.ok(!existsSync(path.join(frontendDir, "package.json")), "does not ship package.json");

  const bundle = JSON.parse(await readFile(path.join(outDir, "job1", "bundle.json"), "utf8"));
  assert.deepEqual(bundle.files.sort(), [
    "frontend/assets/index-abc.js",
    "frontend/index.html",
    "manifest.json",
  ]);
});

test("refuses to submit when no build exists", async () => {
  const scratchDir = await tmp();
  await writeFile(path.join(scratchDir, "package.json"), "{}"); // source only, no dist/

  const outDir = await tmp();
  const submitter = new StubSubmitter(outDir);
  await assert.rejects(
    submitter.submit({ jobId: "job2", manifest: {}, scratchDir }),
    /no built frontend/,
  );
});

async function withMockServer(
  handler: (req: http.IncomingMessage, res: http.ServerResponse) => void,
  fn: (baseUrl: string) => Promise<void>,
): Promise<void> {
  const server = http.createServer(handler);
  await new Promise<void>((resolve) => server.listen(0, resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("expected a network address");
  const baseUrl = `http://127.0.0.1:${address.port}`;
  try {
    await fn(baseUrl);
  } finally {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
}

async function scratchDirWithBuiltDist(): Promise<string> {
  const scratchDir = await tmp();
  await mkdir(path.join(scratchDir, "dist", "assets"), { recursive: true });
  await writeFile(path.join(scratchDir, "dist", "index.html"), "<!doctype html>");
  await writeFile(path.join(scratchDir, "dist", "assets", "index-abc.js"), "console.log(1)");
  return scratchDir;
}

test("HttpSubmitter posts the manifest and bundle, returns the backend's appId", async () => {
  const scratchDir = await scratchDirWithBuiltDist();
  let sawJobId = false;

  await withMockServer(
    (req, res) => {
      const chunks: Buffer[] = [];
      req.on("data", (c: Buffer) => chunks.push(c));
      req.on("end", () => {
        sawJobId = Buffer.concat(chunks).toString("utf8").includes("job-xyz");
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ appId: "myapp", version: 1, jobId: "job-xyz", url: "/ui/myapp/" }));
      });
    },
    async (baseUrl) => {
      const submitter = new HttpSubmitter(baseUrl);
      const { appId } = await submitter.submit({ jobId: "job-xyz", manifest: { app: { id: "myapp" } }, scratchDir });
      assert.equal(appId, "myapp");
    },
  );
  assert.ok(sawJobId, "the jobId field should be present in the posted multipart body");
});

test("HttpSubmitter throws on a backend error envelope", async () => {
  const scratchDir = await scratchDirWithBuiltDist();

  await withMockServer(
    (req, res) => {
      req.resume();
      req.on("end", () => {
        res.writeHead(422, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ error: { code: "manifest_invalid", message: "entities is required" } }));
      });
    },
    async (baseUrl) => {
      const submitter = new HttpSubmitter(baseUrl);
      await assert.rejects(
        submitter.submit({ jobId: "job-1", manifest: {}, scratchDir }),
        /entities is required/,
      );
    },
  );
});

test("HttpSubmitter refuses to submit when no build exists", async () => {
  const scratchDir = await tmp();
  await writeFile(path.join(scratchDir, "package.json"), "{}"); // source only, no dist/

  const submitter = new HttpSubmitter("http://127.0.0.1:1");
  await assert.rejects(
    submitter.submit({ jobId: "job-3", manifest: {}, scratchDir }),
    /no built frontend/,
  );
});
