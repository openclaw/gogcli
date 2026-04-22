#!/usr/bin/env bash
# Regenerate .fork/patches/ from commits carrying a Fork-Patch: trailer.
# Emits numbered .patch files, rebuilds series, injects Reason: headers.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
cd "$repo_root"

patches_dir=".fork/patches"
if [[ ! -d "$patches_dir" ]]; then
  echo "error: $patches_dir not found" >&2
  exit 1
fi

if ! git rev-parse --verify --quiet upstream >/dev/null; then
  echo "error: 'upstream' branch does not exist; cannot compute upstream..HEAD" >&2
  exit 1
fi

# Clean old patch files but keep series and README.
find "$patches_dir" -maxdepth 1 -type f -name '*.patch' -delete

mapfile -t emitted < <(git format-patch upstream..HEAD \
  --grep='Fork-Patch:' \
  -o "$patches_dir" \
  2>/dev/null || true)

# Rebuild series from the emitted files, in emission order (numbered).
series_file="$patches_dir/series"
: >"$series_file"
for p in "${emitted[@]}"; do
  [[ -n "$p" ]] || continue
  basename "$p" >>"$series_file"
done

# Inject a Reason: header at the top of each patch if the source commit had one.
for p in "${emitted[@]}"; do
  [[ -n "$p" ]] || continue
  # The first line of a format-patch file is "From <sha> <date>".
  sha="$(head -n1 "$p" | awk '{print $2}')"
  reason="$(git log -1 --format=%B "$sha" 2>/dev/null | grep -E '^Reason:' | head -n1 || true)"
  if [[ -n "$reason" ]]; then
    # Insert Reason: immediately after the "Subject:" line.
    tmp="$(mktemp)"
    awk -v reason="$reason" '
      BEGIN { inserted = 0 }
      /^Subject:/ && !inserted { print; print reason; inserted = 1; next }
      { print }
    ' "$p" >"$tmp"
    mv "$tmp" "$p"
  fi
done

echo "export-patches.sh: wrote ${#emitted[@]} patch(es) to $patches_dir"
exit 0
