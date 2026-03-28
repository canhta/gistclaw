#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: render-homebrew-formula.sh --version <tag> --url <tarball-url> --sha256 <checksum> [--output <path>]
EOF
}

version=""
url=""
sha256=""
output=""

while (($# > 0)); do
  case "$1" in
    --version)
      version="${2:-}"
      shift 2
      ;;
    --url)
      url="${2:-}"
      shift 2
      ;;
    --sha256)
      sha256="${2:-}"
      shift 2
      ;;
    --output)
      output="${2:-}"
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

if [[ -z "${version}" || -z "${url}" || -z "${sha256}" ]]; then
  usage >&2
  exit 1
fi

script_dir="$(cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
template_path="${script_dir}/../packaging/homebrew/gistclaw.rb.tmpl"
formula_version="${version#v}"

escape_sed() {
  printf '%s' "$1" | sed 's/[&|]/\\&/g'
}

rendered="$(sed \
  -e "s|__VERSION__|$(escape_sed "${formula_version}")|g" \
  -e "s|__URL__|$(escape_sed "${url}")|g" \
  -e "s|__SHA256__|$(escape_sed "${sha256}")|g" \
  "${template_path}")"

if [[ -n "${output}" ]]; then
  mkdir -p "$(dirname "${output}")"
  printf '%s\n' "${rendered}" >"${output}"
  exit 0
fi

printf '%s\n' "${rendered}"
