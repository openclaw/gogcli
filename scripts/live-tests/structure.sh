#!/usr/bin/env bash

set -euo pipefail

# Structure-preservation regression suite.
#
# Seeds one rich fixture per surface (docs / slides / sheets), snapshots the
# `… raw --json` BEFORE, runs each mutating command against a single target,
# re-reads the raw AFTER, and asserts the structural invariants below survive:
#
#   - paragraph bullet / listId (list membership)
#   - paragraphStyle.namedStyleType (heading level)
#   - run textStyle.{bold,italic,underline,link} (run styling)
#   - native person chip
#   - inline image object
#   - table cell text + cell style
#   - sheet cell value + textFormat / backgroundColor
#   - adjacent / untargeted content + document order (diff BEFORE vs AFTER
#     outside the targeted range)
#
# Skip key: "structure". Known-bug ops use run_optional so the suite documents
# them and flips to a hard assert once fixed (see openclaw/gogcli#838, #839).
#
# This is a SKELETON: the seed + assert bodies for the bullet/heading/run cases
# are wired; the remaining invariants (person chip, image, table, slides, sheets)
# are stubbed with TODO markers and run_optional placeholders so the structure of
# the suite is reviewable before the full assert bodies land.

run_structure_tests() {
  if skip "structure"; then
    echo "==> structure (skipped)"
    return 0
  fi

  run_structure_docs_tests
  run_structure_slides_tests
  run_structure_sheets_tests
}

# --- helpers ---------------------------------------------------------------

# assert_paragraph_bullet <raw-json> <text-substr>
# Fails if the paragraph whose first run contains <text-substr> has no bullet.
assert_paragraph_bullet() {
  local raw="$1" needle="$2"
  NEEDLE="$needle" "$PY" -c '
import json,os,sys
obj=json.load(sys.stdin); needle=os.environ["NEEDLE"]
for el in obj.get("body",{}).get("content",[]):
    p=el.get("paragraph")
    if not p: continue
    text="".join(r.get("textRun",{}).get("content","") for r in p.get("elements",[]))
    if needle in text:
        assert p.get("bullet",{}).get("listId"), f"paragraph {needle!r} lost its bullet/listId"
        sys.exit(0)
raise SystemExit(f"paragraph {needle!r} not found")' <<<"$raw"
}

# assert_paragraph_named_style <raw-json> <text-substr> <expected-style>
assert_paragraph_named_style() {
  local raw="$1" needle="$2" want="$3"
  NEEDLE="$needle" WANT="$want" "$PY" -c '
import json,os,sys
obj=json.load(sys.stdin); needle=os.environ["NEEDLE"]; want=os.environ["WANT"]
for el in obj.get("body",{}).get("content",[]):
    p=el.get("paragraph")
    if not p: continue
    text="".join(r.get("textRun",{}).get("content","") for r in p.get("elements",[]))
    if needle in text:
        got=p.get("paragraphStyle",{}).get("namedStyleType")
        assert got==want, f"paragraph {needle!r} namedStyleType {got!r} != {want!r}"
        sys.exit(0)
raise SystemExit(f"paragraph {needle!r} not found")' <<<"$raw"
}

# assert_run_style <raw-json> <run-text> <bold|italic|underline>
assert_run_style() {
  local raw="$1" needle="$2" attr="$3"
  NEEDLE="$needle" ATTR="$attr" "$PY" -c '
import json,os,sys
obj=json.load(sys.stdin); needle=os.environ["NEEDLE"]; attr=os.environ["ATTR"]
def runs(v):
    if isinstance(v,dict):
        if "textRun" in v: yield v["textRun"]
        for x in v.values(): yield from runs(x)
    elif isinstance(v,list):
        for x in v: yield from runs(x)
for r in runs(obj):
    if r.get("content","").strip()==needle:
        assert r.get("textStyle",{}).get(attr) is True, f"run {needle!r} lost {attr}"
        sys.exit(0)
raise SystemExit(f"run {needle!r} not found")' <<<"$raw"
}

# --- docs ------------------------------------------------------------------

