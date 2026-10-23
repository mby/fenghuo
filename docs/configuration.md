# Configuration

[中文](configuration.zh-CN.md)

Every flag can also be set as an environment variable named `FENGHUO_` plus the
flag name in upper case, with dashes replaced by underscores. For example,
`-admin-listen` becomes `FENGHUO_ADMIN_LISTEN`. A flag given on the command line
wins over the environment.

## Server

Set these in `/etc/fenghuo/server.env` (systemd) or as container environment variables.

| Flag | Default | Description |
| --- | --- | --- |
| `-data` | `/var/lib/fenghuo` | Data directory holding `fenghuo.db` |
| `-listen` | `127.0.0.1:8080` | Public listener: agent reports and the read-only status page |
| `-admin-listen` | `127.0.0.1:8081` | Admin listener. `shared` serves admin on `-listen`; `off` disables admin entirely |
| `-tls-cert`, `-tls-key` | | Serve TLS on the public listener directly instead of behind a reverse proxy |
| `-trusted-proxies` | `127.0.0.1/32,::1/128` | CIDRs whose `X-Forwarded-For` and `X-Forwarded-Proto` headers are trusted |
| `-keep-minute-days` | `7` | Days of 1-minute metrics and latency data (1–90) |
| `-keep-hour-days` | `90` | Days of 1-hour metrics and probe summaries (1–730) |
| `-notify-allow-private` | `false` | Allow notifications to private and loopback addresses |
| `-log-level` | `info` | `debug`, `info`, `warn` or `error` |

Other environment variables the server honours:

- `TZ`: time zone for traffic periods, which reset at local midnight on the reset day.
- `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY`: proxy for outgoing notifications. A
  proxy on a private address also needs `-notify-allow-private`.

The `reset-password` and `disable-totp` commands read the same flags. If you
changed `FENGHUO_DATA`, pass `-data` explicitly:
`sudo -u fenghuo fenghuo-server reset-password -data /srv/fenghuo`.

### Settings in the admin UI

| Setting | Purpose |
| --- | --- |
| Site name | Shown in the header and notification titles |
| Public URL | The HTTPS address agents report to; used to render install commands |
| Notification language | English or Chinese alert messages |
| Public status page | When off, anonymous visitors see nothing; admins still see everything |

### Admin access

The admin listener has no business being on the internet. Pick one:

1. **SSH tunnel (default, recommended).** `ssh -L 8081:127.0.0.1:8081 you@server`,
   then open `http://127.0.0.1:8081`.
2. **Private network.** Bind `-admin-listen` to a WireGuard or Tailscale address,
   such as `100.64.0.1:8081`.
3. **Reverse proxy with an IP allowlist.** See `Caddyfile.example`.
4. **`-admin-listen shared`.** Admin is served on the public listener. Only do this
   with two-factor authentication turned on.

### Reverse proxies

The server must see the real client address only for login rate limiting and the
IP shown in the admin list. By default it trusts forwarding headers from loopback,
which fits a proxy on the same host.

Caddy:

```caddy
probe.example.com {
	encode zstd gzip
	reverse_proxy 127.0.0.1:8080
}
```

nginx:

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

