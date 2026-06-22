import { GratitudeLogClient, Entry } from "./client.js";

const client = new GratitudeLogClient();

const list = document.getElementById("entries") as HTMLUListElement;
const form = document.getElementById("new-entry-form") as HTMLFormElement;
const textInput = document.getElementById("text") as HTMLInputElement;
const errorBox = document.getElementById("error") as HTMLDivElement;

function renderEntry(e: Entry): HTMLLIElement {
  const li = document.createElement("li");
  const time = document.createElement("time");
  time.textContent = new Date(e.created_at).toLocaleString();
  const body = document.createElement("p");
  body.textContent = e.text;
  li.appendChild(time);
  li.appendChild(body);
  return li;
}

async function refresh(): Promise<void> {
  const { data } = await client.entry.list({ sort: ["-created_at"] });
  list.replaceChildren(...data.map(renderEntry));
}

form.addEventListener("submit", async (ev) => {
  ev.preventDefault();
  errorBox.textContent = "";
  const text = textInput.value.trim();
  if (!text) return;
  try {
    await client.entry.create({ text });
    textInput.value = "";
    await refresh();
  } catch (err) {
    errorBox.textContent = err instanceof Error ? err.message : "failed to add entry";
  }
});

refresh().catch((err) => {
  errorBox.textContent = err instanceof Error ? err.message : "failed to load entries";
});
