import { describe, it, mock, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { extractChecklist, type BridgeEvent } from "./bridge.js";

// Test emitEvent output format.
describe("emitEvent format", () => {
  let written: string[] = [];

  beforeEach(() => {
    written = [];
    // Monkey-patch process.stdout.write.
    const orig = process.stdout.write.bind(process.stdout);
    mock.method(process.stdout, "write", (chunk: string) => {
      written.push(chunk);
      return true;
    });
  });

  afterEach(() => mock.restoreAll());

  it("emits valid JSON followed by newline", async () => {
    const { emitEvent } = await import("./bridge.js");
    const ev: BridgeEvent = { type: "turn", role: "assistant", text: "Hello!" };
    emitEvent(ev);
    assert.equal(written.length, 1);
    const line = written[0];
    assert.ok(line.endsWith("\n"), "must end with newline");
    const parsed = JSON.parse(line.trim());
    assert.equal(parsed.type, "turn");
    assert.equal(parsed.role, "assistant");
    assert.equal(parsed.text, "Hello!");
  });
});

describe("extractChecklist", () => {
  it("extracts entities from manifest", () => {
    const manifest = { entities: [{ name: "Book" }, { name: "ReadingSession" }] };
    const items = extractChecklist("", manifest);
    assert.equal(items.length, 2);
    assert.ok(items[0].text.includes("Book"));
    assert.equal(items[0].done, false);
  });

  it("returns fallback when no entities", () => {
    const items = extractChecklist("", {});
    assert.equal(items.length, 1);
    assert.equal(items[0].done, false);
  });
});
