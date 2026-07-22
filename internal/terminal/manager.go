package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/everlst/web-terminal/internal/control"
	"github.com/everlst/web-terminal/internal/model"
)

type ManagerOptions struct {
	MaxSessions        int
	ReconnectWindow    time.Duration
	MaxSessionDuration time.Duration
	BufferBytes        int
	Logger             *slog.Logger
}

type Manager struct {
	client   *control.Client
	options  ManagerOptions
	mu       sync.RWMutex
	sessions map[string]*Session
	stop     chan struct{}
	done     chan struct{}
}

type Event struct {
	Type websocket.MessageType
	Data []byte
}

type Attachment struct {
	ID        string
	Events    <-chan Event
	done      <-chan struct{}
	manager   *Manager
	sessionID string
}

type browserAttachment struct {
	id     string
	events chan Event
	done   chan struct{}
	once   sync.Once
}

func (a *browserAttachment) close() {
	a.once.Do(func() { close(a.done) })
}

type Session struct {
	mu             sync.Mutex
	summary        model.SessionSummary
	agent          *websocket.Conn
	agentCancel    context.CancelFunc
	agentWriteMu   sync.Mutex
	buffer         []byte
	bufferLimit    int
	attachment     *browserAttachment
	disconnectedAt *time.Time
	closed         bool
}

