# SEDMAT Comprehensive Test v9
# ============================
# Integrated test covering all sedmat features: headings, inline styles, lists,
# nested lists, regex, backrefs, special chars, horizontal rules, blockquotes,
# code blocks, superscript/subscript, footnotes, tables, images, and combos.
#
# Usage:
#   1. Seed:      gog docs sed <docId> -a <acct> -f testdata/v9_seed.txt -p
#   2. Format:    gog docs sed <docId> -a <acct> -f testdata/v9_test.sed
#   3. Table ops: gog docs sed <docId> -a <acct> -f testdata/v9_table_ops.sed

# ============================================================
# SECTION 1: Headings (H1–H6)
# [EXPECT: HEADING_1 through HEADING_6 paragraph styles]
# ============================================================
s/QQQ_TITLE_QQQ/# SEDMAT Comprehensive Test v9/
s/QQQ_SUBTITLE_QQQ/## Text Formatting \& Styles/
s/QQQ_H3_QQQ/### Heading Level Three/
s/QQQ_H4_QQQ/#### Heading Level Four/
s/QQQ_H5_QQQ/##### Heading Level Five/
s/QQQ_H6_QQQ/###### Heading Level Six/

# ============================================================
# SECTION 2: Inline Text Styles (7 types)
# [EXPECT: bold, italic, bold+italic, strikethrough, code, underline, link]
# ============================================================
s/QQQ_BOLD_QQQ/**This text is bold**/
s/QQQ_ITALIC_QQQ/*This text is italic*/
s/QQQ_BOLDITALIC_QQQ/***Bold and italic combined***/
s/QQQ_STRIKE_QQQ/~~Strikethrough text~~/
s/QQQ_CODE_QQQ/`inline code snippet`/
s/QQQ_UNDERLINE_QQQ/__Underlined text here__/
s/QQQ_LINK_QQQ/[Visit Deft.md](https:\/\/deft.md)/

# ============================================================
# SECTION 3: Lists — Flat (bullets, numbered, checkboxes)
# [EXPECT: ● bullets, 1.2.3. numbers, ☐ checkboxes]
# ============================================================
s/QQQ_BULLET1_QQQ/- First bullet point/
s/QQQ_BULLET2_QQQ/- Second bullet point/
s/QQQ_BULLET3_QQQ/- Third bullet point/
s/QQQ_NUM1_QQQ/1. First numbered item/
s/QQQ_NUM2_QQQ/1. Second numbered item/
s/QQQ_NUM3_QQQ/1. Third numbered item/
s/QQQ_CHECK1_QQQ/+ Unchecked task one/
s/QQQ_CHECK2_QQQ/+ Unchecked task two/

# ============================================================
# SECTION 4: Lists — Nested (indented bullets & numbers)
# [EXPECT: L0 top-level, L1 indented once, L2 indented twice]
# ============================================================
s/QQQ_NESTED_BL0_QQQ/- Top level bullet/
s/QQQ_NESTED_BL1_QQQ/  - Nested bullet level 1/
s/QQQ_NESTED_BL2_QQQ/    - Nested bullet level 2/
s/QQQ_NESTED_NL0_QQQ/1. Top level numbered/
s/QQQ_NESTED_NL1_QQQ/  1. Nested numbered level 1/

# ============================================================
# SECTION 5: Regex Operations
# [EXPECT: global replace, character class, word-level formatting]
# ============================================================
s/QQQ_RHELLO_QQQ/Hello World 2026/
s/QQQ_REMAIL_QQQ/contact: john.doe@example.com/
s/QQQ_RNUMBER_QQQ/The price is 500 dollars/
s/QQQ_RCHARCLASS_QQQ/Classes: AAABBBCCC/
s/QQQ_RWORD_QQQ/Words: apple banana cherry/
s/xaa/XXX/g
s/yaa/YYY/g
s/zaa/ZZZ/g
s/[A]{3}[B]{3}[C]{3}/Three-As Three-Bs Three-Cs/
s/banana/**banana**/

# ============================================================
# SECTION 6: Backreferences & Special Replacements
# [EXPECT: capture group swap, & whole-match, email reformat, dollar backref]
# ============================================================
s/QQQ_BACKREF_QQQ/Name: John Smith/
s/QQQ_AMPERSAND_QQQ/Amp: MATCHME/
s/(John) (Smith)/$2, $1/
s/MATCHME/**&**/
s/([a-z.]+)@([a-z.]+)/$1 at $2/
s/(\d+) dollars/\$$1.00/

