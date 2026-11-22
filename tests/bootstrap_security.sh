#!/usr/bin/env bash
# Run with bash in an isolated environment. No fixture needs network access.
set -euo pipefail

absolute_script_path() {
	local script="$1"
	local dir
	dir="$(cd -- "$(dirname -- "$script")" && pwd)" || return 1
	printf '%s/%s\n' "$dir" "${script##*/}"
}

bootstrap="$(absolute_script_path "${1:-$(dirname -- "${BASH_SOURCE[0]}")/../scripts/bootstrap_anx.sh}")"
posix="$(absolute_script_path "${2:-$(dirname -- "${BASH_SOURCE[0]}")/../install.sh}")"
work="$(mktemp -d)"
trap 'rm -rf -- "$work"' EXIT
mkdir -p "$work"/{tools,payload}

for tool in awk chmod cp cut dirname grep gzip ln mkdir mktemp mv rm sed tar tr; do
	ln -s "$(command -v "$tool")" "$work/tools/$tool"
done
cat >"$work/tools/uname" <<'EOF'
#!/bin/sh
case "$1" in
	-s) echo "${FIXTURE_OS:-Linux}" ;;
	-m) echo x86_64 ;;
esac
EOF
cat >"$work/tools/curl" <<'EOF'
#!/bin/bash
for arg in "$@"; do
	[[ "$arg" == fixture://no-source ]] && exit 1
done
case "${@: -1}" in
	fixture://release) cp "$FIXTURES/release" "${@: -2:1}" ;;
	fixture://sums) cp "$FIXTURES/sums" "${@: -2:1}" ;;
	*) echo 'unexpected curl' >>"$HOME/calls"; exit 1 ;;
esac
EOF
cat >"$work/payload/anx" <<'EOF'
#!/bin/sh
echo 'release executed' >>"$HOME/calls"
EOF
chmod +x "$work/tools/"{uname,curl} "$work/payload/anx"
tar -czf "$work/release" -C "$work/payload" anx
if command -v sha256sum >/dev/null; then
	hash_tool=sha256sum
	digest="$(sha256sum "$work/release" | awk '{print $1}')"
else
	hash_tool=shasum
	digest="$(shasum -a 256 "$work/release" | awk '{print $1}')"
fi
ln -s "$(command -v "$hash_tool")" "$work/tools/$hash_tool"

run_bootstrap() {
	local scenario="$1"
	shift
	mkdir -p "$work/$scenario/home" "$work/$scenario/tmp"
	env -i HOME="$work/$scenario/home" TMPDIR="$work/$scenario/tmp" \
		PATH="$work/tools" FIXTURES="$work" ANX_GO_SYS_BIN="$work/no-system-go" \
		FIXTURE_OS="${fixture_os:-Linux}" ANX_SKIP_CLT=1 \
		ANX_RELEASE_URL=fixture://release ANX_SUMS_URL=fixture://sums \
		/bin/bash "$bootstrap" "$@" >"$work/$scenario/output" 2>&1
}

failures=0
fail() {
	printf 'FAIL: %s\n' "$1" >&2
	failures=$((failures + 1))
}

expect_rejected() {
	local scenario="$1" home="$work/$1/home"
	if run_bootstrap "$scenario" --version; then
		fail "$scenario: bootstrap unexpectedly succeeded"
	fi
	if [[ -e "$home/calls" || -e "$home/.local/bin/anx" ]]; then
		fail "$scenario: executable was run or installed, or an unexpected download was attempted"
	fi
}

expect_installed() {
	local scenario="$1" home="$work/$1/home"
	if ! run_bootstrap "$scenario" --version ||
		[[ ! -x "$home/.local/bin/anx" ]] ||
		[[ "$(grep -c '^release executed$' "$home/calls")" != 2 ]]; then
		fail "$scenario: verified release was not installed and dispatched"
	fi
}

printf '%s  anx_linux_amd64.tar.gz\n' "$digest" >"$work/sums"
expect_installed valid

run_posix() {
	local scenario="$1"
	mkdir -p "$work/$scenario/home" "$work/$scenario/tmp"
	(cd "$work/$scenario/home" &&
		env -i HOME="$PWD" TMPDIR="$work/$scenario/tmp" PATH="$work/tools" FIXTURES="$work" \
			ANX_RELEASE_URL=fixture://release ANX_SUMS_URL=fixture://sums \
			ANX_TARBALL_URL=fixture://no-source ANX_BOOTSTRAP_ONLY=1 /bin/sh "$posix" --install-only) \
		>"$work/$scenario/output" 2>&1
}

if ! run_posix posix-valid || [[ ! -x "$work/posix-valid/home/.local/bin/anx" ]]; then
	fail "posix-valid: verified release was not installed"
fi
printf '%s  other-anx_linux_amd64.tar.gz\n' "$digest" >"$work/sums"
expect_rejected wrong-filename
if run_posix posix-wrong-filename ||
	[[ -e "$work/posix-wrong-filename/home/calls" || -e "$work/posix-wrong-filename/home/.local/bin/anx" ]] ||
	! grep -q 'Missing, invalid, or ambiguous checksum' "$work/posix-wrong-filename/output"; then
	fail "posix-wrong-filename: accepted a checksum for a different filename"
fi
printf 'not-a-digest  anx_linux_amd64.tar.gz\n' >"$work/sums"
expect_rejected malformed
printf '%064d  anx_linux_amd64.tar.gz\n' 0 >"$work/sums"
expect_rejected mismatch
printf '%s  anx_linux_amd64.tar.gz\n' "$digest" "$digest" >"$work/sums"
expect_rejected ambiguous

mv "$work/tools/$hash_tool" "$work/disabled-hash-tool"
printf '%s  anx_linux_amd64.tar.gz\n' "$digest" >"$work/sums"
expect_rejected no-hash-tool

if [[ "$hash_tool" == sha256sum ]] && command -v shasum >/dev/null; then
	ln -s "$(command -v shasum)" "$work/tools/shasum"
	expect_installed valid-shasum
	rm "$work/tools/shasum"
fi

printf '#!/bin/sh\nprintf "%%s\\n" "%s"\nexit 1\n' "$digest" >"$work/tools/$hash_tool"
chmod +x "$work/tools/$hash_tool"
expect_rejected failed-hash-tool

if run_bootstrap no-go --ensure-go-only; then
	fail "no-go: bootstrap succeeded without a Go toolchain"
fi
if [[ -e "$work/no-go/home/calls" ]]; then
	fail "no-go: bootstrap attempted to fetch a remote Go installer"
fi
if ! grep -q 'Please install Go manually' "$work/no-go/output"; then
	fail "no-go: missing manual Go installation guidance"
fi

# Simulate a macOS system with CLT and Git but no GVM, Go, or Homebrew.
mkdir -p "$work/clt/usr/bin"
printf '#!/bin/sh\nexit 0\n' >"$work/clt/usr/bin/git"
ln -s "$work/clt/usr/bin/git" "$work/tools/git"
printf '#!/bin/sh\nprintf "%%s\\n" "$FIXTURES/clt"\n' >"$work/tools/xcode-select"
chmod +x "$work/clt/usr/bin/git" "$work/tools/xcode-select"
if fixture_os=Darwin run_bootstrap macos-no-tools --ensure-go-only ||
	[[ -e "$work/macos-no-tools/home/calls" ]]; then
	fail "macos-no-tools: ran an automatic installer or unexpectedly succeeded"
fi

if grep -Eq 'sudo_local|pam_permit|pam_deny' "$bootstrap" "$posix"; then
	fail "installers must not manipulate macOS sudo PAM modules"
fi

cat >"$work/tools/brew" <<'EOF'
#!/bin/sh
[ "$*" = "install go" ] || exit 1
printf '%s\n' '#!/bin/sh' 'echo "go version go1.27.3 linux/amd64"' >"$HOME/.local/bin/go"
chmod +x "$HOME/.local/bin/go"
EOF
chmod +x "$work/tools/brew"
if ! run_bootstrap native-manager --ensure-go-only ||
	[[ ! -x "$work/native-manager/home/.local/bin/go" || -e "$work/native-manager/home/calls" ]]; then
	fail "native-manager: Go was not installed via the package manager alone"
	cat "$work/native-manager/output" >&2
fi

[[ "$failures" -eq 0 ]]
