#!/usr/bin/env bash
set -euo pipefail

binary=${1:?usage: check-darwin-minos.sh <binary>}
architectures=$(lipo -archs "$binary")
[[ -n "$architectures" ]] || { echo "release: no Mach-O slices in $binary" >&2; exit 1; }
for arch in $architectures; do
  minos=$(otool -arch "$arch" -l "$binary" | awk '
    $1 == "cmd" { build_version = ($2 == "LC_BUILD_VERSION") }
    build_version && $1 == "minos" { print $2 }
  ')
  if [[ "$minos" != 15.0 ]]; then
    echo "release: $binary ($arch) requires minos 15.0; observed ${minos:-missing}" >&2
    exit 1
  fi
  echo "release: $binary ($arch) minos $minos"
done
