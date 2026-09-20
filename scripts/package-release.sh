#!/usr/bin/env bash
set -euo pipefail

# scripts/package-release.sh
# Packages reproducible cross-platform release distributions for zi-setup.
# Supported targets:
#   - linux/amd64 (.tar.gz)
#   - linux/arm64 (.tar.gz)
#   - darwin/amd64 (.tar.gz)
#   - darwin/arm64 (.tar.gz)
#   - windows/amd64 (.zip, requires sh on PATH)
#   - windows/arm64 (.zip, requires sh on PATH)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

usage() {
  cat <<EOF >&2
Usage: $0 <version> [output-dir] [target-os] [target-arch]

Arguments:
  <version>      Strict semantic version tag (e.g. v1.2.3). Required.
                 Must match ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$
  [output-dir]   Output directory for archives and checksums (default: dist)
  [target-os]    Optional target OS (linux, darwin, windows)
  [target-arch]  Optional target architecture (amd64, arm64)

When [target-os] and [target-arch] are omitted, all 6 supported targets are packaged
and SHA256SUMS is generated.
EOF
  exit 1
}

if [ $# -lt 1 ] || [ -z "${1:-}" ]; then
  echo "Error: version is required." >&2
  usage
fi

VERSION="$1"
if [[ ! "$VERSION" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "Error: version '${VERSION}' does not follow strict semantic format vX.Y.Z (no dev fallback permitted)." >&2
  exit 1
fi

OUTPUT_DIR="${2:-dist}"
TARGET_OS="${3:-}"
TARGET_ARCH="${4:-}"

# Ensure output directory exists and resolve absolute path
mkdir -p "${OUTPUT_DIR}"
OUTPUT_DIR="$(cd "${OUTPUT_DIR}" && pwd)"

# Verify required files in repository
if [ ! -f "${REPO_ROOT}/LICENSE" ]; then
  echo "Error: LICENSE file not found at ${REPO_ROOT}/LICENSE" >&2
  exit 1
fi

if [ ! -f "${REPO_ROOT}/docs/README.md" ]; then
  echo "Error: README not found at ${REPO_ROOT}/docs/README.md" >&2
  exit 1
fi

# Determine SOURCE_DATE_EPOCH for reproducible timestamps
if [ -z "${SOURCE_DATE_EPOCH:-}" ]; then
  SOURCE_DATE_EPOCH="$(git -C "${REPO_ROOT}" log -1 --format=%ct 2>/dev/null || echo 1704067200)"
fi

# Format timestamp for touch -t: YYYYMMDDhhmm.ss
if date -u -d "@${SOURCE_DATE_EPOCH}" +%Y%m%d%H%M.%S >/dev/null 2>&1; then
  TOUCH_TIME="$(date -u -d "@${SOURCE_DATE_EPOCH}" +%Y%m%d%H%M.%S)"
elif python3 -c "import datetime, timezone; print(datetime.datetime.fromtimestamp(${SOURCE_DATE_EPOCH}, tz=datetime.timezone.utc).strftime('%Y%m%d%H%M.%S'))" >/dev/null 2>&1; then
  TOUCH_TIME="$(python3 -c "import datetime, timezone; print(datetime.datetime.fromtimestamp(${SOURCE_DATE_EPOCH}, tz=datetime.timezone.utc).strftime('%Y%m%d%H%M.%S'))")"
else
  TOUCH_TIME="202401010000.00"
fi

package_target() {
  local os="$1"
  local arch="$2"
  local ext=".tar.gz"
  local bin_name="zi-setup"
  local win_suffix=""

  if [ "$os" = "windows" ]; then
    ext=".zip"
    bin_name="zi-setup.exe"
    win_suffix="_requires-sh-on-path"
  fi

  local archive_name="zi-setup_${VERSION}_${os}_${arch}${win_suffix}${ext}"
  local archive_path="${OUTPUT_DIR}/${archive_name}"

  echo "==> Packaging ${archive_name} (${os}/${arch})..."

  local stage_dir
  stage_dir="$(mktemp -d)"

  # Build binary: Go 1.26, CGO_ENABLED=0, -trimpath, blank build ID, inject main.version
  (
    cd "${REPO_ROOT}"
    CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build \
      -trimpath \
      -ldflags="-buildid= -X main.version=${VERSION}" \
      -o "${stage_dir}/${bin_name}" \
      ./cmd/zi-setup
  )

  # Copy LICENSE and docs/README.md stored as README.md
  cp "${REPO_ROOT}/LICENSE" "${stage_dir}/LICENSE"
  cp "${REPO_ROOT}/docs/README.md" "${stage_dir}/README.md"

  # Normalize permissions
  chmod 0755 "${stage_dir}/${bin_name}"
  chmod 0644 "${stage_dir}/LICENSE" "${stage_dir}/README.md"

  # Normalize timestamps
  touch -t "${TOUCH_TIME}" "${stage_dir}/LICENSE" "${stage_dir}/README.md" "${stage_dir}/${bin_name}"

  # Create deterministic archive
  rm -f "${archive_path}"
  if [ "$os" = "windows" ]; then
    (
      cd "${stage_dir}"
      # -X: strip extra file attributes (UID/GID, variable timestamps)
      # -q: quiet
      zip -q -X "${archive_path}" LICENSE README.md "${bin_name}"
    )
  else
    local tar_flags=()
    if tar --version 2>&1 | grep -q "GNU tar"; then
      tar_flags+=(--owner=0 --group=0 --numeric-owner)
    fi
    (
      cd "${stage_dir}"
      # Provide files in explicit sorted order: LICENSE README.md zi-setup
      # gzip -n suppresses filename and timestamp in gzip header
      tar "${tar_flags[@]}" -cf - LICENSE README.md "${bin_name}" | gzip -n > "${archive_path}"
    )
  fi

  rm -rf "${stage_dir}"
}

ALL_TARGETS=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
  "windows arm64"
)

if [ -n "${TARGET_OS}" ] && [ -n "${TARGET_ARCH}" ]; then
  package_target "${TARGET_OS}" "${TARGET_ARCH}"
else
  for target in "${ALL_TARGETS[@]}"; do
    # shellcheck disable=SC2086
    package_target $target
  done

  # Generate sorted SHA-256 checksums manifest
  echo "==> Generating SHA256SUMS manifest..."
  (
    cd "${OUTPUT_DIR}"
    sha256sum -- *.tar.gz *.zip | LC_ALL=C sort -k2,2 > SHA256SUMS
    sha256sum --check SHA256SUMS
  )
  echo "==> Successfully packaged all targets and generated ${OUTPUT_DIR}/SHA256SUMS"
fi