func NewManager(client *control.Client, options ManagerOptions) *Manager {
	manager := &Manager{
		client:   client,
		options:  options,
		sessions: make(map[string]*Session),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go manager.cleanupLoop()
	return manager
}

func (m *Manager) Create(ctx context.Context, requested model.Target) (model.SessionSummary, error) {
	target, err := m.resolveTarget(ctx, requested)
	if err != nil {
		return model.SessionSummary{}, err
	}

	m.mu.Lock()
	live := 0
	for _, session := range m.sessions {
		session.mu.Lock()
		if !session.closed {
			live++
		}
		session.mu.Unlock()
	}
	if live >= m.options.MaxSessions {
		m.mu.Unlock()
		return model.SessionSummary{}, fmt.Errorf("最多只能同时运行 %d 个终端", m.options.MaxSessions)
	}
	id, err := randomID()
	if err != nil {
		m.mu.Unlock()
		return model.SessionSummary{}, err
	}
	title := m.uniqueTitleLocked(target.Name)
	now := time.Now()
	deadline := now.Add(m.options.MaxSessionDuration)
	agentCtx, cancel := context.WithDeadline(context.Background(), deadline)
	agent, err := m.client.OpenTerminal(agentCtx, target)
	if err != nil {
		cancel()
		m.mu.Unlock()
		return model.SessionSummary{}, err
	}
	session := &Session{
		summary: model.SessionSummary{
			ID: id, Title: title, Target: target, State: "connecting", CreatedAt: now, ExpiresAt: deadline,
		},
		agent:       agent,
		agentCancel: cancel,
		bufferLimit: m.options.BufferBytes,
	}
	m.sessions[id] = session
	m.mu.Unlock()
	m.options.Logger.Info("会话已创建", "session_id", id, "target_kind", target.Kind, "target_name", target.Name)
	go m.readAgent(session)
	return session.snapshot(), nil
}

func (m *Manager) List() []model.SessionSummary {
	m.mu.RLock()
	list := make([]model.SessionSummary, 0, len(m.sessions))
	for _, session := range m.sessions {
		list = append(list, session.snapshot())
	}
	m.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	return list
}

func (m *Manager) Attach(id string) (*Attachment, error) {
	session := m.get(id)
	if session == nil {
		return nil, fmt.Errorf("终端会话不存在或已过期")
	}
	attachmentID, err := randomID()
	if err != nil {
		return nil, err
	}
	attachment := &browserAttachment{id: attachmentID, events: make(chan Event, 256), done: make(chan struct{})}

	session.mu.Lock()
	if session.attachment != nil {
		session.attachment.close()
	}
	session.attachment = attachment
	session.disconnectedAt = nil
	session.summary.ReconnectUntil = nil
	if !session.closed {
		session.summary.State = "connected"
	}
	bufferCopy := append([]byte(nil), session.buffer...)
	state := session.summary.State
	session.mu.Unlock()

	attachment.events <- jsonEvent(model.ControlMessage{Type: "reset"})
	if len(bufferCopy) > 0 {
		attachment.events <- Event{Type: websocket.MessageBinary, Data: bufferCopy}
	}
	attachment.events <- jsonEvent(model.ControlMessage{Type: "state", State: state})

	return &Attachment{
		ID:        attachmentID,
		Events:    attachment.events,
		done:      attachment.done,
		manager:   m,
		sessionID: id,
	}, nil
}

func (a *Attachment) Done() <-chan struct{} { return a.done }

func (a *Attachment) Detach() { a.manager.detach(a.sessionID, a.ID) }

func (a *Attachment) Input(ctx context.Context, data []byte) error {
	return a.manager.writeAgent(ctx, a.sessionID, websocket.MessageBinary, data)
}

func (a *Attachment) Resize(ctx context.Context, cols, rows uint16) error {
	data, _ := json.Marshal(model.ControlMessage{Type: "resize", Cols: cols, Rows: rows})
	return a.manager.writeAgent(ctx, a.sessionID, websocket.MessageText, data)
}

func (m *Manager) Delete(id string) bool {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if ok {
		m.closeSession(session, "用户关闭")
	}
	return ok
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for id, session := range m.sessions {
		sessions = append(sessions, session)
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	for _, session := range sessions {
		m.closeSession(session, "退出登录")
	}
}

func (m *Manager) Close() {
	select {
	case <-m.stop:
		return
	default:
		close(m.stop)
	}
	<-m.done
	m.CloseAll()
}

func (m *Manager) readAgent(session *Session) {
	ctx := context.Background()
	for {
		messageType, data, err := session.agent.Read(ctx)
		if err != nil {
			status := websocket.CloseStatus(err)
			if status != websocket.StatusNormalClosure && status != websocket.StatusGoingAway {
				m.options.Logger.Warn("控制代理连接结束", "session_id", session.summary.ID, "error", err)
			}
			m.markClosed(session, model.ControlMessage{Type: "exit", Code: 1})
			return
		}
		if messageType == websocket.MessageBinary {
			session.appendOutput(data)
			m.emit(session, Event{Type: messageType, Data: append([]byte(nil), data...)})
			continue
		}
		var controlMessage model.ControlMessage
		if json.Unmarshal(data, &controlMessage) == nil {
			session.mu.Lock()
			switch controlMessage.Type {
			case "state":
				session.summary.State = controlMessage.State
			case "exit", "error":
				session.summary.State = "closed"
				session.closed = true
			}
			session.mu.Unlock()
		}
		m.emit(session, Event{Type: messageType, Data: append([]byte(nil), data...)})
		if controlMessage.Type == "exit" || controlMessage.Type == "error" {
			return
		}
	}
}

func (m *Manager) markClosed(session *Session, message model.ControlMessage) {
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return
	}
	session.closed = true
	session.summary.State = "closed"
	if session.attachment == nil && session.summary.ReconnectUntil == nil {
		now := time.Now()
		reconnectUntil := now.Add(m.options.ReconnectWindow)
		session.disconnectedAt = &now
		session.summary.ReconnectUntil = &reconnectUntil
	}
	session.mu.Unlock()
	m.emit(session, jsonEvent(message))
}

func (m *Manager) emit(session *Session, event Event) {
	session.mu.Lock()
	attachment := session.attachment
	if attachment == nil {
		session.mu.Unlock()
		return
	}
	select {
	case attachment.events <- event:
		session.mu.Unlock()
	default:
		attachment.close()
		session.attachment = nil
		now := time.Now()
		reconnectUntil := now.Add(m.options.ReconnectWindow)
		session.disconnectedAt = &now
		session.summary.State = "recoverable"
		session.summary.ReconnectUntil = &reconnectUntil
		session.mu.Unlock()
	}
}

func (m *Manager) detach(sessionID, attachmentID string) {
	session := m.get(sessionID)
	if session == nil {
		return
	}
	session.mu.Lock()
	if session.attachment == nil || session.attachment.id != attachmentID {
		session.mu.Unlock()
		return
	}
	session.attachment.close()
	session.attachment = nil
	now := time.Now()
	reconnectUntil := now.Add(m.options.ReconnectWindow)
	session.disconnectedAt = &now
	if !session.closed {
		session.summary.State = "recoverable"
	}
	session.summary.ReconnectUntil = &reconnectUntil
	session.mu.Unlock()
}

func (m *Manager) writeAgent(ctx context.Context, sessionID string, messageType websocket.MessageType, data []byte) error {
	session := m.get(sessionID)
	if session == nil {
		return fmt.Errorf("终端会话不存在")
	}
	session.mu.Lock()
	closed := session.closed
	session.mu.Unlock()
	if closed {
		return fmt.Errorf("终端会话已结束")
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	session.agentWriteMu.Lock()
	defer session.agentWriteMu.Unlock()
	return session.agent.Write(writeCtx, messageType, data)
}

func (m *Manager) cleanupLoop() {
	defer close(m.done)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-ticker.C:
			m.cleanup(now)
		}
	}
}

