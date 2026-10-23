#!/usr/bin/env bash
# Safe installer tests: extracted production code, temporary files and mocked privileges.
# 安全安装测试：抽取实际脚本代码，只使用临时文件和模拟权限，不运行真实安装。
set -euo pipefail

DEPLOY_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_ROOT="$(mktemp -d)"
trap 'rm -rf -- "$TEST_ROOT"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

fail() { echo "FAIL: $*" >&2; exit 1; }

extract_atomic() {
	awk '
		/^atomic_install_binary\(\) \{/ { copying = 1; found = 1 }
		copying { print }
		copying && /^}/ { exit }
		END { if (!found) exit 1 }
	' "$1" >"$2"
}

test_atomic() (
	local local_script="$1" atomic_dir
	atomic_dir="$TEST_ROOT/atomic-$(basename "$local_script")"
	mkdir -p "$atomic_dir"
	extract_atomic "$local_script" "$atomic_dir/helper.sh"
	# Generated from the installer above; no static file exists for ShellCheck to follow.
	# shellcheck source=/dev/null
	source "$atomic_dir/helper.sh"
	SOURCE="$atomic_dir/download"
	TARGET="$atomic_dir/bin"
	printf 'new\n' >"$SOURCE"
	FAIL_STAGE=""
	RUNNING_PID=""
	trap 'if [ -n "$RUNNING_PID" ]; then kill "$RUNNING_PID" 2>/dev/null || true; wait "$RUNNING_PID" 2>/dev/null || true; fi' EXIT

	# Keep real filesystem behavior, but verify and strip privileged ownership arguments.
	# The extracted installer calls this mock indirectly.
	# shellcheck disable=SC2317,SC2329
	install() {
		[ "$#" -eq 9 ] || fail "unexpected install arguments"
		[ "$1 $2 $3 $4 $5 $6 $7" = '-m 0755 -o root -g root --' ] || fail "missing root ownership or 0755"
		[ "$8" = "$SOURCE" ] || fail "unexpected source"
		[[ "$9" == "$TARGET".tmp.* ]] || fail "staging must be beside the binary"
		[ -f "$9" ] || fail "staging file must already exist"
		printf 'install\n' >>"$atomic_dir/calls"
		if [ "$FAIL_STAGE" = install ]; then
			printf 'partial\n' >"$9"
			return 1
		fi
		if [ "$FAIL_STAGE" = signal ]; then
			kill -TERM "$BASHPID"
			return 1
		fi
		command install -m 0755 -- "$8" "$9"
	}

	# The extracted installer calls this mock indirectly.
	# shellcheck disable=SC2317,SC2329
	mktemp() {
		printf 'mktemp\n' >>"$atomic_dir/calls"
		[ "$FAIL_STAGE" != mktemp ] || return 1
		command mktemp "$@"
	}

	# The extracted installer calls this mock indirectly.
	# shellcheck disable=SC2317,SC2329
	mv() {
		if [ "$#" -ne 4 ] || [ "$1 $2" != '-fT --' ]; then
			fail "rename must replace the exact destination"
		fi
		if [[ "$3" != "$TARGET".tmp.* ]] || [ "$4" != "$TARGET" ]; then
			fail "rename crossed directories"
		fi
		[ "$(cat "$3")" = new ] || fail "rename before staging completed"
		if [ "$(uname -s)" = Linux ]; then
			[ "$(stat -c %a "$3")" = 755 ] || fail "permissions must be set before rename"
		fi
		printf 'rename\n' >>"$atomic_dir/calls"
		[ "$FAIL_STAGE" != rename ] || return 1
		command mv "$@"
	}

	assert_no_staging() {
		if compgen -G "$TARGET.tmp.*" >/dev/null; then
			fail "staging file leaked ($FAIL_STAGE)"
		fi
	}

	printf 'old\n' >"$TARGET"
	exec 9<"$TARGET"
	atomic_install_binary "$SOURCE" "$TARGET"
	[ "$(cat "$TARGET")" = new ] || fail "new path does not contain new binary"
	IFS= read -r old_contents <&9
	exec 9<&-
	[ "$old_contents" = old ] || fail "old open file was overwritten"
	assert_no_staging

	for FAIL_STAGE in mktemp install rename signal; do
		printf 'old\n' >"$TARGET"
		: >"$atomic_dir/calls"
		if atomic_install_binary "$SOURCE" "$TARGET"; then
			fail "$FAIL_STAGE failure was ignored"
		fi
		[ "$(cat "$TARGET")" = old ] || fail "$FAIL_STAGE damaged old binary"
		if [ "$FAIL_STAGE" != rename ] && grep -q rename "$atomic_dir/calls"; then
			fail "rename attempted after $FAIL_STAGE failure"
		fi
		assert_no_staging
	done
	FAIL_STAGE=""

	# Reject directories before invoking mv; failed replacements must leave them intact.
	command rm -f -- "$TARGET"
	mkdir "$TARGET"
	: >"$atomic_dir/calls"
	if atomic_install_binary "$SOURCE" "$TARGET" 2>"$atomic_dir/rename-error"; then
		fail "directory destination accepted"
	fi
	grep -Fq 'binary destination is a directory' "$atomic_dir/rename-error" || fail "missing directory diagnostic"
	[ -d "$TARGET" ] || fail "directory destination disappeared"
	[ ! -s "$atomic_dir/calls" ] || fail "directory rejection must precede mktemp, install and mv"
	rmdir "$TARGET" || fail "directory destination must remain empty"
	assert_no_staging

	if [ "$(uname -s)" = Linux ]; then
		mkdir "$atomic_dir/link-target"
		ln -s "$atomic_dir/link-target" "$TARGET"
		: >"$atomic_dir/calls"
		if atomic_install_binary "$SOURCE" "$TARGET" 2>"$atomic_dir/rename-error"; then
			fail "symlink to directory accepted"
		fi
		[ -L "$TARGET" ] || fail "directory symlink was replaced"
		[ -d "$TARGET" ] || fail "symlink target directory disappeared"
		[ ! -s "$atomic_dir/calls" ] || fail "directory symlink rejection must precede staging"
		command rm -f -- "$TARGET"
		rmdir "$atomic_dir/link-target" || fail "symlink target must remain empty"
		assert_no_staging

		# Use a disposable copy of sleep, never a real installed service binary.
		cp -- "$(command -v sleep)" "$TARGET"
		"$TARGET" 60 &
		RUNNING_PID=$!
		ready=""
		for ((attempt = 0; attempt < 100; attempt++)); do
			if [ "$(readlink "/proc/$RUNNING_PID/exe" || true)" = "$TARGET" ]; then
				ready=1
				break
			fi
			sleep 0.01
		done
		[ -n "$ready" ] || fail "temporary old process did not start"
		atomic_install_binary "$SOURCE" "$TARGET"
		kill -0 "$RUNNING_PID" || fail "replacement killed old process"
		[ "$(cat "$TARGET")" = new ] || fail "running binary not replaced"
		assert_no_staging
		kill "$RUNNING_PID"
		wait "$RUNNING_PID" 2>/dev/null || true
		RUNNING_PID=""
	else
		echo "SKIP: Linux running executable and directory symlink tests ($(basename "$local_script")); open-file replacement tested"
	fi
	echo "PASS: $(basename "$local_script") atomic replacement, failure preservation and cleanup"
)

