#!/bin/bash
# SEDMAT v7 — Full Test Runner
# Usage: ./testdata/v7_run.sh <docId> <account>
set -euo pipefail

DOC="${1:?Usage: $0 <docId> <account>}"
ACCT="${2:?Usage: $0 <docId> <account>}"
DIR="$(cd "$(dirname "$0")" && pwd)"
GOG="${DIR}/../gog"

echo "=== SEDMAT v7 Test Runner ==="
echo "Doc: $DOC | Account: $ACCT"

# Step 0: Clear
echo "--- Clearing ---"
$GOG docs sed "$DOC" -a "$ACCT" 's/.*\n//g' 2>/dev/null || true
for i in $(seq 1 20); do
    $GOG docs sed "$DOC" -a "$ACCT" 's/|1|//' 2>&1 | grep -q "out of range\|no tables" && break
done 2>/dev/null
$GOG docs sed "$DOC" -a "$ACCT" 's/.*\n//g' 2>/dev/null || true
sleep 2

# Step 1: Seed
echo "--- Seeding ---"
# Build seed expression from file
SEED_EXPR="s/\$/"
while IFS= read -r line; do
    [ -z "$line" ] && continue
    # Escape forward slashes for sed delimiter
    line=$(echo "$line" | sed 's/\//\\\//g')
    SEED_EXPR="${SEED_EXPR}${line}\\n"
done < "$DIR/v7_seed.txt"
SEED_EXPR="${SEED_EXPR%\\n}/"
$GOG docs sed "$DOC" -a "$ACCT" "$SEED_EXPR"
sleep 2

# Step 2: Main formatting
echo "--- v7_test.sed ---"
grep -v '^#' "$DIR/v7_test.sed" | grep -v '^$' | $GOG docs sed "$DOC" -a "$ACCT"
sleep 3

# Step 3: Table operations
echo "--- v7_table_ops.sed ---"
grep -v '^#' "$DIR/v7_table_ops.sed" | grep -v '^$' | $GOG docs sed "$DOC" -a "$ACCT"

echo ""
echo "=== Done ==="
echo "View: https://docs.google.com/document/d/$DOC/edit"
