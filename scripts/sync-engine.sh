#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  printf '%s\n' 'usage: scripts/sync-engine.sh /path/to/z-shell/src FULL_COMMIT_SHA' >&2
  exit 2
fi

source_root=$1
revision=$2
destination=internal/engine/bundled/public
case "${revision}" in
????????????????????????????????????????) ;;
*)
  printf '%s\n' 'FULL_COMMIT_SHA must contain exactly 40 hexadecimal characters' >&2
  exit 2
  ;;
esac
case "${revision}" in *[!0-9a-f]*)
  printf '%s\n' 'FULL_COMMIT_SHA must contain exactly 40 lowercase hexadecimal characters' >&2
  exit 2
  ;;
esac

resolved=$(git -C "${source_root}" rev-parse --verify "${revision}^{commit}" 2>/dev/null) || {
  printf 'cannot resolve source commit %s\n' "${revision}" >&2
  exit 2
}
[ "${resolved}" = "${revision}" ] || {
  printf 'source commit resolved to %s, expected %s\n' "${resolved}" "${revision}" >&2
  exit 2
}

work=$(mktemp -d "${TMPDIR:-/tmp}/zi-setup-engine.XXXXXXXX")
trap 'rm -rf "${work}"' EXIT INT TERM HUP
umask 077

for path in \
  public/sh/setup.sh \
  public/setup/profiles.tsv \
  public/zsh/init.zsh \
  public/checksum.txt
do
  git -C "${source_root}" cat-file -e "${revision}:${path}" 2>/dev/null || {
    printf 'source commit does not contain %s\n' "${path}" >&2
    exit 2
  }
  mkdir -p "${work}/$(dirname "${path}")"
  git -C "${source_root}" show "${revision}:${path}" >"${work}/${path}"
done

mkdir -p "${destination}/sh" "${destination}/setup" "${destination}/zsh"
cp "${work}/public/sh/setup.sh" "${destination}/sh/setup.sh"
cp "${work}/public/setup/profiles.tsv" "${destination}/setup/profiles.tsv"
cp "${work}/public/zsh/init.zsh" "${destination}/zsh/init.zsh"
cp "${work}/public/checksum.txt" "${destination}/checksum.txt"

printf 'Copied engine assets from %s.\n' "${revision}"
printf '%s\n' 'Update BundledEngineRevision and bundledHashes in internal/engine/bundle.go with:'
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum \
    "${destination}/sh/setup.sh" \
    "${destination}/setup/profiles.tsv" \
    "${destination}/zsh/init.zsh" \
    "${destination}/checksum.txt"
else
  shasum -a 256 \
    "${destination}/sh/setup.sh" \
    "${destination}/setup/profiles.tsv" \
    "${destination}/zsh/init.zsh" \
    "${destination}/checksum.txt"
fi
