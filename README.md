# Web Terminal

一个面向个人 NAS 的极简 Web 终端。它只提供终端能力，不包含文件管理、容器生命周期管理或其他面板功能。

- 单密码登录，无用户名
- NAS 宿主机普通用户终端
- 进入当前正在运行的 Docker 容器
- 多标签、5 分钟断线恢复、12 小时会话上限
- 系统深浅主题与手机布局
- 适配现有 Cloudflare Tunnel
- 单镜像双角色部署：无特权 Web 服务 + 无网络控制代理

## 重要安全警告

`control-agent` 必须使用 `privileged: true`、`pid: host` 并挂载 Docker Socket，才能从容器中创建宿主机终端并进入其他容器。该权限接近宿主机 root。

项目通过以下边界降低风险：

- 公网 Web 服务不挂载 Docker Socket，不进入宿主命名空间，并以 UID 10001 运行。
- 高权限 Agent 设置 `network_mode: none`，没有监听 TCP 端口。
- 两个容器只通过权限受限、带 HMAC 请求认证的 Unix Socket 通信。
- Agent 自动选择宿主机中 UID 最小、具有登录 shell 的普通用户，永远不会自动选择 root。
- Web Terminal 自身的两个容器不会出现在可进入容器列表中。
- 不记录命令输入或终端输出。

由于你选择只使用应用密码，必须使用长度至少 16 位、与其他服务不重复的随机密码。强烈建议 Cloudflare 侧再限制访问国家、IP 或设备。

## 快速部署

### 1. 配置端口和密码

```bash
cp .env.example .env
```

编辑 `.env`：

```dotenv
WEB_TERMINAL_PORT=3000
WEB_TERMINAL_PASSWORD=请替换为至少16位的访问密码
```

仅需修改这两个值。无需配置公网地址、生成密码哈希、创建 Docker Secret 或填写宿主机用户名。

应用启动时会在内存中将访问密码转换为 Argon2id 哈希；Agent 通信密钥会在内部共享卷中自动随机生成。`.env` 已被 Git 忽略，请不要将它上传到代码仓库。

### 2. 启动

```bash
docker compose up -d
docker compose ps
curl http://127.0.0.1:3000/healthz
curl http://127.0.0.1:3000/readyz
```

如果修改了默认端口，将命令中的 `3000` 替换为实际端口。端口会发布到 NAS 的所有网络接口，可直接通过 `http://NAS局域网IP:端口` 使用。访问仍必须输入密码。

宿主终端用户由 Agent 从宿主机 `/etc/passwd` 自动识别：选择 UID 不小于 1000、具有可用登录 shell 且 UID 最小的非 root 用户。

## 接入已有 Cloudflare Tunnel

Cloudflare Tunnel 使用从 NAS 到 Cloudflare 的出站连接，无需在路由器上开放入站端口。Web Terminal 的 WebSocket 会话会自动处理 Tunnel 更新或网络切换造成的短暂断开。

不需要把 Cloudflare 公网域名写入 Web Terminal。程序会根据浏览器 Origin、请求 Host 和 Cloudflare 转发头自动完成同源校验，并自动决定 Cookie 是否添加 `Secure`。

### cloudflared 运行在宿主机

将 Public Hostname 的 Origin Service 指向：

```text
http://127.0.0.1:3000
```

### cloudflared 运行在容器中

把现有 cloudflared 容器连接到 Web Terminal 网络：

```bash
docker network connect web-terminal <cloudflared-container-name>
```

Origin Service 使用：

```text
http://web-terminal:3000
```

Cloudflare 参考文档：

- [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/get-started/)
- [Cloudflare WebSockets](https://developers.cloudflare.com/network/websockets/)

## 配置项

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `WEB_TERMINAL_PORT` | `3000` | NAS 对局域网监听的端口 |
| `WEB_TERMINAL_PASSWORD` | 必填 | Web 页面访问密码，至少 16 个字符 |

固定安全默认值：最多 5 个终端、断开后保留 5 分钟、最长运行 12 小时、每个会话保留最后 1 MiB 输出、每 IP 15 分钟最多 5 次失败登录。

## 会话行为

- 首次登录后自动打开一个 `NAS 主机` 标签。
- 新建终端菜单列出宿主机和所有运行中容器。
- 容器终端遵循容器镜像或 Compose 配置的默认用户，不自动使用 root。
- 容器内依次探测 `bash`、`zsh`、`ash`、`sh`。
- 页面刷新或 Cloudflare WebSocket 短暂断开时，服务端会重放最后 1 MiB 输出。
- 服务重启、退出登录、超过 12 小时或断开超过 5 分钟后，会话会被终止。

## 本地开发与测试

前端：

```bash
cd web
npm ci
npm test
npm run build
```

Go 测试无需在宿主机安装 Go：

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.25-alpine \
  sh -c 'go test ./...'
```

构建本地镜像：

```bash
docker build --build-arg VERSION=0.1.1 -t evlst/web-terminal:0.1.1 .
```

## 发布镜像

```bash
docker buildx build \
  --builder multiarch \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=0.1.1 \
  -t evlst/web-terminal:0.1.1 \
  -t evlst/web-terminal:latest \
  --push .

docker buildx imagetools inspect evlst/web-terminal:0.1.1
docker buildx imagetools inspect evlst/web-terminal:latest
```

## API

- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/auth/session`
- `GET /api/targets`
- `GET /api/sessions`
- `POST /api/sessions`
- `DELETE /api/sessions/:id`
- `GET /api/sessions/:id/stream`（WebSocket）
- `GET /healthz`
- `GET /readyz`

终端输入输出使用 WebSocket 二进制帧；`resize`、`state`、`exit`、`error` 和心跳使用 JSON 文本帧。

## v0.1.1 边界

不包含多用户、2FA、文件管理、上传下载、命令审计、容器创建/停止/删除、数据库或服务重启后的长期终端持久化。
