package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/everlst/web-terminal/internal/config"
	"github.com/everlst/web-terminal/internal/model"
)

type Agent struct {
	cfg     config.Agent
	account hostAccount
	auth    *authenticator
	logger  *slog.Logger
	server  *http.Server
}

func NewAgent(cfg config.Agent, logger *slog.Logger) (*Agent, error) {
	if _, err := exec.LookPath(cfg.DockerBin); err != nil {
		return nil, fmt.Errorf("找不到 Docker CLI: %w", err)
	}
	if _, err := exec.LookPath(cfg.NsenterBin); err != nil {
		return nil, fmt.Errorf("找不到 nsenter: %w", err)
	}
	account, err := validateHostAccount(cfg)
	if err != nil {
		return nil, err
	}
	return &Agent{cfg: cfg, account: account, auth: newAuthenticator(cfg.Token), logger: logger}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(a.cfg.SocketPath), 0o750); err != nil {
		return err
	}
	_ = os.Remove(a.cfg.SocketPath)
	listener, err := net.Listen("unix", a.cfg.SocketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(a.cfg.SocketPath, 0o660); err != nil {
		return err
	}
	if err := os.Chown(a.cfg.SocketPath, 0, a.cfg.SocketGID); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/targets", a.authenticated(a.handleTargets))
	mux.HandleFunc("GET /v1/terminal", a.authenticated(a.handleTerminal))
	a.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("控制代理已启动", "socket", a.cfg.SocketPath, "host_user", a.account.Name)
		errCh <- a.server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.server.Shutdown(shutdownCtx)
		_ = os.Remove(a.cfg.SocketPath)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (a *Agent) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.auth.Verify(r, time.Now()); err != nil {
			a.logger.Warn("拒绝未认证的控制请求", "path", r.URL.Path, "error", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (a *Agent) handleTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := listDockerTargets(r.Context(), a.cfg.DockerBin)
	if err != nil {
		http.Error(w, "docker unavailable", http.StatusServiceUnavailable)
		return
	}
	targets = append([]model.Target{{Kind: model.TargetHost, Name: "NAS 主机"}}, targets...)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"targets": targets})
}

func (a *Agent) handleTerminal(w http.ResponseWriter, r *http.Request) {
	target, err := a.resolveTarget(r.Context(), r.URL.Query().Get("kind"), r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(2 << 20)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	process, err := startTerminal(ctx, a.cfg, a.account, target)
	if err != nil {
		_ = writeAgentJSON(ctx, conn, &sync.Mutex{}, model.ControlMessage{Type: "error", Message: err.Error()})
		_ = conn.Close(websocket.StatusInternalError, "terminal start failed")
		return
	}
	defer process.Close()

	writerMu := &sync.Mutex{}
	_ = writeAgentJSON(ctx, conn, writerMu, model.ControlMessage{Type: "state", State: "connected"})
	a.logger.Info("终端已创建", "target_kind", target.Kind, "target_name", target.Name)

	waitCh := make(chan error, 1)
	go func() { waitCh <- process.Wait() }()
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		buffer := make([]byte, 32*1024)
		for {
			n, readErr := process.Read(buffer)
			if n > 0 {
				writerMu.Lock()
				writeErr := conn.Write(ctx, websocket.MessageBinary, append([]byte(nil), buffer[:n]...))
				writerMu.Unlock()
				if writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	readCh := make(chan error, 1)
	go func() {
		readCh <- a.forwardTerminalInput(ctx, conn, process)
	}()

	var exitErr error
	select {
	case exitErr = <-waitCh:
		_ = writeAgentJSON(ctx, conn, writerMu, model.ControlMessage{Type: "exit", Code: exitCode(exitErr)})
	case <-readCh:
		process.Close()
		exitErr = <-waitCh
	case <-ctx.Done():
		process.Close()
		exitErr = ctx.Err()
	}
	cancel()
	process.Close()
	select {
	case <-outputDone:
	case <-time.After(time.Second):
	}
	a.logger.Info("终端已结束", "target_kind", target.Kind, "target_name", target.Name, "exit_code", exitCode(exitErr))
}

func (a *Agent) forwardTerminalInput(ctx context.Context, conn *websocket.Conn, process *terminalProcess) error {
	for {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		switch messageType {
		case websocket.MessageBinary:
			if _, err := process.Write(data); err != nil {
				return err
			}
		case websocket.MessageText:
			var message model.ControlMessage
			if err := json.Unmarshal(data, &message); err != nil {
				continue
			}
			switch message.Type {
			case "resize":
				_ = process.Resize(message.Cols, message.Rows)
			case "close":
				return io.EOF
			}
		}
	}
}

func (a *Agent) resolveTarget(ctx context.Context, kind, id string) (model.Target, error) {
	if kind == model.TargetHost {
		return model.Target{Kind: model.TargetHost, Name: "NAS 主机"}, nil
	}
	if kind != model.TargetContainer || id == "" {
		return model.Target{}, fmt.Errorf("终端目标无效")
	}
	targets, err := listDockerTargets(ctx, a.cfg.DockerBin)
	if err != nil {
		return model.Target{}, err
	}
	for _, target := range targets {
		if target.ID == id {
			return target, nil
		}
	}
	return model.Target{}, fmt.Errorf("容器不存在、已停止或不允许访问")
}

func writeAgentJSON(ctx context.Context, conn *websocket.Conn, mu *sync.Mutex, message model.ControlMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	return conn.Write(ctx, websocket.MessageText, data)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return 1
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
