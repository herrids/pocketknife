import assert from "node:assert/strict";
import { test } from "node:test";

import { isRetryableQueryError, runQueryWithRetry } from "./query-retry.js";
import type { Options, SDKMessage } from "@anthropic-ai/claude-agent-sdk";

function systemInit(sessionId: string): SDKMessage {
  return {
    type: "system",
    subtype: "init",
    session_id: sessionId,
  } as unknown as SDKMessage;
}

function assistantText(text: string): SDKMessage {
  return {
    type: "assistant",
    message: { content: [{ type: "text", text }] },
  } as unknown as SDKMessage;
}

async function* asyncGen(messages: SDKMessage[], failWith?: Error): AsyncGenerator<SDKMessage, void> {
  for (const message of messages) yield message;
  if (failWith) throw failWith;
}

test("isRetryableQueryError matches known transient transport failures", () => {
  assert.ok(isRetryableQueryError(new Error("Claude Code returned an error result: API Error: Connection closed mid-response.")));
  assert.ok(isRetryableQueryError(new Error("read ECONNRESET")));
  assert.ok(isRetryableQueryError(new Error("Claude Code returned an error result: Request timed out")));
  assert.ok(!isRetryableQueryError(new Error("manifest is invalid: entities is required")));
});

test("passes every message through on a clean run with no retry needed", async () => {
  const seen: string[] = [];
  const result = await runQueryWithRetry(
    "hello",
    {},
    (message) => { seen.push(message.type); },
    {
      queryFn: (() => asyncGen([systemInit("s1"), assistantText("hi")])) as any,
    },
  );
  assert.deepEqual(seen, ["system", "assistant"]);
  assert.equal(result.sessionId, "s1");
});

test("retries a connection drop and resumes the captured session", async () => {
  let calls = 0;
  const resumeSeen: (string | undefined)[] = [];
  const sleeps: number[] = [];

  const result = await runQueryWithRetry(
    "hello",
    {},
    () => {},
    {
      queryFn: ((params: { prompt: string; options: Options }) => {
        calls++;
        resumeSeen.push(params.options.resume);
        if (calls === 1) {
          return asyncGen([systemInit("s1")], new Error("API Error: Connection closed mid-response."));
        }
        return asyncGen([systemInit("s1"), assistantText("done")]);
      }) as any,
      sleepFn: async (ms: number) => {
        sleeps.push(ms);
      },
    },
  );

  assert.equal(calls, 2);
  assert.deepEqual(resumeSeen, [undefined, "s1"]);
  assert.deepEqual(sleeps, [1000]);
  assert.equal(result.sessionId, "s1");
});

test("does not retry a non-transient error", async () => {
  let calls = 0;
  await assert.rejects(
    runQueryWithRetry(
      "hello",
      {},
      () => {},
      {
        queryFn: (() => {
          calls++;
          return asyncGen([], new Error("manifest is invalid"));
        }) as any,
        sleepFn: async () => {},
      },
    ),
    /manifest is invalid/,
  );
  assert.equal(calls, 1);
});

test("gives up after maxAttempts and surfaces the last error", async () => {
  let calls = 0;
  await assert.rejects(
    runQueryWithRetry(
      "hello",
      {},
      () => {},
      {
        maxAttempts: 2,
        queryFn: (() => {
          calls++;
          return asyncGen([], new Error("Connection closed mid-response."));
        }) as any,
        sleepFn: async () => {},
      },
    ),
    /Connection closed/,
  );
  assert.equal(calls, 2);
});