Behind Cloudflare, add [Cloudflare's IP ranges](https://www.cloudflare.com/ips/)
to `-trusted-proxies` as well. In Docker, requests arrive from the bridge gateway,
which is why the compose file trusts `172.16.0.0/12`.

### Storage and retention

| Data | Where | Kept |
| --- | --- | --- |
| Latest report, ~10 minutes of live samples | memory | until restart |
| 1-minute averages and TCP latency | SQLite | `-keep-minute-days` |
| 1-hour averages and TCP probe summaries | SQLite | `-keep-hour-days` |
| Alert history | SQLite | 30 days, at most 1000 entries |
| Availability intervals | SQLite | 91 days; adjacent equal states merge |
| Finished notification deliveries | SQLite | 7 days, at most 5000 entries |
| Administrator audit | SQLite | 30 days, at most 10000 entries |
| Logs | journald / container log | managed by the system |

Metric and alert cleanup runs hourly and returns freed space to the filesystem.
Availability, delivery and audit workers also prune their own records. To cap journald,
set `SystemMaxUse=50M` in `/etc/systemd/journald.conf`.

## Agent

Set these in `/etc/fenghuo/agent.env`. The install script writes this file with mode `0600`.
Optional TCP probe settings can also live in `/etc/fenghuo/probes.env`, loaded after
`agent.env` by a local systemd drop-in. This file overrides only the probe variables.

| Flag | Default | Description |
| --- | --- | --- |
| `-server` | | Server base URL, such as `https://probe.example.com` |
| `-token` | | Node token. Prefer `FENGHUO_TOKEN`, because flags are visible in the process list |
| `-interval` | `3s` | Report interval while someone is viewing the dashboard, 1s–60s |
| `-idle-interval` | `15s` | Report interval while nobody is viewing, up to 10m. `0` always uses `-interval` |
| `-nic` | auto | Comma-separated interfaces counted for traffic |
| `-ping` | | TCP latency targets, such as `CT=ct.example.com:80,CU=1.2.3.4:443` (at most 16) |
| `-ping-interval` | `30s` | How often latency targets are measured (10s–10m) |
| `-allow-http` | `false` | Allow a plain `http://` server that is not on loopback |
| `-print` | | Collect once, print the report as JSON and exit |
| `-version` | | Print the version and exit |

Change the configuration by editing the file and running `sudo systemctl restart fenghuo-agent`.

### Optional three-network TCP probes

In **Admin → Settings**, enter up to 16 named IPv4:port targets, one `name=IP:port` per line.
Names such as `Shanghai-CT` and `Beijing-CU` identify each history series. Names are public;
keep IP addresses only in the target value. Choose up to three homepage targets using
the checkboxes; the first three are used by default. All targets remain available in node details.
Leave the target list empty to keep probes disabled. The saved preset only generates commands;
the server never sends targets to agents. New installation commands include the saved
targets. To configure an existing systemd agent, copy the command shown in Settings and
run it yourself on that host over SSH. The local `configure-probes.sh` script writes
`/etc/fenghuo/probes.env` and a systemd drop-in, preserves `agent.env` and its token,
checks the restart, and restores the previous probe configuration if startup fails.
The same command can be run from your own SSH inventory or Ansible for a fleet.
Saving a new preset does not update already installed agents. Clear the target list,
save, and run the generated disable command on an agent to stop its probes.

The card shows recent TCP connect latency and connection failure rate only when an
agent has reported fresh probe samples. Failure rate is not ICMP packet loss. Target
addresses remain admin-only in the preset; public node responses contain target names
and measurements, never addresses. Configure stable targets you operate or have
permission to probe.

Network history has its own period selector, defaulting to 24 hours. Recent periods use
minute records (`-keep-minute-days`, default 7); older periods use completed-hour summaries
(`-keep-hour-days`, default 90). The 1h/6h/24h/7d/30d/90d choices are limited to these
retention settings. Hourly values average available minute values, rather than weighting
by individual attempts; missing values remain unknown. Hourly records accumulate after
this version is installed, starting with the last three complete hours still in minute storage.
Select any retained target to compare latency and connection failures. Renaming a target starts a separate history series.

Storage uses one row per node, target and completed minute, rather than one row per attempt.
Approximate row count is `nodes × targets × 1,440 × retained days`. For 50 nodes and 6 targets:
432,000 rows for one day, or 3,024,000 for seven days. A 100,000-row SQLite sample with
12-byte target names used about 34 bytes per row after compaction; reserve 64–128 bytes
per row (about 185–369 MiB for seven days) for planning. Ninety days of hourly summaries
add 648,000 rows for this fleet, or about 40–79 MiB at the same planning rate. Reserve
additional space for other tables, WAL and free space.
This is an estimate, not a database size limit. Cleanup runs hourly and reclaims free pages;
the retention window can temporarily include another hour. Stable targets are assumed.
If only 24 hours are needed, `-keep-minute-days=1` reduces retention for both probe history
and minute system metrics; hourly probe and system history are controlled by `-keep-hour-days`.
Increasing probe frequency within a minute does not multiply stored minute rows.

### Adaptive reporting

The **idle interval for new installations** setting selects 3, 15, 30 or 60 seconds
(default 15). It only changes generated installation commands. For an existing
agent, edit `FENGHUO_IDLE_INTERVAL=15s` in `/etc/fenghuo/agent.env` locally and
restart `fenghuo-agent`. The idle interval must be at least the normal interval.
No numeric configuration is sent in report responses; `202`/`204` remain the only signal.

See [monitoring and administration](monitoring.md) for public field controls,
network alerts, notification retries, availability, assets, sessions and audit retention.

While a browser has the status page or admin UI open, agents report every
`-interval` (3s). When nobody has looked for 15 seconds, they slow down to
`-idle-interval` (15s), reducing the number of reports by about 80%. Opening the page
switches them back within one idle interval. CPU usage and network speed are
computed from counter deltas, so an idle report is the average over its 15
seconds, and no traffic is missed. Offline detection follows the interval the
agent is on: three missed reports.

### Traffic accounting

Each server has a **traffic counting** mode in the admin UI: both directions,
upload only, download only, or whichever direction is larger. Pick the one your
provider bills by; many plans count only outbound traffic. Lists show upload and
download separately, with the counted direction highlighted.

By default every interface except loopback and virtual ones (`docker*`, `veth*`,
`br-*`, `virbr*`, `vmbr*`, `tun*`, `tap*`, `wg*`, `tailscale*` and similar) is
counted. These carry traffic that is already counted on a physical interface. If
your provider's counter disagrees, list the interface explicitly, for example `FENGHUO_NIC=eth0`.

The agent reports cumulative counters since boot, and the server computes the
difference. Reboots and server restarts don't lose traffic, but traffic sent
while the agent is stopped is not counted.

### Without systemd

The agent is a single static binary. On systems without systemd (Alpine with
OpenRC, containers), run it with any supervisor as an unprivileged user:

```bash
FENGHUO_SERVER=https://probe.example.com FENGHUO_TOKEN=fh_... fenghuo-agent
```

It needs read access to `/proc` and `/sys`, outbound HTTPS, and nothing else.
Run it on the host rather than inside a container; a container only sees its own
network and process namespaces.

## Notification channels

All alerts go to every enabled channel. Use **Send test** after saving.

### Telegram

1. Create a bot with [@BotFather](https://t.me/BotFather) and copy the token.
2. Send the bot a message, then open
   `https://api.telegram.org/bot<TOKEN>/getUpdates` and copy `chat.id`.
3. If Telegram is blocked from your server, set **API address** to a reverse
   proxy of `api.telegram.org`, or set `HTTPS_PROXY` for the server.

### Webhooks

Placeholders work in the URL (URL-escaped) and in the body template (JSON-escaped):
`{{title}}`, `{{text}}`, `{{node}}`, `{{kind}}`, `{{status}}` (`firing` or
`resolved`) and `{{time}}` (RFC 3339). With an empty body, the alert is posted as JSON:

```json
{"title":"[Fenghuo] Alert: offline","text":"Node tokyo has been offline for 3m0s","node":"tokyo","kind":"offline","firing":true,"time":1767225600}
```

| Service | Method | URL | Body template |
| --- | --- | --- | --- |
| Bark | GET | `https://api.day.app/<key>/{{title}}/{{text}}` | |
| WeCom bot | POST | `https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=<key>` | `{"msgtype":"text","text":{"content":"{{title}}\n{{text}}"}}` |
| Feishu bot | POST | `https://open.feishu.cn/open-apis/bot/v2/hook/<token>` | `{"msg_type":"text","content":{"text":"{{title}}\n{{text}}"}}` |
| DingTalk bot | POST | `https://oapi.dingtalk.com/robot/send?access_token=<token>` | `{"msgtype":"text","text":{"content":"{{title}}\n{{text}}"}}` |
| Discord | POST | `https://discord.com/api/webhooks/<id>/<token>` | `{"content":"**{{title}}**\n{{text}}"}` |
| ntfy | POST | `https://ntfy.sh/<topic>` | `{{title}}: {{text}}` |

By default notifications cannot reach private, loopback, link-local or CGNAT
addresses, which blocks SSRF against your internal network. For a self-hosted
endpoint on your LAN, start the server with `-notify-allow-private`.

## Alert rules

| Type | Fires when | Resolves when |
| --- | --- | --- |
| Offline | no report for the configured minutes | the next report arrives |
| CPU / Memory / Disk usage | above the threshold for the configured minutes | back at or below the threshold |
| Traffic quota | usage reaches the threshold percentage of the node's limit | silently, at the next billing period |
| Expiry | the expiry date is within the configured number of days | silently, when the expiry date changes |

A rule applies to all servers or to the servers you select. Servers that have
never reported do not trigger offline alerts. Alert state survives restarts, so
one-shot alerts are not sent twice.
