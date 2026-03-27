#!/bin/sh
set -eu

VERSION="${GISTCLAW_VERSION:-v0.1.0}"
REPO="${GISTCLAW_REPO:-canhta/gistclaw}"
BASE_URL="${GISTCLAW_BASE_URL:-}"
PROVIDER_NAME="${GISTCLAW_PROVIDER_NAME:-}"
PROVIDER_API_KEY="${GISTCLAW_PROVIDER_API_KEY:-}"
CONFIG_FILE="${GISTCLAW_CONFIG_FILE:-}"
PUBLIC_DOMAIN="${GISTCLAW_PUBLIC_DOMAIN:-}"
ALLOW_NON_ROOT="${GISTCLAW_ALLOW_NON_ROOT:-0}"
ARCH_OVERRIDE="${GISTCLAW_ARCH:-}"

RELEASES_DIR="${GISTCLAW_RELEASES_DIR:-/opt/gistclaw/releases}"
BIN_LINK="${GISTCLAW_BIN_LINK:-/usr/local/bin/gistclaw}"
ETC_DIR="${GISTCLAW_ETC_DIR:-/etc/gistclaw}"
CADDY_ETC_DIR="${GISTCLAW_CADDY_ETC_DIR:-$(dirname "${ETC_DIR}")/caddy}"
SYSTEMD_DIR="${GISTCLAW_SYSTEMD_DIR:-/etc/systemd/system}"
VAR_DIR="${GISTCLAW_VAR_DIR:-/var/lib/gistclaw}"
DOWNLOAD_DIR="${GISTCLAW_DOWNLOAD_DIR:-$(mktemp -d)}"
DEFAULT_CONFIG_PATH="/etc/gistclaw/config.yaml"
DEFAULT_SERVICE_PATH="/etc/systemd/system/gistclaw.service"

usage() {
	cat <<'EOF'
Usage: gistclaw-install.sh [options]

Options:
  --version VERSION
  --config-file PATH
  --provider-name NAME
  --provider-api-key KEY
  --public-domain DOMAIN
  --repo OWNER/REPO
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--version)
		VERSION="$2"
		shift 2
		;;
	--config-file)
		CONFIG_FILE="$2"
		shift 2
		;;
	--provider-name)
		PROVIDER_NAME="$2"
		shift 2
		;;
	--provider-api-key)
		PROVIDER_API_KEY="$2"
		shift 2
		;;
	--public-domain)
		PUBLIC_DOMAIN="$2"
		shift 2
		;;
	--repo)
		REPO="$2"
		shift 2
		;;
	-h|--help)
		usage
		exit 0
		;;
	*)
		echo "unknown argument: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

if [ "${ALLOW_NON_ROOT}" != "1" ] && [ "$(id -u)" -ne 0 ]; then
	echo "installer must run as root; set GISTCLAW_ALLOW_NON_ROOT=1 only for smoke tests" >&2
	exit 1
fi

if [ -n "${CONFIG_FILE}" ] && { [ -n "${PROVIDER_NAME}" ] || [ -n "${PROVIDER_API_KEY}" ]; }; then
	echo "config-file mode cannot be combined with provider flags" >&2
	exit 1
fi

if [ -n "${CONFIG_FILE}" ] && [ ! -f "${CONFIG_FILE}" ]; then
	echo "config file not found: ${CONFIG_FILE}" >&2
	exit 1
fi

if [ -z "${CONFIG_FILE}" ] && { [ -z "${PROVIDER_NAME}" ] || [ -z "${PROVIDER_API_KEY}" ]; }; then
	echo "provider name and api key are required; refusing to enable service" >&2
	exit 1
fi

ARCH="${ARCH_OVERRIDE:-$(uname -m)}"
case "${ARCH}" in
	x86_64|amd64)
		GOARCH="amd64"
		;;
	*)
		echo "unsupported architecture: ${ARCH}" >&2
		exit 1
		;;
esac

if [ -z "${BASE_URL}" ]; then
	BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
fi

ARCHIVE="gistclaw_${VERSION}_linux_${GOARCH}.tar.gz"
SUMS_FILE="${DOWNLOAD_DIR}/SHA256SUMS.txt"
ARCHIVE_FILE="${DOWNLOAD_DIR}/${ARCHIVE}"
CHECKSUM_FILE="${DOWNLOAD_DIR}/${ARCHIVE}.sha256"
RELEASE_DIR="${RELEASES_DIR}/${VERSION}"
CONFIG_PATH="${ETC_DIR}/config.yaml"
CADDYFILE_PATH="${CADDY_ETC_DIR}/Caddyfile"
SERVICE_PATH="${SYSTEMD_DIR}/gistclaw.service"
CONFIG_PATHS_ERR="${DOWNLOAD_DIR}/inspect-config-paths.err"

extract_config_value() {
	key="$1"
	printf '%s\n' "$2" | sed -n "s/^${key}: //p" | head -n 1
}

ensure_owned_dir() {
	path="$1"
	if [ -z "${path}" ]; then
		return 0
	fi
	mkdir -p "${path}"
	chown -R gistclaw:gistclaw "${path}"
}

same_path() {
	left_dir=$(cd "$(dirname "$1")" && pwd -P)
	right_dir=$(cd "$(dirname "$2")" && pwd -P)
	[ "${left_dir}/$(basename "$1")" = "${right_dir}/$(basename "$2")" ]
}

config_web_listen_addr() {
	awk '
		$1 == "web:" {
			in_web = 1
			next
		}
		/^[^[:space:]#]/ {
			in_web = 0
		}
		in_web && $1 == "listen_addr:" {
			print $2
			exit
		}
	' "$1"
}

