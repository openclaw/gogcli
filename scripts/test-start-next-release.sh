#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/repo/scripts" "$tmp/repo/internal/cmd" "$tmp/bin"
cp "$root/scripts/start-next-release.sh" "$tmp/repo/scripts/start-next-release.sh"
cat > "$tmp/repo/CHANGELOG.md" <<'EOF'
# Changelog

## 0.33.1 - 2026-07-10

- Release contract fixture.

## 0.33.0 - 2026-07-06
EOF
printf '%s\n' v0.33.1 > "$tmp/repo/internal/cmd/VERSION"
cat > "$tmp/bin/git" <<'EOF'
#!/usr/bin/env bash
case "${1:-} ${2:-}" in
  'branch --show-current') printf '%s\n' main ;;
  'status --porcelain') ;;
  *) exit 2 ;;
esac
EOF
chmod +x "$tmp/bin/git" "$tmp/repo/scripts/start-next-release.sh"

PATH="$tmp/bin:/usr/bin:/bin" "$tmp/repo/scripts/start-next-release.sh" v0.33.1 >/dev/null
[[ "$(tr -d '[:space:]' < "$tmp/repo/internal/cmd/VERSION")" == v0.33.1-dev ]]
[[ "$(grep -Fc '## 0.33.2 - Unreleased' "$tmp/repo/CHANGELOG.md")" == 1 ]]
[[ "$(grep -Fc '## 0.33.1 - 2026-07-10' "$tmp/repo/CHANGELOG.md")" == 1 ]]
if PATH="$tmp/bin:/usr/bin:/bin" "$tmp/repo/scripts/start-next-release.sh" v0.33.1 >/dev/null 2>&1; then
  echo "next release test: repeated closeout unexpectedly succeeded" >&2
  exit 1
fi

echo "next release closeout tests passed"