extract_preflight() {
	# Stop before architecture selection/downloads; redirect every hardcoded writable path.
	# Never source the full installer, even with mocks.
	awk '
		$0 == "case \"$(uname -m)\" in" { found = 1; exit }
		/^BIN=/ { print "BIN=\"$CASE_DIR/bin\""; next }
		/^UNIT=/ { print "UNIT=\"$CASE_DIR/unit\""; next }
		/^CONF_DIR=/ { print "CONF_DIR=\"$CASE_DIR/config\""; next }
		/^DATA_DIR=/ { print "DATA_DIR=\"$CASE_DIR/data\""; next }
		{ print }
		END { if (!found) exit 1 }
	' "$1" >"$2"
}

run_preflight() (
	local prefix="$1"
	shift
	# The dynamically sourced preflight invokes these mocks, including failure guards.
	# shellcheck disable=SC2317,SC2329
	{
		id() { echo 0; }
		uname() { echo Linux; }
		command() {
			if [ "$*" = '-v systemctl' ]; then
				[ "$HAS_SYSTEMD" = 1 ]
			else
				builtin command "$@"
			fi
		}
		unexpected() { echo "unexpected side effect" >>"$CASE_DIR/effects"; exit 90; }
		install() { unexpected; }
		mv() { unexpected; }
		rm() { unexpected; }
		rmdir() { unexpected; }
		systemctl() { unexpected; }
		useradd() { unexpected; }
		curl() { unexpected; }
		wget() { unexpected; }
		tar() { unexpected; }
		mktemp() { unexpected; }
	}
	# Generated preflight only; following the full installer would defeat test isolation.
	# shellcheck source=/dev/null
	source "$prefix" "$@"
)

