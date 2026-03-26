#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
INSTALLER="$ROOT/scripts/gistclaw-install.sh"

assert_contains() {
	local file="$1"
	local want="$2"
	if ! grep -Fq "$want" "$file"; then
		echo "expected $file to contain: $want" >&2
		exit 1
	fi
}

setup_fixture_repo() {
	local fixture_dir="$1"
	local version="$2"
	local asset="gistclaw_${version}_linux_amd64.tar.gz"
	local stage_dir="$fixture_dir/stage"
	mkdir -p "$stage_dir"
	cat > "$stage_dir/gistclaw" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = "inspect" ] && [ "${2:-}" = "systemd-unit" ]; then
cat <<UNIT
[Unit]
Description=GistClaw service

[Service]
User=gistclaw
Group=gistclaw
WorkingDirectory=${GISTCLAW_FAKE_WORKDIR:-/var/lib/gistclaw}
ExecStart=${GISTCLAW_SYSTEMD_BINARY_PATH} --config ${GISTCLAW_SYSTEMD_CONFIG_PATH} serve
Restart=on-failure

[Install]
WantedBy=multi-user.target
UNIT
exit 0
fi
echo "unexpected fake gistclaw invocation: $*" >&2
exit 1
EOF
	chmod +x "$stage_dir/gistclaw"
	tar -czf "$fixture_dir/$asset" -C "$stage_dir" gistclaw
	(
		cd "$fixture_dir"
		sha256sum "$asset" > SHA256SUMS.txt
	)
}

setup_stub_bin() {
	local stub_dir="$1"
	local fixture_dir="$2"
	local log_dir="$3"
	local real_sha256
	real_sha256="$(command -v sha256sum)"

	cat > "$stub_dir/curl" <<EOF
#!/usr/bin/env bash
set -euo pipefail
out=""
url=""
while [ \$# -gt 0 ]; do
  case "\$1" in
    -o)
      out="\$2"
      shift 2
      ;;
    *)
      url="\$1"
      shift
      ;;
  esac
done
cp "$fixture_dir/\$(basename "\$url")" "\$out"
EOF
	chmod +x "$stub_dir/curl"

	cat > "$stub_dir/sha256sum" <<EOF
#!/usr/bin/env bash
set -euo pipefail
if [ "\${FORCE_BAD_CHECKSUM:-0}" = "1" ]; then
  echo "checksum mismatch" >&2
  exit 1
fi
exec "$real_sha256" "\$@"
EOF
	chmod +x "$stub_dir/sha256sum"

	cat > "$stub_dir/systemctl" <<EOF
#!/usr/bin/env bash
set -euo pipefail
echo "\$*" >> "$log_dir/systemctl.log"
EOF
	chmod +x "$stub_dir/systemctl"

	cat > "$stub_dir/groupadd" <<EOF
#!/usr/bin/env bash
set -euo pipefail
echo "groupadd \$*" >> "$log_dir/users.log"
EOF
	chmod +x "$stub_dir/groupadd"

	cat > "$stub_dir/useradd" <<EOF
#!/usr/bin/env bash
set -euo pipefail
echo "useradd \$*" >> "$log_dir/users.log"
EOF
	chmod +x "$stub_dir/useradd"

	cat > "$stub_dir/chown" <<EOF
#!/usr/bin/env bash
set -euo pipefail
echo "chown \$*" >> "$log_dir/ownership.log"
EOF
	chmod +x "$stub_dir/chown"

	cat > "$stub_dir/chmod" <<EOF
#!/usr/bin/env bash
set -euo pipefail
echo "chmod \$*" >> "$log_dir/ownership.log"
EOF
	chmod +x "$stub_dir/chmod"
}

run_happy_path() {
	local tmp
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' RETURN

	local fixture_dir="$tmp/fixtures"
	local stub_dir="$tmp/stubs"
	local log_dir="$tmp/logs"
	local root_dir="$tmp/root"
	local version="v0.1.0"
	mkdir -p "$fixture_dir" "$stub_dir" "$log_dir" "$root_dir"
	: > "$log_dir/systemctl.log"
	: > "$log_dir/users.log"
	: > "$log_dir/ownership.log"
	setup_fixture_repo "$fixture_dir" "$version"
	setup_stub_bin "$stub_dir" "$fixture_dir" "$log_dir"

	PATH="$stub_dir:$PATH" \
	GISTCLAW_ALLOW_NON_ROOT=1 \
	GISTCLAW_ARCH=amd64 \
	GISTCLAW_VERSION="$version" \
	GISTCLAW_BASE_URL="https://example.invalid/$version" \
	GISTCLAW_RELEASES_DIR="$root_dir/opt/gistclaw/releases" \
	GISTCLAW_BIN_LINK="$root_dir/usr/local/bin/gistclaw" \
	GISTCLAW_ETC_DIR="$root_dir/etc/gistclaw" \
	GISTCLAW_SYSTEMD_DIR="$root_dir/etc/systemd/system" \
	GISTCLAW_VAR_DIR="$root_dir/var/lib/gistclaw" \
	GISTCLAW_DOWNLOAD_DIR="$tmp/downloads" \
	GISTCLAW_FAKE_WORKDIR="$root_dir/var/lib/gistclaw" \
	GISTCLAW_PROVIDER_NAME=openai \
	GISTCLAW_PROVIDER_API_KEY=sk-test \
	bash "$INSTALLER" >/dev/null

	test -L "$root_dir/usr/local/bin/gistclaw"
	test -f "$root_dir/etc/gistclaw/config.yaml"
	test -f "$root_dir/etc/systemd/system/gistclaw.service"
	assert_contains "$root_dir/etc/gistclaw/config.yaml" "api_key: sk-test"
	assert_contains "$root_dir/etc/systemd/system/gistclaw.service" "ExecStart=$root_dir/usr/local/bin/gistclaw --config $root_dir/etc/gistclaw/config.yaml serve"
	assert_contains "$log_dir/systemctl.log" "daemon-reload"
	assert_contains "$log_dir/systemctl.log" "enable --now gistclaw"
	assert_contains "$log_dir/ownership.log" "chown -R gistclaw:gistclaw $root_dir/var/lib/gistclaw"
	assert_contains "$log_dir/ownership.log" "chown root:gistclaw $root_dir/etc/gistclaw/config.yaml"
	assert_contains "$log_dir/ownership.log" "chmod 640 $root_dir/etc/gistclaw/config.yaml"
}

