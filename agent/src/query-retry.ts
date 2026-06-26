// Wraps a query() turn with retry-on-transient-disconnect. The Claude Agent
// SDK shells out to a `claude` subprocess over stdio, and that pipe can drop
// mid-turn -- surfacing as a thrown "Connection closed mid-response" error
// from the stream, not a clean result message -- for reasons that have
// nothing to do with the conversation itself. On a retryable failure, this
// resumes the same session (captured from the stream's "system"/"init"
// message) with backoff, so a dropped connection costs a delay-and-retry
// instead of the whole turn.
//
// planner.ts and builder.ts both drive query() through this instead of a
// bare `for await`, so the transient-vs-real distinction and the resume
// bookkeeping live in one place.

import type { Options, SDKMessage } from "@anthropic-ai/claude-agent-sdk";

import { query as defaultQuery } from "./tracing.js";

const RETRYABLE_PATTERNS = [
  /connection closed/i,
  /econnreset/i,
  /epipe/i,
  /socket hang up/i,
  /fetch failed/i,
  /etimedout/i,
  /timed out/i,
];

export function isRetryableQueryError(err: unknown): boolean {
  const message = err instanceof Error ? err.message : String(err);
  return RETRYABLE_PATTERNS.some((pattern) => pattern.test(message));
}

export interface RetryConfig {
  /** Total attempts including the first. Default 3 (i.e. up to 2 retries). */
  maxAttempts?: number;
  /** Delay before the first retry; doubled on each subsequent attempt. Default 1000ms. */
  baseDelayMs?: number;
  /** Called once per retry, before the backoff delay. */
  onRetry?: (info: { attempt: number; error: unknown }) => void;
  /** Injection point for tests. */
  queryFn?: typeof defaultQuery;
  /** Injection point for tests. */
  sleepFn?: (ms: number) => Promise<void>;
}

export interface QueryRetryResult {
  /** The session id last seen via a "system"/"init" message, if any -- callers that resume across turns (the planner) should persist this. */
  sessionId: string | undefined;
}

/**
 * Drives one query() turn to completion, retrying with session resume when
 * the underlying transport drops mid-response. `onMessage` is called for
 * every message of every attempt. A retry resumes the session and resends
 * the same `prompt` -- for a fixed instruction prompt (the builder) that's an
 * idempotent nudge to continue; for a literal user message (the planner) it
 * can appear twice in the resumed transcript if the connection dropped after
 * the model had already started responding. Accepted as the simple, robust
 * default: a possible duplicated turn beats losing the whole job to a
 * transient pipe drop.
 */
export async function runQueryWithRetry(
  prompt: string,
  options: Options,
  onMessage: (message: SDKMessage) => void,
  config: RetryConfig = {},
): Promise<QueryRetryResult> {
  const maxAttempts = config.maxAttempts ?? 3;
  const baseDelayMs = config.baseDelayMs ?? 1000;
  const queryFn = config.queryFn ?? defaultQuery;
  const sleepFn = config.sleepFn ?? ((ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms)));

  let sessionId = options.resume;

  for (let attempt = 1; ; attempt++) {
    try {
      const attemptOptions: Options = sessionId ? { ...options, resume: sessionId } : options;
      for await (const message of queryFn({ prompt, options: attemptOptions })) {
        if (message.type === "system" && message.subtype === "init") {
          sessionId = message.session_id;
        }
        onMessage(message);
      }
      return { sessionId };
    } catch (err) {
      if (attempt >= maxAttempts || !isRetryableQueryError(err)) throw err;
      config.onRetry?.({ attempt, error: err });
      await sleepFn(baseDelayMs * 2 ** (attempt - 1));
    }
  }
}
