package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/everlst/web-terminal/internal/config"
	"github.com/everlst/web-terminal/internal/control"
	"github.com/everlst/web-terminal/internal/model"
	"github.com/everlst/web-terminal/internal/security"
	"github.com/everlst/web-terminal/internal/terminal"
)

const sessionCookie = "wt_session"

type Server struct {
	cfg          config.Server
	control      *control.Client
	terminals    *terminal.Manager
	authSessions *security.SessionStore
	limiter      *security.LoginLimiter
	assets       fs.FS
	version      string
	logger       *slog.Logger
	http         *http.Server
}

func New(cfg config.Server, controlClient *control.Client, terminals *terminal.Manager, embedded embed.FS, version string, logger *slog.Logger) (*Server, error) {
	assets, err := fs.Sub(embedded, "web/dist")
	if err != nil {
		return nil, err
	}
	server := &Server{
		cfg:          cfg,
		control:      controlClient,
		terminals:    terminals,
		authSessions: security.NewSessionStore(cfg.MaxSessionDuration),
		limiter:      security.NewLoginLimiter(5, 15*time.Minute, 250*time.Millisecond),
		assets:       assets,
		version:      version,
		logger:       logger,
	}
	server.http = &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       75 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	return server, nil
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("Web Terminal 已启动", "addr", s.cfg.Addr, "version", s.version)
		errCh <- s.http.ListenAndServe()
	}()
	cleanupTicker := time.NewTicker(time.Minute)
	defer cleanupTicker.Stop()
	for {
		select {
		case <-cleanupTicker.C:
			s.authSessions.Cleanup(time.Now())
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return s.http.Shutdown(shutdownCtx)
		case err := <-errCh:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		}
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("GET /api/auth/session", s.handleAuthSession)
	mux.HandleFunc("GET /api/targets", s.requireAuth(s.handleTargets))
	mux.HandleFunc("GET /api/sessions", s.requireAuth(s.handleListSessions))
	mux.HandleFunc("POST /api/sessions", s.requireAuth(s.handleCreateSession))
	mux.HandleFunc("DELETE /api/sessions/{id}", s.requireAuth(s.handleDeleteSession))
	mux.HandleFunc("GET /api/sessions/{id}/stream", s.requireAuth(s.handleSessionStream))
	mux.HandleFunc("/", s.handleStatic)
	return s.securityHeaders(mux)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data:; connect-src 'self' ws: wss:; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		if security.IsHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		loginSession, ok := s.authenticate(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "登录已失效，请重新输入访问密码")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), loginSessionKey{}, loginSession))
		next(w, r)
	}
}

func (s *Server) authenticate(r *http.Request) (security.LoginSession, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return security.LoginSession{}, false
	}
	return s.authSessions.Validate(cookie.Value, time.Now())
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": s.version})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.control.Ready(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "agent_unavailable", "控制代理不可用")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !security.ValidOrigin(r) {
		writeError(w, http.StatusForbidden, "invalid_origin", "请求来源无效")
		return
	}
	ip := security.ClientIP(r, true)
	allowed, retryAfter := s.limiter.Check(ip, time.Now())
	if !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "登录尝试过多，请稍后再试")
		return
	}
	var payload struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &payload, 4096); err != nil {
		return
	}
	valid, err := security.VerifyPassword(s.cfg.PasswordHash, payload.Password)
	if err != nil {
		s.logger.Error("密码哈希配置无效", "error", err)
		writeError(w, http.StatusInternalServerError, "server_config", "服务配置无效")
		return
	}
	if !valid {
		delay := s.limiter.Failure(ip, time.Now())
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		s.logger.Warn("登录失败", "client_ip", ip)
		writeError(w, http.StatusUnauthorized, "invalid_password", "访问密码错误")
		return
	}
	s.limiter.Success(ip)
	token, session, err := s.authSessions.Create(time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session_error", "无法创建登录会话")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: security.IsHTTPS(r),
		SameSite: http.SameSiteStrictMode, Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds()),
	})
	s.logger.Info("登录成功", "client_ip", ip)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !security.ValidOrigin(r) {
		writeError(w, http.StatusForbidden, "invalid_origin", "请求来源无效")
		return
	}
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		s.authSessions.Delete(cookie.Value)
	}
	s.terminals.CloseAll()
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: security.IsHTTPS(r),
		SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0),
	})
	s.logger.Info("用户已退出")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	session, ok := s.authenticate(r)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		_ = json.NewEncoder(w).Encode(map[string]any{"authenticated": false})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"authenticated": true, "expiresAt": session.ExpiresAt})
}

