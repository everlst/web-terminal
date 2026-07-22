package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"
)

type LoginSession struct {
	ExpiresAt time.Time
	LastSeen  time.Time
}

type SessionStore struct {
	mu       sync.Mutex
	sessions map[[32]byte]LoginSession
	lifetime time.Duration
}

func NewSessionStore(lifetime time.Duration) *SessionStore {
	return &SessionStore{sessions: make(map[[32]byte]LoginSession), lifetime: lifetime}
}

func (s *SessionStore) Create(now time.Time) (string, LoginSession, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", LoginSession{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	session := LoginSession{ExpiresAt: now.Add(s.lifetime), LastSeen: now}
	s.mu.Lock()
	s.sessions[sha256.Sum256([]byte(token))] = session
	s.mu.Unlock()
	return token, session, nil
}

func (s *SessionStore) Validate(token string, now time.Time) (LoginSession, bool) {
	key := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[key]
	if !ok || !now.Before(session.ExpiresAt) {
		delete(s.sessions, key)
		return LoginSession{}, false
	}
	session.LastSeen = now
	s.sessions[key] = session
	return session, true
}

func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, sha256.Sum256([]byte(token)))
	s.mu.Unlock()
}

func (s *SessionStore) DeleteAll() {
	s.mu.Lock()
	s.sessions = make(map[[32]byte]LoginSession)
	s.mu.Unlock()
}

func (s *SessionStore) Cleanup(now time.Time) {
	s.mu.Lock()
	for key, session := range s.sessions {
		if !now.Before(session.ExpiresAt) {
			delete(s.sessions, key)
		}
	}
	s.mu.Unlock()
}
