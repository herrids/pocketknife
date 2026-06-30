import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../lib/api";
import { ThemeToggle } from "../components/ThemeToggle";

export function Login() {
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  // Redirect if already authed.
  useEffect(() => {
    fetch("/platform/registry")
      .then((r) => { if (r.status !== 401) navigate("/home", { replace: true }); })
      .catch(() => {});
  }, [navigate]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      await api.login(email, password);
      navigate("/home", { replace: true });
    } catch {
      setError("Incorrect email or password");
      setPassword("");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-dvh flex flex-col bg-surface dark:bg-ink">
      {/* Hero */}
      <div className="flex-1 flex flex-col items-center justify-center px-6 pt-16 pb-10 bg-ink dark:bg-[#0E0C09] relative overflow-hidden">
        <div className="absolute top-6 right-6 z-10">
          <ThemeToggle />
        </div>
        {/* Memphis decorations */}
        <div className="absolute top-8 right-8 w-24 h-24 rounded-full bg-teal opacity-20" />
        <div className="absolute bottom-12 left-6 w-16 h-16 bg-app-pink opacity-15 rotate-45" />
        <div className="absolute top-20 left-12 w-10 h-10 rounded-full bg-amber opacity-25" />

        <h1 className="font-mono font-bold tracking-[0.25em] text-surface dark:text-[#F3ECDD] text-2xl uppercase mb-3 relative z-10">
          POCKETKNIFE
        </h1>
        <p className="text-ink-muted text-sm text-center relative z-10">
          Your little apps, in one pocket.
        </p>
      </div>

      {/* Login form */}
      <div className="px-6 py-10 bg-surface dark:bg-[#1B1813]">
        <form onSubmit={handleSubmit} className="flex flex-col gap-4 max-w-sm mx-auto">
          <input
            type="email"
            placeholder="you@example.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="email"
            required
            className="w-full px-4 py-3 rounded-lg bg-card dark:bg-[#28231C] border border-ink/10 dark:border-white/10 text-ink dark:text-[#F3ECDD] placeholder:text-ink-faint text-sm outline-none focus:ring-2 focus:ring-terracotta/40"
          />
          <input
            type="password"
            placeholder="••••••••"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
            className="w-full px-4 py-3 rounded-lg bg-card dark:bg-[#28231C] border border-ink/10 dark:border-white/10 text-ink dark:text-[#F3ECDD] placeholder:text-ink-faint text-sm outline-none focus:ring-2 focus:ring-terracotta/40"
          />
          {error && <p className="text-red-500 text-xs text-center">{error}</p>}
          <button
            type="submit"
            disabled={loading || !email || !password}
            className="w-full py-3 rounded-lg bg-ink dark:bg-[#F3ECDD] text-surface dark:text-ink font-semibold text-sm press-spring-sm disabled:opacity-40"
          >
            {loading ? "Signing in…" : "Sign in →"}
          </button>
        </form>
      </div>
    </div>
  );
}
