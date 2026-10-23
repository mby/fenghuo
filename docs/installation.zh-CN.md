# 二进制安装与升级

[English](installation.md)

烽火可以完全通过二进制安装。Server 已内嵌前端，目标机器无需 Go、Node.js、pnpm 或 Docker。
推荐使用 Linux + systemd；不使用 systemd 时也能以前台程序运行，由你自己的进程管理器负责守护。

## 选择安装包

从 [GitHub Releases](https://github.com/mby/fenghuo/releases) 下载已发布版本。
脚本依赖已经发布的安装包；只有源码、尚未发布第一个版本时，请先按下文构建。

| `uname -m` | 包名中的架构 |
| --- | --- |
| `x86_64` | `amd64` |
| `aarch64` | `arm64` |
| `armv7l` / `armv8l` | `armv7`（32 位 ARMv7） |

每个版本有六个包：`fenghuo-server_linux_<架构>.tar.gz` 和 `fenghuo-agent_linux_<架构>.tar.gz`，
以及 `checksums.txt`。包名不带版本，版本由下载地址决定。系统还需有 CA 根证书；安装脚本需要
Bash、curl 或 wget、tar、sha256sum、coreutils 和 systemd。

## 私有仓库如何部署

目前安装脚本通过匿名 HTTPS 下载脚本和 Release 包，不读取 GitHub 登录状态或访问令牌。
私有仓库不能直接使用文中的公开 `curl` 一键命令；只先下载脚本也不够，脚本仍会匿名下载二进制。
后台生成的安装及探测配置命令同样使用公开地址，私有仓库时需要先自行取得相应文件。

测试部署推荐：将代码推送到 `main` 后，**Build binaries** 会自动构建，私有仓库也适用。
也可登录 GitHub，在 Actions → **Build binaries** → **Run workflow** 手动选择分支构建。
完成后下载该次运行的 `fenghuo-linux-<完整提交 SHA>` Artifact，解开外层 ZIP，
选择目标架构的 Server/Agent `.tar.gz` 及 `checksums.txt`，通过 SCP/SFTP 上传到 Linux 服务器。
然后按“方法二”校验、解包、安装 systemd 服务。服务器无需保存 GitHub 凭据，也无需源码或编译环境。

已安装 GitHub CLI 时，也可在有仓库读取权限的电脑上下载；将 `RUN_ID` 和 `COMMIT_SHA` 换成实际值：

```bash
gh auth login
gh run download RUN_ID --repo mby/fenghuo \
  --name fenghuo-linux-COMMIT_SHA --dir fenghuo-download
```

如果已经发布私有 Release，可用登录后的 CLI 下载选定架构的包：

```bash
gh release download vX.Y.Z --repo mby/fenghuo \
  --pattern 'fenghuo-*_linux_amd64.tar.gz' --pattern checksums.txt --dir fenghuo-download
```

`amd64` 按实际架构替换。也可以在已登录的浏览器中从 Releases 下载；取得文件后使用相同的手动安装步骤。
不要把 GitHub 令牌填进 Agent 的 `FENGHUO_TOKEN`，它只接受本系统的节点 Token。
本地探测配置脚本可从源码下载并上传后，在目标机手动执行 `sudo bash configure-probes.sh ...`。

正式发布工作流在私有仓库默认跳过 GitHub 来源证明，保留构建检查、包校验和及发布步骤。
如使用支持私有来源证明的 GitHub Enterprise Cloud，可在仓库 Actions 变量中设置
`ENABLE_PRIVATE_ATTESTATIONS=true`。公开仓库仍默认生成证明。
参见 [GitHub 来源证明支持范围](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations)、
[Artifact 下载权限](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/download-workflow-artifacts)。

## 方法一：安装脚本（推荐）

先下载并查看脚本，再执行。默认安装最新稳定 Release；以下命令在目标 Linux 机器上运行。

```bash
curl -fL --proto '=https' --tlsv1.2 \
  https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-server.sh \
  -o install-server.sh
less install-server.sh
sudo bash install-server.sh
```

脚本校验下载包的 SHA-256，创建无登录权限的 `fenghuo` 用户，安装并启动 systemd 服务。
已有 `/etc/fenghuo/server.env` 会保留。配置修改后运行 `sudo systemctl restart fenghuo-server`。

固定版本时，将示例 `vX.Y.Z` 换成 Releases 中的真实标签；同时固定脚本与二进制版本：

```bash
VERSION=vX.Y.Z
curl -fL --proto '=https' --tlsv1.2 \
  "https://raw.githubusercontent.com/mby/fenghuo/$VERSION/deploy/install-server.sh" \
  -o install-server.sh
less install-server.sh
sudo bash install-server.sh --version "$VERSION"
```

安装 Agent 前，在管理后台添加节点，取得只显示一次的 Token。下载并查看
`https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-agent.sh`，保存为 `install-agent.sh`。
使用隐藏输入，避免把 Token 明文写入 shell 历史或命令参数：

```bash
sudo -v
read -r -s -p 'Agent token: ' FENGHUO_TOKEN; printf '\n'
export FENGHUO_TOKEN
sudo --preserve-env=FENGHUO_TOKEN bash install-agent.sh \
  --server https://probe.example.com --idle-interval 15s
unset FENGHUO_TOKEN
```

若 sudo 策略不允许保留该变量，请使用下面的手动安装方式编辑 `agent.env`。
安装器将 Token 写入仅 root 可读的 `/etc/fenghuo/agent.env`，Agent 以 systemd 临时用户运行。
无人查看时默认 15 秒；后台的 3/15/30/60 秒预设只影响生成的安装命令。
已有 Agent 修改本地 `FENGHUO_IDLE_INTERVAL=15s` 后重启，不接受服务端远程配置或自动升级。

## 方法二：完全手动安装 / 离线安装

以下是**首次安装**步骤。升级已有服务请使用后面的升级流程，避免直接覆盖运行中的二进制。
将 `VERSION` 替换成已发布标签，`ARCH` 按上表选择。在独立目录内下载并校验：

```bash
VERSION=vX.Y.Z
ARCH=amd64
mkdir -p fenghuo-download
cd fenghuo-download
BASE="https://github.com/mby/fenghuo/releases/download/$VERSION"
curl -fL --proto '=https' --tlsv1.2 "$BASE/checksums.txt" -o checksums.txt
curl -fL --proto '=https' --tlsv1.2 "$BASE/fenghuo-server_linux_$ARCH.tar.gz" -o "fenghuo-server_linux_$ARCH.tar.gz"
curl -fL --proto '=https' --tlsv1.2 "$BASE/fenghuo-agent_linux_$ARCH.tar.gz" -o "fenghuo-agent_linux_$ARCH.tar.gz"
sha256sum --check --ignore-missing checksums.txt
```

只需某一个程序时，可省略另一包的下载。校验失败就停止，不要安装。离线机器可从可信电脑经
SCP/移动介质复制相同版本的包和 `checksums.txt`，在目标机再次校验，后续操作相同。
校验和能发现文件损坏；需要验证构建来源时，可在联网电脑使用
`gh attestation verify <安装包路径> --repo mby/fenghuo`。来源证明仅适用于正式发布包。

### Server + systemd

继续在下载目录运行：

```bash
mkdir -p server
tar -xzf "fenghuo-server_linux_$ARCH.tar.gz" -C server
cd server
id fenghuo >/dev/null 2>&1 || sudo useradd --system --home-dir /var/lib/fenghuo \
  --no-create-home --shell /usr/sbin/nologin fenghuo
sudo install -m 0755 -o root -g root fenghuo-server /usr/local/bin/fenghuo-server
sudo install -m 0644 -o root -g root fenghuo-server.service /etc/systemd/system/fenghuo-server.service
sudo install -d -m 0755 -o root -g root /etc/fenghuo
sudo test -e /etc/fenghuo/server.env || sudo install -m 0600 -o root -g root server.env.example /etc/fenghuo/server.env
sudo systemctl daemon-reload
sudo systemctl enable --now fenghuo-server
sudo systemctl status fenghuo-server --no-pager
```

`StateDirectory` 自动创建并授权 `/var/lib/fenghuo`。公开端口默认 `127.0.0.1:8080`，管理端口
默认 `127.0.0.1:8081`。使用 Caddy/nginx 为公开端口提供 HTTPS；示例 `Caddyfile.example`
已随包附带。不要把管理端口直接开放到公网。

域名解析到服务器，开放反向代理所需的 80/443 端口。Caddy 配置示例：

```caddy
probe.example.com {
    encode zstd gzip
    reverse_proxy 127.0.0.1:8080
}
```

在自己的电脑建立隧道：`ssh -N -L 8081:127.0.0.1:8081 you@your-server`，访问
`http://127.0.0.1:8081/login`。用户名 `admin`；在服务器用
`sudo cat /var/lib/fenghuo/initial_admin_password` 查看初始密码。登录后修改密码、设置公开地址
`https://probe.example.com`，并开启两步验证。

### Agent + systemd

回到下载目录，在待监控机器解包 Agent：

```bash
mkdir -p agent
tar -xzf "fenghuo-agent_linux_$ARCH.tar.gz" -C agent
cd agent
sudo install -m 0755 -o root -g root fenghuo-agent /usr/local/bin/fenghuo-agent
sudo install -m 0644 -o root -g root fenghuo-agent.service /etc/systemd/system/fenghuo-agent.service
sudo install -d -m 0700 -o root -g root /etc/fenghuo
sudo test -e /etc/fenghuo/agent.env || sudo install -m 0600 -o root -g root /dev/null /etc/fenghuo/agent.env
sudoedit /etc/fenghuo/agent.env
```

在编辑器中填写真实地址和 Token（不要保留占位符）：

```ini
FENGHUO_SERVER=https://probe.example.com
FENGHUO_TOKEN=fh_你的真实Token
FENGHUO_INTERVAL=3s
FENGHUO_IDLE_INTERVAL=15s
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now fenghuo-agent
sudo systemctl status fenghuo-agent --no-pager
```

Agent 只需向服务端发起出站 HTTPS 请求，不需要开放任何入站端口。

### 没有 systemd

解包后，普通用户可直接运行 `./fenghuo-server serve -data ./data`。数据和初始密码位于该目录，
两个监听器仍默认只绑定回环地址。前台退出即停止服务；生产使用你自己的进程管理器并设置受限用户。

Agent 的环境变量也可由进程管理器提供。临时运行时，在仅当前用户可读、由你自己编写的本地
`agent.env` 中填入上述内容，再执行：

```bash
chmod 600 agent.env
set -a
. ./agent.env
set +a
exec ./fenghuo-agent
```

这里的 `agent.env` 会由 shell 执行，只加载自己编写的文件。不要以 root 运行 Agent。

## 检查与故障排查

```bash
fenghuo-server version
fenghuo-agent -version
curl --fail http://127.0.0.1:8080/api/v1/meta
sudo journalctl -u fenghuo-server -n 100 --no-pager
sudo journalctl -u fenghuo-agent -n 100 --no-pager
```

状态页应有 UI，添加的节点应在上报后出现。`Exec format error` 通常表示架构选错；TLS 错误请检查
域名、证书、CA 根证书与系统时间；Agent 401 请检查 Token 是否有效。端口占用或无法启动请先看日志。
自定义数据目录时，备份和恢复命令也必须使用你的实际路径。

## 备份、升级与回滚

升级顺序：**停服备份 → Server → 验证登录与节点 → Agent**。数据库迁移会在启动时自动执行。
以下备份命令适用于上述默认 systemd 安装，包含数据库、配置、旧 Server 和 unit；备份可能包含
Token、会话等敏感信息，只允许 root 读取，并另存一份到可信位置。

```bash
sudo bash <<'EOF'
set -euo pipefail
umask 077
install -d -m 0700 /var/backups/fenghuo
backup_dir=$(mktemp -d /var/backups/fenghuo/snapshot.XXXXXX)
systemctl stop fenghuo-server
trap 'systemctl start fenghuo-server' EXIT
tar -czf "$backup_dir/backup.tar.gz" -C / var/lib/fenghuo etc/fenghuo \
  usr/local/bin/fenghuo-server etc/systemd/system/fenghuo-server.service
printf 'Backup: %s/backup.tar.gz\n' "$backup_dir"
EOF
```

然后运行已查看过的安装脚本，`vX.Y.Z` 替换成目标发布版本：

```bash
sudo bash install-server.sh --version vX.Y.Z
sudo bash install-agent.sh --update --version vX.Y.Z
```

Server 保留配置和数据，Agent `--update` 保留本地配置。脚本通过同目录暂存与原子重命名替换二进制，
之后重启服务。升级并非自动回滚：服务启动失败时先保留日志、停止服务，再排查。

回滚时不要只降级二进制并继续使用已迁移数据库。停止 Server，先保留当前数据，再将**同一份升级前
备份中的数据库、配置、旧二进制和 unit**恢复到对应路径，保留原所有权与权限；运行
`systemctl daemon-reload` 后启动并验证。先在独立目录/测试机器验证备份可解包、数据库可用。

## 卸载

```bash
sudo bash install-agent.sh --uninstall
sudo bash install-server.sh --uninstall
```

Agent 卸载会删除程序、unit、`agent.env` 和可选的本地探测覆盖文件；Server 卸载保留 `/var/lib/fenghuo` 和 `server.env`。
离线或完全手动安装也可用已审阅脚本卸载（不会下载），或自行停用对应 unit 并移除二进制/unit。
数据和备份请确认不再需要后再删除。

## GitHub Actions 生成安装包

将改动提交并推送到 GitHub 后：

1. **测试包**：推送到 `main` 自动触发 **Build binaries**。默认分支包含工作流后，也可打开
   Actions → **Build binaries** → **Run workflow** 手动选择分支。
   成功后在该次运行的 Artifacts 下载 `fenghuo-linux-<完整提交 SHA>`，保留 14 天，下载需要登录 GitHub。
   解开外层 ZIP 后得到六个 tar.gz 和校验和；二进制版本含 `SNAPSHOT`，按上面的手动方法安装测试。
2. **正式包**：确认 CI 通过，为已提交代码创建并推送真实版本标签，例如
   `git tag v1.2.3`，然后 `git push origin v1.2.3`（示例版本号需按项目实际选择）。
   **Release** 工作流完整检查、构建并验证六个包，启动 amd64 二进制检查内嵌 UI 和双监听器，
   在公开仓库或已启用私有来源证明时，生成证明后才创建 Release。含连字符的标签标记为预发布，不作为稳定版安装入口。
3. 二进制发布成功后才构建原有 Docker 镜像；安装二进制无需等待或使用镜像。测试包工作流不发布镜像。

测试包不创建 Release、不自动部署、不更新任何 Agent。正式生产安装优先使用稳定 Release。
若仅镜像任务失败，优先选择 **Re-run failed jobs**。重跑全部任务时，已发布二进制必须与本次校验和一致，
流程才复用已有 Release；不一致或已有草稿时停止，避免覆盖已经发布的版本。
