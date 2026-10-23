# Fenghuo

[中文](README_zh-CN.md)

A minimal, security-first server monitor for people who run a handful of VPSes.

Fenghuo (烽火, "beacon fire") is named after the signal towers on the Great Wall:
they could only send a signal outwards, never receive orders. The agent works
the same way. **It only sends metrics. The server has no way to make it do
anything**, so a compromised dashboard cannot be turned into a remote shell on
your machines.

## Why another probe

Popular probes let the dashboard run commands, open terminals or push updates to
every agent, usually as root. When the dashboard is breached, every monitored
machine is breached with it. Fenghuo removes that capability instead of guarding
it:

- **Send-only agent.** It POSTs a report over HTTPS and ignores the response body.
  The only thing it reads is whether the status code says someone is viewing the
  dashboard, which picks one of its two locally configured intervals. There is no
  command, terminal, file or update code at all, and a test fails the build if
  one is ever added.
- **No listening ports on agents.** Nothing to scan, nothing to exploit.
- **Unprivileged agent.** It runs as a throwaway systemd user with no
  capabilities, a read-only filesystem and a syscall filter.
- **Zero third-party dependencies in the agent.** Only the Go standard library.
- **Admin UI off the internet by default.** It listens on `127.0.0.1` and you
  reach it through an SSH tunnel. The public listener serves only the read-only
  status page and the report endpoint.
- **Small footprint.** Single binaries, SQLite, no Redis or time-series database.
  Raw samples stay in memory. SQLite stores aggregates, availability intervals,
  configuration and bounded notification/audit history.

## Features

- Search and sort servers, compare up to four nodes with shareable live/history charts
- TCP failure-rate charts and sustained latency/failure alerts
- Durable notification retries and per-attempt delivery history
- Public field controls, private asset records and costs grouped by currency
- 7/30/90-day observed availability with explicit unknown periods and outage timelines
- Active-session revocation and bounded administrator audit history

- Live status every 3 seconds while you watch: CPU, memory, swap, disk, network
  speed, load, TCP/UDP connections, processes, uptime
- Card and list views, a one-click "needs attention" filter, works on phones
- History charts: live (last ~10 minutes), 1 hour to 90 days
- Monthly traffic accounting with upload and download shown separately, a reset
  day, a quota, and two-way or one-way counting per server
- Expiry dates for your VPSes
- Alerts: offline, CPU/memory/disk above a threshold for N minutes, traffic
  quota, upcoming expiry
- Notifications: Telegram and generic webhooks (Bark, WeCom, Feishu, DingTalk,
  Discord, ntfy, …)
- Optional TCP latency targets, configured on each agent (for example, three
  carrier endpoints)
- Public status page with groups, which you can turn off. IP addresses are never shown.
- Admin login with argon2id passwords, optional TOTP two-factor authentication
  and login rate limiting
- English and Chinese UI, light and dark themes

See [monitoring and administration](docs/monitoring.md) for behavior and limitations.

Deliberately not included: web terminal, remote commands, file manager,
scheduled tasks, auto-update, plugins, multi-user roles.

## Quick start

Install **binaries with systemd** directly: no Docker, Go or Node.js is needed; the frontend is embedded.
The [binary installation guide](docs/installation.md) covers manual/offline installation, TLS, backups,
upgrades and rollback. Installers require an existing GitHub Release; see the guide's Actions steps
to create the first release or download a test build.

You need one Linux server with systemd for the dashboard and a domain name
pointing at it. Agents run on any Linux machine with systemd (amd64, arm64, armv7).

### 1. Install the server

```bash
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-server.sh | sudo bash
```

This creates a `fenghuo` system user, installs `/usr/local/bin/fenghuo-server`
and a hardened systemd unit, and writes `/etc/fenghuo/server.env`. Every
download is checked against the release's `checksums.txt`.

