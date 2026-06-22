import { ReadingTrackerClient } from "./client.js";
const client = new ReadingTrackerClient();
const list = document.getElementById("books");
const form = document.getElementById("new-book-form");
const titleInput = document.getElementById("title");
const authorInput = document.getElementById("author");
const ratingInput = document.getElementById("rating");
const errorBox = document.getElementById("error");
function renderBook(b) {
    const li = document.createElement("li");
    const done = document.createElement("input");
    done.type = "checkbox";
    done.checked = b.done === true;
    done.addEventListener("change", async () => {
        try {
            await client.book.update(b.id, { done: done.checked });
            await refresh();
        }
        catch (err) {
            errorBox.textContent = err instanceof Error ? err.message : "failed to update book";
        }
    });
    const label = document.createElement("span");
    const ratingText = b.rating != null ? ` (${b.rating}/5)` : "";
    const authorText = b.author ? ` — ${b.author}` : "";
    label.textContent = `${b.title}${authorText}${ratingText}`;
    const del = document.createElement("button");
    del.textContent = "Delete";
    del.addEventListener("click", async () => {
        try {
            await client.book.delete(b.id);
            await refresh();
        }
        catch (err) {
            errorBox.textContent = err instanceof Error ? err.message : "failed to delete book";
        }
    });
    li.append(done, label, del);
    return li;
}
async function refresh() {
    const { data } = await client.book.list({ sort: ["-created_at"] });
    list.replaceChildren(...data.map(renderBook));
}
form.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    errorBox.textContent = "";
    const title = titleInput.value.trim();
    if (!title)
        return;
    const author = authorInput.value.trim();
    const rating = ratingInput.value ? Number(ratingInput.value) : undefined;
    try {
        await client.book.create({ title, author: author || undefined, rating });
        titleInput.value = "";
        authorInput.value = "";
        ratingInput.value = "";
        await refresh();
    }
    catch (err) {
        errorBox.textContent = err instanceof Error ? err.message : "failed to add book";
    }
});
refresh().catch((err) => {
    errorBox.textContent = err instanceof Error ? err.message : "failed to load books";
});