# Block / multi-paragraph markdown replacement against a list item. Returns
# non-zero while #838 is open (the replacement paragraph loses its bullet).
# Reuses assert_paragraph_bullet, which exits non-zero on failure.
structure_docs_block_replace_keeps_bullet() {
  local doc_id="$1" repl_path raw
  repl_path="$LIVE_TMP/structure-block-$TS.md"
  printf 'Item two block A\n\nItem two block B' >"$repl_path"
  gog docs find-replace "$doc_id" "Item two" \
    --content-file "$repl_path" --first --format markdown --json >/dev/null
  raw=$(gog docs raw "$doc_id" --json)
  assert_paragraph_bullet "$raw" "Item two block A"
}

run_structure_docs_tests() {
  local doc_json doc_id seed_path repl_path before after
  doc_json=$(gog docs create "gogcli-structure-doc-$TS" --json)
  doc_id=$(extract_id "$doc_json")
  [ -n "$doc_id" ] || { echo "Failed to parse structure doc id" >&2; exit 1; }
  register_drive_cleanup "$doc_id"

  seed_path="$LIVE_TMP/structure-seed-$TS.md"
  printf '# Heading A\n\nNormal with **bold word** and *italic word* and a [link word](https://example.com) inside.\n\n## Heading B\n\n- Item one\n- Item two\n- Item three\n\n1. First step\n2. Second step\n3. Third step\n\nPlain trailing paragraph.\n' >"$seed_path"
  run_required "structure" "structure docs seed" gog docs write "$doc_id" \
    --file "$seed_path" --replace --markdown --json >/dev/null

  before=$(gog docs raw "$doc_id" --json)
  assert_paragraph_bullet "$before" "Item one"
  assert_paragraph_named_style "$before" "Heading A" "HEADING_1"
  assert_run_style "$before" "bold word" "bold"

  # PASS: inline single-paragraph markdown replacement preserves bullet (guarded
  # by inlineReplacement, #740).
  run_required "structure" "structure docs inline md replace keeps bullet" \
    gog docs find-replace "$doc_id" "Item one" "Item one edited" --first --format markdown --json >/dev/null
  after=$(gog docs raw "$doc_id" --json)
  assert_paragraph_bullet "$after" "Item one edited"
  assert_paragraph_named_style "$after" "Heading A" "HEADING_1"

  # KNOWN BUG #838: block / multi-paragraph markdown replacement drops the
  # matched paragraph's bullet. run_optional until fixed; flip to a hard
  # assert_paragraph_bullet on "Item two block A" (and remove the xfail
  # wrapper) once #838 lands.
  run_optional "structure" "structure docs block md replace keeps bullet (#838 xfail)" \
    structure_docs_block_replace_keeps_bullet "$doc_id"

  # TODO: assert adjacent/untargeted invariant — diff BEFORE vs AFTER element
  # list outside the targeted paragraph; fail on any change in count/order/text.
  # TODO: person-chip invariant — insert-person at a placeholder, assert chip
  # present and neighbouring runs unchanged.
  # TODO: inline-image invariant — and KNOWN BUG #839: insert-image --at <text>
  # deletes the anchor; run_optional until a non-destructive mode exists.
  # TODO: table invariant — insert-table + cell-update --format markdown, assert
  # sibling cells + cell styles intact.

  run_required "structure" "structure docs delete" gog drive delete "$doc_id" --force >/dev/null
}

# --- slides ----------------------------------------------------------------

run_structure_slides_tests() {
  # TODO: seed a slide with a title shape (multi-word, one bold sub-range + one
  # linked sub-range) and a body shape with 3 bulleted paragraphs.
  # Assert after style-text / link / bullets / insert-text / replace-text that
  # neighbouring runs + links + bullet paragraphs are preserved.
  # Note: slides replace-text uses native ReplaceAllText; an exact-match over a
  # styled sub-run collapses into the surrounding run style (Google API), so
  # that case is a documented caveat, not a hard assert.
  echo "==> structure slides (TODO: seed + asserts)"
}

# --- sheets ----------------------------------------------------------------

run_structure_sheets_tests() {
  # TODO: seed a 3x3 range with a bold + background-coloured header row.
  # Assert after update / find-replace / delete-dimension / merge / unmerge that
  # the header textFormat + backgroundColor and untouched cell values survive.
  echo "==> structure sheets (TODO: seed + asserts)"
}