is_loopback_listen_addr() {
	case "$1" in
	127.0.0.1:*|localhost:*|"[::1]:"*|::1:*)
		return 0
		;;
	*)
		return 1
		;;
	esac
}

mkdir -p "${DOWNLOAD_DIR}" "${RELEASE_DIR}" "$(dirname "${BIN_LINK}")" "${ETC_DIR}" "${SYSTEMD_DIR}" "${VAR_DIR}"

curl -fsSL -o "${SUMS_FILE}" "${BASE_URL}/SHA256SUMS.txt"
curl -fsSL -o "${ARCHIVE_FILE}" "${BASE_URL}/${ARCHIVE}"

if ! grep " ${ARCHIVE}\$" "${SUMS_FILE}" > "${CHECKSUM_FILE}"; then
	echo "missing checksum entry for ${ARCHIVE}" >&2
	exit 1
fi
if ! (cd "${DOWNLOAD_DIR}" && sha256sum -c "$(basename "${CHECKSUM_FILE}")"); then
	echo "checksum mismatch for ${ARCHIVE}" >&2
	exit 1
fi

if command -v groupadd >/dev/null 2>&1; then
	groupadd --system gistclaw 2>/dev/null || true
fi
if command -v useradd >/dev/null 2>&1; then
	useradd --system --gid gistclaw --home-dir "${VAR_DIR}" --shell /usr/sbin/nologin gistclaw 2>/dev/null || true
fi

chown -R gistclaw:gistclaw "${VAR_DIR}"

tar -xzf "${ARCHIVE_FILE}" -C "${RELEASE_DIR}"
chmod +x "${RELEASE_DIR}/gistclaw"
ln -snf "${RELEASE_DIR}/gistclaw" "${BIN_LINK}"

if [ -n "${CONFIG_FILE}" ]; then
	if ! same_path "${CONFIG_FILE}" "${CONFIG_PATH}"; then
		cp "${CONFIG_FILE}" "${CONFIG_PATH}"
	fi
else
	cat > "${CONFIG_PATH}" <<EOF
provider:
  name: ${PROVIDER_NAME}
  api_key: ${PROVIDER_API_KEY}
database_path: ${VAR_DIR}/runtime.db
storage_root: ${VAR_DIR}
web:
  listen_addr: 127.0.0.1:8080
EOF
fi
chown root:gistclaw "${CONFIG_PATH}"
chmod 640 "${CONFIG_PATH}"

WEB_LISTEN_ADDR="$(config_web_listen_addr "${CONFIG_PATH}")"
if [ -n "${PUBLIC_DOMAIN}" ] && ! is_loopback_listen_addr "${WEB_LISTEN_ADDR}"; then
	echo "public-domain mode requires web.listen_addr to stay on loopback; found ${WEB_LISTEN_ADDR:-<missing>}" >&2
	exit 1
fi

CONFIG_PATHS_OUTPUT=$("${BIN_LINK}" inspect --config "${CONFIG_PATH}" config-paths 2>"${CONFIG_PATHS_ERR}") || {
	cat "${CONFIG_PATHS_ERR}" >&2
	exit 1
}

STATE_DIR=$(extract_config_value "state_dir" "${CONFIG_PATHS_OUTPUT}")
DATABASE_DIR=$(extract_config_value "database_dir" "${CONFIG_PATHS_OUTPUT}")
STORAGE_ROOT=$(extract_config_value "storage_root" "${CONFIG_PATHS_OUTPUT}")
if [ -z "${STATE_DIR}" ] || [ -z "${DATABASE_DIR}" ] || [ -z "${STORAGE_ROOT}" ]; then
	echo "inspect config-paths returned incomplete output" >&2
	exit 1
fi

ensure_owned_dir "${VAR_DIR}"
ensure_owned_dir "${STATE_DIR}"
ensure_owned_dir "${DATABASE_DIR}"
ensure_owned_dir "${STORAGE_ROOT}"

# Generate the canonical unit via `gistclaw inspect systemd-unit`.
GISTCLAW_SYSTEMD_BINARY_PATH="${BIN_LINK}" \
GISTCLAW_SYSTEMD_CONFIG_PATH="${CONFIG_PATH}" \
	"${BIN_LINK}" inspect --config "${CONFIG_PATH}" systemd-unit > "${SERVICE_PATH}"

systemctl daemon-reload
systemctl enable gistclaw
if systemctl is-active --quiet gistclaw; then
	systemctl restart gistclaw
else
	systemctl start gistclaw
fi

if [ -n "${PUBLIC_DOMAIN}" ]; then
	apt-get update
	apt-get install -y caddy
	mkdir -p "${CADDY_ETC_DIR}"
	cat > "${CADDYFILE_PATH}" <<EOF
${PUBLIC_DOMAIN} {
	reverse_proxy ${WEB_LISTEN_ADDR}
}
EOF
	chmod 644 "${CADDYFILE_PATH}"
	systemctl enable --now caddy
	systemctl restart caddy
fi

cat <<EOF
Installed GistClaw ${VERSION}.

Next commands:
  gistclaw version
  systemctl status gistclaw
  journalctl -u gistclaw -n 100 --no-pager
  gistclaw doctor --config ${CONFIG_PATH}
  gistclaw security audit --config ${CONFIG_PATH}
EOF

if [ -n "${PUBLIC_DOMAIN}" ]; then
	cat <<EOF
  gistclaw auth set-password --config ${CONFIG_PATH}
  systemctl status caddy
  curl -I https://${PUBLIC_DOMAIN}/login
EOF
fi
