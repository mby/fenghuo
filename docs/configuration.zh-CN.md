# 配置说明

[English](configuration.md)

所有命令行参数都可以用环境变量设置：`FENGHUO_` 加上参数名大写、短横线换成下划线。例如 `-admin-listen`
对应 `FENGHUO_ADMIN_LISTEN`。命令行参数优先于环境变量。

## 服务端

systemd 部署写在 `/etc/fenghuo/server.env`，Docker 部署写在容器环境变量中。

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-data` | `/var/lib/fenghuo` | 数据目录，存放 `fenghuo.db` |
| `-listen` | `127.0.0.1:8080` | 公开端口：Agent 上报和只读状态页 |
| `-admin-listen` | `127.0.0.1:8081` | 管理端口。`shared` 表示在 `-listen` 上同时提供管理后台，`off` 表示完全关闭管理后台 |
| `-tls-cert`、`-tls-key` | | 直接在公开端口上提供 TLS，不使用反向代理 |
| `-trusted-proxies` | `127.0.0.1/32,::1/128` | 信任其 `X-Forwarded-For`、`X-Forwarded-Proto` 头的代理网段 |
| `-keep-minute-days` | `7` | 1 分钟粒度数据和延迟数据的保留天数（1–90） |
| `-keep-hour-days` | `90` | 1 小时粒度指标和探测汇总的保留天数（1–730） |
| `-notify-allow-private` | `false` | 允许向内网、回环地址发送通知 |
| `-log-level` | `info` | `debug`、`info`、`warn` 或 `error` |

服务端还会读取以下环境变量：

- `TZ`：流量周期使用的时区，在重置日的本地零点重置。
- `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY`：发送通知时使用的代理。如果代理在内网地址上，还需要开启 `-notify-allow-private`。

`reset-password` 和 `disable-totp` 子命令使用同样的参数。如果你修改过 `FENGHUO_DATA`，需要显式传入
`-data`：`sudo -u fenghuo fenghuo-server reset-password -data /srv/fenghuo`。

### 管理后台中的设置

| 设置 | 作用 |
| --- | --- |
| 站点名称 | 显示在页面顶部和通知标题中 |
| 公开地址 | Agent 上报使用的 HTTPS 地址，用于生成安装命令 |
| 通知语言 | 告警消息使用中文或英文 |
| 公开状态页 | 关闭后匿名访客看不到任何内容，管理员仍可查看全部 |

### 访问管理后台

管理端口不应该暴露在公网，可以选择以下任一方式：

1. **SSH 隧道（默认，推荐）**：`ssh -L 8081:127.0.0.1:8081 you@server`，然后打开 `http://127.0.0.1:8081`。
2. **内网**：把 `-admin-listen` 绑定到 WireGuard 或 Tailscale 地址，例如 `100.64.0.1:8081`。
3. **反向代理加 IP 白名单**：参见 `Caddyfile.example`。
4. **`-admin-listen shared`**：管理后台和公开页共用端口。只有在开启两步验证后才建议这样做。

### 反向代理

服务端只在两处用到真实客户端 IP：登录限流和管理列表中显示的 IP。默认信任来自回环地址的转发头，
适用于代理与服务端在同一台机器上的情况。

Caddy：

```caddy
probe.example.com {
	encode zstd gzip
	reverse_proxy 127.0.0.1:8080
}
```

nginx：

```nginx
server {
    listen 443 ssl;
    http2 on;
    server_name probe.example.com;
    ssl_certificate     /etc/letsencrypt/live/probe.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/probe.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

如果前面还有 Cloudflare，需要把 [Cloudflare 的 IP 段](https://www.cloudflare.com/ips/)也加入
`-trusted-proxies`。Docker 中请求来自网桥网关，所以 compose 文件信任了 `172.16.0.0/12`。

### 存储与保留

| 数据 | 位置 | 保留时间 |
| --- | --- | --- |
| 最新上报、约 10 分钟的实时采样 | 内存 | 直到重启 |
| 1 分钟平均值和 TCP 延迟 | SQLite | `-keep-minute-days` |
| 1 小时平均值和 TCP 探测汇总 | SQLite | `-keep-hour-days` |
| 告警记录 | SQLite | 30 天，最多 1000 条 |
| 可用性区间 | SQLite | 91 天，相邻同状态区间合并 |
| 已完成的通知发送记录 | SQLite | 7 天，最多 5000 条 |
| 管理操作审计 | SQLite | 30 天，最多 10000 条 |
| 日志 | journald / 容器日志 | 由系统管理 |

指标和告警清理每小时执行一次，释放的空间会归还给文件系统。可用性、通知和审计后台任务也会清理各自记录。
如需限制 journald 占用，可在
`/etc/systemd/journald.conf` 中设置 `SystemMaxUse=50M`。

## Agent

配置写在 `/etc/fenghuo/agent.env`，安装脚本会以 `0600` 权限创建该文件。
可选 TCP 探测也可写入 `/etc/fenghuo/probes.env`；本地 systemd 覆盖文件会在
`agent.env` 后加载它，仅覆盖探测变量。

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-server` | | 服务端地址，例如 `https://probe.example.com` |
| `-token` | | 节点 Token。建议用 `FENGHUO_TOKEN`，因为命令行参数在进程列表中可见 |
| `-interval` | `3s` | 有人查看面板时的上报间隔，1s–60s |
| `-idle-interval` | `15s` | 无人查看时的上报间隔，最长 10m。`0` 表示始终使用 `-interval` |
| `-nic` | 自动 | 计入流量的网卡，逗号分隔 |
| `-ping` | | TCP 延迟测试目标，例如 `电信=ct.example.com:80,联通=1.2.3.4:443`（最多 16 个） |
| `-ping-interval` | `30s` | 延迟测试间隔（10s–10m） |
| `-allow-http` | `false` | 允许使用非回环地址的 `http://` 服务端 |
| `-print` | | 采集一次，以 JSON 打印后退出 |
| `-version` | | 打印版本后退出 |

