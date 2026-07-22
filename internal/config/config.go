package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/everlst/web-terminal/internal/security"
)

type Server struct {
	Addr               string
	PasswordHash       string
	AgentSocket        string
	AgentToken         []byte
	MaxSessions        int
	ReconnectWindow    time.Duration
	MaxSessionDuration time.Duration
	BufferBytes        int
	LogLevel           string
}

type Agent struct {
	SocketPath string
	SocketGID  int
	Token      []byte
	HostUser   string
	HostRoot   string
	DockerBin  string
	NsenterBin string
	LogLevel   string
	AllowRoot  bool
}

func LoadServer() (Server, error) {
	password := os.Getenv("WEB_TERMINAL_PASSWORD")
	if password == "" {
		return Server{}, fmt.Errorf("必须设置 WEB_TERMINAL_PASSWORD")
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return Server{}, fmt.Errorf("访问密码无效: %w", err)
	}
	agentToken, err := waitForSecret(env("AGENT_TOKEN_FILE", "/run/web-terminal/agent_token"), 30*time.Second)
	if err != nil {
		return Server{}, fmt.Errorf("读取 Agent 密钥失败: %w", err)
	}
	return Server{
		Addr:               env("LISTEN_ADDR", ":3000"),
		PasswordHash:       passwordHash,
		AgentSocket:        env("AGENT_SOCKET", "/run/web-terminal/control.sock"),
		AgentToken:         agentToken,
		MaxSessions:        envInt("MAX_SESSIONS", 5),
		ReconnectWindow:    envDuration("RECONNECT_WINDOW", 5*time.Minute),
		MaxSessionDuration: envDuration("MAX_SESSION_DURATION", 12*time.Hour),
		BufferBytes:        envInt("SESSION_BUFFER_BYTES", 1<<20),
		LogLevel:           env("LOG_LEVEL", "info"),
	}, nil
}

func LoadAgent() (Agent, error) {
	socketGID := envInt("AGENT_SOCKET_GID", 10001)
	token, err := readOrCreateSecret(env("AGENT_TOKEN_FILE", "/run/web-terminal/agent_token"), socketGID)
	if err != nil {
		return Agent{}, fmt.Errorf("初始化 Agent 密钥失败: %w", err)
	}
	hostUser := strings.TrimSpace(os.Getenv("HOST_USER"))
	return Agent{
		SocketPath: env("AGENT_SOCKET", "/run/web-terminal/control.sock"),
		SocketGID:  socketGID,
		Token:      token,
		HostUser:   hostUser,
		HostRoot:   env("HOST_ROOT", "/proc/1/root"),
		DockerBin:  env("DOCKER_BIN", "docker"),
		NsenterBin: env("NSENTER_BIN", "nsenter"),
		LogLevel:   env("LOG_LEVEL", "info"),
		AllowRoot:  envBool("ALLOW_ROOT_HOST_USER", false),
	}, nil
}

func waitForSecret(path string, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	for {
		secret, err := readSecret(path)
		if err == nil {
			return secret, nil
		}
		if !errors.Is(err, os.ErrNotExist) || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func readOrCreateSecret(path string, gid int) ([]byte, error) {
	secret, err := readSecret(path)
	if err == nil {
		return secret, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, err
	}
	if err := os.Chown(directory, 0, gid); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0o750); err != nil {
		return nil, err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	secret = []byte(base64.RawURLEncoding.EncodeToString(raw))
	temporary, err := os.CreateTemp(directory, ".agent-token-*")
	if err != nil {
		return nil, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o640); err != nil {
		temporary.Close()
		return nil, err
	}
	if err := temporary.Chown(0, gid); err != nil {
		temporary.Close()
		return nil, err
	}
	if _, err := temporary.Write(secret); err != nil {
		temporary.Close()
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return nil, err
	}
	return secret, nil
}

func readSecret(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) < 16 {
		return nil, fmt.Errorf("密钥内容过短")
	}
	return data, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