func (m *Manager) cleanup(now time.Time) {
	var expired []*Session
	m.mu.Lock()
	for id, session := range m.sessions {
		session.mu.Lock()
		shouldExpire := now.After(session.summary.ExpiresAt)
		if session.attachment == nil && session.summary.ReconnectUntil != nil && now.After(*session.summary.ReconnectUntil) {
			shouldExpire = true
		}
		session.mu.Unlock()
		if shouldExpire {
			delete(m.sessions, id)
			expired = append(expired, session)
		}
	}
	m.mu.Unlock()
	for _, session := range expired {
		m.closeSession(session, "会话过期")
	}
}

func (m *Manager) closeSession(session *Session, reason string) {
	session.mu.Lock()
	if session.attachment != nil {
		session.attachment.close()
		session.attachment = nil
	}
	wasClosed := session.closed
	session.closed = true
	session.summary.State = "closed"
	session.mu.Unlock()
	if !wasClosed {
		data, _ := json.Marshal(model.ControlMessage{Type: "close"})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		session.agentWriteMu.Lock()
		_ = session.agent.Write(ctx, websocket.MessageText, data)
		session.agentWriteMu.Unlock()
		cancel()
	}
	_ = session.agent.Close(websocket.StatusNormalClosure, reason)
	session.agentCancel()
	m.options.Logger.Info("会话已清理", "session_id", session.summary.ID, "reason", reason)
}

func (m *Manager) resolveTarget(ctx context.Context, requested model.Target) (model.Target, error) {
	targets, err := m.client.Targets(ctx)
	if err != nil {
		return model.Target{}, err
	}
	for _, target := range targets {
		if requested.Kind == model.TargetHost && target.Kind == model.TargetHost {
			return target, nil
		}
		if requested.Kind == model.TargetContainer && target.Kind == model.TargetContainer && target.ID == requested.ID {
			return target, nil
		}
	}
	return model.Target{}, fmt.Errorf("终端目标不存在、已停止或不可访问")
}

func (m *Manager) get(id string) *Session {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	return session
}

func (m *Manager) uniqueTitleLocked(base string) string {
	used := make(map[string]bool)
	for _, session := range m.sessions {
		used[session.summary.Title] = true
	}
	if !used[base] {
		return base
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s · %d", base, index)
		if !used[candidate] {
			return candidate
		}
	}
}

func (s *Session) snapshot() model.SessionSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.summary
}

func (s *Session) appendOutput(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(data) >= s.bufferLimit {
		s.buffer = append(s.buffer[:0], data[len(data)-s.bufferLimit:]...)
		return
	}
	overflow := len(s.buffer) + len(data) - s.bufferLimit
	if overflow > 0 {
		copy(s.buffer, s.buffer[overflow:])
		s.buffer = s.buffer[:len(s.buffer)-overflow]
	}
	s.buffer = append(s.buffer, data...)
}

func jsonEvent(message model.ControlMessage) Event {
	data, _ := json.Marshal(message)
	return Event{Type: websocket.MessageText, Data: data}
}

func randomID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}
