# 二进制安装与升级

[English](installation.md)

烽火可以完全通过二进制安装。Server 已内嵌前端，目标机器无需 Go、Node.js、pnpm 或 Docker。
推荐使用 Linux + systemd；不使用 systemd 时也能以前台程序运行，由你自己的进程管理器负责守护。

## 选择安装包

从 [GitHub Releases](https://github.com/mby/fenghuo/releases) 下载已发布版本。
安装脚本默认使用最新稳定 Release。安装已发布的预发布版本（如 `v1.2.3-rc.1`）时，
必须在 `--version` 或下载地址中指定完整标签；预发布版本不会成为默认版本。
如果还没有已发布的 Release，可使用下文的 GitHub Actions 测试包和手动安装步骤。

| `uname -m` | 包名中的架构 |
| --- | --- |
| `x86_64` | `amd64` |
| `aarch64` | `arm64` |
| `armv7l` / `armv8l` | `armv7`（32 位 ARMv7） |

每个版本有六个包：`fenghuo-server_linux_<架构>.tar.gz` 和 `fenghuo-agent_linux_<架构>.tar.gz`，
以及 `checksums.txt`。包名不带版本，版本由下载地址决定。系统还需有 CA 根证书；安装脚本需要
Bash、curl 或 wget、tar、sha256sum、coreutils 和 systemd。

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
SCP/移动介质复制相同版本的包和 `checksums.txt`。在每台目标机进入文件所在目录，按目标架构
设置 `ARCH`（例如 `ARCH=amd64`），解包前再次运行 `sha256sum --check --ignore-missing checksums.txt`。
校验和能发现文件损坏；需要验证构建来源时，可在联网电脑、传输文件前分别运行
`gh attestation verify <安装包路径> --repo mby/fenghuo` 和
`gh attestation verify checksums.txt --repo mby/fenghuo`。
公开仓库的 Release 工作流会为安装包和 `checksums.txt` 生成来源证明，预发布版本也适用；
测试包 Artifact 没有这些证明。

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

升级顺序：**停服备份 Server → 升级 Server → 验证登录与节点 → 逐台备份并升级 Agent**。
数据库迁移会在启动时执行。以下命令适用于上文的默认 systemd 路径；自定义数据目录和配置需另行
纳入备份。备份可能包含 Token 和会话，必须仅供 root 读取，并另存一份到可信位置。请先在隔离机器
验证备份可以恢复。

### 停止并备份 Server

在 Server 主机停止服务，备份数据、配置、二进制、unit 和可选的 unit 覆盖文件。备份后服务保持
停止；如果备份失败，应先排查再继续。

```bash
sudo bash <<'EOF'
set -euo pipefail
umask 077
install -d -m 0700 /var/backups/fenghuo
backup_dir=$(mktemp -d /var/backups/fenghuo/server.XXXXXX)
systemctl stop fenghuo-server
paths=(var/lib/fenghuo etc/fenghuo usr/local/bin/fenghuo-server
       etc/systemd/system/fenghuo-server.service)
if [ -d /etc/systemd/system/fenghuo-server.service.d ]; then
  paths+=(etc/systemd/system/fenghuo-server.service.d)
fi
tar -czf "$backup_dir/backup.tar.gz" -C / "${paths[@]}"
tar -tzf "$backup_dir/backup.tar.gz" >/dev/null
printf 'Backup: %s/backup.tar.gz\n' "$backup_dir"
EOF
```

### 在线升级 Server

网络可用时，运行已审阅的安装脚本，将 `vX.Y.Z` 换成目标已发布标签。脚本校验下载文件，保留现有
配置和数据，替换二进制及 unit，然后启动服务。

```bash
sudo bash install-server.sh --version vX.Y.Z
```

### 使用已下载文件升级 Server

离线 Release 或 Actions 测试包均可使用此方法：将目标架构的安装包及其配套 `checksums.txt` 复制到
Server 主机的 `~/fenghuo-download`。完成上述停服备份后，按该主机架构修改下方 `ARCH`。命令先检查
清单中该安装包的校验和，再替换已停止的二进制和 unit；不会下载文件，也不修改 `server.env`。
如直接改过 unit 文件，请先与新包中的 unit 合并；示例会保留 systemd drop-in 覆盖文件。

```bash
bash <<'EOF'
set -euo pipefail
ARCH=amd64
cd "$HOME/fenghuo-download"
archive="fenghuo-server_linux_${ARCH}.tar.gz"
test -f "$archive"
awk -v file="$archive" '$2 == file {print}' checksums.txt | sha256sum --check -
if sudo systemctl is-active --quiet fenghuo-server; then
  echo 'Stop and back up fenghuo-server first.' >&2
  exit 1
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
tar -xzf "$archive" -C "$work" fenghuo-server fenghuo-server.service
sudo install -m 0755 -o root -g root "$work/fenghuo-server" /usr/local/bin/fenghuo-server
sudo install -m 0644 -o root -g root "$work/fenghuo-server.service" /etc/systemd/system/fenghuo-server.service
sudo systemctl daemon-reload
sudo systemctl start fenghuo-server
sudo systemctl is-active --quiet fenghuo-server
EOF
```

用 `sudo systemctl status fenghuo-server --no-pager` 检查服务，并验证管理登录和节点上报，再升级
Agent。若启动失败，保留日志并排查；升级不会自动回滚。

### 逐台备份并升级 Agent

在每台 Agent 主机停止服务，备份配置、二进制、unit 及可选的 drop-in；配置备份也包含本地探测覆盖
文件。备份后服务保持停止，直到执行升级。

```bash
sudo bash <<'EOF'
set -euo pipefail
umask 077
install -d -m 0700 /var/backups/fenghuo
backup_dir=$(mktemp -d /var/backups/fenghuo/agent.XXXXXX)
systemctl stop fenghuo-agent
paths=(etc/fenghuo usr/local/bin/fenghuo-agent
       etc/systemd/system/fenghuo-agent.service)
if [ -d /etc/systemd/system/fenghuo-agent.service.d ]; then
  paths+=(etc/systemd/system/fenghuo-agent.service.d)
fi
tar -czf "$backup_dir/backup.tar.gz" -C / "${paths[@]}"
tar -tzf "$backup_dir/backup.tar.gz" >/dev/null
printf 'Backup: %s/backup.tar.gz\n' "$backup_dir"
EOF
```

网络可用时运行已审阅脚本，`--update` 保留本地配置：

```bash
sudo bash install-agent.sh --update --version vX.Y.Z
```

离线 Release 或已下载测试包：将匹配的 Agent 安装包和 `checksums.txt` 复制到该 Agent 主机的
`~/fenghuo-download`。备份后按主机架构设置 `ARCH` 并运行以下命令。它先校验选定安装包，再替换已
停止的二进制和 unit；保留 `agent.env` 和探测覆盖文件。unit 文件本身如有自定义修改，须先合并。

```bash
bash <<'EOF'
set -euo pipefail
ARCH=amd64
cd "$HOME/fenghuo-download"
archive="fenghuo-agent_linux_${ARCH}.tar.gz"
test -f "$archive"
awk -v file="$archive" '$2 == file {print}' checksums.txt | sha256sum --check -
if sudo systemctl is-active --quiet fenghuo-agent; then
  echo 'Stop and back up fenghuo-agent first.' >&2
  exit 1
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
tar -xzf "$archive" -C "$work" fenghuo-agent fenghuo-agent.service
sudo install -m 0755 -o root -g root "$work/fenghuo-agent" /usr/local/bin/fenghuo-agent
sudo install -m 0644 -o root -g root "$work/fenghuo-agent.service" /etc/systemd/system/fenghuo-agent.service
sudo systemctl daemon-reload
sudo systemctl start fenghuo-agent
sudo systemctl is-active --quiet fenghuo-agent
EOF
```

用 `sudo systemctl status fenghuo-agent --no-pager` 检查，并确认节点恢复上报。

### Server 回滚

不要只降级二进制并继续使用已迁移的数据库。先停服并另行保存当前数据库、配置、二进制和 unit，
再从**同一份升级前备份**恢复数据库、配置、旧二进制和 unit，保留原所有权与权限；不要将旧数据库
与新文件混合。运行 `systemctl daemon-reload` 后启动服务，验证登录与节点。如需恢复 Agent，分别在
各主机使用对应备份。

## Docker 备份、升级与回滚

以下流程适用于 Linux 主机上的本地 rootful Docker，以及仓库提供的 `deploy/docker-compose.yml`。其中
`fenghuo` 服务把 `fenghuo-data` 命名卷挂载到 `/data`；Compose 通常会在实际卷名前加项目名。
请在该 Compose 文件所在目录运行，并使用已有部署相同的项目名和 Compose 参数。还需停止所有其他
会写入该卷的容器或进程。

Server 镜像是 distroless。停服前请在 Docker 主机准备独立的 `ubuntu:24.04` 工具镜像，例如联网时
执行 `docker pull ubuntu:24.04`，或通过 `docker save`/`docker load` 离线传输。下面使用禁用网络的
临时工具容器处理卷，不依赖烽火镜像内的 shell 或 tar。

### 停止并备份 Docker 服务

命令先识别现有容器、实际本地卷和不可变镜像 ID，再停止服务。它保存卷数据、展开后的 Compose
配置、镜像引用、镜像 ID、二进制版本和镜像本身。输出的是新的仅 root 可访问备份目录；请记录路径并
复制到可信位置。服务保持停止。如果工具镜像不存在、挂载不是命名卷或卷驱动不是 local，流程会停止；
其他存储驱动需使用其对应的备份方式。

```bash
sudo bash <<'EOF'
set -euo pipefail
umask 077
helper=ubuntu:24.04
docker image inspect "$helper" >/dev/null
container=$(docker compose ps --all --quiet fenghuo)
test -n "$container"
project=$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$container")
volume=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' "$container")
image_ref=$(docker inspect --format '{{.Config.Image}}' "$container")
image_id=$(docker inspect --format '{{.Image}}' "$container")
test -n "$project" && test -n "$volume" && test -n "$image_id"
test "$(docker volume inspect --format '{{.Driver}}' "$volume")" = local
docker compose stop fenghuo
install -d -m 0700 /var/backups/fenghuo
backup_dir=$(mktemp -d /var/backups/fenghuo/docker.XXXXXX)
printf '%s\n' "$project" > "$backup_dir/project.txt"
printf '%s\n' "$volume" > "$backup_dir/volume.txt"
printf '%s\n' "$image_ref" > "$backup_dir/image-ref.txt"
printf '%s\n' "$image_id" > "$backup_dir/image-id.txt"
docker compose config > "$backup_dir/compose.yaml"
docker image save -o "$backup_dir/image.tar" "$image_id"
docker run --rm --pull=never --network none "$image_id" version > "$backup_dir/version.txt"
docker run --rm --pull=never --network none --user 0:0 \
  --mount "type=volume,src=$volume,dst=/data,readonly" \
  --mount "type=bind,src=$backup_dir,dst=/backup" \
  "$helper" tar --numeric-owner -C /data -czf /backup/data.tar.gz .
docker run --rm --pull=never --network none \
  --mount "type=bind,src=$backup_dir,dst=/backup,readonly" \
  "$helper" tar -tzf /backup/data.tar.gz >/dev/null
(
  cd "$backup_dir"
  sha256sum data.tar.gz image.tar compose.yaml image-id.txt image-ref.txt \
    version.txt project.txt volume.txt > SHA256SUMS
  sha256sum -c SHA256SUMS
)
printf 'Backup: %s\n' "$backup_dir"
EOF
```

### 升级 Docker 服务

完成备份后，在 Compose 文件中选择目标已发布 `ghcr.io/mby/fenghuo` 镜像标签。联网时拉取并启动：

```bash
docker compose pull fenghuo
docker compose up -d --no-deps --pull never fenghuo
docker compose ps fenghuo
docker compose logs --tail 100 fenghuo
```

离线升级时，先传输并用 `docker load` 导入目标镜像，确认其标签与 Compose 文件一致；跳过
`docker compose pull`，执行其余命令。检查日志、登录和节点上报。发生数据库迁移时，升级前卷备份
必须与其对应的旧镜像一起保存。

### 回滚 Docker 服务

**恢复前先保存当前状态**：即使升级后的容器已停止，也应重复上述 Docker 备份流程，生成一个新目录，
保存当前卷、配置和镜像，并保持服务停止。请单独保留这份新备份；下方 `backup_dir` 必须填写更早的
**升级前备份**目录。

命令先校验该备份，再用临时工具容器将其恢复到新的命名卷，加载匹配的旧镜像，并结合覆盖配置启动
备份中的 Compose。原卷会保留，避免把旧备份与新数据库文件混合。

```bash
sudo bash <<'EOF'
set -euo pipefail
backup_dir=/var/backups/fenghuo/docker.REPLACE
helper=ubuntu:24.04
docker image inspect "$helper" >/dev/null
(
  cd "$backup_dir"
  sha256sum -c SHA256SUMS
)
project=$(cat "$backup_dir/project.txt")
old_image=$(cat "$backup_dir/image-id.txt")
[[ "$project" =~ ^[a-z0-9][a-z0-9_-]*$ ]]
[[ "$old_image" =~ ^sha256:[0-9a-f]{64}$ ]]
if [ -n "$(docker ps --quiet --filter "label=com.docker.compose.project=$project" \
    --filter "label=com.docker.compose.service=fenghuo")" ]; then
  echo 'Stop and back up the current Docker service first.' >&2
  exit 1
fi
restore_volume="fenghuo-restore-$(date +%s)-$$"
if docker volume inspect "$restore_volume" >/dev/null 2>&1; then
  echo 'Restore volume already exists.' >&2
  exit 1
fi
docker volume create "$restore_volume" >/dev/null
docker run --rm --pull=never --network none --user 0:0 \
  --mount "type=volume,src=$restore_volume,dst=/data" \
  --mount "type=bind,src=$backup_dir,dst=/backup,readonly" \
  "$helper" tar --numeric-owner --same-permissions -C /data -xzf /backup/data.tar.gz
docker image load -i "$backup_dir/image.tar"
rollback_tag="ghcr.io/mby/fenghuo:rollback-${old_image#sha256:}"
docker image tag "$old_image" "$rollback_tag"
cat > "$backup_dir/rollback.override.yaml" <<YAML
services:
  fenghuo:
    image: $rollback_tag
volumes:
  fenghuo-data:
    external: true
    name: $restore_volume
YAML
docker compose -p "$project" -f "$backup_dir/compose.yaml" \
  -f "$backup_dir/rollback.override.yaml" up -d --no-deps --pull never fenghuo
docker compose -p "$project" -f "$backup_dir/compose.yaml" \
  -f "$backup_dir/rollback.override.yaml" ps fenghuo
printf 'Restored volume: %s\nProject: %s\nCompose files: %s/compose.yaml and %s/rollback.override.yaml\n' \
  "$restore_volume" "$project" "$backup_dir" "$backup_dir"
EOF
```

验证登录和节点恢复后，继续使用命令中打印的 Compose 文件与项目名，或将常用 Compose 配置改为指向
恢复后的卷和旧镜像。直接使用未修改的新 Compose 配置可能切回较新的卷或镜像。确认恢复成功前，请
保留两份备份和两个卷。

## 卸载

```bash
sudo bash install-agent.sh --uninstall
sudo bash install-server.sh --uninstall
```

Agent 卸载会删除程序、unit、`agent.env` 和可选的本地探测覆盖文件；Server 卸载保留 `/var/lib/fenghuo` 和 `server.env`。
离线或完全手动安装也可用已审阅脚本卸载（不会下载），或自行停用对应 unit 并移除二进制/unit。
数据和备份请确认不再需要后再删除。

## GitHub Actions 生成安装包

以下工作流在公开仓库 `mby/fenghuo` 中运行：

1. **测试包**：推送到 `main` 自动触发 **Build binaries**。默认分支包含工作流后，也可打开
   Actions → **Build binaries** → **Run workflow** 手动选择分支。工作流执行检查、构建
   GoReleaser snapshot、验证六个包，并对 amd64 二进制运行冒烟测试。登录 GitHub，在该次运行的
   Artifacts 中于 14 天内下载 `fenghuo-linux-<完整提交 SHA>`；解开外层 ZIP 后得到六个 tar.gz 和
   `checksums.txt`。这是测试 Artifact，不是 Release；按上面的手动步骤校验和安装，跳过 Release
   下载命令。
2. **已发布版本**：推送匹配 `v*` 的标签会触发 **Release**。例如，确认 CI 通过后，为已提交代码
   执行 `git tag v1.2.3` 和 `git push origin v1.2.3`，版本号按实际情况选择。工作流执行
   `make check` 和 ShellCheck，构建并验证六个包、检查 amd64 二进制版本，并启动 Server 检查内嵌 UI
   和双监听器；随后为安装包和 `checksums.txt` 生成来源证明，再发布 Release。含连字符的标签会被
   标记为预发布，不会成为安装脚本默认使用的稳定版入口。
3. **容器镜像**：二进制任务成功后，Release 才构建并发布 Docker 镜像。安装二进制无需等待或使用该
   镜像；测试包工作流不发布镜像。

使用 GitHub CLI 下载测试 Artifact 时，先登录，将 `RUN_ID` 和 `COMMIT_SHA` 替换为该次运行的 ID
和完整提交 SHA：

```bash
gh run download RUN_ID --repo mby/fenghuo \
  --name fenghuo-linux-COMMIT_SHA --dir fenghuo-download
```

### 安装已下载的测试包

在可信电脑解开下载的 Actions Artifact ZIP，将所需安装包及其配套 `checksums.txt` 复制到目标机器。
在目标机进入文件所在目录，按架构设置 `ARCH` 并校验已复制的文件；这里不使用 Release 下载地址或
安装脚本。

```bash
cd /path/to/fenghuo-download
ARCH=amd64
sha256sum --check --ignore-missing checksums.txt
```

校验失败时立即停止。首次安装请继续上文对应的手动 Server 或 Agent 步骤；已有服务请先停服备份，
再使用上文离线升级步骤，该步骤会在替换前再次校验选定的安装包。测试构建不会创建 Release、
自动部署或更新 Agent。生产环境优先使用稳定 Release。

若仅镜像任务失败，选择 **Re-run failed jobs**。重跑全部任务时，必须先确认已发布 Release 的校验和
文件与本次构建一致，且下载的六个包全部通过验证，才会保留已有 Release。校验不一致或已有草稿时，
发布会停止，不会覆盖已有资产。