# ============================================================
# SECTION 7: Dollar Signs & Special Characters
# [EXPECT: literal $ amounts, escaped markdown, escaped slashes]
# ============================================================
s/QQQ_DOLLAR1_QQQ/Price: $$49.99 each/
s/QQQ_DOLLAR2_QQQ/Total: $$100 + $$200 = $$300/
s/QQQ_ESCMD_QQQ/Literal: \*asterisks\* and \#hashes/
s/QQQ_ESCSLASH_QQQ/Path: \/usr\/local\/bin/

# ============================================================
# SECTION 8: Horizontal Rules
# [EXPECT: paragraph with bottom border, followed by normal text]
# ============================================================
s/QQQ_HRULE_QQQ/---/
s/QQQ_HRULE_AFTER_QQQ/Text after the horizontal rule/

# ============================================================
# SECTION 9: Blockquotes
# [EXPECT: indented paragraphs with left grey border]
# ============================================================
s/QQQ_BLOCKQUOTE1_QQQ/> This is a simple blockquote/
s/QQQ_BLOCKQUOTE2_QQQ/> The only way to do great work is to love what you do. — Steve Jobs/

# ============================================================
# SECTION 10: Code Blocks
# [EXPECT: Courier New font, grey background, multi-line code]
# ============================================================
s/QQQ_CODEBLOCK1_QQQ/```js\nfunction greet(name) {\n  return `Hello, \${name}!`;\n}\n```/
s/QQQ_CODEBLOCK2_QQQ/```go\nfunc main() {\n  fmt.Println("Hello")\n}\n```/

# ============================================================
# SECTION 11: Superscript & Subscript
# [EXPECT: raised/lowered text via baselineOffset]
# ============================================================
s/QQQ_SUPER_WHOLE_QQQ/^{TM}/
s/QQQ_SUPER_INLINE_QQQ/E = mc^{2}/
s/QQQ_SUPER_MULTI_QQQ/x^{2} + y^{2} = z^{2}/
s/QQQ_SUB_WHOLE_QQQ/~{0}/
s/QQQ_SUB_INLINE_QQQ/H~{2}O/
s/QQQ_SUB_MULTI_QQQ/C~{6}H~{12}O~{6}/

# ============================================================
# SECTION 12: Footnotes
# [EXPECT: footnote markers in body, footnote text at page bottom]
# ============================================================
s/QQQ_FOOTNOTE1_QQQ/[^This is a simple footnote]/
s/QQQ_FOOTNOTE2_QQQ/[^According to research published in Nature, 2024]/

# ============================================================
# SECTION 13: Tables — Pipe Syntax
# [EXPECT: native tables with cell content and formatting]
# ============================================================
s/QQQ_TPIPE1_QQQ/| Col A | Col B |\n| Data 1 | Data 2 |\n| Data 3 | Data 4 |/
s/QQQ_TPIPE2_QQQ/| **Feature** | **Status** | **Notes** |\n|---|---|---|\n| Headings | Done | H1-H6 |\n| Tables | Done | Pipe syntax |\n| Regex | Done | All ops |/

# ============================================================
# SECTION 14: Tables — Explicit Creation
# [EXPECT: empty native tables of specified dimensions]
# ============================================================
s/QQQ_TEXPLICIT_QQQ/|3x4|/
s/QQQ_TMERGE_QQQ/|4x3|/
s/QQQ_TCELLOPS_QQQ/|3x3|/

# ============================================================
# SECTION 15: Image
# [EXPECT: Google logo image inline]
# ============================================================
s/QQQ_IMAGE_QQQ/![Google Logo](https:\/\/www.google.com\/images\/branding\/googlelogo\/2x\/googlelogo_color_272x92dp.png)/

# ============================================================
# SECTION 16: Combos & Final
# [EXPECT: mixed formatting combos, section heading, multi-line, final text]
# ============================================================
s/QQQ_SECHEAD_QQQ/## Results \& Summary/
s/QQQ_H3BOLD_QQQ/### Heading and Bold Test/
s/QQQ_MULTILINE_QQQ/Line one of insert\nLine two of insert\nLine three of insert/
s/QQQ_COMBO_BOLDSUPER_QQQ/**Energy**/
s/QQQ_COMBO_CODEBLOCK_QQQ/```\nconst x = 42;\n```/
s/QQQ_COMBO_BQNEST_QQQ/> To be or not to be/
s/QQQ_FINAL_QQQ/End of SEDMAT v9 comprehensive test/
