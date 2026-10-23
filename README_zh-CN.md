# 烽火 Fenghuo

[English](README.md)

一个极简、安全优先的服务器探针，适合管理少量 VPS 的个人用户。

"烽火"取自长城烽火台：烽火只能向外传递信号，不接受回传的指令。烽火的 Agent 也是如此：
**它只上报数据，服务端没有任何办法让它执行操作**。即使面板被攻破，也无法借此控制你的服务器。

## 为什么再做一个探针

常见探针允许面板向所有 Agent 下发命令、打开终端或推送更新，而 Agent 通常以 root 运行。
面板一旦失守，所有被监控的机器也随之失守。烽火的做法不是把这些能力保护得更严，而是直接去掉：

- **只发不收的 Agent**：通过 HTTPS POST 上报，忽略响应体，唯一读取的是状态码所表示的"是否有人在看面板"，
  用来在本地配置的两个上报间隔之间切换。代码里根本没有命令、终端、文件或更新相关的逻辑，一旦有人加入，
  测试会直接失败。
- **Agent 不监听任何端口**：没有可扫描、可利用的入口。
- **Agent 无特权运行**：以 systemd 临时用户运行，没有任何 capability，文件系统只读，并有系统调用过滤。
- **Agent 零第三方依赖**：只使用 Go 标准库。
- **管理后台默认不暴露在公网**：只监听 `127.0.0.1`，通过 SSH 隧道访问。公网端口只提供只读状态页和上报接口。
- **资源占用小**：单文件二进制 + SQLite，不需要 Redis 或时序数据库。原始数据只在内存，SQLite 保存
  聚合指标、可用性区间、配置和有容量限制的通知及审计记录。

## 功能

- 搜索与排序节点，最多四节点实时/历史对比，可分享对比链接
- TCP 连接失败率图表、持续高延迟与失败率告警
- 通知持久化重试、每次发送结果记录
- 公开字段控制、私有资产资料、按币种汇总费用
- 7/30/90 天采样可用率、未知时段与故障时间线
- 活跃会话撤销与有容量限制的管理操作审计

- 有人查看时每 3 秒刷新的实时状态：CPU、内存、交换、磁盘、网速、负载、TCP/UDP 连接数、进程数、运行时间
- 卡片和列表两种视图，一键筛选需要关注的服务器，适配手机
- 历史图表：实时（最近约 10 分钟）、1 小时到 90 天
- 按月统计流量，分别显示上行和下行，可设置重置日、额度，每台服务器可选双向或单向统计
- 记录 VPS 到期时间
- 告警：离线、CPU/内存/磁盘超过阈值持续 N 分钟、流量额度、即将到期
- 通知：Telegram 和通用 Webhook（Bark、企业微信、飞书、钉钉、Discord、ntfy 等）
- 可选的 TCP 延迟测试，目标在每台 Agent 本地配置（例如三网延迟）
- 带分组的公开状态页，可关闭，始终不显示 IP
- 管理员登录：argon2id 密码哈希、可选 TOTP 两步验证、登录限流
- 中英文界面，浅色和深色主题

具体行为与限制见[监控与管理说明](docs/monitoring.zh-CN.md)。

刻意不做：Web 终端、远程命令、文件管理、计划任务、自动更新、插件、多用户权限。

## 快速开始

推荐直接安装 **二进制 + systemd**，无需 Docker、Go 或 Node.js，前端已内嵌。
完整的[二进制安装教程](docs/installation.zh-CN.md)包含手动/离线安装、TLS、备份、升级与回滚。
安装脚本需要已有 GitHub Release；首次发布或下载测试包见教程中的 GitHub Actions 步骤。

需要一台装有 systemd 的 Linux 服务器运行面板，以及一个指向它的域名。Agent 可运行在任何装有
systemd 的 Linux 上（amd64、arm64、armv7）。

### 1. 安装服务端

```bash
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-server.sh | sudo bash
```

脚本会创建 `fenghuo` 系统用户，安装 `/usr/local/bin/fenghuo-server` 和加固过的 systemd 服务，
并生成 `/etc/fenghuo/server.env`。所有下载都会用发布版本中的 `checksums.txt` 校验。

