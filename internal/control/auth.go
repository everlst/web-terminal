package control

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	headerTimestamp = "X-WT-Timestamp"
	headerNonce     = "X-WT-Nonce"
	headerSignature = "X-WT-Signature"
)

type authenticator struct {
	secret []byte
	mu     sync.Mutex
	nonces map[string]time.Time
}

func newAuthenticator(secret []byte) *authenticator {
	return &authenticator{secret: append([]byte(nil), secret...), nonces: make(map[string]time.Time)}
}

func (a *authenticator) Headers(method, requestURI string, now time.Time) (http.Header, error) {
	nonceBytes := make([]byte, 18)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	timestamp := strconv.FormatInt(now.Unix(), 10)
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	signature := a.signature(method, requestURI, timestamp, nonce)
	header := make(http.Header)
	header.Set(headerTimestamp, timestamp)
	header.Set(headerNonce, nonce)
	header.Set(headerSignature, signature)
	return header, nil
}

func (a *authenticator) Verify(r *http.Request, now time.Time) error {
	timestamp := r.Header.Get(headerTimestamp)
	nonce := r.Header.Get(headerNonce)
	signature := r.Header.Get(headerSignature)
	if timestamp == "" || nonce == "" || signature == "" {
		return errors.New("缺少 Agent 认证头")
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("Agent 时间戳无效")
	}
	requestTime := time.Unix(seconds, 0)
	if delta := now.Sub(requestTime); delta > 30*time.Second || delta < -30*time.Second {
		return errors.New("Agent 请求已过期")
	}
	expected := a.signature(r.Method, r.URL.RequestURI(), timestamp, nonce)
	provided, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return errors.New("Agent 签名无效")
	}
	expectedBytes, _ := base64.RawURLEncoding.DecodeString(expected)
	if !hmac.Equal(provided, expectedBytes) {
		return errors.New("Agent 签名不匹配")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	for value, expires := range a.nonces {
		if now.After(expires) {
			delete(a.nonces, value)
		}
	}
	if _, exists := a.nonces[nonce]; exists {
		return errors.New("Agent nonce 已使用")
	}
	a.nonces[nonce] = now.Add(time.Minute)
	return nil
}

func (a *authenticator) signature(method, requestURI, timestamp, nonce string) string {
	mac := hmac.New(sha256.New, a.secret)
	fmt.Fprintf(mac, "%s\n%s\n%s\n%s", method, requestURI, timestamp, nonce)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
