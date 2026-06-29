import { Moon, Sun } from "lucide-react";

import { Button } from "@/components/ui/button";
import { APP_EMOJI, APP_NAME, APP_TAGLINE } from "@/lib/app-info";
import { useTheme } from "@/lib/use-theme";

/** App shell header: identity + the light/dark toggle. Sticky so it stays reachable while scrolling a long list. */
export function Header() {
  const { theme, toggle } = useTheme();

  return (
    <header className="sticky top-0 z-40 border-b bg-background/80 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="mx-auto flex w-full max-w-3xl items-center justify-between gap-3 px-4 py-3 sm:px-6">
        <div className="flex items-center gap-2.5">
          <span className="text-2xl leading-none" role="img" aria-label={`${APP_NAME} logo`}>
            {APP_EMOJI}
          </span>
          <div className="leading-tight">
            <h1 className="text-base font-semibold tracking-tight">{APP_NAME}</h1>
            <p className="text-xs text-muted-foreground">{APP_TAGLINE}</p>
          </div>
        </div>
        <Button
          variant="outline"
          size="icon"
          onClick={toggle}
          aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
        >
          {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </Button>
      </div>
    </header>
  );
}
