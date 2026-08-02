import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { terminalSocketURL } from "../api";
import { detectTerminalPlatform, terminalShortcutAction } from "../terminalShortcuts";
import type { SessionState, SessionSummary, TerminalControlMessage } from "../types";

interface TerminalViewProps {
  session: SessionSummary;
  active: boolean;
  onState: (id: string, state: SessionState) => void;
}

const MINIMUM_FONT_SIZE = 9;
const MAXIMUM_FONT_SIZE = 32;

function currentTheme() {
  const light = window.matchMedia("(prefers-color-scheme: light)").matches;
  return light ? {
    background: "#ffffff",
    foreground: "#1d2329",
    cursor: "#20262c",
    cursorAccent: "#ffffff",
    selectionBackground: "#b8ede0",
    black: "#1d2329",
    brightBlack: "#69737d",
    green: "#159b72",
    brightGreen: "#20b985",
    cyan: "#148b8b",
    brightCyan: "#1faeaa",
    white: "#e9edf0",
    brightWhite: "#ffffff"
  } : {
    background: "#0b1117",
    foreground: "#d7dde2",
    cursor: "#f2f5f7",
    cursorAccent: "#0b1117",
    selectionBackground: "#245c50",
    black: "#0b1117",
    brightBlack: "#6d7882",
    green: "#38c996",
    brightGreen: "#4bddaa",
    cyan: "#41bfc0",
    brightCyan: "#56d4d1",
    white: "#d7dde2",
    brightWhite: "#ffffff"
  };
}

export function TerminalView({ session, active, onState }: TerminalViewProps) {
  const hostRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const socketRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!hostRef.current) return;
    const defaultFontSize = window.innerWidth <= 680 ? 14 : 15;
    const platform = detectTerminalPlatform();
    const terminal = new Terminal({
      allowProposedApi: false,
      convertEol: false,
      cursorBlink: true,
      cursorStyle: "block",
      disableStdin: false,
      fontFamily: '"SFMono-Regular", "SF Mono", "Cascadia Mono", "Cascadia Code", "Liberation Mono", Menlo, Consolas, monospace',
      fontSize: defaultFontSize,
      fontWeight: "400",
      lineHeight: 1.45,
      letterSpacing: 0,
      scrollback: 10000,
      theme: currentTheme()
    });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(hostRef.current);
    let disposed = false;
    let fitFrame = 0;
    const scheduleFit = () => {
      if (disposed) return;
      window.cancelAnimationFrame(fitFrame);
      fitFrame = window.requestAnimationFrame(() => {
        if (!disposed) fit.fit();
      });
    };
    terminal.attachCustomKeyEventHandler((event) => {
      const action = terminalShortcutAction(event, platform, terminal.hasSelection());
      if (!action) return true;
      if (action === "browser-copy" || action === "browser-paste") return false;

      event.preventDefault();
      if (action === "clear") {
        terminal.clear();
        return false;
      }

      const currentFontSize = terminal.options.fontSize ?? defaultFontSize;
      terminal.options.fontSize = action === "font-reset"
        ? defaultFontSize
        : Math.min(MAXIMUM_FONT_SIZE, Math.max(MINIMUM_FONT_SIZE, currentFontSize + (action === "font-increase" ? 1 : -1)));
      scheduleFit();
      return false;
    });
    terminalRef.current = terminal;
    fitRef.current = fit;
    scheduleFit();

    const themeQuery = window.matchMedia("(prefers-color-scheme: light)");
    const updateTheme = () => { terminal.options.theme = currentTheme(); };
    themeQuery.addEventListener("change", updateTheme);
    const resizeObserver = new ResizeObserver(() => {
      if (hostRef.current?.offsetParent !== null) scheduleFit();
    });
    resizeObserver.observe(hostRef.current);
    void document.fonts?.ready.then(scheduleFit);
    return () => {
      disposed = true;
      window.cancelAnimationFrame(fitFrame);
      resizeObserver.disconnect();
      themeQuery.removeEventListener("change", updateTheme);
      terminal.dispose();
      terminalRef.current = null;
      fitRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!active) return;
    requestAnimationFrame(() => {
      fitRef.current?.fit();
      terminalRef.current?.focus();
    });
  }, [active]);

  useEffect(() => {
    let disposed = false;
    let retryTimer = 0;
    let heartbeat = 0;
    let firstDisconnect = 0;
    let terminalClosed = session.state === "closed";
    const encoder = new TextEncoder();
    const decoder = new TextDecoder();

    const sendSize = () => {
      const socket = socketRef.current;
      const terminal = terminalRef.current;
      if (socket?.readyState === WebSocket.OPEN && terminal) {
        socket.send(JSON.stringify({ type: "resize", cols: terminal.cols, rows: terminal.rows }));
      }
    };

    const inputDisposable = terminalRef.current?.onData((data) => {
      const socket = socketRef.current;
      if (socket?.readyState === WebSocket.OPEN) socket.send(encoder.encode(data));
    });
    const resizeDisposable = terminalRef.current?.onResize(sendSize);

    const connect = () => {
      if (disposed || terminalClosed) return;
      const socket = new WebSocket(terminalSocketURL(session.id));
      socket.binaryType = "arraybuffer";
      socketRef.current = socket;
      onState(session.id, "connecting");
      socket.onopen = () => {
        firstDisconnect = 0;
        onState(session.id, "connected");
        requestAnimationFrame(() => {
          fitRef.current?.fit();
          sendSize();
        });
        heartbeat = window.setInterval(() => {
          if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: "ping" }));
        }, 25000);
      };
      socket.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) {
          terminalRef.current?.write(decoder.decode(event.data, { stream: true }));
          return;
        }
        if (event.data instanceof Blob) {
          void event.data.arrayBuffer().then((data) => terminalRef.current?.write(decoder.decode(data, { stream: true })));
          return;
        }
        try {
          const message = JSON.parse(event.data as string) as TerminalControlMessage;
          if (message.type === "reset") terminalRef.current?.reset();
          if (message.type === "state" && message.state) onState(session.id, message.state);
          if (message.type === "exit") {
            terminalClosed = true;
            onState(session.id, "closed");
            terminalRef.current?.write(`\r\n\x1b[90m[终端进程已退出，代码 ${message.code ?? 0}]\x1b[0m\r\n`);
          }
          if (message.type === "error") {
            terminalClosed = true;
            onState(session.id, "closed");
            terminalRef.current?.write(`\r\n\x1b[31m[${message.message ?? "终端启动失败"}]\x1b[0m\r\n`);
          }
        } catch {
          // Ignore malformed control messages; terminal output only uses binary frames.
        }
      };
      socket.onclose = () => {
        window.clearInterval(heartbeat);
        if (disposed || terminalClosed) return;
        if (!firstDisconnect) firstDisconnect = Date.now();
        if (Date.now() - firstDisconnect >= 5 * 60 * 1000) {
          onState(session.id, "closed");
          return;
        }
        onState(session.id, "recoverable");
        retryTimer = window.setTimeout(connect, Math.min(10000, 1000 + (Date.now() - firstDisconnect) / 10));
      };
      socket.onerror = () => socket.close();
    };

    connect();
    return () => {
      disposed = true;
      inputDisposable?.dispose();
      resizeDisposable?.dispose();
      window.clearTimeout(retryTimer);
      window.clearInterval(heartbeat);
      socketRef.current?.close(1000, "view disposed");
      socketRef.current = null;
    };
  }, [session.id, onState]);

  return <div className={`terminal-pane${active ? " is-active" : ""}`} aria-hidden={!active}><div ref={hostRef} className="terminal-host" /></div>;
}
