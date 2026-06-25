import { Toaster } from "@/components/ui/sonner";

// Placeholder shell. The builder replaces this with the real app: an App shell
// (header + nav across entities), one page per read-enabled entity, and all
// data flowing through the generated client in ./client. Keep <Toaster />
// mounted once at the root so any view can raise success / ApiError toasts.
export default function App() {
  return (
    <div className="min-h-dvh bg-background text-foreground">
      <main className="mx-auto flex min-h-dvh max-w-2xl flex-col items-center justify-center gap-3 px-6 text-center">
        <h1 className="text-2xl font-semibold tracking-tight">Pocketknife app scaffold</h1>
        <p className="text-muted-foreground">
          Replace this shell with the app's real views, authored against{" "}
          <code className="rounded bg-muted px-1.5 py-0.5 text-sm">./client</code>.
        </p>
      </main>
      <Toaster richColors closeButton />
    </div>
  );
}