func (s *Server) handleTargets(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	targets, err := s.control.Targets(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "agent_unavailable", "控制代理不可用，请检查 Agent 容器")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": targets})
}

func (s *Server) handleListSessions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": s.terminals.List()})
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if !security.ValidOrigin(r) {
		writeError(w, http.StatusForbidden, "invalid_origin", "请求来源无效")
		return
	}
	var payload model.CreateSessionRequest
	if err := decodeJSON(w, r, &payload, 8192); err != nil {
		return
	}
	session, err := s.terminals.Create(r.Context(), payload.Target)
	if err != nil {
		writeError(w, http.StatusBadRequest, "session_create_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if !security.ValidOrigin(r) {
		writeError(w, http.StatusForbidden, "invalid_origin", "请求来源无效")
		return
	}
	if !s.terminals.Delete(r.PathValue("id")) {
		writeError(w, http.StatusNotFound, "session_not_found", "终端会话不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSessionStream(w http.ResponseWriter, r *http.Request) {
	if !security.ValidOrigin(r) {
		writeError(w, http.StatusForbidden, "invalid_origin", "请求来源无效")
		return
	}
	loginSession, _ := r.Context().Value(loginSessionKey{}).(security.LoginSession)
	ctx, cancel := context.WithDeadline(r.Context(), loginSession.ExpiresAt)
	defer cancel()
	attachment, err := s.terminals.Attach(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "session_not_found", err.Error())
		return
	}
	defer attachment.Detach()

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)

	direct := make(chan terminal.Event, 16)
	writerDone := make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				writerDone <- ctx.Err()
				return
			case <-attachment.Done():
				writerDone <- io.EOF
				return
			case event := <-attachment.Events:
				if err := conn.Write(ctx, event.Type, event.Data); err != nil {
					writerDone <- err
					return
				}
			case event := <-direct:
				if err := conn.Write(ctx, event.Type, event.Data); err != nil {
					writerDone <- err
					return
				}
			}
		}
	}()

	readDone := make(chan error, 1)
	go func() {
		for {
			messageType, data, err := conn.Read(ctx)
			if err != nil {
				readDone <- err
				return
			}
			if messageType == websocket.MessageBinary {
				if err := attachment.Input(ctx, data); err != nil {
					readDone <- err
					return
				}
				continue
			}
			var message model.ControlMessage
			if json.Unmarshal(data, &message) != nil {
				continue
			}
			switch message.Type {
			case "resize":
				if err := attachment.Resize(ctx, message.Cols, message.Rows); err != nil {
					readDone <- err
					return
				}
			case "ping":
				direct <- jsonTerminalEvent(model.ControlMessage{Type: "pong"})
			}
		}
	}()

	select {
	case <-ctx.Done():
	case <-attachment.Done():
	case <-writerDone:
	case <-readDone:
	}
	_ = conn.Close(websocket.StatusNormalClosure, "session detached")
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if requested == "." || requested == "" {
		requested = "index.html"
	}
	data, err := fs.ReadFile(s.assets, requested)
	if err != nil {
		data, err = fs.ReadFile(s.assets, "index.html")
		requested = "index.html"
	}
	if err != nil {
		http.Error(w, "前端资源尚未构建", http.StatusServiceUnavailable)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(requested)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if requested == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	_, _ = w.Write(data)
}

func CheckHealth(ctx context.Context, address string) error {
	parsed, err := url.Parse(address)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("健康检查 URL 必须使用 http 或 https")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("健康检查返回状态 %d", response.StatusCode)
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any, maxBytes int64) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content_type", "请求必须使用 application/json")
		return errors.New("invalid content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "请求内容无效")
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, model.APIError{Error: model.ErrorBody{Code: code, Message: message}})
}

func jsonTerminalEvent(message model.ControlMessage) terminal.Event {
	data, _ := json.Marshal(message)
	return terminal.Event{Type: websocket.MessageText, Data: data}
}

type loginSessionKey struct{}
