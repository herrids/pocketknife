import { type ReactNode, useEffect, useState } from "react";
import { Navigate } from "react-router-dom";

// Checks auth by making a request to a guarded endpoint.
// If we get 401 we're not logged in.
async function checkAuth(): Promise<boolean> {
  try {
    const res = await fetch("/platform/registry");
    return res.status !== 401;
  } catch {
    return false;
  }
}

export function PrivateRoute({ children }: { children: ReactNode }) {
  const [state, setState] = useState<"loading" | "ok" | "unauth">("loading");

  useEffect(() => {
    checkAuth().then((ok) => setState(ok ? "ok" : "unauth"));
  }, []);

  if (state === "loading") return null;
  if (state === "unauth") return <Navigate to="/login" replace />;
  return <>{children}</>;
}