run_missing_provider_case() {
	local tmp
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' RETURN

	local fixture_dir="$tmp/fixtures"
	local stub_dir="$tmp/stubs"
	local log_dir="$tmp/logs"
	local root_dir="$tmp/root"
	local version="v0.1.0"
	mkdir -p "$fixture_dir" "$stub_dir" "$log_dir" "$root_dir"
	: > "$log_dir/systemctl.log"
	: > "$log_dir/users.log"
	: > "$log_dir/ownership.log"
	setup_fixture_repo "$fixture_dir" "$version"
	setup_stub_bin "$stub_dir" "$fixture_dir" "$log_dir"

	set +e
	PATH="$stub_dir:$PATH" \
	GISTCLAW_ALLOW_NON_ROOT=1 \
	GISTCLAW_ARCH=amd64 \
	GISTCLAW_VERSION="$version" \
	GISTCLAW_BASE_URL="https://example.invalid/$version" \
	GISTCLAW_RELEASES_DIR="$root_dir/opt/gistclaw/releases" \
	GISTCLAW_BIN_LINK="$root_dir/usr/local/bin/gistclaw" \
	GISTCLAW_ETC_DIR="$root_dir/etc/gistclaw" \
	GISTCLAW_SYSTEMD_DIR="$root_dir/etc/systemd/system" \
	GISTCLAW_VAR_DIR="$root_dir/var/lib/gistclaw" \
	GISTCLAW_DOWNLOAD_DIR="$tmp/downloads" \
	bash "$INSTALLER" >"$tmp/out" 2>"$tmp/err"
	local status=$?
	set -e

	if [ "$status" -eq 0 ]; then
		echo "expected installer to fail without provider config" >&2
		exit 1
	fi
	assert_contains "$tmp/err" "provider name and api key are required"
	if [ -s "$log_dir/systemctl.log" ]; then
		echo "systemctl should not run when provider config is missing" >&2
		exit 1
	fi
}

run_checksum_failure_case() {
	local tmp
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' RETURN

	local fixture_dir="$tmp/fixtures"
	local stub_dir="$tmp/stubs"
	local log_dir="$tmp/logs"
	local root_dir="$tmp/root"
	local version="v0.1.0"
	mkdir -p "$fixture_dir" "$stub_dir" "$log_dir" "$root_dir"
	: > "$log_dir/systemctl.log"
	: > "$log_dir/users.log"
	: > "$log_dir/ownership.log"
	setup_fixture_repo "$fixture_dir" "$version"
	setup_stub_bin "$stub_dir" "$fixture_dir" "$log_dir"

	set +e
	PATH="$stub_dir:$PATH" \
	FORCE_BAD_CHECKSUM=1 \
	GISTCLAW_ALLOW_NON_ROOT=1 \
	GISTCLAW_ARCH=amd64 \
	GISTCLAW_VERSION="$version" \
	GISTCLAW_BASE_URL="https://example.invalid/$version" \
	GISTCLAW_RELEASES_DIR="$root_dir/opt/gistclaw/releases" \
	GISTCLAW_BIN_LINK="$root_dir/usr/local/bin/gistclaw" \
	GISTCLAW_ETC_DIR="$root_dir/etc/gistclaw" \
	GISTCLAW_SYSTEMD_DIR="$root_dir/etc/systemd/system" \
	GISTCLAW_VAR_DIR="$root_dir/var/lib/gistclaw" \
	GISTCLAW_DOWNLOAD_DIR="$tmp/downloads" \
	GISTCLAW_PROVIDER_NAME=openai \
	GISTCLAW_PROVIDER_API_KEY=sk-test \
	bash "$INSTALLER" >"$tmp/out" 2>"$tmp/err"
	local status=$?
	set -e

	if [ "$status" -eq 0 ]; then
		echo "expected installer to fail on checksum mismatch" >&2
		exit 1
	fi
	assert_contains "$tmp/err" "checksum mismatch"
}

run_happy_path
run_missing_provider_case
run_checksum_failure_case
