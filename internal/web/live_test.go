package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/everlst/web-terminal/internal/model"
)

func TestLiveTerminal(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("WT_E2E_BASE_URL"), "/")
	password := os.Getenv("WT_E2E_PASSWORD")
	origin := os.Getenv("WT_E2E_ORIGIN")
	targetName := os.Getenv("WT_E2E_TARGET_NAME")
	if baseURL == "" || password == "" || origin == "" || targetName == "" {
		t.Skip("live Web Terminal environment is not configured")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	loginBody, _ := json.Marshal(map[string]string{"password": password})
	request, _ := http.NewRequest(http.MethodPost, baseURL+"/api/auth/login", bytes.NewReader(loginBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("login returned %d", response.StatusCode)
	}

	var targetsPayload struct {
		Targets []model.Target `json:"targets"`
	}
	getJSON(t, client, baseURL+"/api/targets", &targetsPayload)
	var target *model.Target
	for index := range targetsPayload.Targets {
		if targetsPayload.Targets[index].Name == targetName {
			target = &targetsPayload.Targets[index]
			break
		}
	}
	if target == nil {
		t.Fatalf("target %q was not listed", targetName)
	}

	createBody, _ := json.Marshal(model.CreateSessionRequest{Target: *target})
	request, _ = http.NewRequest(http.MethodPost, baseURL+"/api/sessions", bytes.NewReader(createBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create session returned %d", response.StatusCode)
	}
	var session model.SessionSummary
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	defer deleteLiveSession(t, client, baseURL, origin, session.ID)

	parsed, _ := url.Parse(baseURL)
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else {
		parsed.Scheme = "ws"
	}
	parsed.Path = "/api/sessions/" + session.ID + "/stream"
	header := make(http.Header)
	header.Set("Origin", origin)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, parsed.String(), &websocket.DialOptions{HTTPClient: client, HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageBinary, []byte("echo WT_E2E_OK\r")); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	for !bytes.Contains(output.Bytes(), []byte("WT_E2E_OK")) {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read terminal: %v; output=%q", err, output.String())
		}
		if messageType == websocket.MessageBinary {
			output.Write(data)
		}
	}
	if err := conn.Close(websocket.StatusNormalClosure, "test reconnect"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	conn, _, err = websocket.Dial(ctx, parsed.String(), &websocket.DialOptions{HTTPClient: client, HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var replay bytes.Buffer
	for !bytes.Contains(replay.Bytes(), []byte("WT_E2E_OK")) {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read replay: %v; output=%q", err, replay.String())
		}
		if messageType == websocket.MessageBinary {
			replay.Write(data)
		}
	}
	if err := conn.Write(ctx, websocket.MessageBinary, []byte("echo WT_AFTER_RECONNECT\r")); err != nil {
		t.Fatal(err)
	}
	for !bytes.Contains(replay.Bytes(), []byte("WT_AFTER_RECONNECT")) {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read reconnected terminal: %v", err)
		}
		if messageType == websocket.MessageBinary {
			replay.Write(data)
		}
	}
	if err := conn.Write(ctx, websocket.MessageBinary, []byte("exit\r")); err != nil {
		t.Fatal(err)
	}
}

func getJSON(t *testing.T, client *http.Client, address string, destination any) {
	t.Helper()
	response, err := client.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %d", address, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatal(err)
	}
}

func deleteLiveSession(t *testing.T, client *http.Client, baseURL, origin, sessionID string) {
	t.Helper()
	request, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/sessions/%s", baseURL, sessionID), bytes.NewReader([]byte("{}")))
	request.Header.Set("Origin", origin)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err == nil {
		response.Body.Close()
	}
}
