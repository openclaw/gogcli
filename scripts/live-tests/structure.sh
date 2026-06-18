#!/usr/bin/env bash

set -euo pipefail

# Docs structure-preservation regressions. Target paragraphs contain STRUCT_;
# every other paragraph is fingerprinted before edits and compared after each
# mutation so adjacent content, order, paragraph structure, and run styles stay
# unchanged.

docs_structure_fingerprint() {
  local raw="$1"
  "$PY" -c '
import json,sys
obj=json.load(sys.stdin)
out=[]
for el in obj.get("body",{}).get("content",[]):
    p=el.get("paragraph")
    if not p:
        continue
    runs=[]
    text=""
    for item in p.get("elements",[]):
        run=item.get("textRun")
        if not run:
            continue
        content=run.get("content","")
        text+=content
        runs.append({"text":content,"style":run.get("textStyle",{})})
    if "STRUCT_" in text or not text.strip():
        continue
    style={k:v for k,v in p.get("paragraphStyle",{}).items() if k != "headingId"}
    out.append({"text":text,"bullet":p.get("bullet"),"style":style,"runs":runs})
json.dump(out,sys.stdout,sort_keys=True,separators=(",",":"))' <<<"$raw"
}

assert_docs_structure_fingerprint() {
  local before="$1" raw="$2" after
  after=$(docs_structure_fingerprint "$raw")
  if [ "$after" != "$before" ]; then
    echo "Untargeted Docs structure changed" >&2
    BEFORE="$before" AFTER="$after" "$PY" -c '
import difflib,json,os
before=json.dumps(json.loads(os.environ["BEFORE"]),indent=2,sort_keys=True).splitlines()
after=json.dumps(json.loads(os.environ["AFTER"]),indent=2,sort_keys=True).splitlines()
print("\n".join(difflib.unified_diff(before,after,fromfile="before",tofile="after")))' >&2
    return 1
  fi
}

docs_paragraph_property() {
  local raw="$1" needle="$2" property="$3"
  NEEDLE="$needle" PROPERTY="$property" "$PY" -c '
import json,os,sys
obj=json.load(sys.stdin); needle=os.environ["NEEDLE"]; prop=os.environ["PROPERTY"]
for el in obj.get("body",{}).get("content",[]):
    p=el.get("paragraph")
    if not p:
        continue
    text="".join(r.get("textRun",{}).get("content","") for r in p.get("elements",[]))
    if needle not in text:
        continue
    if prop == "bullet.listId":
        print(p.get("bullet",{}).get("listId", ""))
    elif prop == "paragraphStyle.namedStyleType":
        print(p.get("paragraphStyle",{}).get("namedStyleType", ""))
    elif prop == "text":
        print(text, end="")
    sys.exit(0)
raise SystemExit(f"paragraph {needle!r} not found")' <<<"$raw"
}

assert_docs_paragraph_property() {
  local raw="$1" needle="$2" property="$3" want="$4" got
  got=$(docs_paragraph_property "$raw" "$needle" "$property")
  [ "$got" = "$want" ] || {
    echo "Paragraph $needle property $property: got '$got', want '$want'" >&2
    return 1
  }
}

assert_docs_run_style() {
  local raw="$1" needle="$2" property="$3" want="$4"
  NEEDLE="$needle" PROPERTY="$property" WANT="$want" "$PY" -c '
import json,os,sys
obj=json.load(sys.stdin); needle=os.environ["NEEDLE"]; prop=os.environ["PROPERTY"]; want=os.environ["WANT"]
def walk(value):
    if isinstance(value,dict):
        if "textRun" in value:
            yield value["textRun"]
        for child in value.values():
            yield from walk(child)
    elif isinstance(value,list):
        for child in value:
            yield from walk(child)
for run in walk(obj):
    if run.get("content","").strip() != needle:
        continue
    style=run.get("textStyle",{})
    got=style.get("link",{}).get("url","") if prop == "link.url" else style.get(prop)
    expected=True if want == "true" else want
    assert got == expected, f"run {needle!r} property {prop}: {got!r} != {expected!r}"
    sys.exit(0)
raise SystemExit(f"run {needle!r} not found")' <<<"$raw"
}

assert_docs_inline_image_count() {
  local raw="$1" want="$2"
  WANT="$want" "$PY" -c '
import json,os,sys
obj=json.load(sys.stdin)
def count(value):
    if isinstance(value,dict):
        return (1 if "inlineObjectElement" in value else 0) + sum(count(v) for v in value.values())
    if isinstance(value,list):
        return sum(count(v) for v in value)
    return 0
got=count(obj); want=int(os.environ["WANT"])
assert got == want, f"inline image count {got} != {want}"' <<<"$raw"
}

