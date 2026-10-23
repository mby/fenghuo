#!/usr/bin/env bash
# Install, update or remove fenghuo-agent as a hardened systemd service.
#
#   curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-agent.sh \
#     | sudo bash -s -- --server https://probe.example.com --token fh_xxx
#
# Updating keeps the existing configuration:   ... | sudo bash -s -- --update
# Removing:                                      ... | sudo bash -s -- --uninstall
set -euo pipefail

REPO="mby/fenghuo"
BIN=/usr/local/bin/fenghuo-agent
UNIT=/etc/systemd/system/fenghuo-agent.service
CONF_DIR=/etc/fenghuo
ENV_FILE=$CONF_DIR/agent.env

SERVER=""
TOKEN="${FENGHUO_TOKEN:-}"
VERSION="latest"
INTERVAL=""
IDLE_INTERVAL=""
NIC=""
PING=""
PING_INTERVAL=""
MIRROR=""
ALLOW_HTTP=""
MODE="install"

die() { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }

require_value() {
	if [ "$#" -lt 2 ] || [ -z "$2" ] || [[ "$2" == -* ]]; then
		die "$1 requires a value"
	fi
}

atomic_install_binary() {
	# Reject directories (including symlinks to them) before any staging or platform-specific mv behavior.
	# 暂存前拒绝目录及指向目录的链接，不依赖不同平台的 mv 拒绝覆盖行为。
	if [ -d "$2" ]; then
		echo "error: binary destination is a directory: $2" >&2
		return 1
	fi
	(
		local staged
		# Stage beside the destination so rename stays atomic, even while the old binary runs.
		# 在目标同目录暂存，确保旧进程运行时仍可通过原子重命名替换。
		staged="$(mktemp "${2}.tmp.XXXXXX")" || exit 1
		trap 'rm -f -- "$staged"' EXIT
		trap 'exit 129' HUP
		trap 'exit 130' INT
		trap 'exit 143' TERM
		install -m 0755 -o root -g root -- "$1" "$staged" || exit 1
		mv -fT -- "$staged" "$2" || exit 1
	)
}

usage() {
	cat <<'EOF'
Usage: install-agent.sh [options]

  --server URL        server address, e.g. https://probe.example.com (required for a new install)
  --token TOKEN       node token from the admin UI (or set FENGHUO_TOKEN)
  --interval 3s       report interval while someone views the dashboard, 1s-60s
  --idle-interval 15s report interval while nobody is viewing (0 = always use --interval)
  --nic eth0,ens3     interfaces counted for traffic (default: auto)
  --ping "CT=host:80,CU=host:443"
                      TCP latency targets measured by this agent
  --ping-interval 30s  TCP probe interval (15s, 30s or 60s)
  --version vX.Y.Z    install a specific release (default: latest)
  --mirror PREFIX     prefix for GitHub download URLs, e.g. https://ghproxy.example.com/
  --allow-http        allow a plain http server URL (sends the token unencrypted)
  --update            only replace the binary and unit, keep the configuration
  --uninstall         stop and remove the agent
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--server) require_value "$@"; SERVER="$2"; shift 2 ;;
	--token) require_value "$@"; TOKEN="$2"; shift 2 ;;
	--interval) require_value "$@"; INTERVAL="$2"; shift 2 ;;
	--idle-interval) require_value "$@"; IDLE_INTERVAL="$2"; shift 2 ;;
	--nic) require_value "$@"; NIC="$2"; shift 2 ;;
	--ping) require_value "$@"; PING="$2"; shift 2 ;;
	--ping-interval) require_value "$@"; PING_INTERVAL="$2"; shift 2 ;;
	--version) require_value "$@"; VERSION="$2"; shift 2 ;;
	--mirror) require_value "$@"; MIRROR="$2"; shift 2 ;;
	--allow-http) ALLOW_HTTP=1; shift ;;
	--update) MODE="update"; shift ;;
	--uninstall) MODE="uninstall"; shift ;;
	-h | --help) usage; exit 0 ;;
	*) die "unknown option: $1 (see --help)" ;;
	esac
done

[ "$(id -u)" -eq 0 ] || die "please run as root (sudo)"
[ "$(uname -s)" = "Linux" ] || die "only Linux is supported"
command -v systemctl >/dev/null 2>&1 || die "systemd is required; see docs for running the agent manually"

if [ "$MODE" = "uninstall" ]; then
	info "removing fenghuo-agent"
	systemctl disable --now fenghuo-agent 2>/dev/null || true
	rm -f "$UNIT" "$BIN" "$ENV_FILE" "$CONF_DIR/probes.env" /etc/systemd/system/fenghuo-agent.service.d/10-probes.conf
	rmdir "$CONF_DIR" 2>/dev/null || true
	systemctl daemon-reload
	info "done"
	exit 0
fi

# An update cannot work without local configuration; check before downloads or replacement.
# 更新依赖已有本地配置，下载或替换任何文件前必须先检查。
if [ "$MODE" = "update" ] && [ ! -f "$ENV_FILE" ]; then
	die "$ENV_FILE not found; run a full install with --server and --token first"
fi

