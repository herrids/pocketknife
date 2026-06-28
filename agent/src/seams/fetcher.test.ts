import assert from "node:assert/strict";
import http from "node:http";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import * as tar from "tar";

import { HttpFetcher, StubFetcher } from "./fetcher.js";

const tmps: string[] = [];
after(async () => {
  for (const dir of tmps) await rm(dir, { recursive: true, force: true });
});

async function tmp(): Promise<string> {
  const dir = await mkdtemp(path.join(tmpdir(), "pk-fetch-"));
  tmps.push(dir);
  return dir;
}

async function withMockServer(
  handler: (req: http.IncomingMessage, res: http.ServerResponse) => void,
  fn: (baseUrl: string) => Promise<void>,
): Promise<void> {
  const server = http.createServer(handler);
  await new Promise<void>((resolve) => server.listen(0, resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("expected network address");
  const baseUrl = `http://127.0.0.1:${address.port}`;
  try {
    await fn(baseUrl);
  } finally {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
}

function makeTarGz(files: Record<string, string>): Promise<Buffer> {
  return new Promise(async (resolve, reject) => {
    const dir = await tmp();
    for (const [name, content] of Object.entries(files)) {
      const full = path.join(dir, name);
      await mkdir(path.dirname(full), { recursive: true });
      await writeFile(full, content);
    }
    const stream = tar.create({ cwd: dir, gzip: true }, ["."]);
    const chunks: Buffer[] = [];
    stream.on("data", (c: Buffer) => chunks.push(c));
    stream.on("end", () => resolve(Buffer.concat(chunks)));
    stream.on("error", reject);
  });
}

test("StubFetcher throws when called", async () => {
  const fetcher = new StubFetcher();
  await assert.rejects(() => fetcher.fetch("myapp"), /SUBMIT_MODE=http/);
});

test("HttpFetcher returns manifest and hasSource=false for sourceless app", async () => {
  const manifest = { app: { id: "myapp", name: "My App", version: 1 }, entities: [] };

  await withMockServer(
    (req, res) => {
      if (req.url === "/export/myapp") {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ manifest, hasSource: false }));
      } else {
        res.writeHead(404);
        res.end();
      }
    },
    async (baseUrl) => {
      const fetcher = new HttpFetcher(baseUrl);
      const result = await fetcher.fetch("myapp");
      assert.equal(result.hasSource, false);
      assert.deepEqual(result.manifest, manifest);
      assert.equal(result.sourceBuffer, undefined);
    },
  );
});

test("HttpFetcher fetches source bytes when hasSource=true", async () => {
  const manifest = { app: { id: "myapp", name: "My App", version: 1 }, entities: [] };
  const sourceTar = await makeTarGz({ "src/App.tsx": "export default function App() {}" });

  await withMockServer(
    (req, res) => {
      if (req.url === "/export/myapp") {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ manifest, hasSource: true }));
      } else if (req.url === "/export/myapp/source") {
        res.writeHead(200, { "Content-Type": "application/gzip" });
        res.end(sourceTar);
      } else {
        res.writeHead(404);
        res.end();
      }
    },
    async (baseUrl) => {
      const fetcher = new HttpFetcher(baseUrl);
      const result = await fetcher.fetch("myapp");
      assert.equal(result.hasSource, true);
      assert.ok(result.sourceBuffer instanceof Buffer);
      assert.ok(result.sourceBuffer.length > 0);
    },
  );
});

test("HttpFetcher throws a clear error for unknown app (404)", async () => {
  await withMockServer(
    (req, res) => {
      res.writeHead(404, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: { code: "app_not_found", message: "app \"gone\" not found" } }));
    },
    async (baseUrl) => {
      const fetcher = new HttpFetcher(baseUrl);
      await assert.rejects(() => fetcher.fetch("gone"), /not found/);
    },
  );
});

test("HttpFetcher encodes app id in the URL", async () => {
  let capturedUrl = "";
  await withMockServer(
    (req, res) => {
      capturedUrl = req.url ?? "";
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ manifest: {}, hasSource: false }));
    },
    async (baseUrl) => {
      const fetcher = new HttpFetcher(baseUrl);
      await fetcher.fetch("my app/id");
    },
  );
  assert.ok(capturedUrl.includes("my%20app%2Fid"), `expected encoded URL, got: ${capturedUrl}`);
});
