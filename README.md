# 服务状态监控页面 (Go)

基于可用性 API 实现的 Web 监控状态页，支持实时状态展示和告警推送。

## 功能特性

- 📊 **实时状态展示** - 显示 5/30/60 分钟窗口的错误率
- 📈 **趋势图表** - 可视化错误率变化趋势
- 🔔 **告警推送** - 支持 Webhook 方式推送告警
- 🔄 **自动刷新** - 每 30 秒自动更新数据
- 📱 **响应式设计** - 支持桌面和移动端

## 快速开始

### 编译

```bash
cd status-page-go
go mod tidy
go build -o status-page .
```

### 配置文件

默认读取当前工作目录下的 `status-config.json`。首次启动时如果文件不存在，程序会按默认值创建该文件；因为登录鉴权默认开启，必须设置 `authPassword` 后才能正式启动。

```json
{
  "upstreamAPI": "https://api.fenno.ai/open/v1/upstream-status",
  "port": "3000",
  "alertThreshold": 0.05,
  "alertConsecutivePoints": 2,
  "checkInterval": 5,
  "stateFile": "data/status-state.json",
  "errorLogFile": "data/status-error.log",
  "authEnabled": true,
  "authUsername": "admin",
  "authPassword": "change-me-to-a-strong-password",
  "authSecret": "change-me-to-a-random-session-secret",
  "authSessionHours": 12,
  "authCookieSecure": false
}
```

字段说明：

| 字段 | 说明 |
|------|------|
| `upstreamAPI` | 上游状态接口地址 |
| `port` | Web 服务监听端口 |
| `alertThreshold` | 告警阈值，`0.05` 表示 5% 失败率 |
| `alertConsecutivePoints` | 连续 N 个采样点严格高于阈值才触发告警 |
| `checkInterval` | 上游接口请求间隔，单位分钟 |
| `stateFile` | 最近约 24 小时历史、告警记录和 Webhook 配置的本地持久化文件 |
| `errorLogFile` | 上游接口重试后仍失败时追加写入的本地错误日志 |
| `authEnabled` | 是否开启登录鉴权，公网部署建议保持 `true` |
| `authUsername` | 登录用户名 |
| `authPassword` | 登录密码，鉴权开启时必填 |
| `authSecret` | 会话签名密钥，建议设置为独立随机字符串 |
| `authSessionHours` | 登录有效期，单位小时 |
| `authCookieSecure` | HTTPS 部署建议设为 `true` |

如需把配置文件放到其他路径，可设置一次 `CONFIG_FILE` 指向配置文件；生产部署推荐使用默认路径并固定程序工作目录，避免依赖 shell `export`。

状态页会把最近约 24 小时的历史数据、告警记录和 Webhook 配置保存到 `stateFile`。服务重启后会自动读取该文件恢复状态。
页面和 `/api/*` 接口默认都需要登录后访问，避免直接暴露到公网。
告警规则默认为最近 2 个采样点的 5 分钟错误率都高于 5% 时触发；任一新采样点不高于阈值时发送恢复通知。
告警阈值和连续点数会写入配置文件；配置文件存在时启动优先读取配置文件，页面或 API 修改后也会持久化到该文件。
上游状态接口单次获取失败时会自动重试 1 次；重试后仍失败会写入 `errorLogFile`。

### 启动服务

```bash
./status-page
```

访问 http://localhost:3000 查看状态页面。

## 本地演示（使用模拟 API）

```bash
# 终端 1：启动模拟上游 API
go run mock_upstream.go

# 终端 2：在 status-config.json 中设置：
# "upstreamAPI": "http://localhost:8080/open/v1/upstream-status"
# "authPassword": "demo-password"
./status-page
```

### 模拟不同场景

```bash
# 正常 (2% 错误率)
curl http://localhost:8080/mock/scenario/normal

# 性能下降 (8%)
curl http://localhost:8080/mock/scenario/degraded

# 部分中断 (25%)
curl http://localhost:8080/mock/scenario/partial

# 严重故障 (60%)
curl http://localhost:8080/mock/scenario/outage

# 自定义错误率
curl "http://localhost:8080/mock/set-error?ratio=0.15"
```

## API 接口

### 状态查询

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/status` | GET | 获取当前状态 |
| `/api/status/history` | GET | 获取状态历史 |
| `/api/config` | GET/PUT | 获取或更新告警规则配置 |
| `/api/alerts` | GET | 获取告警历史 |

### Webhook 管理

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/webhooks` | GET | 获取 Webhook 列表 |
| `/api/webhooks` | POST | 注册新 Webhook |
| `/api/webhooks/:id` | DELETE | 删除 Webhook |
| `/api/webhooks/:id/test` | POST | 测试 Webhook |

