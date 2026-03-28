#!/bin/sh
set -eu

usage() {
  cat <<'EOF'
Usage: update-homebrew-tap.sh --formula <path-to-rendered-formula>
EOF
}

formula_path=

while [ "$#" -gt 0 ]; do
  case "$1" in
    --formula)
      formula_path=${2:-}
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

if [ -z "${formula_path}" ]; then
  usage >&2
  exit 1
fi

if [ ! -f "${formula_path}" ]; then
  echo "homebrew tap update failed: formula not found: ${formula_path}" >&2
  exit 1
fi

repo=${HOMEBREW_TAP_REPO:-}
branch=${HOMEBREW_TAP_BRANCH:-main}
token=${HOMEBREW_TAP_TOKEN:-}

if [ -z "${repo}" ]; then
  echo "homebrew tap update skipped: HOMEBREW_TAP_REPO is not set" >&2
  exit 0
fi

if [ -z "${token}" ]; then
  echo "homebrew tap update skipped: HOMEBREW_TAP_TOKEN is not set" >&2
  exit 0
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "${tmpdir}"' EXIT INT TERM

remote_url="https://x-access-token:${token}@github.com/${repo}.git"
tap_dir="${tmpdir}/tap"
target_formula="${tap_dir}/Formula/gistclaw.rb"

git clone --depth 1 --branch "${branch}" "${remote_url}" "${tap_dir}"
mkdir -p "${tap_dir}/Formula"

if [ -f "${target_formula}" ] && cmp -s "${formula_path}" "${target_formula}"; then
  echo "homebrew tap update skipped: Formula/gistclaw.rb is already current"
  exit 0
fi

cp "${formula_path}" "${target_formula}"

version=$(sed -n 's/^  version "\(.*\)"/\1/p' "${formula_path}" | head -n 1)
if [ -z "${version}" ]; then
  version=unknown
fi

(
  cd "${tap_dir}"
  git config user.name "github-actions[bot]"
  git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
  git add Formula/gistclaw.rb

  if git diff --cached --quiet; then
    echo "homebrew tap update skipped: no staged changes"
    exit 0
  fi

  git commit -m "Update gistclaw ${version}"
  git push origin "${branch}"
)
