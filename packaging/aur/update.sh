#!/usr/bin/env bash
# Fills the AUR PKGBUILD for a release and regenerates .SRCINFO.
#
#   packaging/aur/update.sh VERSION CHECKSUMS
#
# VERSION without "v" (1.2.3), CHECKSUMS = the release's checksums.txt.
# Needs makepkg (Arch Linux); must not run as root.
set -euo pipefail

version=${1:?usage: update.sh VERSION CHECKSUMS}
sums=${2:?usage: update.sh VERSION CHECKSUMS}
dir=$(cd "$(dirname "$0")/doormutex-companion-bin" && pwd)

sum() {
  local s
  s=$(awk -v f="doormutex-companion_${version}_linux_$1.tar.gz" '$2 == f { print $1 }' "$sums")
  [[ -n $s ]] || { echo "update.sh: no checksum for $1 in $sums" >&2; exit 1; }
  echo "$s"
}
amd64=$(sum amd64)
arm64=$(sum arm64)

sed -i \
  -e "s/^pkgver=.*/pkgver=${version}/" \
  -e "s/^pkgrel=.*/pkgrel=1/" \
  -e "s/^sha256sums_x86_64=.*/sha256sums_x86_64=('${amd64}')/" \
  -e "s/^sha256sums_aarch64=.*/sha256sums_aarch64=('${arm64}')/" \
  "$dir/PKGBUILD"
(cd "$dir" && makepkg --printsrcinfo > .SRCINFO)
echo "PKGBUILD updated to ${version}"
