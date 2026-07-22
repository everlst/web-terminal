export type TargetKind = "host" | "container";

export interface Target {
  kind: TargetKind;
  id?: string;
  name: string;
  image?: string;
  status?: string;
}

export type SessionState = "connecting" | "connected" | "recoverable" | "closed";

export interface SessionSummary {
  id: string;
  title: string;
  target: Target;
  state: SessionState;
  createdAt: string;
  expiresAt: string;
  reconnectUntil?: string;
}

export interface AuthSession {
  authenticated: boolean;
  expiresAt?: string;
}

export interface APIErrorBody {
  error?: {
    code?: string;
    message?: string;
  };
}

export interface TerminalControlMessage {
  type: "reset" | "state" | "exit" | "error" | "pong";
  state?: SessionState;
  code?: number;
  message?: string;
}
