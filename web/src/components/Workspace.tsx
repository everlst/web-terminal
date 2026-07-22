import { useCallback, useEffect, useMemo, useState } from "react";
import { APIError, api } from "../api";
import { CloseIcon, ContainerIcon, LogoutIcon, PlusIcon, TerminalIcon, UserIcon } from "../icons";
import type { SessionState, SessionSummary, Target } from "../types";
import { Brand } from "./Brand";
import { NewSessionMenu } from "./NewSessionMenu";
import { TerminalView } from "./TerminalView";

interface WorkspaceProps { onLoggedOut: () => void; }

export function Workspace({ onLoggedOut }: WorkspaceProps) {
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [activeID, setActiveID] = useState("");
  const [targets, setTargets] = useState<Target[]>([]);
  const [menuOpen, setMenuOpen] = useState(false);
  const [menuLoading, setMenuLoading] = useState(false);
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const [message, setMessage] = useState("");
  const [booting, setBooting] = useState(true);

  const updateState = useCallback((id: string, state: SessionState) => {
    setSessions((current) => current.map((session) => session.id === id ? { ...session, state } : session));
  }, []);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        let current = await api.sessions();
        if (cancelled) return;
        if (current.length === 0) {
          const available = await api.targets();
          if (cancelled) return;
          setTargets(available);
          const host = available.find((target) => target.kind === "host");
          if (host) current = [await api.createSession(host)];
        }
        if (!cancelled) {
          setSessions(current);
          setActiveID(current[0]?.id ?? "");
        }
      } catch (reason) {
        if (!cancelled) setMessage(reason instanceof APIError ? reason.message : "无法连接控制代理");
      } finally {
        if (!cancelled) setBooting(false);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  const active = useMemo(() => sessions.find((session) => session.id === activeID), [sessions, activeID]);

  async function openNewSessionMenu() {
    setMenuOpen(true);
    setMenuLoading(true);
    setMessage("");
    try {
      setTargets(await api.targets());
    } catch (reason) {
      setMessage(reason instanceof APIError ? reason.message : "无法读取终端目标");
    } finally {
      setMenuLoading(false);
    }
  }

  async function createSession(target: Target) {
    setMenuOpen(false);
    setMessage("");
    try {
      const session = await api.createSession(target);
      setSessions((current) => [...current, session]);
      setActiveID(session.id);
    } catch (reason) {
      setMessage(reason instanceof APIError ? reason.message : "无法创建终端")
    }
  }

  async function closeSession(id: string) {
    const index = sessions.findIndex((session) => session.id === id);
    setSessions((current) => current.filter((session) => session.id !== id));
    if (activeID === id) {
      const next = sessions[index + 1] ?? sessions[index - 1];
      setActiveID(next?.id ?? "");
    }
    try {
      await api.deleteSession(id);
    } catch (reason) {
      setMessage(reason instanceof APIError ? reason.message : "关闭终端失败");
    }
  }

  async function logout() {
    try { await api.logout(); } finally { onLoggedOut(); }
  }

  const connected = active?.state === "connected";
  const statusText = active?.state === "recoverable" ? "可恢复" : active?.state === "closed" ? "已结束" : connected ? "已连接" : "连接中";

  return (
    <main className="workspace-shell">
      <header className="app-header">
        <Brand />
        <div className="header-actions">
          <div className={`connection-state state-${active?.state ?? "connecting"}`}><i />{statusText}</div>
          <span className="header-divider" />
          <button className="user-button icon-button" aria-label="用户菜单" aria-expanded={userMenuOpen} onClick={() => setUserMenuOpen((value) => !value)}><UserIcon size={25}/></button>
          {userMenuOpen && <button className="logout-menu" onClick={logout}><LogoutIcon size={19}/>退出登录</button>}
        </div>
      </header>
      <nav className="session-strip" aria-label="终端会话">
        <div className="session-tabs">
          {sessions.map((session) => (
            <div className={`session-tab${session.id === activeID ? " is-active" : ""}`} key={session.id}>
              <button className="tab-main" onClick={() => setActiveID(session.id)}>
                {session.target.kind === "host" ? <TerminalIcon size={22}/> : <ContainerIcon size={21}/>}
                <span>{session.title}</span>
              </button>
              <button className="tab-close" aria-label={`关闭 ${session.title}`} onClick={() => void closeSession(session.id)}><CloseIcon size={18}/></button>
            </div>
          ))}
          <button className="new-session-button" onClick={openNewSessionMenu}><PlusIcon size={21}/><span>新建终端</span></button>
        </div>
      </nav>
      <section className="terminal-stage">
        {sessions.map((session) => <TerminalView key={session.id} session={session} active={session.id === activeID} onState={updateState}/>) }
        {!booting && sessions.length === 0 && <div className="empty-terminal"><TerminalIcon size={34}/><span>选择“新建终端”开始会话</span></div>}
        {booting && <div className="empty-terminal"><span className="loading-dot"/>正在连接控制代理…</div>}
      </section>
      {message && <div className="toast" role="alert">{message}<button onClick={() => setMessage("")}><CloseIcon size={17}/></button></div>}
      {menuOpen && <NewSessionMenu targets={targets} loading={menuLoading} onSelect={(target) => void createSession(target)} onClose={() => setMenuOpen(false)}/>}
    </main>
  );
}