run_structure_tests() {
  if skip "structure"; then
    echo "==> structure (skipped)"
    return 0
  fi

  local doc_json doc_id seed_path block_path heading_path before baseline after
  local inline_list_id block_list_id image_url
  doc_json=$(gog docs create "gogcli-structure-doc-$TS" --json)
  doc_id=$(extract_id "$doc_json")
  [ -n "$doc_id" ] || { echo "Failed to parse structure doc id" >&2; exit 1; }
  register_drive_cleanup "$doc_id"

  seed_path="$LIVE_TMP/structure-seed-$TS.md"
  block_path="$LIVE_TMP/structure-block-$TS.md"
  heading_path="$LIVE_TMP/structure-heading-$TS.md"
  printf '# Heading control\n\nNormal with **bold word** and *italic word* and a [link word](https://example.com) inside.\n\n## STRUCT_HEADING_TARGET\n\nHeading neighbor.\n\n- Item one\n- STRUCT_INLINE_TARGET\n- STRUCT_BLOCK_TARGET\n- Item four\n\n1. First step\n2. Second step\n3. Third step\n\nSTRUCT_IMAGE_TARGET\n\nPlain trailing paragraph.\n' >"$seed_path"
  printf 'STRUCT_BLOCK_TARGET A\n\nSTRUCT_BLOCK_TARGET B' >"$block_path"
  printf 'STRUCT_HEADING_TARGET A\n\nSTRUCT_HEADING_TARGET B' >"$heading_path"

  run_required "structure" "structure docs seed" gog docs write "$doc_id" \
    --file "$seed_path" --replace --markdown --json >/dev/null
  before=$(gog docs raw "$doc_id" --json)
  baseline=$(docs_structure_fingerprint "$before")
  inline_list_id=$(docs_paragraph_property "$before" "STRUCT_INLINE_TARGET" "bullet.listId")
  block_list_id=$(docs_paragraph_property "$before" "STRUCT_BLOCK_TARGET" "bullet.listId")
  [ -n "$inline_list_id" ] && [ -n "$block_list_id" ] || {
    echo "Structure fixture list IDs missing" >&2
    exit 1
  }
  assert_docs_run_style "$before" "bold word" "bold" "true"
  assert_docs_run_style "$before" "italic word" "italic" "true"
  assert_docs_run_style "$before" "link word" "link.url" "https://example.com"

  run_required "structure" "structure docs inline markdown replacement" \
    gog docs find-replace "$doc_id" "STRUCT_INLINE_TARGET" "STRUCT_INLINE_TARGET edited" \
      --first --format markdown --json >/dev/null
  after=$(gog docs raw "$doc_id" --json)
  assert_docs_paragraph_property "$after" "STRUCT_INLINE_TARGET edited" "bullet.listId" "$inline_list_id"
  assert_docs_structure_fingerprint "$baseline" "$after"

  run_required "structure" "structure docs block markdown list replacement" \
    gog docs find-replace "$doc_id" "STRUCT_BLOCK_TARGET" --content-file "$block_path" \
      --first --format markdown --json >/dev/null
  after=$(gog docs raw "$doc_id" --json)
  assert_docs_paragraph_property "$after" "STRUCT_BLOCK_TARGET A" "bullet.listId" "$block_list_id"
  assert_docs_paragraph_property "$after" "STRUCT_BLOCK_TARGET B" "bullet.listId" ""
  assert_docs_structure_fingerprint "$baseline" "$after"

  run_required "structure" "structure docs block markdown heading replacement" \
    gog docs find-replace "$doc_id" "STRUCT_HEADING_TARGET" --content-file "$heading_path" \
      --first --format markdown --json >/dev/null
  after=$(gog docs raw "$doc_id" --json)
  assert_docs_paragraph_property "$after" "STRUCT_HEADING_TARGET A" "paragraphStyle.namedStyleType" "HEADING_2"
  assert_docs_paragraph_property "$after" "STRUCT_HEADING_TARGET B" "paragraphStyle.namedStyleType" "NORMAL_TEXT"
  assert_docs_structure_fingerprint "$baseline" "$after"

  image_url="https://www.gstatic.com/images/branding/product/2x/docs_96dp.png"
  run_required "structure" "structure docs image before preserved anchor" \
    gog docs insert-image "$doc_id" --url "$image_url" --before "STRUCT_IMAGE_TARGET" \
      --width 40 --json >/dev/null
  run_required "structure" "structure docs image after preserved anchor" \
    gog docs insert-image "$doc_id" --url "$image_url" --after "STRUCT_IMAGE_TARGET" \
      --width 40 --json >/dev/null
  after=$(gog docs raw "$doc_id" --json)
  assert_docs_paragraph_property "$after" "STRUCT_IMAGE_TARGET" "text" "STRUCT_IMAGE_TARGET"
  assert_docs_inline_image_count "$after" 2
  assert_docs_structure_fingerprint "$baseline" "$after"

  run_required "structure" "structure docs delete" gog drive delete "$doc_id" --force >/dev/null
}
