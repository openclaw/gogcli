#!/usr/bin/env bash
set -euo pipefail

response_file=${1:-}
seen_ids_file=${2:-}
run_file=${3:-}
expected_workflow_id=${4:-}
expected_title=${5:-}
expected_branch=${6:-}
expected_sha=${7:-}
created_after=${8:-}

if [[ ! -f "$response_file" || -L "$response_file" ||
  ! -f "$seen_ids_file" || -L "$seen_ids_file" ||
  ! -f "$run_file" || -L "$run_file" ||
  ! "$expected_workflow_id" =~ ^[0-9]+$ ||
  -z "$expected_title" ||
  ! "$expected_branch" =~ ^[A-Za-z0-9._/-]+$ ||
  ! "$expected_sha" =~ ^[[:xdigit:]]{40}$ ||
  ! "$created_after" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T ]]; then
  echo "usage: $0 response.json seen-run-ids.json run.json workflow-id expected-title expected-branch expected-sha created-after" >&2
  exit 2
fi
command -v jq >/dev/null 2>&1 || {
  echo "release dispatch: jq is required" >&2
  exit 1
}

run_id=$(jq -er '
  .workflow_run_id |
  select(type == "number" and . > 0 and . == floor) |
  tostring
' "$response_file")
api_url="https://api.github.com/repos/openclaw/gogcli/actions/runs/$run_id"
html_url="https://github.com/openclaw/gogcli/actions/runs/$run_id"

jq -e \
  --argjson id "$run_id" \
  --arg api_url "$api_url" \
  --arg html_url "$html_url" '
    type == "object" and
    .workflow_run_id == $id and
    .run_url == $api_url and
    .html_url == $html_url
  ' "$response_file" >/dev/null || {
  echo "release dispatch: response does not identify the exact GogCLI workflow run" >&2
  exit 1
}

jq -e \
  --argjson id "$run_id" '
    type == "array" and
    all(.[]; type == "number" and . > 0 and . == floor) and
    index($id) == null
  ' "$seen_ids_file" >/dev/null || {
  echo "release dispatch: returned workflow run ID existed before dispatch" >&2
  exit 1
}

jq -e \
  --argjson id "$run_id" \
  --argjson workflow_id "$expected_workflow_id" \
  --arg title "$expected_title" \
  --arg branch "$expected_branch" \
  --arg sha "$expected_sha" \
  --arg created_after "$created_after" '
    type == "object" and
    .id == $id and
    .workflow_id == $workflow_id and
    .path == ".github/workflows/release-assets.yml" and
    .event == "workflow_dispatch" and
    .display_title == $title and
    .head_branch == $branch and
    .head_sha == $sha and
    (.created_at | type) == "string" and
    .created_at > $created_after
  ' "$run_file" >/dev/null || {
  echo "release dispatch: returned run does not match the protected verifier request" >&2
  exit 1
}

printf '%s\n' "$run_id"
