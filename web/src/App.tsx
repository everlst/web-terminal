import { useEffect, useState } from "react";
import { api } from "./api";
import { LoginView } from "./components/LoginView";
import { Workspace } from "./components/Workspace";

type AppState = "loading" | "guest" | "authenticated";

export default function App() {
  const [state, setState] = useState<AppState>("loading");
  useEffect(() => {
    let cancelled = false;
    void api.authSession().then((session) => {
      if (!cancelled) setState(session.authenticated ? "authenticated" : "guest");
    }).catch(() => { if (!cancelled) setState("guest"); });
    return () => { cancelled = true; };
  }, []);

  if (state === "loading") return <div className="app-loading" aria-label="正在加载"><span /></div>;
  if (state === "guest") return <LoginView onAuthenticated={() => setState("authenticated")} />;
  return <Workspace onLoggedOut={() => setState("guest")} />;
}
