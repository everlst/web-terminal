package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/everlst/web-terminal/internal/config"
	"github.com/everlst/web-terminal/internal/control"
	"github.com/everlst/web-terminal/internal/security"
	"github.com/everlst/web-terminal/internal/terminal"
	webserver "github.com/everlst/web-terminal/internal/web"
)

//go:embed all:web/dist
var webAssets embed.FS

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		os.Args = append([]string{os.Args[0], "server"}, os.Args[1:]...)
	}

	var err error
	switch os.Args[1] {
	case "server":
		err = runServer()
	case "agent":
		err = runAgent()
	case "hash-password":
		err = runHashPassword(os.Args[2:])
	case "healthcheck":
		err = runHealthcheck(os.Args[2:])
	case "version":
		fmt.Println(version)
		return
	default:
		err = fmt.Errorf("未知命令 %q，可用命令：server、agent、hash-password、healthcheck、version", os.Args[1])
	}
	if err != nil {
		slog.Error("命令执行失败", "command", os.Args[1], "error", err)
		os.Exit(1)
	}
}

func runServer() error {
	cfg, err := config.LoadServer()
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogLevel)
	client, err := control.NewClient(cfg.AgentSocket, cfg.AgentToken)
	if err != nil {
		return err
	}
	sessions := terminal.NewManager(client, terminal.ManagerOptions{
		MaxSessions:        cfg.MaxSessions,
		ReconnectWindow:    cfg.ReconnectWindow,
		MaxSessionDuration: cfg.MaxSessionDuration,
		BufferBytes:        cfg.BufferBytes,
		Logger:             logger,
	})
	defer sessions.Close()

	server, err := webserver.New(cfg, client, sessions, webAssets, version, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return server.Run(ctx)
}

func runAgent() error {
	cfg, err := config.LoadAgent()
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogLevel)
	agent, err := control.NewAgent(cfg, logger)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return agent.Run(ctx)
}

func runHealthcheck(args []string) error {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	url := fs.String("url", "http://127.0.0.1:3000/healthz", "健康检查地址")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return webserver.CheckHealth(ctx, *url)
}

func runHashPassword(args []string) error {
	fs := flag.NewFlagSet("hash-password", flag.ContinueOnError)
	outputPath := fs.String("output", "", "将密码哈希写入指定文件")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *outputPath == "" {
		return security.RunPasswordHasher(os.Stdin, os.Stdout)
	}
	file, err := os.OpenFile(*outputPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return security.RunPasswordHasher(os.Stdin, file)
}

func newLogger(level string) *slog.Logger {
	var parsed slog.Level
	if err := parsed.UnmarshalText([]byte(level)); err != nil {
		parsed = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parsed}))
}
