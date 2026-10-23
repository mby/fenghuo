#!/usr/bin/env bash
# Install or update fenghuo-server as a hardened systemd service.
#
#   curl -fsSL https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-server.sh | sudo bash
#
# Options: --version vX.Y.Z, --mirror PREFIX, --uninstall (keeps the data directory).
set -euo pipefail

REPO="mby/fenghuo"
BIN=/usr/local/bin/fenghuo-server
UNIT=/etc/systemd/system/fenghuo-server.service
CONF_DIR=/etc/fenghuo
ENV_FILE=$CONF_DIR/server.env
DATA_DIR=/var/lib/fenghuo

VERSION="latest"
MIRROR=""
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

while [ $# -gt 0 ]; do
	case "$1" in
	--version) require_value "$@"; VERSION="$2"; shift 2 ;;
	--mirror) require_value "$@"; MIRROR="$2"; shift 2 ;;
	--uninstall) MODE="uninstall"; shift ;;
	-h | --help)
		sed -n '2,7p' "$0" 2>/dev/null || true
		exit 0
		;;
	*) die "unknown option: $1" ;;
	esac
done

[ "$(id -u)" -eq 0 ] || die "please run as root (sudo)"
[ "$(uname -s)" = "Linux" ] || die "only Linux is supported"
command -v systemctl >/dev/null 2>&1 || die "systemd is required for this installer; see docs for running fenghuo-server manually"

if [ "$MODE" = "uninstall" ]; then
	info "removing fenghuo-server (data in $DATA_DIR is kept)"
	systemctl disable --now fenghuo-server 2>/dev/null || true
	rm -f "$UNIT" "$BIN"
	systemctl daemon-reload
	info "done. Delete $DATA_DIR and $ENV_FILE yourself if you no longer need them."
	exit 0
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
ARCHIVE="fenghuo-server_linux_${ARCH}.tar.gz"

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

tar -xzf "$TMP/$ARCHIVE" -C "$TMP" fenghuo-server fenghuo-server.service server.env.example

if ! id fenghuo >/dev/null 2>&1; then
	info "creating system user fenghuo"
	useradd --system --home-dir "$DATA_DIR" --no-create-home --shell /usr/sbin/nologin fenghuo
fi

atomic_install_binary "$TMP/fenghuo-server" "$BIN"
install -m 0644 -o root -g root "$TMP/fenghuo-server.service" "$UNIT"
install -d -m 0755 -o root -g root "$CONF_DIR"
if [ ! -f "$ENV_FILE" ]; then
	install -m 0600 -o root -g root "$TMP/server.env.example" "$ENV_FILE"
	info "wrote default configuration to $ENV_FILE"
fi

systemctl daemon-reload
systemctl enable fenghuo-server >/dev/null 2>&1
systemctl restart fenghuo-server
sleep 2
systemctl is-active --quiet fenghuo-server || die "fenghuo-server failed to start. Check: journalctl -u fenghuo-server -n 50"

info "installed $("$BIN" version)"
cat <<EOF

Next steps
  1. Put a TLS reverse proxy in front of 127.0.0.1:8080 (see Caddyfile.example in the release).
  2. Open the admin UI through an SSH tunnel from your computer:
       ssh -L 8081:127.0.0.1:8081 ${SUDO_USER:-root}@<this-server>
     then visit http://127.0.0.1:8081/login
EOF
if [ -f "$DATA_DIR/initial_admin_password" ]; then
	echo "  3. Sign in as admin with the password in $DATA_DIR/initial_admin_password:"
	echo "       sudo cat $DATA_DIR/initial_admin_password"
	echo "     and change it under Settings."
fi