修改配置文件后执行 `sudo systemctl restart fenghuo-agent` 生效。

### 可选三网 TCP 探测

在**管理后台 → 设置**中填写最多 16 个带名称的目标，每行 `名称=IPv4:端口`，
例如“上海电信”“北京联通”。名称公开显示，IP 只放在目标值中。
通过勾选指定首页最多展示三个，默认使用配置中的前三个；所有目标均可在详情查看历史。
清空目标列表即可不配置探测。保存的模板只用于生成命令；服务端不会向 Agent 下发目标。
新安装命令会带上已保存目标。已有 systemd Agent 可在设置页复制命令，
由你通过 SSH 在目标主机执行。`configure-probes.sh` 仅在本机写入
`/etc/fenghuo/probes.env` 和 systemd 覆盖文件，保留 `agent.env` 及其中的 Token；
重启检查失败时恢复原探测配置。多台机器可从自己的运维电脑通过 SSH 清单或 Ansible
批量执行同一命令。修改后台模板不会自动更新已安装 Agent。清空目标列表并保存后，
在目标机器运行生成的关闭命令即可停止探测。

卡片只在收到新鲜样本时显示 TCP 建连延迟和连接失败率；失败率不是 ICMP 丢包率。
目标地址只存于管理员可见的模板，公开节点接口仅返回目标名称和测量结果。
请使用自己管理或获准探测的稳定目标。

网络历史有独立的时间选择，默认最近 24 小时。近期使用分钟记录（`-keep-minute-days`，默认 7 天），
较早时段使用完整小时汇总（`-keep-hour-days`，默认 90 天）；1 小时、6 小时、24 小时、7 天、30 天、90 天
选项随保留配置调整。小时值是已有分钟值的平均，不按原始尝试次数加权；缺失保持未知。
小时记录从更新后开始累积，并可汇总分钟表中仍存在的最近三个完整小时。
可选择任意保留中的目标对比延迟与连接失败率。目标改名后会形成独立历史序列。

存储按每台机器、每个目标、每个已完成分钟一条记录聚合，不保存每次探测的原始记录。
记录数约为 `服务器数 × 目标数 × 1,440 × 保留天数`。50 台机器、6 个目标，
一天约 43.2 万条，七天约 302.4 万条。使用 12 字节目标名的 10 万行 SQLite 样本在整理后
约占 34 字节/行；容量规划可按 64–128 字节/行预留，七天探测表约 185–369 MiB，
90 天小时汇总再增加约 64.8 万条，同口径约 40–79 MiB；还需为其他表、WAL 和剩余空间留出余量。
这是估算，并非数据库文件大小硬上限，且假设目标名称稳定。
每小时清理过期记录并回收空闲页，因此保留范围可能暂时多出约一小时。
只需 24 小时时，可设 `-keep-minute-days=1`，它同时缩短探测与系统分钟指标保留期；
探测和系统小时历史由 `-keep-hour-days` 单独控制。提高同一分钟内的探测次数不会成倍增加分钟记录数。

### 自适应上报

后台的**新安装 Agent 的空闲上报间隔**可选 3、15、30、60 秒，默认 15 秒，仅修改生成的安装命令。
已有 Agent 需在本机 `/etc/fenghuo/agent.env` 修改 `FENGHUO_IDLE_INTERVAL=15s` 并重启
`fenghuo-agent`。空闲间隔不得短于正常间隔。上报响应不下发任何具体配置数值，仍只有 `202`/`204` 信号。

公开字段控制、网络告警、通知重试、可用率、资产、会话及审计保留策略见[监控与管理说明](monitoring.zh-CN.md)。

