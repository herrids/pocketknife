export interface RegistryEntry {
  appId: string;
  emoji: string;
  color: string;
  displayName: string;
  gridOrder: number;
  buildState: "none" | "queued" | "building" | "activating" | "ready" | "failed";
  manifestVersion: number | null;
  activeBuildId: string | null;
}

export const api = {
  async login(email: string, password: string): Promise<void> {
    const res = await fetch("/platform/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    if (!res.ok) throw new Error("invalid email or password");
  },

  async logout(): Promise<void> {
    await fetch("/platform/auth/logout", { method: "POST" });
  },

  async registry(): Promise<RegistryEntry[]> {
    const res = await fetch("/platform/registry");
    if (!res.ok) throw new Error("failed to load registry");
    return res.json();
  },

  async patchApp(
    appId: string,
    patch: Partial<{ emoji: string; color: string; displayName: string; gridOrder: number }>,
  ): Promise<RegistryEntry> {
    const res = await fetch(`/platform/registry/${appId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(patch),
    });
    if (!res.ok) throw new Error("patch failed");
    return res.json();
  },

  async reorderApps(order: string[]): Promise<void> {
    const res = await fetch("/platform/registry/reorder", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ order }),
    });
    if (!res.ok) throw new Error("reorder failed");
  },

  async startPlan(prompt: string, appId?: string): Promise<{ sessionId: string }> {
    const res = await fetch("/platform/plan", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ prompt, appId }),
    });
    if (!res.ok) throw new Error("failed to start plan");
    return res.json();
  },

  async sendMessage(sessionId: string, text: string): Promise<void> {
    const res = await fetch(`/platform/plan/${sessionId}/message`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text }),
    });
    if (!res.ok) throw new Error("failed to send message");
  },

  async approvePlan(sessionId: string): Promise<{ appId: string }> {
    const res = await fetch(`/platform/plan/${sessionId}/approve`, {
      method: "POST",
    });
    if (!res.ok) throw new Error("failed to approve plan");
    return res.json();
  },

  eventsUrl(sessionId: string): string {
    return `/platform/plan/${sessionId}/events`;
  },
};
