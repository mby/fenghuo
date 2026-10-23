# AGENTS.md

本文件写给参与本项目开发的 AI 编码助手（以及人类贡献者）。修改代码前请完整阅读。

## 项目是什么

烽火（Fenghuo）是一个极简、安全优先的服务器探针：

- `fenghuo-agent`：装在被监控机器上，读取 `/proc`，每 3 秒用 HTTPS POST 上报一次。**只发不收**。
- `fenghuo-server`：接收上报、内存保存实时状态、SQLite 保存聚合历史、提供状态页和管理后台。
- `web/`：React 前端，构建产物通过 `go:embed` 打进 server 二进制。

## 安全红线（任何情况下不得违反）

1. **Agent 只发送数据**：不接收、不解析任何服务端返回的指令或配置；HTTP 响应体只丢弃，只看状态码。唯一的例外是 `proto.StatusWatched`（204）/ `proto.StatusIdle`（202）这 1 个比特，Agent 只能据此在**本地配置**的 `-interval` 和 `-idle-interval` 之间切换。不得扩展这个信号，不得让服务端指定任何具体数值。
2. **Agent 禁止**：引入任何第三方依赖；import `os/exec`、`plugin`、`net/rpc`、`net/http/cgi`、`net/http/fcgi`、`net/http/pprof`、`net/smtp`；监听任何端口；写磁盘；自动更新。`cmd/fenghuo-agent/deps_test.go` 会强制检查，**不得为了让测试通过而修改这个测试**。
3. **不做任何"服务端控制 Agent"的功能**：远程命令、Web 终端、文件管理、计划任务、远程改配置、自动升级。需求再合理也不做。
4. 管理接口只能在 `api.mountAdmin` 中注册，并且必须经过 `requireAdmin`（登录接口除外）。
5. 公开接口（`/api/v1/*`）不得返回 IP、Token、私有备注等字段；新增字段先想清楚是否会泄露信息。
6. 不使用 JWT；会话存服务端（`sessions` 表只存 Token 哈希）。
7. 所有请求体都要限制大小并 `DisallowUnknownFields`；上报数据必须经过 `proto.Report.Validate`。
8. 前端禁止 `dangerouslySetInnerHTML`（`src/components/ui` 中 shadcn 生成的 chart 样式除外，其内容来自静态配置）；用户输入一律按文本渲染。
9. 新增任何依赖（Go 或 npm）前必须先询问维护者。

## 架构与分层

```
cmd/fenghuo-agent        Agent 入口（flag + 环境变量）
cmd/fenghuo-server       Server 入口与子命令（serve / reset-password / disable-totp）
internal/proto           Agent↔Server 唯一的消息结构，两端共用
internal/agent           上报循环、HTTP 客户端、TCP 延迟测试
internal/agent/collect   /proc 解析（parse.go 纯函数可在任意系统测试，linux.go 读文件）
internal/server/config   配置解析
internal/server/store    SQLite（database/sql + modernc），schema/*.sql 为迁移
internal/server/hub      内存实时状态、环形缓冲、分钟累加器、流量计算
internal/server/rollup   每分钟落库、每小时聚合与清理
internal/server/alert    告警规则评估
internal/server/notify   Telegram / Webhook 发送，带 SSRF 防护
internal/server/auth     argon2id、TOTP、Token、限流
internal/server/api      HTTP 处理（handler 只做参数与响应，逻辑放 hub/store/alert）
internal/server/webui    嵌入前端
web/                     React + Vite + Tailwind + shadcn/ui
deploy/                  systemd unit、安装脚本、示例配置
```

数据流：Agent POST → `api.handleReport` → `hub.Ingest`（只改内存）→ `rollup` 每分钟 `hub.Drain` → `store.SaveBatch`。

## 约定

- **实时数据只存内存**，禁止在请求处理路径中写数据库（上报接口尤其如此）。
- **数据库变更**：新增 `internal/server/store/schema/00N_xxx.sql`，不要修改已发布的迁移文件。
- **接口变更**：同步修改 `web/src/lib/types.ts`（与 `api/dto.go`、`store/*.go` 的 JSON 字段一致）。
- **前端文案**：所有用户可见文字放在 `web/src/lib/i18n.tsx`，中英文同时添加（TypeScript 会检查键是否齐全）。
- **前端组件**：优先使用 `src/components/ui`（shadcn），不要重复造组件；服务端数据用 TanStack Query。
- **视觉原则**：健康节点保持安静，只有需要关注的节点使用 ember 色（`.lit`、`bg-ember`）。不要给正常状态加醒目颜色。
- **Go**：错误用 `fmt.Errorf("...: %w", err)` 包装；日志用 `log/slog`，不要记录 Token、密码。
- **测试**：新逻辑至少一个测试。Go 用表驱动；`collect` 的解析函数必须有纯文本输入的测试。
- **注释**：只写代码本身表达不了的约束和原因，不写"这行做了什么"。

## 命令

```bash
make check        # 提交前必须通过：go vet、go test、前端 lint/typecheck/build
make dev-server   # 本地运行 server（数据在 ./data）
make dev-web      # 前端热更新 http://localhost:5173（代理到 127.0.0.1:8081）
make build        # 构建前端 + 两个二进制到 bin/
```

Windows 上没有 make 时：`go test ./...`、`go vet ./...`、`pnpm --dir web lint`、`pnpm --dir web build`。

## 完成标准

一个改动只有在以下全部满足时才算完成：`make check` 通过；文档（中英文）已更新；没有违反安全红线。
