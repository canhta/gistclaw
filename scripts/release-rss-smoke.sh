#!/bin/sh
set -eu

usage() {
  cat <<'EOF'
Usage: release-rss-smoke.sh <path-to-gistclaw-binary>
EOF
}

if [ "$#" -ne 1 ]; then
  usage >&2
  exit 1
fi

binary_path=$1
if [ ! -x "${binary_path}" ]; then
  echo "RSS smoke failed: binary is not executable: ${binary_path}" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "RSS smoke failed: curl is required" >&2
  exit 1
fi

max_rss_kb=${RELEASE_RSS_MAX_KB:-40960}
port=${RELEASE_RSS_PORT:-18080}
startup_timeout_sec=${RELEASE_RSS_STARTUP_TIMEOUT_SEC:-20}

tmpdir=$(mktemp -d)
pid=

cleanup() {
  if [ -n "${pid}" ] && kill -0 "${pid}" >/dev/null 2>&1; then
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  fi
  rm -rf "${tmpdir}"
}
trap cleanup EXIT INT TERM

state_dir=${tmpdir}/state
storage_root=${tmpdir}/storage
config_path=${tmpdir}/config.yaml
log_path=${tmpdir}/serve.log

mkdir -p "${state_dir}" "${storage_root}"

cat >"${config_path}" <<EOF
storage_root: ${storage_root}
state_dir: ${state_dir}
database_path: ${state_dir}/runtime.db
provider:
  name: openai
  api_key: sk-test
web:
  listen_addr: 127.0.0.1:${port}
EOF

"${binary_path}" --config "${config_path}" serve >"${log_path}" 2>&1 &
pid=$!

port_ready() {
  curl -fsS --max-time 1 "http://127.0.0.1:${port}/" >/dev/null 2>&1
}

deadline=$(($(date +%s) + startup_timeout_sec))
while [ "$(date +%s)" -lt "${deadline}" ]; do
  if ! kill -0 "${pid}" >/dev/null 2>&1; then
    echo "RSS smoke failed: gistclaw exited before listening" >&2
    cat "${log_path}" >&2
    exit 1
  fi
  if port_ready; then
    break
  fi
  sleep 1
done

if ! port_ready; then
  echo "RSS smoke failed: gistclaw did not listen on 127.0.0.1:${port} within ${startup_timeout_sec}s" >&2
  cat "${log_path}" >&2
  exit 1
fi

sleep 1
rss_kb=$(ps -o rss= -p "${pid}" | tr -d '[:space:]')
if [ -z "${rss_kb}" ]; then
  echo "RSS smoke failed: unable to sample RSS for pid ${pid}" >&2
  cat "${log_path}" >&2
  exit 1
fi

echo "RSS_KB=${rss_kb}"
echo "rss_max_kb=${max_rss_kb}"

if [ "${rss_kb}" -gt "${max_rss_kb}" ]; then
  echo "RSS smoke failed: sample ${rss_kb} KB exceeds ${max_rss_kb} KB" >&2
  exit 1
fi
