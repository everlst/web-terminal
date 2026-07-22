package control

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/everlst/web-terminal/internal/config"
	"github.com/everlst/web-terminal/internal/model"
)

type terminalProcess struct {
	file *os.File
	cmd  *exec.Cmd
	once sync.Once
}

func startTerminal(ctx context.Context, cfg config.Agent, account hostAccount, target model.Target) (*terminalProcess, error) {
	var cmd *exec.Cmd
	switch target.Kind {
	case model.TargetHost:
		cmd = exec.CommandContext(ctx, cfg.NsenterBin,
			"--target", "1",
			"--mount", "--uts", "--ipc", "--net", "--pid",
			"--root="+cfg.HostRoot,
			"--wd=/",
			"--", "su", "-l", account.Name,
		)
	case model.TargetContainer:
		shell, err := detectContainerShell(ctx, cfg.DockerBin, target.ID)
		if err != nil {
			return nil, err
		}
		cmd = exec.CommandContext(ctx, cfg.DockerBin, "exec", "-i", "-t", target.ID, shell)
	default:
		return nil, fmt.Errorf("未知终端目标")
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		return nil, err
	}
	return &terminalProcess{file: file, cmd: cmd}, nil
}

func detectContainerShell(ctx context.Context, dockerBin, containerID string) (string, error) {
	for _, shell := range []string{"/bin/bash", "/usr/bin/bash", "/bin/zsh", "/usr/bin/zsh", "/bin/ash", "/bin/sh"} {
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		command := exec.CommandContext(checkCtx, dockerBin, "exec", containerID, shell, "-c", "exit 0")
		err := command.Run()
		cancel()
		if err == nil {
			return shell, nil
		}
	}
	return "", fmt.Errorf("容器中未找到 bash、zsh、ash 或 sh")
}

func (p *terminalProcess) Read(buffer []byte) (int, error) {
	return p.file.Read(buffer)
}

func (p *terminalProcess) Write(data []byte) (int, error) {
	return p.file.Write(data)
}

func (p *terminalProcess) Resize(cols, rows uint16) error {
	if cols < 2 || rows < 2 || cols > 1000 || rows > 1000 {
		return fmt.Errorf("终端尺寸无效")
	}
	return pty.Setsize(p.file, &pty.Winsize{Cols: cols, Rows: rows})
}

func (p *terminalProcess) Wait() error {
	err := p.cmd.Wait()
	if strings.Contains(fmt.Sprint(err), "signal: killed") {
		return io.EOF
	}
	return err
}

func (p *terminalProcess) Close() {
	p.once.Do(func() {
		_ = p.file.Close()
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
	})
}