# Values end up in a systemd EnvironmentFile; reject anything that could break out of a line.
safe() { [[ "$1" != *[$'\n\r\t "\x27\\$`']* ]]; }

if [ "$MODE" = "install" ]; then
	[ -n "$SERVER" ] || die "--server is required"
	[ -n "$TOKEN" ] || die "--token is required"
	[[ "$SERVER" =~ ^https?://[A-Za-z0-9._:/\[\]-]+$ ]] || die "invalid --server URL"
	[[ "$SERVER" == https://* || -n "$ALLOW_HTTP" ]] || die "the server URL must use https (or pass --allow-http)"
	[[ "$TOKEN" =~ ^fh_[A-Za-z0-9_-]{43}$ ]] || die "invalid token format"
	[ -z "$INTERVAL" ] || [[ "$INTERVAL" =~ ^[0-9]+(ms|s|m)$ ]] || die "invalid --interval"
	[ -z "$IDLE_INTERVAL" ] || [[ "$IDLE_INTERVAL" =~ ^(0|[0-9]+(ms|s|m))$ ]] || die "invalid --idle-interval"
	[ -z "$NIC" ] || [[ "$NIC" =~ ^[A-Za-z0-9_.@,:-]+$ ]] || die "invalid --nic"
	[ -z "$PING" ] || safe "$PING" || die "invalid --ping"
	[ -z "$PING_INTERVAL" ] || [[ "$PING_INTERVAL" =~ ^(15|30|60)s$ ]] || die "invalid --ping-interval"
fi
[[ "$VERSION" == "latest" || "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][A-Za-z0-9.]+)?$ ]] || die "invalid --version"
[ -z "$MIRROR" ] || [[ "$MIRROR" =~ ^https://[A-Za-z0-9._:/-]+$ ]] || die "--mirror must be an https URL prefix"

case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
armv7* | armv8l) ARCH=armv7 ;;
*) die "unsupported architecture: $(uname -m)" ;;
esac

if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL --proto '=https' --tlsv1.2 --retry 3 -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -q --https-only -O "$2" "$1"; }
else
	die "curl or wget is required"
fi
command -v sha256sum >/dev/null 2>&1 || die "sha256sum is required"

if [ "$VERSION" = "latest" ]; then
	BASE="https://github.com/$REPO/releases/latest/download"
else
	BASE="https://github.com/$REPO/releases/download/$VERSION"
fi
BASE="${MIRROR}${BASE}"
ARCHIVE="fenghuo-agent_linux_${ARCH}.tar.gz"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

info "downloading $ARCHIVE ($VERSION)"
fetch "$BASE/$ARCHIVE" "$TMP/$ARCHIVE"
fetch "$BASE/checksums.txt" "$TMP/checksums.txt"

EXPECTED="$(awk -v f="$ARCHIVE" '$2 == f { print $1 }' "$TMP/checksums.txt")"
ACTUAL="$(sha256sum "$TMP/$ARCHIVE" | awk '{ print $1 }')"
[ -n "$EXPECTED" ] || die "$ARCHIVE is not listed in checksums.txt"
[ "$EXPECTED" = "$ACTUAL" ] || die "checksum mismatch for $ARCHIVE"
info "checksum verified"

tar -xzf "$TMP/$ARCHIVE" -C "$TMP" fenghuo-agent fenghuo-agent.service
atomic_install_binary "$TMP/fenghuo-agent" "$BIN"
install -m 0644 -o root -g root "$TMP/fenghuo-agent.service" "$UNIT"

if [ "$MODE" = "install" ]; then
	install -d -m 0700 -o root -g root "$CONF_DIR"
	(
		umask 077
		{
			echo "FENGHUO_SERVER=$SERVER"
			echo "FENGHUO_TOKEN=$TOKEN"
			[ -z "$INTERVAL" ] || echo "FENGHUO_INTERVAL=$INTERVAL"
			[ -z "$IDLE_INTERVAL" ] || echo "FENGHUO_IDLE_INTERVAL=$IDLE_INTERVAL"
			[ -z "$NIC" ] || echo "FENGHUO_NIC=$NIC"
			[ -z "$PING" ] || echo "FENGHUO_PING=$PING"
			[ -z "$PING_INTERVAL" ] || echo "FENGHUO_PING_INTERVAL=$PING_INTERVAL"
			[ -z "$ALLOW_HTTP" ] || echo "FENGHUO_ALLOW_HTTP=true"
		} >"$ENV_FILE.tmp"
		mv "$ENV_FILE.tmp" "$ENV_FILE"
	)
	chmod 0600 "$ENV_FILE"
	# A full install replaces local configuration; remove an older probe override.
	rm -f "$CONF_DIR/probes.env" /etc/systemd/system/fenghuo-agent.service.d/10-probes.conf
fi

systemctl daemon-reload
systemctl enable fenghuo-agent >/dev/null 2>&1
systemctl restart fenghuo-agent

info "installed $("$BIN" -version)"
sleep 2
if systemctl is-active --quiet fenghuo-agent; then
	info "fenghuo-agent is running. Logs: journalctl -u fenghuo-agent -f"
else
	echo "fenghuo-agent failed to start. Check: journalctl -u fenghuo-agent -n 50" >&2
	exit 1
fi
