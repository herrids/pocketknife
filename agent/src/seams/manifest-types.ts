// Mirrors the closed shape described by schema/manifest.schema.json. Kept
// independent of the Go schema package — this is the agent's own read-only
// view of a candidate manifest, used only to drive the stub validator's
// semantic checks and client generation.

export type StableId = string;
export type Operation = "create" | "read" | "update" | "delete";

export interface AppManifest {
  app: AppInfo;
  entities: Entity[];
  frontend?: FrontendInfo;
  functions?: FunctionDecl[];
}

export interface AppInfo {
  id: StableId;
  name: string;
  emoji?: string;
  version: number;
}

export interface FrontendInfo {
  dist: string;
  entry?: string;
}

export interface Entity {
  id: StableId;
  name: string;
  operations?: Operation[];
  fields: Field[];
}

export type FieldType =
  | "text"
  | "integer"
  | "real"
  | "boolean"
  | "datetime"
  | "enum"
  | "reference";

interface FieldBase {
  id: StableId;
  name: string;
  type: FieldType;
  required?: boolean;
}

export interface TextField extends FieldBase {
  type: "text";
  unique?: boolean;
  default?: string;
  min?: number;
  max?: number;
}

export interface IntegerField extends FieldBase {
  type: "integer";
  unique?: boolean;
  default?: number;
  min?: number;
  max?: number;
}

export interface RealField extends FieldBase {
  type: "real";
  unique?: boolean;
  default?: number;
  min?: number;
  max?: number;
}

export interface BooleanField extends FieldBase {
  type: "boolean";
  default?: boolean;
}

export interface DatetimeField extends FieldBase {
  type: "datetime";
  default?: string;
}

export interface EnumField extends FieldBase {
  type: "enum";
  default?: string;
  values: string[];
}

export interface ReferenceField extends FieldBase {
  type: "reference";
  target: StableId;
  onDelete?: "set_null" | "restrict" | "cascade";
}

export type Field =
  | TextField
  | IntegerField
  | RealField
  | BooleanField
  | DatetimeField
  | EnumField
  | ReferenceField;

export interface WasmFunctionDecl {
  id: StableId;
  name: string;
  entry: string;
  description?: string;
  capabilities: Capabilities;
}

export interface PromptFunctionDecl {
  id: StableId;
  name: string;
  prompt: string;
  description?: string;
  capabilities: { model: true };
}

export type FunctionDecl = WasmFunctionDecl | PromptFunctionDecl;

export function isPromptFunction(fn: FunctionDecl): fn is PromptFunctionDecl {
  return "prompt" in fn && typeof fn.prompt === "string" && fn.prompt !== "";
}

export interface Capabilities {
  data?: DataScope[];
  network?: string[];
  model?: boolean;
}

export interface DataScope {
  entity: StableId;
  operations: Operation[];
}

export const RESERVED_NAMES = ["id", "created_at", "updated_at"];

// Entity names claimed by the platform's own routes: "functions" is the
// function-invocation sub-path under /apps/{app}/. Mirrors
// schema.ReservedEntityNames in the Go backend.
export const RESERVED_ENTITY_NAMES = ["functions"];

// Mirrors schema/prompt.go: one well-formed {{param}} placeholder. Param
// names follow the machineName rule; there is no escaping mechanism.
const PLACEHOLDER_PATTERN = /\{\{\s*([a-z][a-z0-9_]*)\s*\}\}/g;

// scanPrompt returns the template's well-formed placeholder names
// (first-appearance order, deduplicated) and the byte offset of every "{{"
// that does not begin a well-formed placeholder.
export function scanPrompt(prompt: string): { params: string[]; malformed: number[] } {
  const params: string[] = [];
  const starts = new Set<number>();
  for (const m of prompt.matchAll(PLACEHOLDER_PATTERN)) {
    starts.add(m.index);
    if (!params.includes(m[1])) params.push(m[1]);
  }
  const malformed: number[] = [];
  for (let i = 0; i + 1 < prompt.length; i++) {
    if (prompt[i] !== "{" || prompt[i + 1] !== "{") continue;
    if (starts.has(i)) continue;
    malformed.push(i);
    i++; // consume both braces so "{{{" reports once
  }
  return { params, malformed };
}

// promptParams returns the placeholder names of a prompt function's template.
export function promptParams(fn: FunctionDecl): string[] {
  if (!isPromptFunction(fn)) return [];
  return scanPrompt(fn.prompt).params;
}

const ALL_OPERATIONS: Operation[] = ["create", "read", "update", "delete"];

export function entityAllows(entity: Entity, op: Operation): boolean {
  return (entity.operations ?? ALL_OPERATIONS).includes(op);
}

export function entityById(manifest: AppManifest, id: StableId): Entity | undefined {
  return manifest.entities.find((e) => e.id === id);
}

export function fieldHasDefault(field: Field): boolean {
  return "default" in field && field.default !== undefined;
}
