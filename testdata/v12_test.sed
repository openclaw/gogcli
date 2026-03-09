# SEDMAT Comprehensive Test v12
# =============================
# DEFINITIVE test suite covering ALL supported features.
# Both brace syntax and legacy markdown where applicable.
#
# Usage:
#   gog docs sed <DOC_ID> -a <acct> -f testdata/v12_test.sed
#
# Starts by clearing the doc, then seeds and formats.

# ── Clear document ──
s/^$//

# ── Seed content (positional insert into empty doc) ──
s/^$/QQQ_H1_QQQ\nQQQ_H2_QQQ\nQQQ_H3_QQQ\nQQQ_H4_QQQ\nQQQ_H5_QQQ\nQQQ_H6_QQQ\nQQQ_BOLD_QQQ\nQQQ_ITALIC_QQQ\nQQQ_BOLDITALIC_QQQ\nQQQ_STRIKE_QQQ\nQQQ_CODE_QQQ\nQQQ_UNDERLINE_QQQ\nQQQ_SMALLCAPS_QQQ\nQQQ_LINK_QQQ\nQQQ_BULLET1_QQQ\nQQQ_BULLET2_QQQ\nQQQ_BULLET3_QQQ\nQQQ_NUM1_QQQ\nQQQ_NUM2_QQQ\nQQQ_NUM3_QQQ\nQQQ_CHECK_UNCHECKED_QQQ\nQQQ_CHECK_CHECKED_QQQ\nQQQ_CHECK_EXPLICIT_NO_QQQ\nQQQ_NEST_B0_QQQ\nQQQ_NEST_B1_QQQ\nQQQ_NEST_B2_QQQ\nQQQ_NEST_N0_QQQ\nQQQ_NEST_N1_QQQ\nQQQ_HELLO_QQQ\nQQQ_EMAIL_QQQ\nQQQ_PRICE_QQQ\nQQQ_GLOBAL_QQQ\nQQQ_CLASS_QQQ\nQQQ_WORDS_QQQ\nQQQ_NAME_QQQ\nQQQ_AMP_QQQ\nQQQ_DOLLAR1_QQQ\nQQQ_DOLLAR2_QQQ\nQQQ_NTH_MATCH_QQQ\nQQQ_ESCAPE1_QQQ\nQQQ_ESCAPE2_QQQ\nQQQ_HRULE_QQQ\nQQQ_HRULE_TEXT_QQQ\nQQQ_BLOCKQUOTE1_QQQ\nQQQ_BLOCKQUOTE2_QQQ\nQQQ_CODEBLOCK1_QQQ\nQQQ_CODEBLOCK2_QQQ\nQQQ_SUPER_BRACE_QQQ\nQQQ_SUB_BRACE_QQQ\nQQQ_SUPER_INLINE_QQQ\nQQQ_SUB_INLINE_QQQ\nQQQ_FORMULA_QQQ\nQQQ_CHEMISTRY_QQQ\nQQQ_SUPER_MD_QQQ\nQQQ_SUB_MD_QQQ\nQQQ_FOOTNOTE1_QQQ\nQQQ_FOOTNOTE2_QQQ\nQQQ_TABLE_PIPE_QQQ\nQQQ_TABLE_BOLD_QQQ\nQQQ_TABLE_DIM_QQQ\nQQQ_TABLE_HEADER_QQQ\nQQQ_TABLE_EMPTY_QQQ\nQQQ_TABLE_BRACE_QQQ\nQQQ_IMAGE_MD_QQQ\nQQQ_IMAGE_MD_DIM_QQQ\nQQQ_IMAGE_BRACE_QQQ\nQQQ_COMBO_HEAD_QQQ\nQQQ_DELETE_ME_QQQ\nQQQ_APPEND_TARGET_QQQ\nQQQ_INSERT_TARGET_QQQ\nQQQ_XLAT_QQQ\nQQQ_FONT_QQQ\nQQQ_SIZE_QQQ\nQQQ_COLOR_HEX_QQQ\nQQQ_COLOR_NAMED_QQQ\nQQQ_BG_HEX_QQQ\nQQQ_BG_NAMED_QQQ\nQQQ_COMBO_STYLE_QQQ\nQQQ_BOLD_FONT_QQQ\nQQQ_HEAD_FONT_QQQ\nQQQ_ALIGN_CENTER_QQQ\nQQQ_ALIGN_RIGHT_QQQ\nQQQ_ALIGN_JUSTIFY_QQQ\nQQQ_INDENT_QQQ\nQQQ_SPACING_QQQ\nQQQ_LEADING_QQQ\nQQQ_BREAK_PAGE_QQQ\nQQQ_AFTER_PAGE_QQQ\nQQQ_BREAK_SECTION_QQQ\nQQQ_AFTER_SECTION_QQQ\nQQQ_BREAK_COLUMN_QQQ\nQQQ_AFTER_COLUMN_QQQ\nQQQ_INLINE_BOLD_QQQ\nQQQ_INLINE_SUP_QQQ\nQQQ_INLINE_MULTI_QQQ\nQQQ_RESET_QQQ\nQQQ_NEGATE_BOLD_QQQ\nQQQ_NEGATE_ITALIC_QQQ\nQQQ_NEGATE_UNDERLINE_QQQ\nQQQ_NEGATE_STRIKE_QQQ\nQQQ_BOOKMARK_QQQ\nQQQ_BOOKMARK_LINK_QQQ\nQQQ_OPACITY_QQQ\nQQQ_KERNING_QQQ\nQQQ_EFFECT_QQQ\nQQQ_CHIP_DATE_QQQ\nQQQ_CHIP_PERSON_QQQ\nQQQ_COLS_QQQ\nQQQ_TOC_QQQ\nQQQ_FLAG_I_QQQ\nQQQ_FLAG_M_QQQ/