在公开端口（`127.0.0.1:8080`）前面放一个 TLS 反向代理。以 [Caddy](https://caddyserver.com/) 为例，
`/etc/caddy/Caddyfile`：

```caddy
probe.example.com {
	encode zstd gzip
	reverse_proxy 127.0.0.1:8080
}
```

<details>
<summary>使用 Docker</summary>

```bash
curl -fsSLO https://raw.githubusercontent.com/mby/fenghuo/main/deploy/docker-compose.yml
docker compose up -d
docker compose exec fenghuo cat /data/initial_admin_password
```

两个端口都只发布在 `127.0.0.1` 上，把反向代理指向 `127.0.0.1:8080` 即可。
</details>

### 2. 登录

在你自己的电脑上，建立到管理端口的 SSH 隧道：

```bash
ssh -L 8081:127.0.0.1:8081 you@your-server
```

打开 <http://127.0.0.1:8081/login>，用户名 `admin`，密码通过以下命令查看：

```bash
sudo cat /var/lib/fenghuo/initial_admin_password
```

然后进入**设置**：修改密码，把**公开地址**设为 `https://probe.example.com`，建议同时开启两步验证。

### 3. 添加服务器

在**管理 → 服务器 → 添加服务器**中填写名称（可选填写流量额度和到期日期）。对话框会显示一次性
Token 和安装命令：

```bash
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-agent.sh \
  | sudo bash -s -- --server https://probe.example.com --token fh_...
```

在要监控的机器上执行，几秒后它就会出现在状态页上。

可以附加 Agent 选项，例如 TCP 延迟测试：

```bash
... | sudo bash -s -- --server https://probe.example.com --token fh_... \
  --ping "电信=ct.example.com:80,联通=cu.example.com:80,移动=cm.example.com:80"
```

国内机器下载 GitHub 较慢时，可以加 `--mirror https://你的GitHub加速地址/`。

## 升级

先按[备份与回滚说明](docs/installation.zh-CN.md#备份升级与回滚)停服备份，再升级服务端，验证正常后升级 Agent。

```bash
# 服务端
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-server.sh | sudo bash
# Agent（保留现有配置）
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-agent.sh | sudo bash -s -- --update
# Docker
docker compose pull && docker compose up -d
```

Agent 不会自动更新，因为自动更新本身就是远程代码执行通道。

## 卸载

```bash
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-agent.sh | sudo bash -s -- --uninstall
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-server.sh | sudo bash -s -- --uninstall
```

服务端数据保存在 `/var/lib/fenghuo`，需要手动删除。

## 日常运维

| 任务 | 命令 |
| --- | --- |
| 服务端日志 | `journalctl -u fenghuo-server -f` |
| Agent 日志 | `journalctl -u fenghuo-agent -f` |
| 查看 Agent 采集到的内容 | `fenghuo-agent -print` |
| 重置管理员密码 | `sudo -u fenghuo fenghuo-server reset-password` |
| 关闭两步验证 | `sudo -u fenghuo fenghuo-server disable-totp` |
| 备份 | [停服备份数据库、配置和旧二进制](docs/installation.zh-CN.md#备份升级与回滚) |

Docker 部署时在命令前加 `docker compose exec fenghuo`。

## UI 演示数据

本地调整界面时，运行 `make dev-web`（或 `pnpm --dir web dev`），打开
[演示预览](http://localhost:5173/?demo=1)。预览包含 9 个虚构的公开节点，覆盖
正常、高 CPU、内存/磁盘压力、流量配额、即将到期、离线、待上报、长名称和公开信息受限等状态。
详情和对比图表还包含指标断点、延迟及可用性历史。演示状态在当前浏览器标签页中保持；
点击“退出演示”或访问 `http://localhost:5173/?demo=0` 即可返回本地服务数据。
该入口仅在 Vite 开发模式可用，不修改数据库。状态提醒由指标生成，样本公开备注不会重复提醒内容。

## 文档

- [二进制安装与升级](docs/installation.zh-CN.md)：脚本/手动/离线安装、systemd、备份回滚、Actions 构建
- [配置说明](docs/configuration.zh-CN.md)：服务端与 Agent 全部选项、反向代理、通知渠道、数据保留
- [安全模型](docs/security.zh-CN.md)：威胁模型、加固措施、如何校验发布文件
- [贡献指南](CONTRIBUTING.md)：开发环境与发布流程
- [安全策略](SECURITY.md)：如何报告漏洞

## 资源占用

两个程序都是单个静态 Go 可执行文件。Agent 每次只读取 `/proc` 下的几个文件，不在磁盘保存任何状态。
服务端把实时数据放在内存，每分钟批量保存聚合指标。可用性、通知和审计由各自的后台任务保存有容量限制的记录。
Agent 默认有人查看时每 3 秒上报，无人查看时每 15 秒上报。实际流量与存储占用取决于探测目标、历史数据和使用情况。
两个间隔都在本地配置，后台的空闲间隔预设只影响生成的安装命令。

## 许可证

[MIT](LICENSE)
