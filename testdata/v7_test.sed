# SEDMAT Comprehensive Test v7
# ============================
# Transform v7_seed.txt into a fully formatted Google Doc.
# All seed tokens use QQQ_..._QQQ prefix/suffix to prevent substring conflicts.
#
# Usage:
#   1. Seed: gog docs sed <docId> -a <acct> 's/$/...seed.../'
#   2. Format: gog docs sed <docId> -a <acct> < testdata/v7_test.sed
#   3. Table ops: gog docs sed <docId> -a <acct> < testdata/v7_table_ops.sed

# ============================================================
# SECTION 1: Headings (H1–H6)
# [EXPECT: HEADING_1 through HEADING_6 paragraph styles]
# ============================================================
s/QQQ_TITLE_QQQ/# SEDMAT Comprehensive Test v7/
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
# SECTION 3: List Types (bullets, numbered, checkboxes)
# [EXPECT: ● bullets, 1.2.3. numbers, ☐ checkboxes; consecutive items share list ID]
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
# SECTION 4: Regex Operations
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
# SECTION 5: Backreferences & Special Replacements
# [EXPECT: capture group swap, & whole-match, email reformat, dollar backref]
# ============================================================
s/QQQ_BACKREF_QQQ/Name: John Smith/
s/QQQ_AMPERSAND_QQQ/Amp: MATCHME/
s/(John) (Smith)/$2, $1/
s/MATCHME/**&**/
s/([a-z.]+)@([a-z.]+)/$1 at $2/
s/(\d+) dollars/\$$1.00/

# ============================================================
# SECTION 6: Dollar Signs & Special Characters
# [EXPECT: literal $ amounts, escaped markdown, escaped slashes]
# ============================================================
s/QQQ_DOLLAR1_QQQ/Price: $$49.99 each/
s/QQQ_DOLLAR2_QQQ/Total: $$100 + $$200 = $$300/
s/QQQ_ESCMD_QQQ/Literal: \*asterisks\* and \#hashes/
s/QQQ_ESCSLASH_QQQ/Path: \/usr\/local\/bin/

# ============================================================
# SECTION 7: Tables — Pipe Syntax
# [EXPECT: native tables with cell content and formatting]
# ============================================================
s/QQQ_TPIPE1_QQQ/| Col A | Col B |\n| Data 1 | Data 2 |\n| Data 3 | Data 4 |/
s/QQQ_TPIPE2_QQQ/| **Feature** | **Status** | **Notes** |\n|---|---|---|\n| Headings | Done | H1-H6 |\n| Tables | Done | Pipe syntax |\n| Regex | Done | All ops |/

# ============================================================
# SECTION 8: Tables — Explicit Creation
# [EXPECT: empty native tables of specified dimensions]
# ============================================================
s/QQQ_TEXPLICIT_QQQ/|3x4|/
s/QQQ_TMERGE_QQQ/|4x3|/
s/QQQ_TCELLOPS_QQQ/|3x3|/

# ============================================================
# SECTION 9: Image
# [EXPECT: Google logo image inline]
# ============================================================
s/QQQ_IMAGE_QQQ/![Google Logo](https:\/\/www.google.com\/images\/branding\/googlelogo\/2x\/googlelogo_color_272x92dp.png)/

# ============================================================
# SECTION 10: Additional Features
# [EXPECT: H2 heading, H3 heading, multi-line insert, final text]
# ============================================================
s/QQQ_SECHEAD_QQQ/## Results \& Summary/
s/QQQ_H3BOLD_QQQ/### Heading and Bold Test/
s/QQQ_MULTILINE_QQQ/Line one of insert\nLine two of insert\nLine three of insert/
s/QQQ_FINAL_QQQ/End of SEDMAT v7 comprehensive test/