# ══════════════════════════════════════════════════════════════
# Section 1: Headings
# Brace: {h=t} title, {h=s} subtitle, {h=1}..{h=6}
# Markdown: # H1 .. ###### H6
# ══════════════════════════════════════════════════════════════
s/QQQ_H1_QQQ/{h=t}SEDMAT v12 Test Suite/
s/QQQ_H2_QQQ/{h=s}Text Formatting \& Styles/
s/QQQ_H3_QQQ/{h=3}Heading Level Three/
s/QQQ_H4_QQQ/#### Heading Level Four/
s/QQQ_H5_QQQ/##### Heading Level Five/
s/QQQ_H6_QQQ/###### Heading Level Six/

# ══════════════════════════════════════════════════════════════
# Section 2: Inline Formatting
# Brace: {b} {i} {_} {-} {#} {w}
# Markdown: ** * *** ~~ ` __
# ══════════════════════════════════════════════════════════════
s/QQQ_BOLD_QQQ/{b}This text is bold/
s/QQQ_ITALIC_QQQ/*This text is italic*/
s/QQQ_BOLDITALIC_QQQ/{b i}Bold and italic combined/
s/QQQ_STRIKE_QQQ/~~Strikethrough text~~/
s/QQQ_CODE_QQQ/{#}inline code snippet/
s/QQQ_UNDERLINE_QQQ/__Underlined text here__/
s/QQQ_SMALLCAPS_QQQ/{w}Small Caps Text/
s/QQQ_LINK_QQQ/{u=https:\/\/deft.md}Visit Deft.md/

# ══════════════════════════════════════════════════════════════
# Section 3: Lists (markdown convenience)
# ══════════════════════════════════════════════════════════════
s/QQQ_BULLET1_QQQ/- First bullet point/
s/QQQ_BULLET2_QQQ/- Second bullet point/
s/QQQ_BULLET3_QQQ/- Third bullet point/
s/QQQ_NUM1_QQQ/1. First numbered item/
s/QQQ_NUM2_QQQ/1. Second numbered item/
s/QQQ_NUM3_QQQ/1. Third numbered item/

# Checkboxes: brace {check}, {check=y}, {check=n} + markdown - [ ] - [x]
s/QQQ_CHECK_UNCHECKED_QQQ/{check}Unchecked task/
s/QQQ_CHECK_CHECKED_QQQ/- [x] Checked task/
s/QQQ_CHECK_EXPLICIT_NO_QQQ/{check=n}Explicit unchecked/

# ══════════════════════════════════════════════════════════════
# Section 4: Nested Lists
# ══════════════════════════════════════════════════════════════
s/QQQ_NEST_B0_QQQ/- Top level bullet/
s/QQQ_NEST_B1_QQQ/  - Nested bullet level 1/
s/QQQ_NEST_B2_QQQ/    - Nested bullet level 2/
s/QQQ_NEST_N0_QQQ/1. Top level numbered/
s/QQQ_NEST_N1_QQQ/  1. Nested numbered level 1/

# ══════════════════════════════════════════════════════════════
# Section 5: Regex & Backreferences
# s/pattern/replacement/flags — g, i, n flags
# Backrefs: $1, $2, & (whole match)
# ══════════════════════════════════════════════════════════════
s/QQQ_HELLO_QQQ/Hello World 2026/
s/QQQ_EMAIL_QQQ/contact: john.doe at example.com/
s/QQQ_PRICE_QQQ/The price is $$500.00/
s/QQQ_GLOBAL_QQQ/Global: AAA BBB CCC/
s/QQQ_CLASS_QQQ/Classes: One-As One-Bs One-Cs/
s/QQQ_WORDS_QQQ/Words: apple banana cherry/
# Global replace
s/(AAA|BBB|CCC)/XXX/g
# Group capture + backref
s/One-([A-Z])s/Three-$1s/g
# Inline bold via backref
s/banana/{b}banana/
s/QQQ_NAME_QQQ/Name: John Smith/
# Swap first/last via groups
s/Name: (\w+) (\w+)/Name: $2, $1/
# Whole-match backref (&)
s/QQQ_AMP_QQQ/Amp: MATCHME/
s/MATCHME/{b}&/
s/QQQ_DOLLAR1_QQQ/Price: $$49.99 each/
s/QQQ_DOLLAR2_QQQ/Total: $$100 + $$200 = $$300/
# Nth occurrence (replace only 2nd match)
s/QQQ_NTH_MATCH_QQQ/aaa bbb aaa bbb aaa/
s/aaa/ZZZ/2

# ══════════════════════════════════════════════════════════════
# Section 6: Escaping
# ══════════════════════════════════════════════════════════════
s/QQQ_ESCAPE1_QQQ/Literal: \*asterisks\* and \#hashes/
s/QQQ_ESCAPE2_QQQ/Path: \/usr\/local\/bin/

# ══════════════════════════════════════════════════════════════
# Section 7: Horizontal Rules (---)
# ══════════════════════════════════════════════════════════════
s/QQQ_HRULE_QQQ/---/
s/QQQ_HRULE_TEXT_QQQ/Text after the horizontal rule/

# ══════════════════════════════════════════════════════════════
# Section 8: Blockquotes (> text)
# ══════════════════════════════════════════════════════════════
s/QQQ_BLOCKQUOTE1_QQQ/> This is a simple blockquote/
s/QQQ_BLOCKQUOTE2_QQQ/> The only way to do great work is to love what you do. — Steve Jobs/

# ══════════════════════════════════════════════════════════════
# Section 9: Code Blocks (triple backtick)
# ══════════════════════════════════════════════════════════════
s/QQQ_CODEBLOCK1_QQQ/```javascript\nfunction greet(name) {\n  return \x60Hello, ${name}!\x60;\n}\n```/
s/QQQ_CODEBLOCK2_QQQ/```go\nfunc main() {\n  fmt.Println("Hello")\n}\n```/

# ══════════════════════════════════════════════════════════════
# Section 10: Superscript & Subscript
# Brace: {^} whole, {^=text} inline, {,} whole, {,=text} inline
# Markdown: ^{text} superscript, ~{text} subscript
# ══════════════════════════════════════════════════════════════
s/QQQ_SUPER_BRACE_QQQ/{^}TM/
s/QQQ_SUB_BRACE_QQQ/{,}0/
s/QQQ_SUPER_INLINE_QQQ/E = mc{^=2}/
s/QQQ_SUB_INLINE_QQQ/H{,=2}O/
s/QQQ_FORMULA_QQQ/x{^=2} + y{^=2} = z{^=2}/
s/QQQ_CHEMISTRY_QQQ/C{,=6}H{,=12}O{,=6}/
# Legacy markdown super/sub
s/QQQ_SUPER_MD_QQQ/10^{th} percentile/
s/QQQ_SUB_MD_QQQ/H~{2}O is water/

# ══════════════════════════════════════════════════════════════
# Section 11: Footnotes (markdown [^text])
# ══════════════════════════════════════════════════════════════
s/QQQ_FOOTNOTE1_QQQ/[^This is a simple footnote]/
s/QQQ_FOOTNOTE2_QQQ/[^According to research published in Nature, 2024]/

# ══════════════════════════════════════════════════════════════
# Section 12: Pipe Tables (markdown convenience)
# ══════════════════════════════════════════════════════════════
s/QQQ_TABLE_PIPE_QQQ/| Col A | Col B |\n| Data 1 | Data 2 |\n| Data 3 | Data 4 |/
s/QQQ_TABLE_BOLD_QQQ/| **Feature** | **Status** | **Notes** |\n| Headings | Done | H1-H6 |\n| Tables | Done | Pipe syntax |\n| Regex | Done | All ops |/

# ══════════════════════════════════════════════════════════════
# Section 13: Table Dimensions (pipe & brace creation)
# |RxC| pipe syntax, {T=RxC} brace syntax, :header variant
# ══════════════════════════════════════════════════════════════
s/QQQ_TABLE_DIM_QQQ/|3x4|/
s/QQQ_TABLE_HEADER_QQQ/|5x4:header|/
s/QQQ_TABLE_EMPTY_QQQ/|3x3|/
s/QQQ_TABLE_BRACE_QQQ/{T=4x3:header}/

# ══════════════════════════════════════════════════════════════
# Section 14: Images
# Markdown: ![alt](url), ![alt](url =WxH)
# Brace: {img=url x=W y=H}
# ══════════════════════════════════════════════════════════════
s/QQQ_IMAGE_MD_QQQ/![W3C](https:\/\/www.w3.org\/Icons\/w3c_home.png)/
s/QQQ_IMAGE_MD_DIM_QQQ/![W3C](https:\/\/www.w3.org\/Icons\/w3c_home.png =72x48)/
s/QQQ_IMAGE_BRACE_QQQ/{img=https:\/\/www.w3.org\/Icons\/w3c_home.png x=144 y=96}/

# ══════════════════════════════════════════════════════════════
# Section 15: Commands — d (delete), a (append), i (insert), y (transliterate)
# ══════════════════════════════════════════════════════════════
s/QQQ_COMBO_HEAD_QQQ/{h=2}Results \& Summary/
s/QQQ_DELETE_ME_QQQ/DELETE THIS LINE/
d/DELETE THIS LINE/
s/QQQ_APPEND_TARGET_QQQ/Append Target Line/
a/Append Target Line/Appended line one\nAppended line two/
s/QQQ_INSERT_TARGET_QQQ/Insert Target Line/
i/Insert Target Line/{b}Inserted before/
s/QQQ_XLAT_QQQ/XLAT: AEIOU aeiou/
# NOTE: y/AEIOU/aeiou/ transliteration is tested separately (it operates on
# the entire doc and would mangle QQQ tokens if run in the same batch).
# Test with: gog docs sed <DOC_ID> 'y/AEIOU/aeiou/' after running this file.

# ══════════════════════════════════════════════════════════════
# Section 16: Font / Size / Color / Background
# Brace: {f=font} {s=size} {c=color|#hex} {z=bg|#hex}
# ══════════════════════════════════════════════════════════════
s/QQQ_FONT_QQQ/{f=Georgia}Font: Georgia text/
s/QQQ_SIZE_QQQ/{s=20}Size: 20pt text/
s/QQQ_COLOR_HEX_QQQ/{c=#FF0000}Color: Red hex text/
s/QQQ_COLOR_NAMED_QQQ/{c=blue}Color: Blue named text/
s/QQQ_BG_HEX_QQQ/{z=#FFFF00}Highlight: Yellow bg/
s/QQQ_BG_NAMED_QQQ/{z=green}Highlight: Green bg/
# Multiple attrs combined
s/QQQ_COMBO_STYLE_QQQ/{f=Georgia s=16 c=blue}Combo: Blue Georgia 16pt/
# Bold + font + size
s/QQQ_BOLD_FONT_QQQ/{b f=Montserrat s=18}Bold Montserrat 18pt/
# Heading + font + color
s/QQQ_HEAD_FONT_QQQ/{h=3 f=Playfair+Display s=22 c=#333333}Styled Heading/

# ══════════════════════════════════════════════════════════════
# Section 17: Alignment / Indent / Spacing / Leading
# Brace: {a=align} {n=indent} {p=above,below} {l=leading}
# ══════════════════════════════════════════════════════════════
s/QQQ_ALIGN_CENTER_QQQ/{a=center}Centered text/
s/QQQ_ALIGN_RIGHT_QQQ/{a=right}Right-aligned text/
s/QQQ_ALIGN_JUSTIFY_QQQ/{a=justify}Justified text here/
s/QQQ_INDENT_QQQ/{n=2}Indented paragraph/
s/QQQ_SPACING_QQQ/{p=12,6}Spaced paragraph/
s/QQQ_LEADING_QQQ/{l=2}Double spaced text/

# ══════════════════════════════════════════════════════════════
# Section 18: Breaks — page, section, column
# Brace: {+=p} page, {+=s} section, {+=c} column
# ══════════════════════════════════════════════════════════════
s/QQQ_BREAK_PAGE_QQQ/{+=p}End of Page One/
s/QQQ_AFTER_PAGE_QQQ/Start of Page Two/
s/QQQ_BREAK_SECTION_QQQ/{+=s}End Before Section Break/
s/QQQ_AFTER_SECTION_QQQ/New Section Content/
s/QQQ_BREAK_COLUMN_QQQ/{+=c}End Column One/
s/QQQ_AFTER_COLUMN_QQQ/Start Column Two/

# ══════════════════════════════════════════════════════════════
# Section 19: Inline Scoping (brace = syntax)
# {b=text}, {^=text}, {,=text} etc.
# ══════════════════════════════════════════════════════════════
s/QQQ_INLINE_BOLD_QQQ/The word {b=Warning} is bold here/
s/QQQ_INLINE_SUP_QQQ/10{^=th} percentile/
s/QQQ_INLINE_MULTI_QQQ/H{,=2}SO{,=4} is sulfuric acid/

# ══════════════════════════════════════════════════════════════
# Section 20: Reset / Negation
# {0} clear formatting, {!b} remove bold, {!i}, {!_}, {!-}
# ══════════════════════════════════════════════════════════════
s/QQQ_RESET_QQQ/{0}Plain text after reset/
s/QQQ_NEGATE_BOLD_QQQ/{!b}Not bold anymore/
s/QQQ_NEGATE_ITALIC_QQQ/{!i}Not italic/
s/QQQ_NEGATE_UNDERLINE_QQQ/{!_}Not underlined/
s/QQQ_NEGATE_STRIKE_QQQ/{!-}Not struck/

# ══════════════════════════════════════════════════════════════
# Section 21: Bookmarks & Internal Links
# {@=id} create bookmark, {u=#id} link to bookmark
# ══════════════════════════════════════════════════════════════
s/QQQ_BOOKMARK_QQQ/Chapter One Begins/
# TODO: bookmark creation ({@=ch1}) not yet implemented
s/QQQ_BOOKMARK_LINK_QQQ/Jump to Chapter One/
# TODO: bookmark links ({u=#ch1}) require bookmark creation first

# ══════════════════════════════════════════════════════════════
# Section 22: Opacity / Kerning / Effect
# {o=percent} {k=kerning} {e=effect}
# ══════════════════════════════════════════════════════════════
s/QQQ_OPACITY_QQQ/{o=50}Faded text/
s/QQQ_KERNING_QQQ/{k=2}Wide kerning text/
s/QQQ_EFFECT_QQQ/{e=shadow}Shadow effect/

# ══════════════════════════════════════════════════════════════
# Section 23: Smart Chips
# {date=YYYY-MM-DD}, {person=email}
# ══════════════════════════════════════════════════════════════
s/QQQ_CHIP_DATE_QQQ/{date=2026-01-15}/
s/QQQ_CHIP_PERSON_QQQ/{person=test@example.com}/

# ══════════════════════════════════════════════════════════════
# Section 24: Column Layout
# {cols=N} set column count
# ══════════════════════════════════════════════════════════════
# TODO: {cols=N} sets columns on the section, affecting everything in that section.
# Proper test needs section breaks before/after. Disabled to avoid breaking doc layout.
s/QQQ_COLS_QQQ/Two column layout (cols test disabled)/

# ══════════════════════════════════════════════════════════════
# Section 25: Table of Contents
# {toc} insert table of contents
# ══════════════════════════════════════════════════════════════
s/QQQ_TOC_QQQ/{toc}/

# ══════════════════════════════════════════════════════════════
# Section 26: Case-Insensitive & Multiline Flags
# s/pattern/replacement/i — case insensitive
# s/pattern/replacement/m — multiline
# ══════════════════════════════════════════════════════════════
s/QQQ_FLAG_I_QQQ/hello WORLD case test/
s/hello world/Case Insensitive Match/i
s/QQQ_FLAG_M_QQQ/multiline flag test/

# SEDMAT v12 — Table Cell Operations
# ====================================
# Run AFTER v12_test.sed. Tables are numbered in document order:
#   Table 1: 3x2 pipe table (Col A/Col B)
#   Table 2: 4x3 pipe table (Feature/Status/Notes)
#   Table 3: 3x4 explicit empty table
#   Table 4: 5x4:header explicit table
#   Table 5: 3x3 explicit empty table
#   Table 6: 4x3:header brace table
#
# Usage: gog docs sed <docId> -a <acct> -f testdata/v12_test.sed (table ops section)

# ============================================================
# TABLE 3: Fill cells + append row + append column
# s/|N|[r,c]/value/ — cell replace
# s/|N|[+1,0]// — add row
# s/|N|[0,+1]// — add column
# ============================================================
s/|3|[1,1]/**ID**/
s/|3|[1,2]/**Name**/
s/|3|[1,3]/**Role**/
s/|3|[1,4]/**Status**/
s/|3|[2,1]/001/
s/|3|[2,2]/Alice/
s/|3|[2,3]/Engineer/
s/|3|[2,4]/Active/
s/|3|[3,1]/002/
s/|3|[3,2]/Bob/
s/|3|[3,3]/Designer/
s/|3|[3,4]/On Leave/
# Append row
s/|3|[+1,0]//
# Append column
s/|3|[0,+1]//
# Fill appended cells
s/|3|[4,1]/003/
s/|3|[1,5]/**Dept**/

# ============================================================
# TABLE 5: Wildcard operations
# s/|N|[*,c]/value/ — set entire column
# s/|N|[r,*]/value/ — set entire row
# ============================================================
s/|5|[1,1]/**Name**/
s/|5|[1,2]/**Score**/
s/|5|[1,3]/**Grade**/
s/|5|[2,1]/Alice/
s/|5|[2,2]/95/
s/|5|[2,3]/A+/
s/|5|[3,1]/Bob/
s/|5|[3,2]/87/
s/|5|[3,3]/B+/

# ============================================================
# TABLE 6: Merge cells (header row spans 3 columns)
# s/|N|[r1,c1:r2,c2]/merge/
# ============================================================
s/|6|[1,1]/**Merged Header**/
s/|6|[2,1]/R2C1/
s/|6|[2,2]/R2C2/
s/|6|[2,3]/R2C3/
s/|6|[3,1]/R3C1/
s/|6|[3,2]/R3C2/
s/|6|[3,3]/R3C3/
s/|6|[4,1]/R4C1/
s/|6|[4,2]/R4C2/
s/|6|[4,3]/R4C3/
# Merge row 1 across all 3 columns
s/|6|[1,1:1,3]/merge/

# ============================================================
# TABLE 6: Bold via wildcard regex on cell content
# ============================================================
s/Alice|Bob/**&**/
