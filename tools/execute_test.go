package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"pocketknife/domain"
	"pocketknife/registry"
	"pocketknife/tools"
)

func bootApp(t *testing.T, appID, manifest string) *registry.Registry {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, appID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, results, err := registry.Load(root)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	for _, r := range results {
		if !r.OK {
			t.Fatalf("app %s failed to load: errors=%v err=%v", r.ManifestPath, r.Errors, r.Err)
		}
	}
	t.Cleanup(func() { reg.Close() })
	return reg
}

func rawParams(m map[string]any) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		b, _ := json.Marshal(v)
		out[k] = b
	}
	return out
}

const tasksManifest = `{
  "app": { "id": "tasks", "name": "Tasks", "version": 1 },
  "entities": [
    { "id": "ent_project", "name": "project", "fields": [
      { "id": "fld_pname", "name": "name", "type": "text", "required": true, "unique": true }
    ]},
    { "id": "ent_task", "name": "task", "fields": [
      { "id": "fld_title",   "name": "title",   "type": "text", "required": true },
      { "id": "fld_status",  "name": "status",  "type": "enum", "values": ["planned", "done"], "default": "planned" },
      { "id": "fld_project", "name": "project", "type": "reference", "target": "ent_project" }
    ]}
  ],
  "tools": [
    {
      "id": "tool_mark_done",
      "name": "mark_task_done",
      "description": "Mark a task as done",
      "params": [
        { "id": "p_task_id", "name": "task_id", "type": "reference", "target": "ent_task", "required": true }
      ],
      "steps": [
        { "id": "updated", "op": "update", "entity": "ent_task", "rowId": "$params.task_id", "set": { "status": "done" } }
      ]
    },
    {
      "id": "tool_new_project_task",
      "name": "new_project_task",
      "description": "Create a project, then a task inside it",
      "params": [
        { "id": "p_project_name", "name": "project_name", "type": "text", "required": true },
        { "id": "p_title", "name": "title", "type": "text", "required": true }
      ],
      "steps": [
        { "id": "project", "op": "create", "entity": "ent_project", "set": { "name": "$params.project_name" } },
        { "id": "task", "op": "create", "entity": "ent_task", "set": { "title": "$params.title", "project": "$steps.project.id" } }
      ]
    },
    {
      "id": "tool_fail_second_step",
      "name": "fail_second_step",
      "description": "First step succeeds, second step always fails validation - proves atomicity",
      "params": [
        { "id": "p_name2", "name": "project_name", "type": "text", "required": true }
      ],
      "steps": [
        { "id": "project", "op": "create", "entity": "ent_project", "set": { "name": "$params.project_name" } },
        { "id": "task", "op": "create", "entity": "ent_task", "set": { "project": "$steps.project.id" } }
      ]
    }
  ]
}`

func TestExecuteSingleStepUpdate(t *testing.T) {
	reg := bootApp(t, "tasks", tasksManifest)

	task, operr := domain.Create(reg, "tasks", "task", map[string]json.RawMessage{"title": json.RawMessage(`"Mow"`)})
	if operr != nil {
		t.Fatalf("seed task: %+v", operr)
	}
	taskID := task["id"].(string)

	res, operr := tools.Execute(context.Background(), reg, "tasks", "mark_task_done", rawParams(map[string]any{"task_id": taskID}))
	if operr != nil {
		t.Fatalf("execute: %+v", operr)
	}
	if res.Result["status"] != "done" {
		t.Fatalf("result status = %v, want done", res.Result["status"])
	}

	got, operr := domain.Get(reg, "tasks", "task", taskID)
	if operr != nil {
		t.Fatalf("get: %+v", operr)
	}
	if got["status"] != "done" {
		t.Fatalf("persisted status = %v, want done", got["status"])
	}
}

func TestExecuteResolvesUnknownTool(t *testing.T) {
	reg := bootApp(t, "tasks", tasksManifest)
	_, operr := tools.Execute(context.Background(), reg, "tasks", "no_such_tool", nil)
	if operr == nil || operr.Kind != domain.ErrToolNotFound {
		t.Fatalf("expected ErrToolNotFound, got %+v", operr)
	}
}

func TestExecuteMultiStepChaining(t *testing.T) {
	reg := bootApp(t, "tasks", tasksManifest)

	res, operr := tools.Execute(context.Background(), reg, "tasks", "new_project_task", rawParams(map[string]any{
		"project_name": "Home",
		"title":        "Mow the lawn",
	}))
	if operr != nil {
		t.Fatalf("execute: %+v", operr)
	}
	task := res.Result
	if task["title"] != "Mow the lawn" {
		t.Fatalf("task title = %v", task["title"])
	}
	projectID, _ := res.Steps["project"]["id"].(string)
	if projectID == "" || task["project"] != projectID {
		t.Fatalf("task.project = %v, want project id %v", task["project"], projectID)
	}

	// The project row created by step 1 must actually be committed.
	got, operr := domain.Get(reg, "tasks", "project", projectID)
	if operr != nil {
		t.Fatalf("get project: %+v", operr)
	}
	if got["name"] != "Home" {
		t.Fatalf("project name = %v", got["name"])
	}
}

func TestExecuteRollsBackOnLaterStepFailure(t *testing.T) {
	reg := bootApp(t, "tasks", tasksManifest)

	_, operr := tools.Execute(context.Background(), reg, "tasks", "fail_second_step", rawParams(map[string]any{
		"project_name": "Orphan",
	}))
	if operr == nil {
		t.Fatalf("expected the second step (missing required title) to fail")
	}

	res, opErr2 := domain.List(reg, "tasks", "project", map[string][]string{"filter": {"name:eq:Orphan"}})
	if opErr2 != nil {
		t.Fatalf("list projects: %+v", opErr2)
	}
	if res.Total != 0 {
		t.Fatalf("project from the failed tool call was not rolled back: found %d matching rows", res.Total)
	}
}
