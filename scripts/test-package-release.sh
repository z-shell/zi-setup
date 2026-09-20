#!/usr/bin/env bash
set -euo pipefail

# scripts/test-package-release.sh
# Comprehensive dry-run test suite for release packaging.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PKG_SCRIPT="${SCRIPT_DIR}/package-release.sh"

echo "==> [1/6] Validating shell syntax..."
bash -n "${PKG_SCRIPT}"
bash -n "${BASH_SOURCE[0]}"
echo "    Shell syntax OK."

echo "==> [2/6] Testing invalid version rejection..."
# No argument
if "${PKG_SCRIPT}" 2>/dev/null; then
  echo "FAIL: Expected failure with no version argument" >&2
  exit 1
fi

# Invalid versions (no dev fallback allowed)
for bad_ver in "dev" "1.0.0" "v1.2" "v01.0.0" "v1.0.0-beta" "release"; do
  if "${PKG_SCRIPT}" "${bad_ver}" 2>/dev/null; then
    echo "FAIL: Expected failure for invalid version '${bad_ver}'" >&2
    exit 1
  fi
done
echo "    Invalid versions properly rejected."

TEST_DIR="$(mktemp -d)"
trap 'rm -rf "${TEST_DIR}"' EXIT

TEST_VERSION="v9.9.9"
OUT1="${TEST_DIR}/run1"
OUT2="${TEST_DIR}/run2"
export SOURCE_DATE_EPOCH=1704067200

echo "==> [3/6] Running dry-run package (Run 1)..."
"${PKG_SCRIPT}" "${TEST_VERSION}" "${OUT1}"

EXPECTED_FILES=(
  "zi-setup_${TEST_VERSION}_linux_amd64.tar.gz"
  "zi-setup_${TEST_VERSION}_linux_arm64.tar.gz"
  "zi-setup_${TEST_VERSION}_darwin_amd64.tar.gz"
  "zi-setup_${TEST_VERSION}_darwin_arm64.tar.gz"
  "zi-setup_${TEST_VERSION}_windows_amd64_requires-sh-on-path.zip"
  "zi-setup_${TEST_VERSION}_windows_arm64_requires-sh-on-path.zip"
  "SHA256SUMS"
)

echo "==> [4/6] Verifying generated archives and manifest..."
for f in "${EXPECTED_FILES[@]}"; do
  if [ ! -f "${OUT1}/${f}" ]; then
    echo "FAIL: Missing expected output file ${f}" >&2
    exit 1
  fi
done

# Verify sorted SHA256SUMS
(
  cd "${OUT1}"
  sha256sum --check SHA256SUMS >/dev/null
  # Verify sorted order
  if ! LC_ALL=C sort -c -k2,2 SHA256SUMS; then
    echo "FAIL: SHA256SUMS is not sorted by filename" >&2
    exit 1
  fi
)
echo "    Checksum manifest verified and sorted."

# Verify contents of a tar.gz archive
EXTRACT_LINUX="${TEST_DIR}/extracted_linux"
mkdir -p "${EXTRACT_LINUX}"
tar -xzf "${OUT1}/zi-setup_${TEST_VERSION}_linux_amd64.tar.gz" -C "${EXTRACT_LINUX}"

if [ ! -f "${EXTRACT_LINUX}/zi-setup" ] || [ ! -x "${EXTRACT_LINUX}/zi-setup" ]; then
  echo "FAIL: Linux archive missing executable zi-setup binary" >&2
  exit 1
fi
if ! cmp -s "${REPO_ROOT}/LICENSE" "${EXTRACT_LINUX}/LICENSE"; then
  echo "FAIL: Extracted LICENSE does not match repository LICENSE" >&2
  exit 1
fi
if ! cmp -s "${REPO_ROOT}/docs/README.md" "${EXTRACT_LINUX}/README.md"; then
  echo "FAIL: Extracted README.md does not match repository docs/README.md" >&2
  exit 1
fi

# Verify version injected into binary
bin_ver="$("${EXTRACT_LINUX}/zi-setup" -version)"
if [[ "$bin_ver" != "zi-setup ${TEST_VERSION}"* ]]; then
  echo "FAIL: Expected 'zi-setup ${TEST_VERSION}', got '${bin_ver}'" >&2
  exit 1
fi

# Verify contents of a windows zip archive
EXTRACT_WIN="${TEST_DIR}/extracted_windows"
mkdir -p "${EXTRACT_WIN}"
if command -v unzip >/dev/null 2>&1; then
  unzip -q "${OUT1}/zi-setup_${TEST_VERSION}_windows_amd64_requires-sh-on-path.zip" -d "${EXTRACT_WIN}"
else
  python3 -m zipfile -e "${OUT1}/zi-setup_${TEST_VERSION}_windows_amd64_requires-sh-on-path.zip" "${EXTRACT_WIN}"
fi

if [ ! -f "${EXTRACT_WIN}/zi-setup.exe" ]; then
  echo "FAIL: Windows archive missing zi-setup.exe" >&2
  exit 1
fi
if ! cmp -s "${REPO_ROOT}/LICENSE" "${EXTRACT_WIN}/LICENSE"; then
  echo "FAIL: Extracted Windows LICENSE does not match repository LICENSE" >&2
  exit 1
fi
if ! cmp -s "${REPO_ROOT}/docs/README.md" "${EXTRACT_WIN}/README.md"; then
  echo "FAIL: Extracted Windows README.md does not match repository docs/README.md" >&2
  exit 1
fi
echo "    Archive contents and version injection verified."

echo "==> [5/6] Verifying reproducibility across runs (Run 2)..."
"${PKG_SCRIPT}" "${TEST_VERSION}" "${OUT2}"

for f in "${EXPECTED_FILES[@]}"; do
  if ! cmp -s "${OUT1}/${f}" "${OUT2}/${f}"; then
    echo "FAIL: Non-deterministic output detected for ${f}" >&2
    diff <(sha256sum "${OUT1}/${f}") <(sha256sum "${OUT2}/${f}") || true
    exit 1
  fi
done
echo "    Reproducibility verified: byte-for-byte identical output across independent runs."

echo "==> [6/6] All release packaging tests passed successfully!"
