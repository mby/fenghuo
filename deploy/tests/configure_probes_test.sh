#!/usr/bin/env bash
set -euo pipefail

root=$(mktemp -d)
trap 'rm -rf -- "$root"' EXIT
mkdir -p "$root/etc/fenghuo" "$root/etc/systemd/system" "$root/bin"
printf 'FENGHUO_SERVER=https://example.com\nFENGHUO_TOKEN=secret\n' > "$root/etc/fenghuo/agent.env"
: > "$root/etc/systemd/system/fenghuo-agent.service"

# Run the production script against temporary paths with mocked privileges and systemctl.
sed -e "s|/etc/fenghuo|$root/etc/fenghuo|g" \
    -e "s|/etc/systemd/system|$root/etc/systemd/system|g" \
    -e 's/ -o root -g root//g' -e 's/sleep 2/sleep 0/' \
    "$(dirname "$0")/../configure-probes.sh" > "$root/configure.sh"
printf '#!/usr/bin/env bash\necho 0\n' > "$root/bin/id"
cat > "$root/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
if [ "$1" = restart ] && [ "${FAIL_RESTART:-0}" = 1 ]; then exit 1; fi
EOF
if [[ "$(uname -s)" != Linux ]]; then
	cat > "$root/bin/install" <<'EOF'
#!/usr/bin/env bash
if [ "$1" = -d ]; then mkdir -p -- "${@: -1}"; else cp -- "${@: -2:1}" "${@: -1}"; fi
EOF
	chmod +x "$root/bin/install"
fi
chmod +x "$root/bin/id" "$root/bin/systemctl"
export PATH="$root/bin:$PATH"

bash "$root/configure.sh" --ping 'CT=203.0.113.10:443,CU=198.51.100.2:80' --ping-interval 30s
grep -Fxq 'FENGHUO_PING=CT=203.0.113.10:443,CU=198.51.100.2:80' "$root/etc/fenghuo/probes.env"
grep -Fxq 'FENGHUO_TOKEN=secret' "$root/etc/fenghuo/agent.env"
if [[ "$(uname -s)" = Linux ]]; then [ "$(stat -c %a "$root/etc/fenghuo/probes.env")" = 600 ]; fi

for invalid in 'CT=203.0.113.10:0' 'CT=203.0.113.10:65536' '=203.0.113.10:443' 'CT=203.0.113.10:443,CT=192.0.2.3:443'; do
	if bash "$root/configure.sh" --ping "$invalid" > "$root/output" 2>&1; then
		echo 'invalid probe target accepted' >&2; exit 1
	fi
done
for option in --ping --ping-interval; do
	if bash "$root/configure.sh" "$option" > "$root/output" 2>&1; then
		echo 'missing option value accepted' >&2; exit 1
	fi
	grep -Fq -- "$option requires a value" "$root/output" || { cat "$root/output" >&2; exit 1; }
done
grep -Fxq 'FENGHUO_PING=CT=203.0.113.10:443,CU=198.51.100.2:80' "$root/etc/fenghuo/probes.env"

if FAIL_RESTART=1 bash "$root/configure.sh" --ping 'CM=192.0.2.3:443' > "$root/output" 2>&1; then
	echo 'failed restart accepted' >&2; exit 1
fi
grep -Fxq 'FENGHUO_PING=CT=203.0.113.10:443,CU=198.51.100.2:80' "$root/etc/fenghuo/probes.env"

bash "$root/configure.sh" --ping '上海电信=203.0.113.10:443,上海联通=198.51.100.2:80,北京移动=192.0.2.3:443,北京电信=192.0.2.4:80'
grep -Fq '北京电信=192.0.2.4:80' "$root/etc/fenghuo/probes.env"
bash "$root/configure.sh" --disable
grep -Fxq 'FENGHUO_PING=' "$root/etc/fenghuo/probes.env"
echo 'configure-probes tests passed'
