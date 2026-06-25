// Semantic checks JSON Schema structural validation cannot express, mirroring
// validate/semantic.go's rules closely enough that a manifest passing here
// will also pass the real gate: stable-id uniqueness, sibling-name
// uniqueness, reserved-name avoidance, reference resolution, enum value
// integrity, and defaults that satisfy the field's own constraints.

import type { AppManifest, Entity, Field } from "./manifest-types.js";
import { RESERVED_NAMES, entityById, fieldHasDefault } from "./manifest-types.js";
import type { ValidationError } from "./validator.js";

export function semanticErrors(manifest: AppManifest): ValidationError[] {
  const errors: ValidationError[] = [];

  const entityIds = new Set<string>();
  const entityNames = new Set<string>();

  manifest.entities.forEach((entity, ei) => {
    const epath = `/entities/${ei}`;

    if (entityIds.has(entity.id)) {
      errors.push({ path: `${epath}/id`, message: `entity id "${entity.id}" is not unique` });
    }
    entityIds.add(entity.id);

    if (entityNames.has(entity.name)) {
      errors.push({ path: `${epath}/name`, message: `entity name "${entity.name}" is not unique` });
    }
    entityNames.add(entity.name);

    const fieldIds = new Set<string>();
    const fieldNames = new Set<string>();

    entity.fields.forEach((field, fi) => {
      const fpath = `${epath}/fields/${fi}`;

      if (fieldIds.has(field.id)) {
        errors.push({
          path: `${fpath}/id`,
          message: `field id "${field.id}" is not unique within entity "${entity.name}"`,
        });
      }
      fieldIds.add(field.id);

      if (RESERVED_NAMES.includes(field.id)) {
        errors.push({ path: `${fpath}/id`, message: `field id "${field.id}" is reserved by the platform` });
      }

      if (fieldNames.has(field.name)) {
        errors.push({
          path: `${fpath}/name`,
          message: `field name "${field.name}" is not unique within entity "${entity.name}"`,
        });
      }
      fieldNames.add(field.name);

      if (RESERVED_NAMES.includes(field.name)) {
        errors.push({ path: `${fpath}/name`, message: `field name "${field.name}" is reserved by the platform` });
      }

      errors.push(...validateField(fpath, manifest, entity, field));
    });
  });

  const fnIds = new Set<string>();
  const fnNames = new Set<string>();
  (manifest.functions ?? []).forEach((fn, fi) => {
    const fpath = `/functions/${fi}`;

    if (fnIds.has(fn.id)) {
      errors.push({ path: `${fpath}/id`, message: `function id "${fn.id}" is not unique` });
    }
    fnIds.add(fn.id);

    if (fnNames.has(fn.name)) {
      errors.push({ path: `${fpath}/name`, message: `function name "${fn.name}" is not unique` });
    }
    fnNames.add(fn.name);

    (fn.capabilities.data ?? []).forEach((scope, di) => {
      if (entityById(manifest, scope.entity) === undefined) {
        errors.push({
          path: `${fpath}/capabilities/data/${di}/entity`,
          message: `data scope entity "${scope.entity}" does not resolve to an entity in this manifest`,
        });
      }
    });
  });

  return errors;
}

function validateField(path: string, manifest: AppManifest, _entity: Entity, field: Field): ValidationError[] {
  const errors: ValidationError[] = [];

  if ("min" in field && "max" in field && field.min !== undefined && field.max !== undefined && field.min > field.max) {
    errors.push({ path, message: `min (${field.min}) is greater than max (${field.max})` });
  }

  if (field.type === "reference") {
    if (entityById(manifest, field.target) === undefined) {
      errors.push({
        path: `${path}/target`,
        message: `reference target "${field.target}" does not resolve to an entity in this manifest`,
      });
    }
  }

  if (field.type === "enum") {
    const seen = new Set<string>();
    field.values.forEach((v) => {
      if (seen.has(v)) {
        errors.push({ path: `${path}/values`, message: `enum value "${v}" is repeated` });
      }
      seen.add(v);
    });
  }

  if (fieldHasDefault(field)) {
    errors.push(...validateDefault(`${path}/default`, field));
  }

  return errors;
}

function validateDefault(path: string, field: Field): ValidationError[] {
  switch (field.type) {
    case "text": {
      const n = field.default!.length;
      const errors: ValidationError[] = [];
      if (field.min !== undefined && n < field.min) {
        errors.push({ path, message: `default length ${n} is below min ${field.min}` });
      }
      if (field.max !== undefined && n > field.max) {
        errors.push({ path, message: `default length ${n} exceeds max ${field.max}` });
      }
      return errors;
    }
    case "integer":
    case "real": {
      const n = field.default!;
      const errors: ValidationError[] = [];
      if (field.min !== undefined && n < field.min) {
        errors.push({ path, message: `default ${n} is below min ${field.min}` });
      }
      if (field.max !== undefined && n > field.max) {
        errors.push({ path, message: `default ${n} exceeds max ${field.max}` });
      }
      return errors;
    }
    case "enum": {
      if (!field.values.includes(field.default!)) {
        return [{ path, message: `enum default "${field.default}" is not one of the declared values` }];
      }
      return [];
    }
    default:
      return [];
  }
}
