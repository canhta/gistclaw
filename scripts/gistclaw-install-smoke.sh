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

assert_files_equal() {
	local want="$1"
	local got="$2"
	if ! cmp -s "$want" "$got"; then
		echo "expected $got to match $want" >&2
		diff -u "$want" "$got" >&2 || true
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
if [ "${1:-}" = "inspect" ] && [ "${!#:-}" = "systemd-unit" ]; then
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
if [ "${1:-}" = "inspect" ] && [ "${!#:-}" = "config-paths" ]; then
if [ "${GISTCLAW_FAKE_CONFIG_PATHS_FAIL:-0}" = "1" ]; then
	echo "invalid config" >&2
	exit 1
fi
cat <<PATHS
state_dir: ${GISTCLAW_FAKE_STATE_DIR:-/var/lib/gistclaw}
database_dir: ${GISTCLAW_FAKE_DATABASE_DIR:-/var/lib/gistclaw}
workspace_root: ${GISTCLAW_FAKE_WORKSPACE_ROOT:-/var/lib/gistclaw/projects}
PATHS
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

	cat > "$stub_dir/apt-get" <<EOF
#!/usr/bin/env bash
set -euo pipefail
echo "\$*" >> "$log_dir/apt.log"
EOF
	chmod +x "$stub_dir/apt-get"

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
	: > "$log_dir/apt.log"
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
	GISTCLAW_FAKE_STATE_DIR="$root_dir/var/lib/gistclaw" \
	GISTCLAW_FAKE_DATABASE_DIR="$root_dir/var/lib/gistclaw" \
	GISTCLAW_FAKE_WORKSPACE_ROOT="$root_dir/var/lib/gistclaw/projects" \
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

run_config_file_happy_path() {
	local tmp
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' RETURN

	local fixture_dir="$tmp/fixtures"
	local stub_dir="$tmp/stubs"
	local log_dir="$tmp/logs"
	local root_dir="$tmp/root"
	local version="v0.1.0"
	local source_cfg="$tmp/source-config.yaml"
	mkdir -p "$fixture_dir" "$stub_dir" "$log_dir" "$root_dir"
	: > "$log_dir/systemctl.log"
	: > "$log_dir/apt.log"
	: > "$log_dir/users.log"
	: > "$log_dir/ownership.log"
	setup_fixture_repo "$fixture_dir" "$version"
	setup_stub_bin "$stub_dir" "$fixture_dir" "$log_dir"

	cat > "$source_cfg" <<EOF
# operator-managed config
provider:
  name: openai
  api_key: sk-test
  base_url: https://example.invalid/v1
  wire_api: chat_completions
  models:
    cheap: cx/gpt-5.4
    strong: cx/gpt-5.4

telegram:
  bot_token: tg-test
  agent_id: assistant

database_path: $root_dir/srv/data/runtime.db
workspace_root: $root_dir/srv/projects

web:
  listen_addr: 127.0.0.1:8080
EOF

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
	GISTCLAW_FAKE_STATE_DIR="$root_dir/var/lib/gistclaw" \
	GISTCLAW_FAKE_DATABASE_DIR="$root_dir/srv/data" \
	GISTCLAW_FAKE_WORKSPACE_ROOT="$root_dir/srv/projects" \
	bash "$INSTALLER" --config-file "$source_cfg" >/dev/null

	assert_files_equal "$source_cfg" "$root_dir/etc/gistclaw/config.yaml"
	assert_contains "$log_dir/ownership.log" "chown -R gistclaw:gistclaw $root_dir/var/lib/gistclaw"
	assert_contains "$log_dir/ownership.log" "chown -R gistclaw:gistclaw $root_dir/srv/data"
	assert_contains "$log_dir/ownership.log" "chown -R gistclaw:gistclaw $root_dir/srv/projects"
	assert_contains "$log_dir/systemctl.log" "enable --now gistclaw"
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
	: > "$log_dir/apt.log"
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

run_missing_config_file_case() {
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
	: > "$log_dir/apt.log"
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
	bash "$INSTALLER" --config-file "$tmp/missing.yaml" >"$tmp/out" 2>"$tmp/err"
	local status=$?
	set -e

	if [ "$status" -eq 0 ]; then
		echo "expected installer to fail with missing config file" >&2
		exit 1
	fi
	assert_contains "$tmp/err" "config file not found"
	if [ -s "$log_dir/systemctl.log" ]; then
		echo "systemctl should not run when config file is missing" >&2
		exit 1
	fi
}

run_mixed_mode_case() {
	local tmp
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' RETURN

	local fixture_dir="$tmp/fixtures"
	local stub_dir="$tmp/stubs"
	local log_dir="$tmp/logs"
	local root_dir="$tmp/root"
	local version="v0.1.0"
	local source_cfg="$tmp/source-config.yaml"
	mkdir -p "$fixture_dir" "$stub_dir" "$log_dir" "$root_dir"
	: > "$log_dir/systemctl.log"
	: > "$log_dir/apt.log"
	: > "$log_dir/users.log"
	: > "$log_dir/ownership.log"
	setup_fixture_repo "$fixture_dir" "$version"
	setup_stub_bin "$stub_dir" "$fixture_dir" "$log_dir"
	printf '%s\n' 'provider:' '  name: openai' '  api_key: sk-test' > "$source_cfg"

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
	bash "$INSTALLER" --config-file "$source_cfg" --provider-name openai --provider-api-key sk-test >"$tmp/out" 2>"$tmp/err"
	local status=$?
	set -e

	if [ "$status" -eq 0 ]; then
		echo "expected installer to fail when config-file and provider flags are mixed" >&2
		exit 1
	fi
	assert_contains "$tmp/err" "config-file mode cannot be combined"
	if [ -s "$log_dir/systemctl.log" ]; then
		echo "systemctl should not run when modes are mixed" >&2
		exit 1
	fi
}

run_invalid_config_helper_case() {
	local tmp
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' RETURN

	local fixture_dir="$tmp/fixtures"
	local stub_dir="$tmp/stubs"
	local log_dir="$tmp/logs"
	local root_dir="$tmp/root"
	local version="v0.1.0"
	local source_cfg="$tmp/source-config.yaml"
	mkdir -p "$fixture_dir" "$stub_dir" "$log_dir" "$root_dir"
	: > "$log_dir/systemctl.log"
	: > "$log_dir/apt.log"
	: > "$log_dir/users.log"
	: > "$log_dir/ownership.log"
	setup_fixture_repo "$fixture_dir" "$version"
	setup_stub_bin "$stub_dir" "$fixture_dir" "$log_dir"
	printf '%s\n' 'provider:' '  name: openai' '  api_key: sk-test' > "$source_cfg"

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
	GISTCLAW_FAKE_CONFIG_PATHS_FAIL=1 \
	bash "$INSTALLER" --config-file "$source_cfg" >"$tmp/out" 2>"$tmp/err"
	local status=$?
	set -e

	if [ "$status" -eq 0 ]; then
		echo "expected installer to fail when inspect config-paths fails" >&2
		exit 1
	fi
	assert_contains "$tmp/err" "invalid config"
	if [ -s "$log_dir/systemctl.log" ]; then
		echo "systemctl should not run when config-paths validation fails" >&2
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
	: > "$log_dir/apt.log"
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

run_public_domain_happy_path() {
	local tmp
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' RETURN

	local fixture_dir="$tmp/fixtures"
	local stub_dir="$tmp/stubs"
	local log_dir="$tmp/logs"
	local root_dir="$tmp/root"
	local version="v0.1.0"
	local domain="gistclaw.example.com"
	mkdir -p "$fixture_dir" "$stub_dir" "$log_dir" "$root_dir"
	: > "$log_dir/systemctl.log"
	: > "$log_dir/apt.log"
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
	GISTCLAW_FAKE_STATE_DIR="$root_dir/var/lib/gistclaw" \
	GISTCLAW_FAKE_DATABASE_DIR="$root_dir/var/lib/gistclaw" \
	GISTCLAW_FAKE_WORKSPACE_ROOT="$root_dir/var/lib/gistclaw/projects" \
	GISTCLAW_PROVIDER_NAME=openai \
	GISTCLAW_PROVIDER_API_KEY=sk-test \
	bash "$INSTALLER" --public-domain "$domain" >"$tmp/out"

	test -f "$root_dir/etc/caddy/Caddyfile"
	assert_contains "$root_dir/etc/caddy/Caddyfile" "$domain"
	assert_contains "$root_dir/etc/caddy/Caddyfile" "reverse_proxy 127.0.0.1:8080"
	assert_contains "$log_dir/apt.log" "update"
	assert_contains "$log_dir/apt.log" "install -y caddy"
	assert_contains "$log_dir/systemctl.log" "enable --now gistclaw"
	assert_contains "$log_dir/systemctl.log" "enable --now caddy"
	assert_contains "$log_dir/systemctl.log" "restart caddy"
	assert_contains "$tmp/out" "gistclaw auth set-password --config $root_dir/etc/gistclaw/config.yaml"
	assert_contains "$tmp/out" "https://$domain/login"
}

run_public_domain_requires_loopback_case() {
	local tmp
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' RETURN

	local fixture_dir="$tmp/fixtures"
	local stub_dir="$tmp/stubs"
	local log_dir="$tmp/logs"
	local root_dir="$tmp/root"
	local version="v0.1.0"
	local source_cfg="$tmp/source-config.yaml"
	mkdir -p "$fixture_dir" "$stub_dir" "$log_dir" "$root_dir"
	: > "$log_dir/systemctl.log"
	: > "$log_dir/apt.log"
	: > "$log_dir/users.log"
	: > "$log_dir/ownership.log"
	setup_fixture_repo "$fixture_dir" "$version"
	setup_stub_bin "$stub_dir" "$fixture_dir" "$log_dir"

	cat > "$source_cfg" <<EOF
provider:
  name: openai
  api_key: sk-test
database_path: $root_dir/srv/data/runtime.db
workspace_root: $root_dir/srv/projects
web:
  listen_addr: 0.0.0.0:8080
EOF

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
	GISTCLAW_FAKE_WORKDIR="$root_dir/var/lib/gistclaw" \
	GISTCLAW_FAKE_STATE_DIR="$root_dir/var/lib/gistclaw" \
	GISTCLAW_FAKE_DATABASE_DIR="$root_dir/srv/data" \
	GISTCLAW_FAKE_WORKSPACE_ROOT="$root_dir/srv/projects" \
	bash "$INSTALLER" --config-file "$source_cfg" --public-domain gistclaw.example.com >"$tmp/out" 2>"$tmp/err"
	local status=$?
	set -e

	if [ "$status" -eq 0 ]; then
		echo "expected installer to fail when public-domain mode is used with non-loopback web.listen_addr" >&2
		exit 1
	fi
	assert_contains "$tmp/err" "public-domain mode requires web.listen_addr to stay on loopback"
	if [ -s "$log_dir/apt.log" ]; then
		echo "apt-get should not run when public-domain validation fails" >&2
		exit 1
	fi
	if grep -Fq "enable --now gistclaw" "$log_dir/systemctl.log"; then
		echo "gistclaw service should not start when public-domain validation fails" >&2
		exit 1
	fi
}

run_happy_path
run_config_file_happy_path
run_missing_provider_case
run_missing_config_file_case
run_mixed_mode_case
run_checksum_failure_case
run_invalid_config_helper_case
run_public_domain_happy_path
run_public_domain_requires_loopback_case
