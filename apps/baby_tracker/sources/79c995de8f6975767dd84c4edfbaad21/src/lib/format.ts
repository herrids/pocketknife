export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Intl.DateTimeFormat("de-DE", {
    year: "numeric",
    month: "long",
    day: "numeric",
  }).format(new Date(iso));
}

export function isoToDateInput(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function dateInputToISO(value: string): string {
  if (!value) return "";
  return `${value}T00:00:00.000Z`;
}

export function calcAge(birthdateISO: string): string {
  const birth = new Date(birthdateISO);
  const now = new Date();
  const diffDays = Math.floor((now.getTime() - birth.getTime()) / 86400000);
  const weeks = Math.floor(diffDays / 7);
  const months = Math.floor(diffDays / 30.44);
  const years = Math.floor(diffDays / 365.25);
  if (years >= 2) return `${years} Jahre alt`;
  if (years === 1) return "1 Jahr alt";
  if (months >= 1) return `${months} Monat${months === 1 ? "" : "e"} alt`;
  return `${weeks} Woche${weeks === 1 ? "" : "n"} alt`;
}
