// Bridge mode adapter: reads newline-delimited JSON messages from stdin and
// drives the orchestrator loop. Emits newline-delimited JSON events to stdout.
// This module is only active when the --bridge-mode flag is passed to cli.ts.

import { createInterface } from "readline";

export interface BridgeMessage {
  type: "message" | "approve";
  text?: string;
}

export type BridgeEventType = "turn" | "plan" | "ready" | "error" | "done";

export interface BridgeEvent {
  type: BridgeEventType;
  role?: string;
  text?: string;
  checklist?: Array<{ text: string; done: boolean }>;
  manifestVersion?: number;
  reason?: string;
  appId?: string;
}

// emitEvent writes one JSON event line to stdout.
export function emitEvent(ev: BridgeEvent): void {
  process.stdout.write(JSON.stringify(ev) + "\n");
}

type MessageCallback = (text: string) => void;

// BridgeInput reads newline-delimited JSON messages from stdin.
export class BridgeInput {
  private waiters: Array<MessageCallback> = [];
  private approveResolve: (() => void) | undefined;
  private approvePromise: Promise<void>;
  private rl: ReturnType<typeof createInterface>;

  constructor() {
    this.approvePromise = new Promise<void>((resolve) => {
      this.approveResolve = resolve;
    });

    this.rl = createInterface({ input: process.stdin, terminal: false });
    this.rl.on("line", (line: string) => {
      const trimmed = line.trim();
      if (!trimmed) return;
      try {
        const msg = JSON.parse(trimmed) as BridgeMessage;
        if (msg.type === "message" && msg.text) {
          const cb = this.waiters.shift();
          if (cb) cb(msg.text);
        } else if (msg.type === "approve") {
          if (this.approveResolve) this.approveResolve();
        }
      } catch {
        // Ignore malformed lines.
      }
    });
  }

  // nextMessage waits for the next user message from stdin.
  nextMessage(): Promise<string> {
    return new Promise<string>((resolve) => {
      this.waiters.push(resolve);
    });
  }

  // waitForApprove waits for the {"type":"approve"} message.
  waitForApprove(): Promise<void> {
    return this.approvePromise;
  }

  close(): void {
    this.rl.close();
  }
}

// extractChecklist tries to pull a feature list from the manifest's entities.
export function extractChecklist(
  _client: string,
  manifest: unknown,
): Array<{ text: string; done: boolean }> {
  const items: Array<{ text: string; done: boolean }> = [];
  try {
    const m = manifest as { entities?: Array<{ name: string }> };
    if (m.entities) {
      for (const e of m.entities) {
        items.push({ text: `Store and manage ${e.name}`, done: false });
      }
    }
  } catch {
    // Ignore.
  }
  if (items.length === 0) {
    items.push({ text: "Build app features", done: false });
  }
  return items;
}