## Webhook 告警推送

### 注册 Webhook

```bash
curl -X POST https://api.fenno.ai/api/webhooks \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://your-webhook-endpoint.com/alert",
    "name": "主告警通道",
    "type": "generic",
    "secret": "your-secret-key"
  }'
```

### 企业微信机器人 Webhook

企业微信机器人需要发送 `msgtype=text` 格式。注册时把 `type` 设置为 `wecom`，或直接使用 `https://qyapi.weixin.qq.com/cgi-bin/webhook/send?...` 地址，系统会自动识别。

```bash
curl -X POST http://localhost:3000/api/webhooks \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=YOUR_KEY",
    "name": "企业微信群告警",
    "type": "wecom"
  }'
```

发送到企业微信的请求体格式为：

```json
{
  "msgtype": "text",
  "text": {
    "content": "[Status Page] 服务可用性告警\n消息: ...\n时间: ...\n状态: degraded\n错误率: 6.00%\n阈值: 5.00%\n连续点数: 2"
  }
}
```

### 告警推送格式

```json
{
  "type": "alert",
  "message": "服务可用性告警: 连续 2 个采样点错误率高于 5.00%，当前错误率 15.00%",
  "timestamp": "2026-07-06T08:30:00Z",
  "data": {
    "error_ratio": 0.15,
    "threshold": 0.05,
    "consecutive_points": 2,
    "status": "degraded"
  }
}
```

### 恢复通知

```json
{
  "type": "recovery",
  "message": "服务已恢复正常",
  "timestamp": "2026-07-06T09:00:00Z",
  "data": {
    "error_ratio": 0.02,
    "status": "operational"
  }
}
```

## 状态说明

| 状态 | 条件 | 说明 |
|------|------|------|
| `operational` | 错误率 < 阈值 | 服务正常运行 |
| `degraded` | 错误率 ≥ 阈值 且 < 20% | 服务性能下降 |
| `partial_outage` | 错误率 ≥ 20% 且 < 50% | 服务部分中断 |
| `major_outage` | 错误率 ≥ 50% | 服务严重故障 |
| `unknown` | 无法获取数据 | 状态未知 |

## 项目结构

```
status-page-go/
├── main.go           # 主程序
├── templates.go      # HTML 模板
├── mock_upstream.go  # 模拟上游 API（演示用）
├── data/             # 默认本地状态文件目录（运行时自动创建）
├── go.mod
├── go.sum
└── README.md
```

## 生产部署

### Linux 直接启动

假设本地生成的 Linux 二进制为：

```bash
/Users/vincent/today/status-page-linux-amd64-202607071114-1178bce2fbaaa79ecd3bc2f06dd8a285
```

上传到 Linux 服务器后，建议统一重命名为 `status-page`：

```bash
sudo mkdir -p /opt/status-page
sudo cp status-page-linux-amd64-202607071114-1178bce2fbaaa79ecd3bc2f06dd8a285 /opt/status-page/status-page
sudo cp status-config.json /opt/status-page/status-config.json
sudo chmod +x /opt/status-page/status-page
```

编辑 `/opt/status-page/status-config.json`，至少确认这些配置：

```json
{
  "upstreamAPI": "https://api.fenno.ai/open/v1/upstream-status",
  "port": "13000",
  "authUsername": "admin",
  "authPassword": "replace-with-a-strong-password",
  "authSecret": "replace-with-a-long-random-secret",
  "authCookieSecure": false
}
```

前台启动：

```bash
cd /opt/status-page
./status-page
```

后台启动：

```bash
cd /opt/status-page
nohup ./status-page > status-page.log 2>&1 &
```

查看进程和日志：

```bash
pgrep -af status-page
tail -f /opt/status-page/status-page.log
```

如配置中的 `port` 是 `13000`，访问：

```bash
curl -I http://127.0.0.1:13000/
```

### 使用 systemd

将二进制放到 `/opt/status-page/status-page`，并在 `/opt/status-page/status-config.json` 配置运行参数。服务的工作目录固定为 `/opt/status-page`，因此默认配置会从该目录读取，状态和日志文件默认落在 `/opt/status-page/data` 下，不需要 shell `export`。

```ini
[Unit]
Description=Status Page Service
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/status-page
ExecStart=/opt/status-page/status-page
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

保存为 `/etc/systemd/system/status-page.service` 后执行：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now status-page
sudo systemctl status status-page
```

查看运行日志：

```bash
journalctl -u status-page -f
```

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o status-page .

FROM alpine:latest
COPY --from=builder /app/status-page /usr/local/bin/
EXPOSE 3000
CMD ["status-page"]
```

### Nginx 反向代理

```nginx
server {
    listen 80;
    server_name status.fenno.ai;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```
