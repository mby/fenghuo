#!/usr/bin/env bash
# Run locally on an installed agent host. No credentials or server connection are needed.
set -euo pipefail

ENV_FILE=/etc/fenghuo/agent.env
PROBES_FILE=/etc/fenghuo/probes.env
DROPIN_DIR=/etc/systemd/system/fenghuo-agent.service.d
DROPIN_FILE=$DROPIN_DIR/10-probes.conf
PING=""
INTERVAL=30s
DISABLE=0

die() { echo "error: $*" >&2; exit 1; }

usage() {
	cat <<'EOF'
Usage: configure-probes.sh --ping 'CT=1.2.3.4:80,CU=...' [--ping-interval 30s]
       configure-probes.sh --disable

Run as root on each agent host. Targets are TCP connect probes, not ICMP ping.
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--ping | --ping-interval)
		if [ "$#" -lt 2 ] || [ -z "$2" ] || [[ "$2" == -* ]]; then
			die "$1 requires a value"
		fi
		if [ "$1" = --ping ]; then PING=$2; else INTERVAL=$2; fi
		shift 2 ;;
	--disable) DISABLE=1; shift ;;
	-h | --help) usage; exit 0 ;;
	*) die "unknown option: $1" ;;
	esac
done

[ "$(id -u)" -eq 0 ] || die "please run as root (sudo)"
if [ ! -f "$ENV_FILE" ] || [ ! -f /etc/systemd/system/fenghuo-agent.service ]; then
	die "fenghuo-agent systemd installation not found"
fi
command -v systemctl >/dev/null 2>&1 || die "systemd is required"
[[ "$INTERVAL" =~ ^(15|30|60)s$ ]] || die "--ping-interval must be 15s, 30s or 60s"
if [ "$DISABLE" -eq 1 ]; then
	[ -z "$PING" ] || die "--disable cannot be combined with --ping"
else
	[ -n "$PING" ] || die "--ping is required (or use --disable)"
	[[ "$PING" != *[$'\n\r\t "\x27\\$`']* ]] || die "invalid --ping"
	[[ "$PING" != ,* && "$PING" != *, && "$PING" != *,,* ]] || die "empty probe target"
	seen_names=$'\n'
	IFS=',' read -r -a entries <<< "$PING"
	[ "${#entries[@]}" -le 16 ] || die "at most 16 targets are allowed"
	for entry in "${entries[@]}"; do
		[[ "$entry" == *=* ]] || die "invalid target: $entry"
		name=${entry%%=*}
		address=${entry#*=}
		if [ -z "$name" ] || [ "$(printf %s "$name" | wc -c)" -gt 32 ] || [[ "$name" == *=* ]]; then
			die "target names must be 1-32 bytes"
		fi
		[[ "$seen_names" != *$'\n'"$name"$'\n'* ]] || die "duplicate target name"
		seen_names+="$name"$'\n'
		[[ "$address" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}:[0-9]{1,5}$ ]] || die "target must be IPv4:port"
		host=${address%:*}
		port=${address##*:}
		IFS='.' read -r -a octets <<< "$host"
		for octet in "${octets[@]}"; do
			[[ "$octet" != 0* || "$octet" = 0 ]] || die "invalid IPv4 address"
			[ "$((10#$octet))" -le 255 ] || die "invalid IPv4 address"
		done
		if [ "$((10#$port))" -lt 1 ] || [ "$((10#$port))" -gt 65535 ]; then
			die "invalid TCP port"
		fi
	done
fi

if [ -L "$PROBES_FILE" ] || [ -L "$DROPIN_FILE" ]; then
	die "probe configuration must not be symlinks"
fi
install -d -m 0700 -o root -g root /etc/fenghuo
install -d -m 0755 -o root -g root "$DROPIN_DIR"
work=$(mktemp -d /etc/fenghuo/.probes.XXXXXX)
trap 'rm -rf -- "$work"' EXIT
had_probes=0
had_dropin=0
if [ -e "$PROBES_FILE" ]; then cp -p -- "$PROBES_FILE" "$work/previous.env"; had_probes=1; fi
if [ -e "$DROPIN_FILE" ]; then cp -p -- "$DROPIN_FILE" "$work/previous.conf"; had_dropin=1; fi

{
	printf 'FENGHUO_PING=%s\n' "$PING"
	printf 'FENGHUO_PING_INTERVAL=%s\n' "$INTERVAL"
} > "$work/probes.env"
install -m 0600 -o root -g root "$work/probes.env" "$work/next.env"
cat > "$work/10-probes.conf" <<'EOF'
[Service]
EnvironmentFile=-/etc/fenghuo/probes.env
EOF
install -m 0644 -o root -g root "$work/10-probes.conf" "$work/next.conf"
mv -fT -- "$work/next.env" "$PROBES_FILE"
mv -fT -- "$work/next.conf" "$DROPIN_FILE"

if ! systemctl daemon-reload || ! systemctl restart fenghuo-agent || { sleep 2; ! systemctl is-active --quiet fenghuo-agent; }; then
	if [ "$had_probes" -eq 1 ]; then mv -fT -- "$work/previous.env" "$PROBES_FILE"; else rm -f -- "$PROBES_FILE"; fi
	if [ "$had_dropin" -eq 1 ]; then mv -fT -- "$work/previous.conf" "$DROPIN_FILE"; else rm -f -- "$DROPIN_FILE"; fi
	systemctl daemon-reload
	systemctl restart fenghuo-agent || true
	die "agent failed to start; previous probe configuration restored"
fi
echo "fenghuo-agent probe configuration applied locally"
