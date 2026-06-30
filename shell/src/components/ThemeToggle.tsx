import { useTheme } from "../lib/useTheme";

export function ThemeToggle() {
  const [theme, toggle] = useTheme();

  return (
    <button
      onClick={toggle}
      aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
      className="w-9 h-9 rounded-full flex items-center justify-center text-sm press-spring-sm bg-card dark:bg-[#28231C] border border-ink/10 dark:border-white/10 text-ink dark:text-[#F3ECDD]"
    >
      {theme === "dark" ? "☀" : "☾"}
    </button>
  );
}