Put a TLS reverse proxy in front of the public listener (`127.0.0.1:8080`).
With [Caddy](https://caddyserver.com/), `/etc/caddy/Caddyfile`:

```caddy
probe.example.com {
	encode zstd gzip
	reverse_proxy 127.0.0.1:8080
}
```

<details>
<summary>Docker instead</summary>

```bash
curl -fsSLO https://raw.githubusercontent.com/mby/fenghuo/main/deploy/docker-compose.yml
docker compose up -d
docker compose exec fenghuo cat /data/initial_admin_password
```

Both ports are published on `127.0.0.1` only. Point your reverse proxy at `127.0.0.1:8080`.
</details>

### 2. Sign in

From your own computer, open an SSH tunnel to the admin listener:

```bash
ssh -L 8081:127.0.0.1:8081 you@your-server
```

Visit <http://127.0.0.1:8081/login> and sign in as `admin` with the password from:

```bash
sudo cat /var/lib/fenghuo/initial_admin_password
```

Then go to **Settings**: change the password, set **Public URL** to
`https://probe.example.com`, and consider turning on two-factor authentication.

### 3. Add servers

In **Admin → Servers → Add server**, fill in a name (and optionally traffic
quota and expiry date). The dialog shows a one-time token and an install command:

```bash
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-agent.sh \
  | sudo bash -s -- --server https://probe.example.com --token fh_...
```

Run it on the machine you want to monitor. It appears on the dashboard within seconds.

Agent options, such as TCP latency targets:

```bash
... | sudo bash -s -- --server https://probe.example.com --token fh_... \
  --ping "CT=ct.example.com:80,CU=cu.example.com:80,CM=cm.example.com:80"
```

Users in mainland China can pass `--mirror https://your-github-proxy/` to
download release files through a GitHub proxy.

## Upgrading

Follow the [backup and rollback guide](docs/installation.md#backup-upgrade-and-rollback) to stop and back up first,
then upgrade the server, verify it, and upgrade the agents.

```bash
# server
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-server.sh | sudo bash
# agent, keeping its configuration
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-agent.sh | sudo bash -s -- --update
# docker
docker compose pull && docker compose up -d
```

Agents never update themselves; that would be a remote code execution channel.

## Uninstalling

```bash
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-agent.sh | sudo bash -s -- --uninstall
curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-server.sh | sudo bash -s -- --uninstall
```

The server data in `/var/lib/fenghuo` is kept until you delete it.

## Operations

| Task | Command |
| --- | --- |
| Server logs | `journalctl -u fenghuo-server -f` |
| Agent logs | `journalctl -u fenghuo-agent -f` |
| Check what an agent collects | `fenghuo-agent -print` |
| Reset the admin password | `sudo -u fenghuo fenghuo-server reset-password` |
| Turn off two-factor auth | `sudo -u fenghuo fenghuo-server disable-totp` |
| Back up | [Stop and back up the database, configuration and previous binary](docs/installation.md#backup-upgrade-and-rollback) |

In Docker, prefix the commands with `docker compose exec fenghuo`.

## UI demo data

For local UI work, start Vite with `make dev-web` (or `pnpm --dir web dev`) and open
[the demo preview](http://localhost:5173/?demo=1). The preview supplies nine
synthetic public nodes with healthy, high CPU, memory/disk pressure, quota,
expiry, offline, pending, long-name and restricted-view cases. Detail and
comparison charts include metric gaps, latency and availability history.
The demo stays active in the current browser tab; use **Exit demo** or
`http://localhost:5173/?demo=0` to return to the local server data. It is
available only in Vite development mode and does not change the database.
Status warnings are computed from metrics; the sample public notes do not repeat them.

## Documentation

- [Binary installation and upgrades](docs/installation.md): scripts, manual/offline setup, systemd, backups, Actions builds
- [Configuration](docs/configuration.md): all server and agent options, reverse
  proxies, notification channels, retention
- [Security model](docs/security.md): threat model, hardening, verifying releases
- [Contributing](CONTRIBUTING.md): development setup and release process
- [Security policy](SECURITY.md): how to report vulnerabilities

## Resource usage

Both binaries are single static Go executables. The agent reads a few files in
`/proc` each interval and keeps no state on disk. The server keeps live data in
memory and flushes metric aggregates once a minute. Availability, notification
and audit workers persist their own bounded records. Agents report every 3 seconds
while the dashboard is viewed and every 15 seconds otherwise by default. Actual
traffic and storage depend on the enabled probes, history and usage. Both reporting
intervals are configured locally; the admin idle preset only affects installation commands.

## License

[MIT](LICENSE)
