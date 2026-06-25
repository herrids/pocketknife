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

export interface FunctionDecl {
  id: StableId;
  name: string;
  entry: string;
  capabilities: Capabilities;
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
