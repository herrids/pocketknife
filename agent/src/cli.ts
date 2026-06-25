#!/usr/bin/env node
// Standalone entrypoint: npm run agent -- "<prompt>" [--paste <file>]
// Drives the orchestrator from the terminal. The planner reads the user's
// intent to proceed (any phrasing) and reports it via ready_to_build; this
// loop just polls that flag after each turn. The actual transition to build,
// and the final submit confirmation, still happen here deterministically --
// the model can signal intent but never triggers either itself.

import { createInterface } from "node:readline/promises";
import { readFile } from "node:fs/promises";

import { shutdownTracing } from "./tracing.js";
import { Orchestrator } from "./orchestrator.js";

function parseArgs(argv: string[]): { prompt: string; pasteFile?: string } {
  const rest: string[] = [];
  let pasteFile: string | undefined;
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--paste") {
      pasteFile = argv[++i];
    } else {
      rest.push(argv[i]);
    }
  }
  return { prompt: rest.join(" "), pasteFile };
}

async function main(): Promise<void> {
  const { prompt, pasteFile } = parseArgs(process.argv.slice(2));
  if (!prompt) {
    console.error('usage: npm run agent -- "<prompt>" [--paste <file>]');
    process.exitCode = 1;
    return;
  }

  const pastedCode = pasteFile ? await readFile(pasteFile, "utf8") : undefined;

  const orchestrator = new Orchestrator({
    onPlannerText: (text) => console.log(`\n${text}\n`),
    onBuilderText: (text) => console.log(`\n[builder] ${text}\n`),
  });

  console.log(`pocketknife-agent — job ${orchestrator.jobId}`);
  console.log("Describe refinements in plain language. Say when you're ready to build.\n");

  await orchestrator.startPlanning(prompt, pastedCode);

  const rl = createInterface({ input: process.stdin, output: process.stdout });
  try {
    while (!orchestrator.isReadyToBuild()) {
      const line = (await rl.question("> ")).trim();
      if (line.length === 0) continue;

      await orchestrator.refinePlan(line);
    }

    console.log("\nBuilding frontend...\n");
    await orchestrator.build();

    const confirm = (await rl.question("\nSubmit this app? [y/N] ")).trim().toLowerCase();
    if (confirm !== "y" && confirm !== "yes") {
      console.log("Not submitted.");
      return;
    }
  } finally {
    rl.close();
  }

  const result = await orchestrator.submit();
  console.log(`\nSubmitted. appId: ${result.appId}`);
}

main()
  .catch((err) => {
    console.error(err);
    process.exitCode = 1;
  })
  .finally(() => shutdownTracing());
