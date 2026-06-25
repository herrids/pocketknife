import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { after, test } from "node:test";

import { StubSubmitter } from "./submitter.js";

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
