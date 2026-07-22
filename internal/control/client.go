package control

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
	"github.com/everlst/web-terminal/internal/model"
)

type Client struct {
	socket string
	auth   *authenticator
	http   *http.Client
	wsHTTP *http.Client
}

func NewClient(socket string, secret []byte) (*Client, error) {
	if socket == "" || len(secret) < 16 {
		return nil, fmt.Errorf("Agent 客户端配置无效")
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
	}
	return &Client{
		socket: socket,
		auth:   newAuthenticator(secret),
		http:   &http.Client{Transport: transport, Timeout: 5 * time.Second},
		wsHTTP: &http.Client{Transport: transport},
	}, nil
}

func (c *Client) Targets(ctx context.Context) ([]model.Target, error) {
	const requestURI = "/v1/targets"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix"+requestURI, nil)
	if err != nil {
		return nil, err
	}
	headers, err := c.auth.Headers(req.Method, requestURI, time.Now())
	if err != nil {
		return nil, err
	}
	req.Header = headers
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接控制代理失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("控制代理返回状态 %d", response.StatusCode)
	}
	var payload struct {
		Targets []model.Target `json:"targets"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Targets, nil
}

func (c *Client) OpenTerminal(ctx context.Context, target model.Target) (*websocket.Conn, error) {
	values := make(url.Values)
	values.Set("kind", target.Kind)
	if target.ID != "" {
		values.Set("id", target.ID)
	}
	requestURI := "/v1/terminal?" + values.Encode()
	headers, err := c.auth.Headers(http.MethodGet, requestURI, time.Now())
	if err != nil {
		return nil, err
	}
	conn, response, err := websocket.Dial(ctx, "ws://unix"+requestURI, &websocket.DialOptions{
		HTTPClient: c.wsHTTP,
		HTTPHeader: headers,
	})
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("控制代理终端连接失败（状态 %d）: %w", response.StatusCode, err)
		}
		return nil, fmt.Errorf("控制代理终端连接失败: %w", err)
	}
	conn.SetReadLimit(2 << 20)
	return conn, nil
}

func (c *Client) Ready(ctx context.Context) error {
	_, err := c.Targets(ctx)
	return err
}
