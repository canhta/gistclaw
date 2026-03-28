#!/bin/sh
set -eu

output_dir=${1:-internal/web/appdist}
max_total_kb=${RELEASE_BUNDLE_MAX_TOTAL_KB:-700}
max_js_gzip_bytes=${RELEASE_BUNDLE_MAX_JS_GZIP_BYTES:-71680}
max_css_gzip_bytes=${RELEASE_BUNDLE_MAX_CSS_GZIP_BYTES:-0}

if [ ! -d "${output_dir}" ]; then
  echo "bundle budget failed: output directory not found: ${output_dir}" >&2
  exit 1
fi

measure_largest() {
  pattern=$1
  list_file=$(mktemp)
  largest_path=
  largest_bytes=0
  largest_gzip_bytes=0

  find "${output_dir}" -type f -name "${pattern}" -print >"${list_file}"
  while IFS= read -r file; do
    [ -n "${file}" ] || continue
    bytes=$(wc -c <"${file}" | tr -d '[:space:]')
    gzip_bytes=$(gzip -c "${file}" | wc -c | tr -d '[:space:]')
    if [ "${gzip_bytes}" -gt "${largest_gzip_bytes}" ]; then
      largest_path=${file}
      largest_bytes=${bytes}
      largest_gzip_bytes=${gzip_bytes}
    fi
  done <"${list_file}"
  rm -f "${list_file}"

  printf '%s\n%s\n%s\n' "${largest_path}" "${largest_bytes}" "${largest_gzip_bytes}"
}

bundle_total_kb=$(du -sk "${output_dir}" | awk '{print $1}')

js_metrics=$(mktemp)
css_metrics=$(mktemp)
trap 'rm -f "${js_metrics}" "${css_metrics}"' EXIT INT TERM

measure_largest '*.js' >"${js_metrics}"
measure_largest '*.css' >"${css_metrics}"

largest_js_path=$(sed -n '1p' "${js_metrics}")
largest_js_bytes=$(sed -n '2p' "${js_metrics}")
largest_js_gzip_bytes=$(sed -n '3p' "${js_metrics}")
largest_css_path=$(sed -n '1p' "${css_metrics}")
largest_css_bytes=$(sed -n '2p' "${css_metrics}")
largest_css_gzip_bytes=$(sed -n '3p' "${css_metrics}")

echo "bundle_total_kb=${bundle_total_kb}"
echo "largest_js_path=${largest_js_path}"
echo "largest_js_bytes=${largest_js_bytes}"
echo "largest_js_gzip_bytes=${largest_js_gzip_bytes}"
echo "largest_css_path=${largest_css_path}"
echo "largest_css_bytes=${largest_css_bytes}"
echo "largest_css_gzip_bytes=${largest_css_gzip_bytes}"

if [ "${bundle_total_kb}" -gt "${max_total_kb}" ]; then
  echo "bundle budget failed: total output ${bundle_total_kb} KB exceeds ${max_total_kb} KB" >&2
  exit 1
fi

if [ "${largest_js_gzip_bytes}" -gt "${max_js_gzip_bytes}" ]; then
  echo "bundle budget failed: largest JS gzip size ${largest_js_gzip_bytes} exceeds ${max_js_gzip_bytes} bytes" >&2
  exit 1
fi

if [ "${max_css_gzip_bytes}" -gt 0 ] && [ "${largest_css_gzip_bytes}" -gt "${max_css_gzip_bytes}" ]; then
  echo "bundle budget failed: largest CSS gzip size ${largest_css_gzip_bytes} exceeds ${max_css_gzip_bytes} bytes" >&2
  exit 1
fi
