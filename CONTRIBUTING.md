# Contributing / 贡献指南

[English](#english) | [中文](#中文)

## English

### Ground rules

Fenghuo stays small on purpose. Before writing a feature, open an issue to
discuss it. Anything that lets the server control agents (commands, terminals,
file access, remote configuration, auto-update) will not be accepted. Read
[AGENTS.md](AGENTS.md) for the architecture, conventions and security rules;
they apply to human contributors too.

### Development setup

Requirements: Go 1.27.3+, Node.js 24+, pnpm 11, Python 3.11+ and Bash. Python checks use only the standard library.

```bash
pnpm --dir web install
make dev-server   # server on 127.0.0.1:8080 (public) and 127.0.0.1:8081 (admin), data in ./data
make dev-web      # UI with hot reload on http://localhost:5173
```

The first start writes the admin password to `./data/initial_admin_password`.

The agent only runs on Linux. On other systems run its tests, or cross-compile
with `GOOS=linux go build ./cmd/fenghuo-agent`. `fenghuo-agent -print` collects
once and prints the report, which is handy for checking a new machine.

### Before opening a pull request

```bash
make check
```

This runs `go vet`, `go test`, the web linter, the type checker and a production
build, installer regression tests and release archive checks. CI additionally runs `staticcheck`, `govulncheck` and ShellCheck.
Hosted CI/build/release jobs use Ubuntu 24.04 explicitly; pnpm setup and GoReleaser Action v7 run on Node.js 24.
The minimum Go patch version includes the standard library security fixes required by `govulncheck`.
After workflow fixes, verify both CI and Build binaries on the pushed commit; local Windows checks do not replace these Linux gates.

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):
`feat(agent): ...`, `fix(server): ...`, `docs: ...`.

### Releasing (maintainers)

1. Make sure `main` is green.
2. Tag and push: `git tag v1.2.3 && git push origin v1.2.3`.
3. The release workflow runs `make check`, builds six Linux archives (Server/Agent for
   amd64, arm64 and armv7), verifies checksums and archive contents, and smoke tests
   the amd64 binaries, embedded frontend and separate listeners. Public repositories and
   opted-in private repositories generate provenance before publishing the Release.
   The multi-arch image job runs after binary publication.

Pushes to `main` automatically run **Build binaries** without publishing. To select a
branch manually, use Actions → **Build binaries** → **Run workflow**.
Download `fenghuo-linux-<full commit SHA>` from the run's Artifacts (14-day retention).
These builds have a `SNAPSHOT` version and include installation guides. They do not deploy
or update agents. See [binary installation](docs/installation.md) for the end-to-end process.
If only the image job fails, prefer **Re-run failed jobs**. Full reruns only reuse an existing
release when its archives match the new build; drafts and mismatches require maintainer review.

Upgrade the server before agents: the server rejects report fields it does not know.

### Repository settings (maintainers)

- Settings → Code security: enable **Private vulnerability reporting**,
  **Dependabot alerts** and **secret scanning**.
- Settings → Branches: protect `main`, require the CI checks to pass.
- After the first release, set the `fenghuo` package on GHCR to **public**.

## 中文

### 基本原则

烽火刻意保持精简。实现新功能前请先开 Issue 讨论。任何让服务端控制 Agent 的功能（命令、终端、
文件访问、远程配置、自动更新）都不会被接受。请阅读 [AGENTS.md](AGENTS.md) 了解架构、约定和安全
红线，这些同样适用于人类贡献者。

### 开发环境

需要 Go 1.27.3+、Node.js 24+、pnpm 11、Python 3.11+ 和 Bash。Python 检查仅使用标准库。

```bash
pnpm --dir web install
make dev-server   # server 监听 127.0.0.1:8080（公开）和 127.0.0.1:8081（管理），数据在 ./data
make dev-web      # 前端热更新 http://localhost:5173
```

首次启动会把管理员密码写入 `./data/initial_admin_password`。

Agent 只能在 Linux 上运行。其他系统可以运行它的测试，或用 `GOOS=linux go build ./cmd/fenghuo-agent`
交叉编译。`fenghuo-agent -print` 会采集一次并打印上报内容，适合在新机器上检查。

### 提交 PR 之前

```bash
make check
```

会依次执行 `go vet`、`go test`、前端 lint、类型检查、生产构建、安装脚本回归测试和发布包检查。CI 还会额外运行 `staticcheck`、
`govulncheck` 和 ShellCheck。
CI、构建及发布任务固定使用 Ubuntu 24.04；pnpm 安装 Action 和 GoReleaser Action v7 使用 Node.js 24。
Go 的最低补丁版本包含 `govulncheck` 要求的标准库安全修复。
修复工作流后应核对推送提交的 CI 与 Build binaries 都成功，本地 Windows 检查不能替代这些 Linux 检查。

提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/)：
`feat(agent): ...`、`fix(server): ...`、`docs: ...`。

### 发布流程（维护者）

1. 确认 `main` 分支 CI 全部通过。
2. 打 tag 并推送：`git tag v1.2.3 && git push origin v1.2.3`。
3. Release 工作流运行 `make check`，构建六个 Linux 安装包（Server/Agent，各有 amd64、arm64、armv7），
   检查校验和及包内文件，启动 amd64 二进制检查内嵌前端和双监听器；公开仓库或已启用私有来源证明时，生成证明后才发布 Release。
   原有多架构镜像任务在二进制发布成功后运行。

推送 `main` 自动触发 **Build binaries** 生成测试包，也可用 Actions → **Build binaries** → **Run workflow** 手动选择分支。在该次运行的 Artifacts
下载 `fenghuo-linux-<完整提交 SHA>`（保留 14 天）。这些包的版本带 `SNAPSHOT`，附带安装教程，
不会部署服务或更新 Agent。完整步骤见[二进制安装教程](docs/installation.zh-CN.md)。
仅镜像失败时优先选择 **Re-run failed jobs**。重跑全部任务时，已有 Release 只有在安装包与本次构建
一致时才会复用；草稿或内容不一致需要维护者处理。

升级时先升级 Server 再升级 Agent：Server 会拒绝它不认识的上报字段。

### 仓库设置（维护者）

- Settings → Code security：开启 **Private vulnerability reporting**、**Dependabot alerts**、**Secret scanning**。
- Settings → Branches：保护 `main`，要求 CI 检查通过才能合并。
- 首次发布后，把 GHCR 上的 `fenghuo` 镜像设为 **public**。
