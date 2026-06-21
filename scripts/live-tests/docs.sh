#!/usr/bin/env bash

set -euo pipefail

run_docs_tests() {
  if skip "docs"; then
    echo "==> docs (skipped)"
    return 0
  fi

  local doc_json doc_id copy_json copy_id export_path markdown_path replacement_path tab_markdown_path tab_export_path
  doc_json=$(gog docs create "gogcli-smoke-doc-$TS" --json)
  doc_id=$(extract_id "$doc_json")
  [ -n "$doc_id" ] || { echo "Failed to parse doc id" >&2; exit 1; }
  register_drive_cleanup "$doc_id"

  markdown_path="$LIVE_TMP/docs-recent-$TS.md"
  replacement_path="$LIVE_TMP/docs-replacement-$TS.md"
  tab_markdown_path="$LIVE_TMP/docs-tab-recent-$TS.md"
  tab_export_path="$LIVE_TMP/docs-tab-export-$TS.txt"
  printf '# Heading {#heading}\n\nAnchorTarget and LinkTarget.\n\nEmoji 😀 range.\n\nOrphanTarget remains quoted.\n\nInsertHere UpdateHere DeleteHere PersonHere BreakHere\n\nBatchAnchor.\n\n~~Strike~~ and `code`.\n\n- Parent\n  - Child\n' >"$markdown_path"
  printf '# Replacement\n\nNo quoted content remains.\n' >"$replacement_path"
  printf '# Recent Tab {#recent-tab}\n\n```go\nfmt.Println("ok")\n```\n\n| Solo |\n| --- |\n| value |\n| --- |\n\n| Kind | Steps |\n| --- | --- |\n| nested | - parent<br>  - child |\n' >"$tab_markdown_path"

  run_required "docs" "docs markdown write" gog docs write "$doc_id" \
    --file "$markdown_path" --replace --markdown --json >/dev/null

  local range_json
  range_json=$(gog docs find-range "$doc_id" "😀" --json)
  "$PY" -c 'import json,sys
obj=json.load(sys.stdin)
def walk(value):
    if isinstance(value, dict):
        if value.get("endIndex", 0) - value.get("startIndex", 0) == 2:
            return True
        return any(walk(v) for v in value.values())
    if isinstance(value, list):
        return any(walk(v) for v in value)
    return False
assert walk(obj)' <<<"$range_json"

  run_required "docs" "docs format link and code" gog docs format "$doc_id" \
    --match LinkTarget --link https://example.com --code --json >/dev/null
  local raw_json
  raw_json=$(gog docs raw "$doc_id" --json)
  "$PY" -c 'import json,sys
obj=json.load(sys.stdin)
def walk(value):
    if isinstance(value, dict):
        if value.get("link", {}).get("url") == "https://example.com":
            return True
        return any(walk(v) for v in value.values())
    if isinstance(value, list):
        return any(walk(v) for v in value)
    return False
assert walk(obj)' <<<"$raw_json"
  run_required "docs" "docs format clear link" gog docs format "$doc_id" \
    --match LinkTarget --no-link --json >/dev/null

  run_required "docs" "docs add recent tab" gog docs add-tab "$doc_id" \
    --title Recent --json >/dev/null
  run_required "docs" "docs markdown write recent tab" gog docs write "$doc_id" \
    --file "$tab_markdown_path" --replace --markdown --tab Recent --json >/dev/null
  local tab_raw_json
  tab_raw_json=$(gog docs raw "$doc_id" --tab Recent --json)
  "$PY" -c 'import json,sys
obj=json.load(sys.stdin)
def walk(value):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from walk(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk(child)
objects=list(walk(obj))
assert sum("tableRows" in item for item in objects) == 2
assert any(item.get("textRun", {}).get("content", "").strip() == "---" for item in objects)
assert any(item.get("bullet", {}).get("nestingLevel") == 1 for item in objects)
assert any(item.get("textStyle", {}).get("weightedFontFamily", {}).get("fontFamily") == "Roboto Mono" for item in objects)
assert not any("{#recent-tab}" in value for item in objects for value in item.values() if isinstance(value, str))' <<<"$tab_raw_json"
  run_required "docs" "docs export recent tab" gog docs export "$doc_id" \
    --tab Recent --format txt --out "$tab_export_path" >/dev/null
  [ -s "$tab_export_path" ] || { echo "Docs tab export was empty" >&2; exit 1; }

  run_required "docs" "docs named range create" gog docs named-range create "$doc_id" \
    --name smoke_anchor --at AnchorTarget --json >/dev/null
  run_required "docs" "docs named range list" gog docs named-range list "$doc_id" \
    --name smoke_anchor --json >/dev/null
  run_required "docs" "docs named range replace" gog docs named-range replace "$doc_id" \
    smoke_anchor --text AnchorReplaced --json >/dev/null

  local since comment_json comment_id comments_state
  since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  comment_json=$(gog docs comments add "$doc_id" "gogcli locate $TS" --quoted OrphanTarget --json)
  comment_id=$(extract_id "$comment_json")
  [ -n "$comment_id" ] || { echo "Failed to parse Docs comment id" >&2; exit 1; }
  run_required "docs" "docs comments since" gog docs comments list "$doc_id" \
    --since "$since" --json >/dev/null
  run_required "docs" "docs comments locate" gog docs comments locate "$doc_id" \
    "$comment_id" --json >/dev/null
  comments_state="$LIVE_TMP/docs-comments-$TS.json"
  run_required "docs" "docs comments poll" gog docs comments poll "$doc_id" \
    --state-file "$comments_state" --max-iterations 1 --interval 1ms --json >/dev/null
  [ -s "$comments_state" ] || { echo "Docs comments poll did not persist state" >&2; exit 1; }

  local orphan_rc
  if gog docs write "$doc_id" --file "$replacement_path" --replace --markdown \
    --check-orphans --json >/dev/null 2>"$LIVE_TMP/docs-orphan-$TS.err"; then
    echo "Expected orphan guard to block replacement, but it succeeded" >&2
    exit 1
  else
    orphan_rc=$?
  fi
  [ "$orphan_rc" -eq 11 ] || { echo "Unexpected orphan guard exit: $orphan_rc" >&2; exit 1; }

  run_required "docs" "docs insert at anchor" gog docs insert "$doc_id" "Inserted-" \
    --at InsertHere --json >/dev/null
  run_required "docs" "docs update at anchor" gog docs update "$doc_id" \
    --at UpdateHere --text Updated --json >/dev/null
  run_required "docs" "docs delete at anchor" gog docs delete "$doc_id" \
    --at DeleteHere --json >/dev/null
  run_required "docs" "docs person chip at anchor" gog docs insert-person "$doc_id" \
    --email "$ACCOUNT" --at PersonHere --json >/dev/null
  run_required "docs" "docs page break at anchor" gog docs insert-page-break "$doc_id" \
    --at BreakHere --json >/dev/null

  local footnote_json header_json header_id footer_json footer_id segment_raw_json
  footnote_json=$(gog docs insert-footnote "$doc_id" --at AnchorReplaced \
    --text "gogcli live footnote $TS" --json)
  "$PY" -c 'import json,sys; assert json.load(sys.stdin)["footnoteId"]' <<<"$footnote_json"
  run_required "docs" "docs horizontal rule at anchor" gog docs insert-horizontal-rule "$doc_id" \
    --at AnchorReplaced --json >/dev/null
  run_required "docs" "docs continuous section break" gog docs insert-section-break "$doc_id" \
    --at BatchAnchor --type continuous --json >/dev/null
  run_required "docs" "docs section columns" gog docs section-columns "$doc_id" \
    --at Heading --count 2 --separator between --json >/dev/null

  header_json=$(gog docs header create "$doc_id" --text "Live Header $TS" --json)
  header_id=$("$PY" -c 'import json,sys; print(json.load(sys.stdin)["segmentId"])' <<<"$header_json")
  footer_json=$(gog docs footer create "$doc_id" --text "Live Footer $TS" --json)
  footer_id=$("$PY" -c 'import json,sys; print(json.load(sys.stdin)["segmentId"])' <<<"$footer_json")
  [ -n "$header_id" ] && [ -n "$footer_id" ] || { echo "Failed to create Docs header/footer" >&2; exit 1; }
  run_required "docs" "docs header list" gog docs header list "$doc_id" --json >/dev/null
  run_required "docs" "docs footer list" gog docs footer list "$doc_id" --json >/dev/null
  run_required "docs" "docs segment insert" gog docs insert "$doc_id" "!" \
    --segment "$header_id" --json >/dev/null
  run_required "docs" "docs segment update" gog docs update "$doc_id" \
    --segment "$header_id" --at Live --text Verified --json >/dev/null
  run_required "docs" "docs segment format" gog docs format "$doc_id" \
    --segment "$header_id" --match Verified --bold --json >/dev/null
  run_required "docs" "docs segment delete" gog docs delete "$doc_id" \
    --segment "$header_id" --at Header --json >/dev/null
  segment_raw_json=$(gog docs raw "$doc_id" --json)
  "$PY" -c 'import json,sys
obj=json.load(sys.stdin)
segment=sys.argv[1]
content=json.dumps(obj["headers"][segment].get("content", []))
assert "Verified" in content
assert any(run.get("textRun", {}).get("textStyle", {}).get("bold") is True for element in obj["headers"][segment].get("content", []) for para in [element.get("paragraph", {})] for run in para.get("elements", []))' "$header_id" <<<"$segment_raw_json"
  run_required "docs" "docs header delete" gog --force docs header delete "$doc_id" "$header_id" --json >/dev/null
  run_required "docs" "docs footer delete" gog --force docs footer delete "$doc_id" "$footer_id" --json >/dev/null

  local batch_id batch_json
  batch_id=$("$BIN" --account "$ACCOUNT" batch begin --service docs --doc "$doc_id" --name "live-$TS")
  [ -n "$batch_id" ] || { echo "Failed to create Docs batch" >&2; exit 1; }
  register_batch_cleanup "$batch_id"
  run_required "docs" "docs batch insert" gog docs insert "$doc_id" "BatchedText " \
    --at BatchAnchor --batch "$batch_id" --json >/dev/null
  run_required "docs" "docs batch format" gog docs format "$doc_id" \
    --match AnchorReplaced --bold --batch "$batch_id" --json >/dev/null
  batch_json=$("$BIN" batch show "$batch_id" --json)
  "$PY" -c 'import json,sys; assert len(json.load(sys.stdin)["batch"]["requests"]) >= 2' <<<"$batch_json"
  run_required "docs" "docs batch dry-run" "$BIN" --dry-run batch end "$batch_id" --json >/dev/null
  run_required "docs" "docs batch submit" "$BIN" batch end "$batch_id" --json >/dev/null

  run_required "docs" "docs insert table" gog docs insert-table "$doc_id" \
    --rows 2 --cols 2 --at-end --values-json '[["A","B"],["C","D"]]' --json >/dev/null
  run_required "docs" "docs table row insert" gog docs table-row insert "$doc_id" \
    --table 1 --at end --values-json '["E","F"]' --json >/dev/null
  run_required "docs" "docs table column insert" gog docs table-column insert "$doc_id" \
    --table 1 --at end --json >/dev/null
  run_required "docs" "docs table merge" gog docs table-merge "$doc_id" \
    --table 1 --range 1,1:1,2 --json >/dev/null
  run_required "docs" "docs table unmerge" gog docs table-unmerge "$doc_id" \
    --table 1 --cell 1,1 --json >/dev/null

  run_required "docs" "docs URL image insert" gog docs insert-image "$doc_id" \
    --url https://www.gstatic.com/images/branding/product/2x/docs_48dp.png \
    --at end --width 48 --json >/dev/null
  local images_json
  images_json=$(gog docs images list "$doc_id" --json)
  "$PY" -c 'import json,sys; assert len(json.load(sys.stdin).get("images", [])) >= 1' <<<"$images_json"
  run_required "docs" "docs tables list" gog docs tables list "$doc_id" --json >/dev/null
  run_required "docs" "docs headings list" gog docs headings list "$doc_id" --json >/dev/null
  run_required "docs" "docs paragraphs list" gog docs paragraphs list "$doc_id" --json >/dev/null
  run_required "docs" "docs raw all tabs" gog docs raw "$doc_id" --all-tabs --json >/dev/null

  run_required "docs" "docs info" gog docs info "$doc_id" --json >/dev/null
  run_required "docs" "docs cat" gog docs cat "$doc_id" >/dev/null

  export_path="$LIVE_TMP/docs-export-$TS.pdf"
  run_required "docs" "docs export" gog docs export "$doc_id" --format pdf --out "$export_path" >/dev/null

  copy_json=$(gog docs copy "$doc_id" "gogcli-smoke-doc-copy-$TS" --json)
  copy_id=$(extract_id "$copy_json")
  [ -n "$copy_id" ] || { echo "Failed to parse doc copy id" >&2; exit 1; }
  register_drive_cleanup "$copy_id"

  run_required "docs" "docs comments delete" gog docs comments delete "$doc_id" \
    "$comment_id" --force >/dev/null
  run_required "docs" "docs named range delete" gog docs named-range delete "$doc_id" \
    smoke_anchor --force --json >/dev/null
  run_required "docs" "drive delete doc copy" gog drive delete "$copy_id" --force >/dev/null
  run_required "docs" "drive delete doc" gog drive delete "$doc_id" --force >/dev/null
}
