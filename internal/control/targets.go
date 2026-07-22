package control

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/everlst/web-terminal/internal/config"
	"github.com/everlst/web-terminal/internal/model"
)

var hostUserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]*[$]?$`)

type hostAccount struct {
	Name  string
	UID   int
	GID   int
	Home  string
	Shell string
}

func validateHostAccount(cfg config.Agent) (hostAccount, error) {
	if cfg.HostUser != "" && !hostUserPattern.MatchString(cfg.HostUser) {
		return hostAccount{}, fmt.Errorf("HOST_USER 格式无效")
	}
	passwdPath := filepath.Join(cfg.HostRoot, "etc/passwd")
	file, err := os.Open(passwdPath)
	if err != nil {
		return hostAccount{}, fmt.Errorf("读取宿主机 passwd 失败: %w", err)
	}
	defer file.Close()
	var detected *hostAccount
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 7 {
			continue
		}
		uid, errUID := strconv.Atoi(fields[2])
		gid, errGID := strconv.Atoi(fields[3])
		if errUID != nil || errGID != nil {
			if fields[0] == cfg.HostUser {
				return hostAccount{}, fmt.Errorf("宿主机用户 UID/GID 无效")
			}
			continue
		}
		account := hostAccount{Name: fields[0], UID: uid, GID: gid, Home: fields[5], Shell: fields[6]}
		unusableShell := strings.Contains(account.Shell, "nologin") || strings.HasSuffix(account.Shell, "/false")
		if cfg.HostUser != "" {
			if account.Name != cfg.HostUser {
				continue
			}
			if uid == 0 && !cfg.AllowRoot {
				return hostAccount{}, fmt.Errorf("HOST_USER 不允许为 root")
			}
			if unusableShell {
				return hostAccount{}, fmt.Errorf("HOST_USER 没有可用登录 shell")
			}
			return account, nil
		}
		if uid < 1000 || uid == 65534 || unusableShell || !hostUserPattern.MatchString(account.Name) {
			continue
		}
		if detected == nil || account.UID < detected.UID {
			candidate := account
			detected = &candidate
		}
	}
	if err := scanner.Err(); err != nil {
		return hostAccount{}, err
	}
	if detected != nil {
		return *detected, nil
	}
	if cfg.HostUser == "" {
		return hostAccount{}, fmt.Errorf("未找到可用的非 root 宿主机用户")
	}
	return hostAccount{}, fmt.Errorf("宿主机用户 %q 不存在", cfg.HostUser)
}

type dockerRow struct {
	ID     string `json:"ID"`
	Image  string `json:"Image"`
	Names  string `json:"Names"`
	Status string `json:"Status"`
	Labels string `json:"Labels"`
}

func listDockerTargets(ctx context.Context, dockerBin string) ([]model.Target, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, dockerBin, "ps", "--filter", "status=running", "--format", "{{json .}}").Output()
	if err != nil {
		return nil, fmt.Errorf("列出 Docker 容器失败: %w", err)
	}
	var targets []model.Target
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		var row dockerRow
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, fmt.Errorf("解析 Docker 容器失败: %w", err)
		}
		if hasInternalLabel(row.Labels) {
			continue
		}
		targets = append(targets, model.Target{
			Kind: model.TargetContainer, ID: row.ID, Name: row.Names, Image: row.Image, Status: row.Status,
		})
	}
	return targets, scanner.Err()
}

func hasInternalLabel(labels string) bool {
	for _, label := range strings.Split(labels, ",") {
		if strings.TrimSpace(label) == "com.evlst.web-terminal.internal=true" {
			return true
		}
	}
	return false
}