当有浏览器打开状态页或管理后台时，Agent 按 `-interval`（3 秒）上报；连续 15 秒无人查看后，降为
`-idle-interval`（15 秒），上报次数减少约 80%。打开页面后，最多一个空闲间隔内即切回 3 秒。CPU 使用率
和网速都是按计数器差值计算的，所以空闲时的一次上报就是这 15 秒内的平均值，流量统计不会遗漏。离线
判定跟随 Agent 当前的间隔：连续错过三次上报即判定离线。

### 流量统计

后台可以为每台服务器设置**流量统计方式**：双向、仅上行、仅下行或取较大的一方。请按服务商的计费规则
选择，不少 VPS 只统计出站流量。列表中会分别显示上行和下行流量，计入额度的方向会高亮显示。

默认统计除回环和虚拟网卡（`docker*`、`veth*`、`br-*`、`virbr*`、`vmbr*`、`tun*`、`tap*`、`wg*`、
`tailscale*` 等）以外的所有网卡，因为这些虚拟网卡上的流量已经计入物理网卡。如果与服务商的统计
不一致，可以显式指定网卡，例如 `FENGHUO_NIC=eth0`。

Agent 上报的是开机以来的累计值，由服务端计算差值。机器重启和服务端重启都不会丢失流量，但 Agent
停止期间产生的流量不会被统计。

### 没有 systemd 的系统

Agent 是单个静态二进制。在没有 systemd 的系统上（例如使用 OpenRC 的 Alpine、容器），可以用任意进程
管理工具以非特权用户运行：

```bash
FENGHUO_SERVER=https://probe.example.com FENGHUO_TOKEN=fh_... fenghuo-agent
```

它只需要读取 `/proc` 和 `/sys`，以及出站 HTTPS。请在宿主机上运行，不要放在容器里，因为容器只能看到
自己的网络和进程命名空间。

## 通知渠道

所有告警都会发送到每个已启用的渠道。保存后可以点击**发送测试**。

### Telegram

1. 通过 [@BotFather](https://t.me/BotFather) 创建机器人并复制 Token。
2. 给机器人发一条消息，然后打开 `https://api.telegram.org/bot<TOKEN>/getUpdates`，复制其中的 `chat.id`。
3. 如果服务器无法访问 Telegram，可以把 **API 地址**设为 `api.telegram.org` 的反向代理，或为服务端设置 `HTTPS_PROXY`。

### Webhook

URL（会做 URL 转义）和请求体模板（会做 JSON 转义）中都可以使用占位符：`{{title}}`、`{{text}}`、
`{{node}}`、`{{kind}}`、`{{status}}`（`firing` 或 `resolved`）、`{{time}}`（RFC 3339）。请求体留空时，
告警会以 JSON 发送：

```json
{"title":"[Fenghuo] 告警: 离线","text":"节点 tokyo 已离线 3 分 0 秒","node":"tokyo","kind":"offline","firing":true,"time":1767225600}
```

| 服务 | 方法 | URL | 请求体模板 |
| --- | --- | --- | --- |
| Bark | GET | `https://api.day.app/<key>/{{title}}/{{text}}` | |
| 企业微信机器人 | POST | `https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=<key>` | `{"msgtype":"text","text":{"content":"{{title}}\n{{text}}"}}` |
| 飞书机器人 | POST | `https://open.feishu.cn/open-apis/bot/v2/hook/<token>` | `{"msg_type":"text","content":{"text":"{{title}}\n{{text}}"}}` |
| 钉钉机器人 | POST | `https://oapi.dingtalk.com/robot/send?access_token=<token>` | `{"msgtype":"text","text":{"content":"{{title}}\n{{text}}"}}` |
| Discord | POST | `https://discord.com/api/webhooks/<id>/<token>` | `{"content":"**{{title}}**\n{{text}}"}` |
| ntfy | POST | `https://ntfy.sh/<topic>` | `{{title}}: {{text}}` |

默认情况下通知无法发往内网、回环、链路本地和 CGNAT 地址，以防止通过 Webhook 探测你的内网（SSRF）。
如果要使用局域网内自建的接收端，请以 `-notify-allow-private` 启动服务端。

## 告警规则

| 类型 | 触发条件 | 恢复条件 |
| --- | --- | --- |
| 离线 | 超过设定分钟数没有上报 | 收到下一次上报 |
| CPU / 内存 / 磁盘使用率 | 超过阈值并持续设定分钟数 | 回落到阈值及以下 |
| 流量额度 | 已用流量达到额度的设定百分比 | 下一个计费周期开始时静默恢复 |
| 到期提醒 | 距离到期日不超过设定天数 | 到期日修改后静默恢复 |

规则可以作用于全部服务器或选定的服务器。从未上报过的服务器不会触发离线告警。告警状态在重启后
仍然保留，一次性告警不会重复发送。
