import type { APIErrorBody, AuthSession, SessionSummary, Target } from "./types";

export class APIError extends Error {
  readonly status: number;
  readonly code?: string;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.code = code;
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    credentials: "same-origin",
    ...options,
    headers: {
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...options.headers
    }
  });
  if (!response.ok) {
    let body: APIErrorBody | undefined;
    try {
      body = (await response.json()) as APIErrorBody;
    } catch {
      body = undefined;
    }
    throw new APIError(body?.error?.message ?? "请求失败，请稍后重试", response.status, body?.error?.code);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

export const api = {
  authSession: () => request<AuthSession>("/api/auth/session"),
  login: (password: string) => request<void>("/api/auth/login", {
    method: "POST",
    body: JSON.stringify({ password })
  }),
  logout: () => request<void>("/api/auth/logout", { method: "POST", body: "{}" }),
  targets: async () => (await request<{ targets: Target[] }>("/api/targets")).targets,
  sessions: async () => (await request<{ sessions: SessionSummary[] }>("/api/sessions")).sessions,
  createSession: (target: Target) => request<SessionSummary>("/api/sessions", {
    method: "POST",
    body: JSON.stringify({ target })
  }),
  deleteSession: (id: string) => request<void>(`/api/sessions/${encodeURIComponent(id)}`, { method: "DELETE", body: "{}" })
};

export function terminalSocketURL(sessionID: string): string {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/api/sessions/${encodeURIComponent(sessionID)}/stream`;
}
