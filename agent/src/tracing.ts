// Wires the Claude Agent SDK's query() calls into Langfuse so every model
// turn and tool call either session takes (planner or builder) shows up as a
// trace. planner.ts and builder.ts import `query` from here instead of
// directly from the SDK, so they always get whichever one (instrumented or
// plain passthrough) this module decided to hand out.
//
// No-op when LANGFUSE_PUBLIC_KEY/LANGFUSE_SECRET_KEY aren't set, so local
// development never requires a Langfuse account.

import { NodeTracerProvider } from "@opentelemetry/sdk-trace-node";
import { LangfuseSpanProcessor } from "@langfuse/otel";
import { propagateAttributes } from "@langfuse/tracing";
import { ClaudeAgentSDKInstrumentation } from "@arizeai/openinference-instrumentation-claude-agent-sdk";
import * as ClaudeAgentSDKModule from "@anthropic-ai/claude-agent-sdk";

const enabled = Boolean(process.env.LANGFUSE_PUBLIC_KEY && process.env.LANGFUSE_SECRET_KEY);

let provider: NodeTracerProvider | undefined;

export const query: typeof ClaudeAgentSDKModule.query = enabled ? patchQuery() : ClaudeAgentSDKModule.query;

function patchQuery(): typeof ClaudeAgentSDKModule.query {
  provider = new NodeTracerProvider({
    spanProcessors: [new LangfuseSpanProcessor()],
  });
  provider.register();

  const instrumentation = new ClaudeAgentSDKInstrumentation({ tracerProvider: provider });

  // @anthropic-ai/claude-agent-sdk ships as plain ESM, whose namespace-object
  // properties can't be reassigned in place. instrumentation.patch() knows this
  // and falls back to returning a patched shallow copy instead of mutating --
  // but manuallyInstrument(), the documented entry point, calls patch() and
  // discards that return value, leaving the live `query` binding untouched
  // (verified against installed v0.2.7: it throws "Cannot assign to read only
  // property" if you let it try in-place mutation on the real namespace).
  // Work around both bugs: hand patch() a real mutable copy ourselves, and use
  // its return value directly instead of going through manuallyInstrument().
  // `patch` is only `private` at the type level -- it's an ordinary method at
  // runtime -- so this is a deliberate, narrow bypass of that annotation.
  // @ts-expect-error -- patch() is private
  const patched: typeof ClaudeAgentSDKModule = instrumentation.patch({ ...ClaudeAgentSDKModule });
  return patched.query;
}

/**
 * Tags every span created while `fn` runs with `jobId` as the Langfuse session
 * id and `name` as the trace name, so the planner's turns and the builder's
 * run for one job group together in the Langfuse UI. A no-op pass-through
 * when tracing isn't configured.
 */
export function withJobTrace<T>(jobId: string, name: string, fn: () => Promise<T>): Promise<T> {
  if (!enabled) return fn();
  return propagateAttributes({ sessionId: jobId, traceName: name }, fn);
}

/** Flushes and closes the Langfuse exporter so buffered spans aren't lost on exit. No-op if tracing was never enabled. */
export async function shutdownTracing(): Promise<void> {
  await provider?.forceFlush();
  await provider?.shutdown();
}
