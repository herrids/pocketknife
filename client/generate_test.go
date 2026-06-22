package client_test

import (
	"strings"
	"testing"

	"pocketknife/client"
	"pocketknife/validate"
)

const tasksManifest = `{
  "app": { "id": "tasks", "name": "Tasks", "version": 1 },
  "entities": [
    { "id": "ent_project", "name": "project", "fields": [
      { "id": "fld_name", "name": "name", "type": "text", "required": true, "unique": true }
    ]},
    { "id": "ent_task", "name": "task", "fields": [
      { "id": "fld_title",   "name": "title",    "type": "text",      "required": true },
      { "id": "fld_proj",    "name": "project",  "type": "reference", "target": "ent_project", "onDelete": "set_null" },
      { "id": "fld_prio",    "name": "priority", "type": "enum",      "values": ["low", "medium", "high"], "default": "medium" }
    ]}
  ]
}`

const appendOnlyManifest = `{
  "app": { "id": "gratitude_log", "name": "Gratitude Log", "version": 1 },
  "entities": [
    { "id": "ent_entry", "name": "entry", "operations": ["create", "read"], "fields": [
      { "id": "fld_text", "name": "text", "type": "text", "required": true }
    ]}
  ]
}`

func TestGenerateIsDeterministic(t *testing.T) {
	app, errs := validate.Manifest([]byte(tasksManifest))
	if len(errs) > 0 {
		t.Fatalf("manifest failed validation: %v", errs)
	}
	a := client.Generate(app)
	b := client.Generate(app)
	if string(a) != string(b) {
		t.Fatal("Generate is not deterministic for an unchanged schema")
	}
}

func TestGenerateEntityShapes(t *testing.T) {
	app, errs := validate.Manifest([]byte(tasksManifest))
	if len(errs) > 0 {
		t.Fatalf("manifest failed validation: %v", errs)
	}
	out := string(client.Generate(app))

	wantSubstrings := []string{
		`export type TaskPriority = "low" | "medium" | "high";`,
		"export interface Task {",
		"id: string;",
		"created_at: ISODateTime;",
		"title: string;",
		"project: string | null;",
		"priority: TaskPriority | null;",
		"export interface TaskCreateInput {",
		"title: string;",
		"project?: string | null;",
		`export type TaskFilterableField = "id" | "created_at" | "updated_at" | "title" | "project" | "priority";`,
		"export class TaskClient {",
		`"POST", "/apps/tasks/task", input`,
		"export class ProjectClient {",
		"export class TasksClient {",
		"readonly task: TaskClient;",
		"readonly project: ProjectClient;",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("generated client missing expected fragment: %s", want)
		}
	}
}

func TestGenerateOmitsDisabledOperations(t *testing.T) {
	app, errs := validate.Manifest([]byte(appendOnlyManifest))
	if len(errs) > 0 {
		t.Fatalf("manifest failed validation: %v", errs)
	}
	out := string(client.Generate(app))

	if !strings.Contains(out, "create(input: EntryCreateInput)") {
		t.Error("expected create() for an entity that allows create")
	}
	if !strings.Contains(out, "get(id: string)") {
		t.Error("expected get() for an entity that allows read")
	}
	if strings.Contains(out, "update(id: string, input: EntryUpdateInput)") {
		t.Error("update is disabled for this entity but a method was generated")
	}
	if strings.Contains(out, "delete(id: string)") {
		t.Error("delete is disabled for this entity but a method was generated")
	}
	if strings.Contains(out, "EntryUpdateInput") {
		t.Error("EntryUpdateInput should not be generated when update is disabled")
	}
}

func TestGenerateRequiredFieldWithDefaultIsOptionalInCreateInput(t *testing.T) {
	const m = `{
      "app": { "id": "a", "name": "A", "version": 1 },
      "entities": [{ "id": "ent_x", "name": "x", "fields": [
        { "id": "fld_n", "name": "n", "type": "integer", "required": true, "default": 3 }
      ]}]
    }`
	app, errs := validate.Manifest([]byte(m))
	if len(errs) > 0 {
		t.Fatalf("manifest failed validation: %v", errs)
	}
	out := string(client.Generate(app))
	if !strings.Contains(out, "n?: number;") {
		t.Errorf("a required field with a default must be optional in CreateInput, got:\n%s", out)
	}
	if !strings.Contains(out, "export interface X {\n  id: string;\n  created_at: ISODateTime;\n  updated_at: ISODateTime;\n  n: number;\n}") {
		t.Errorf("a required field's row type must stay non-null, got:\n%s", out)
	}
}
