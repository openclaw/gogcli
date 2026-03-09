#!/usr/bin/env bash
# v10_verify.sh — Verify v10 test doc content via gog docs sed --dry-run
# Each check uses sed to search for expected text in the document.
# Usage: ./testdata/v10_verify.sh [docId] [account]
set -uo pipefail

DOC="${1:-${GOG_TEST_DOC_ID:-}}"
ACCT="${2:-${GOG_TEST_ACCOUNT:-}}"
if [ -z "$DOC" ] || [ -z "$ACCT" ]; then
    echo "Usage: $0 [docId] [account]"
    echo "Or set GOG_TEST_DOC_ID and GOG_TEST_ACCOUNT environment variables."
    echo "See .env.example for details."
    exit 1
fi
GOG="${GOG_BIN:-./gog}"

PASS=0
FAIL=0
TOTAL=0

# Fetch full doc text once (identity-replace all content, but we use find-replace count)
# Use a real sed identity replacement to check existence. We cache results.
DOC_TEXT=""
fetch_doc_text() {
    if [ -n "$DOC_TEXT" ]; then return; fi
    # Export doc as plain text
    local export_out export_path
    export_out=$($GOG docs export "$DOC" -a "$ACCT" --format txt 2>/dev/null || true)
    export_path=$(echo "$export_out" | grep "^path" | cut -f2-)
    if [ -n "$export_path" ] && [ -f "$export_path" ]; then
        DOC_TEXT=$(cat "$export_path")
    fi
    if [ -z "$DOC_TEXT" ]; then
        echo "⚠️  Could not export doc text. Aborting." >&2
        exit 1
    fi
}

exists() {
    fetch_doc_text
    echo "$DOC_TEXT" | grep -qF "$1"
}

check() {
    local name="$1" pattern="$2"
    TOTAL=$((TOTAL + 1))
    if exists "$pattern"; then
        PASS=$((PASS + 1))
        printf "  ✅ %s\n" "$name"
    else
        FAIL=$((FAIL + 1))
        printf "  ❌ %s\n" "$name"
    fi
}

echo "═══ v10 Live Document Verification ═══"
echo "Doc: $DOC"
echo ""

echo "── Headings ──"
check "Title" "SEDMAT Comprehensive Test v10"
check "Subtitle" "Text Formatting"
check "H3" "Heading Level Three"
check "H4" "Heading Level Four"
check "H5" "Heading Level Five"
check "H6" "Heading Level Six"

echo ""
echo "── Inline Styles ──"
check "Bold text" "This text is bold"
check "Italic text" "This text is italic"
check "Bold+italic" "Bold and italic combined"
check "Strikethrough" "Strikethrough text"
check "Inline code" "inline code snippet"
check "Underline" "Underlined text here"
check "Link" "Visit Deft.md"

echo ""
echo "── Lists ──"
check "Bullet 1" "First bullet point"
check "Bullet 2" "Second bullet point"
check "Bullet 3" "Third bullet point"
check "Numbered 1" "First numbered item"
check "Numbered 2" "Second numbered item"
check "Check 1" "Unchecked task one"

echo ""
echo "── Nested Lists ──"
check "Nested bullet L0" "Top level bullet"
check "Nested bullet L1" "Nested bullet level 1"
check "Nested bullet L2" "Nested bullet level 2"
check "Nested numbered L0" "Top level numbered"
check "Nested numbered L1" "Nested numbered level 1"

echo ""
echo "── Regex & Backrefs ──"
check "Hello World" "Hello World 2026"
check "Dollar 500" "price is \$500.00"
check "Global replace" "Global:"
check "Backref Three-As" "Three-As"
check "Name reversal" "Name: Smith, John"
check "Amp bold" "MATCHME"
check "Dollar 49.99" "\$49.99 each"
check "Dollar math" "\$100 + \$200 = \$300"

echo ""
echo "── Escaping ──"
check "Literal asterisks" "asterisks"
check "Path" "/usr/local/bin"

echo ""
echo "── v8: Horizontal Rule ──"
check "Text after hrule" "Text after the horizontal rule"

echo ""
echo "── v8: Blockquotes ──"
check "Simple blockquote" "simple blockquote"
check "Steve Jobs quote" "Steve Jobs"

echo ""
echo "── v8: Code Blocks ──"
check "JS code block" "function greet"
check "Go code block" "fmt.Println"

echo ""
echo "── v8: Super/Subscript (legacy) ──"
check "Superscript E=mc2" "E = mc"
check "Subscript H2O" "H2O"
check "Multi subscript C6H12O6" "C6H12O6"

echo ""
echo "── v8: Footnotes ──"
check "Footnote 1" "This is a simple footnote"
check "Footnote 2" "Nature, 2024"

echo ""
echo "── Tables ──"
check "Pipe table Col A" "Col A"
check "Pipe table Data 2" "Data 2"
check "Feature table" "Feature"
check "Table data" "Headings"

echo ""
echo "── Commands ──"
# DELETE THIS LINE should NOT exist (inverted check)
TOTAL=$((TOTAL + 1))
if exists "DELETE THIS LINE"; then
    FAIL=$((FAIL + 1))
    printf "  ❌ Delete line gone (still exists!)\n"
else
    PASS=$((PASS + 1))
    printf "  ✅ Delete line gone\n"
fi
check "Append line 1" "Line one of insert"
check "Append line 2" "Line two of insert"
check "Results heading" "Results"

echo ""
echo "── v10: Style Attributes ──"
check "Font Georgia" "Font: Georgia text"
check "Size 20pt" "Size: 20pt text"
check "Color Red" "Color: Red text"
check "Highlight Yellow" "Highlight: Yellow bg"
check "Combo Blue Georgia" "Combo: Blue Georgia 16pt"
check "Bold Montserrat" "Bold Montserrat 18pt"
check "Styled Heading" "Styled Heading"
check "Page break" "End of Page One"
check "After page break" "Start of Page Two"
check "Section break" "End Before Section Break"
check "After section" "New Section Content"

echo ""
echo "── v10: New Super/Sub Syntax ──"
check "New {super=TM}" "TM"
check "New {super=2} E=mc" "E = mc"
check "New {sub=2} H2O" "H2O"

echo ""
echo "═══════════════════════════════"
echo "Results: $PASS/$TOTAL passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then
    echo "⚠️  Some checks failed!"
    exit 1
else
    echo "✅ All checks passed!"
fi
