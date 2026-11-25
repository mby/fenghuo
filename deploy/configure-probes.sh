#!/usr/bin/env bash
# Run locally on an installed agent host. No credentials or server connection are needed.
set -euo pipefail

ENV_FILE=/etc/fenghuo/agent.env
PROBES_FILE=/etc/fenghuo/probes.env
DROPIN_DIR=/etc/systemd/system/fenghuo-agent.service.d
DROPIN_FILE=$DROPIN_DIR/10-probes.conf
PING=""
INTERVAL=30s
ICMP=""
ICMP_INTERVAL=""
DISABLE=0

die() { echo "error: $*" >&2; exit 1; }

usage() {
	cat <<'EOF'
Usage: configure-probes.sh --ping 'CT=1.2.3.4:80,CU=...' [--ping-interval 30s]
       configure-probes.sh --disable

       configure-probes.sh --icmp 'CT=1.2.3.4,CU=...' [--ping-interval 30s]
       --icmp-interval overrides the shared interval for manual local configuration.
Run as root on each agent host. TCP and ICMP may be enabled together.
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--ping | --ping-interval | --icmp | --icmp-interval)
		if [ "$#" -lt 2 ] || [ -z "$2" ] || [[ "$2" == -* ]]; then
			die "$1 requires a value"
		fi
		case "$1" in
		--ping) PING=$2 ;;
		--ping-interval) INTERVAL=$2 ;;
		--icmp) ICMP=$2 ;;
		--icmp-interval) ICMP_INTERVAL=$2 ;;
		esac
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
ICMP_INTERVAL=${ICMP_INTERVAL:-$INTERVAL}
[[ "$ICMP_INTERVAL" =~ ^([1-9]|[1-5][0-9]|60)s$ ]] || die "--icmp-interval must be 1s-60s"
if [ "$DISABLE" -eq 1 ]; then
	[ -z "$PING$ICMP" ] || die "--disable cannot be combined with probe targets"
else
	[ -n "$PING$ICMP" ] || die "--ping or --icmp is required (or use --disable)"
fi
total_targets=0
validate_targets() {
	local raw=$1 kind=$2
	[ -n "$raw" ] || return 0
	[[ "$raw" != *[$'\n\r\t "\x27\\$`']* ]] || die "invalid probe targets"
	[[ "$raw" != ,* && "$raw" != *, && "$raw" != *,,* ]] || die "empty probe target"
	seen_names=$'\n'
	IFS=',' read -r -a entries <<< "$raw"
	total_targets=$((total_targets+${#entries[@]}))
	[ "$total_targets" -le 16 ] || die "at most 16 combined targets are allowed"
	for entry in "${entries[@]}"; do
		[[ "$entry" == *=* ]] || die "invalid target: $entry"
		name=${entry%%=*}
		address=${entry#*=}
		if [ "$kind" = icmp ]; then
			[[ "$address" != *:* ]] || die "ICMP target must be IPv4 without a port"
			address="$address:80"
		fi
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
}
validate_targets "$PING" tcp
validate_targets "$ICMP" icmp

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
	printf 'FENGHUO_ICMP=%s\n' "$ICMP"
	printf 'FENGHUO_ICMP_INTERVAL=%s\n' "$ICMP_INTERVAL"
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