expect_preflight_error() {
	local expected="$1"
	shift
	if run_preflight "$@" >"$CASE_DIR/output" 2>&1; then
		fail "preflight unexpectedly passed: $*"
	fi
	grep -Fq -- "$expected" "$CASE_DIR/output" || fail "missing diagnostic: $expected"
	[ ! -e "$CASE_DIR/effects" ] || fail "preflight had side effects"
}

test_preflight() (
	local local_script="$1" CASE_DIR prefix
	CASE_DIR="$TEST_ROOT/preflight-$(basename "$local_script")"
	mkdir -p "$CASE_DIR/config"
	prefix="$CASE_DIR/preflight.sh"
	extract_preflight "$local_script" "$prefix"
	HAS_SYSTEMD=1
	# Isolate the generated installer's environment from the caller's credentials.
	export FENGHUO_TOKEN=""
	options=(--version --mirror)
	if [[ "$local_script" == *install-agent.sh ]]; then
		options+=(--server --token --interval --idle-interval --nic --ping)
	fi
	for option in "${options[@]}"; do
		for missing in absent empty option; do
			args=("$option")
			case "$missing" in
			empty) args+=("") ;;
			option) args+=(--uninstall) ;;
			esac
			expect_preflight_error "$option requires a value" "$prefix" "${args[@]}"
		done
	done

	printf 'old binary\n' >"$CASE_DIR/bin"
	printf 'old unit\n' >"$CASE_DIR/unit"
	if [[ "$local_script" == *install-agent.sh ]]; then
		expect_preflight_error 'not found; run a full install' "$prefix" --update
		mkdir "$CASE_DIR/config/agent.env"
		expect_preflight_error 'not found; run a full install' "$prefix" --update
		rmdir "$CASE_DIR/config/agent.env"
		printf 'existing configuration\n' >"$CASE_DIR/config/agent.env"
		run_preflight "$prefix" --update --version v1.2.3 --mirror https://mirror.example/
		[ "$(cat "$CASE_DIR/config/agent.env")" = 'existing configuration' ] || fail "update changed config"
	else
		HAS_SYSTEMD=0
		expect_preflight_error 'running fenghuo-server manually' "$prefix"
		if grep -qi docker "$CASE_DIR/output"; then
			fail "non-systemd diagnostic still forces Docker"
		fi
		HAS_SYSTEMD=1
		run_preflight "$prefix" --version v1.2.3 --mirror https://mirror.example/
	fi
	[ "$(cat "$CASE_DIR/bin")" = 'old binary' ] || fail "preflight changed binary"
	[ "$(cat "$CASE_DIR/unit")" = 'old unit' ] || fail "preflight changed unit"
	[ ! -e "$CASE_DIR/effects" ] || fail "preflight had side effects"
	echo "PASS: $(basename "$local_script") missing arguments and preflight validation"
)

for script in "$DEPLOY_DIR/install-server.sh" "$DEPLOY_DIR/install-agent.sh"; do
	bash -n "$script"
	test_atomic "$script"
	test_preflight "$script"
done
echo 'All installer tests passed.'
