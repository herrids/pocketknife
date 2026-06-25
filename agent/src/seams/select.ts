// The ONLY place in the agent that reads VALIDATE_MODE, SUBMIT_MODE or
// GO_BASE_URL. Every other module depends on the Validator/Submitter
// interfaces and has no idea which implementation — or which transport —
// backs them.

import { StubValidator, HttpValidator, type Validator } from "./validator.js";
import { StubSubmitter, HttpSubmitter, type Submitter } from "./submitter.js";

export function selectValidator(): Validator {
  const mode = process.env.VALIDATE_MODE ?? "stub";
  switch (mode) {
    case "stub":
      return new StubValidator();
    case "http":
      return new HttpValidator(goBaseUrl());
    default:
      throw new Error(`unknown VALIDATE_MODE "${mode}" (expected "stub" or "http")`);
  }
}

export function selectSubmitter(outDir: string): Submitter {
  const mode = process.env.SUBMIT_MODE ?? "stub";
  switch (mode) {
    case "stub":
      return new StubSubmitter(outDir);
    case "http":
      return new HttpSubmitter(goBaseUrl());
    default:
      throw new Error(`unknown SUBMIT_MODE "${mode}" (expected "stub" or "http")`);
  }
}

function goBaseUrl(): string {
  return process.env.GO_BASE_URL ?? "http://localhost:8080";
}
