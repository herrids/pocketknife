import { TasksClient, Project, Task, TaskPriority } from "./client.js";

const client = new TasksClient();

const projectList = document.getElementById("projects") as HTMLUListElement;
const projectForm = document.getElementById("new-project-form") as HTMLFormElement;
const projectNameInput = document.getElementById("project-name") as HTMLInputElement;

const taskList = document.getElementById("tasks") as HTMLUListElement;
const taskForm = document.getElementById("new-task-form") as HTMLFormElement;
const taskTitleInput = document.getElementById("task-title") as HTMLInputElement;
const taskProjectSelect = document.getElementById("task-project") as HTMLSelectElement;
const taskPrioritySelect = document.getElementById("task-priority") as HTMLSelectElement;

const errorBox = document.getElementById("error") as HTMLDivElement;

let projectsById = new Map<string, Project>();

function renderProject(p: Project): HTMLLIElement {
  const li = document.createElement("li");
  li.textContent = p.name;
  return li;
}

function renderTask(t: Task): HTMLLIElement {
  const li = document.createElement("li");
  const projectName = t.project ? projectsById.get(t.project)?.name ?? "(unknown project)" : "(no project)";
  li.textContent = `[${t.priority ?? "medium"}] ${t.title} — ${projectName}`;
  return li;
}

function populateProjectSelect(projects: Project[]): void {
  const previous = taskProjectSelect.value;
  taskProjectSelect.replaceChildren();
  const none = document.createElement("option");
  none.value = "";
  none.textContent = "(no project)";
  taskProjectSelect.appendChild(none);
  for (const p of projects) {
    const opt = document.createElement("option");
    opt.value = p.id;
    opt.textContent = p.name;
    taskProjectSelect.appendChild(opt);
  }
  taskProjectSelect.value = previous;
}

async function refreshProjects(): Promise<Project[]> {
  const { data } = await client.project.list({ sort: ["name"] });
  projectsById = new Map(data.map((p) => [p.id, p]));
  projectList.replaceChildren(...data.map(renderProject));
  populateProjectSelect(data);
  return data;
}

async function refreshTasks(): Promise<void> {
  const { data } = await client.task.list({ sort: ["-created_at"] });
  taskList.replaceChildren(...data.map(renderTask));
}

projectForm.addEventListener("submit", async (ev) => {
  ev.preventDefault();
  errorBox.textContent = "";
  const name = projectNameInput.value.trim();
  if (!name) return;
  try {
    await client.project.create({ name });
    projectNameInput.value = "";
    await refreshProjects();
  } catch (err) {
    errorBox.textContent = err instanceof Error ? err.message : "failed to add project";
  }
});

taskForm.addEventListener("submit", async (ev) => {
  ev.preventDefault();
  errorBox.textContent = "";
  const title = taskTitleInput.value.trim();
  if (!title) return;
  const project = taskProjectSelect.value || undefined;
  const priority = taskPrioritySelect.value as TaskPriority;
  try {
    await client.task.create({ title, project, priority });
    taskTitleInput.value = "";
    await refreshTasks();
  } catch (err) {
    errorBox.textContent = err instanceof Error ? err.message : "failed to add task";
  }
});

(async () => {
  try {
    await refreshProjects();
    await refreshTasks();
  } catch (err) {
    errorBox.textContent = err instanceof Error ? err.message : "failed to load data";
  }
})();
