# SEDMAT Comprehensive Test v10
# =============================
# All v9 features + {key=value} style attributes, {super=}/{sub=} new syntax,
# page/section breaks, composable attrs with markdown, image dimensions.

# ── Section 1: Headings ──
s/QQQ_TITLE_QQQ/# SEDMAT Comprehensive Test v10/
s/QQQ_SUBTITLE_QQQ/## Text Formatting \& Styles/
s/QQQ_H3_QQQ/### Heading Level Three/
s/QQQ_H4_QQQ/#### Heading Level Four/
s/QQQ_H5_QQQ/##### Heading Level Five/
s/QQQ_H6_QQQ/###### Heading Level Six/

# ── Section 2: Inline Styles ──
s/QQQ_BOLD_QQQ/**This text is bold**/
s/QQQ_ITALIC_QQQ/*This text is italic*/
s/QQQ_BOLDITALIC_QQQ/***Bold and italic combined***/
s/QQQ_STRIKE_QQQ/~~Strikethrough text~~/
s/QQQ_CODE_QQQ/`inline code snippet`/
s/QQQ_UNDERLINE_QQQ/__Underlined text here__/
s/QQQ_LINK_QQQ/[Visit Deft.md](https:\/\/deft.md)/

# ── Section 3: Lists ──
s/QQQ_BULLET1_QQQ/- First bullet point/
s/QQQ_BULLET2_QQQ/- Second bullet point/
s/QQQ_BULLET3_QQQ/- Third bullet point/
s/QQQ_NUM1_QQQ/1. First numbered item/
s/QQQ_NUM2_QQQ/1. Second numbered item/
s/QQQ_NUM3_QQQ/1. Third numbered item/
s/QQQ_CHECK1_QQQ/- Unchecked task one/
s/QQQ_CHECK2_QQQ/- Unchecked task two/

# ── Section 4: Nested Lists ──
s/QQQ_NEST_B0_QQQ/- Top level bullet/
s/QQQ_NEST_B1_QQQ/  - Nested bullet level 1/
s/QQQ_NEST_B2_QQQ/    - Nested bullet level 2/
s/QQQ_NEST_N0_QQQ/1. Top level numbered/
s/QQQ_NEST_N1_QQQ/  1. Nested numbered level 1/

# ── Section 5: Regex & Backreferences ──
s/QQQ_HELLO_QQQ/Hello World 2026/
s/QQQ_EMAIL_QQQ/contact: john.doe at example.com/
s/QQQ_PRICE_QQQ/The price is $$500.00/
s/QQQ_GLOBAL_QQQ/Global: AAA BBB CCC/
s/QQQ_CLASS_QQQ/Classes: One-As One-Bs One-Cs/
s/QQQ_WORDS_QQQ/Words: apple banana cherry/
s/(AAA|BBB|CCC)/XXX/g
s/One-([A-Z])s/Three-$1s/g
s/banana/**banana**/
s/QQQ_NAME_QQQ/Name: John Smith/
s/Name: (\w+) (\w+)/Name: \2, \1/
s/QQQ_AMP_QQQ/Amp: MATCHME/
s/MATCHME/**&**/
s/QQQ_DOLLAR1_QQQ/Price: $$49.99 each/
s/QQQ_DOLLAR2_QQQ/Total: $$100 + $$200 = $$300/

# ── Section 6: Escaping ──
s/QQQ_ESCAPE1_QQQ/Literal: \*asterisks\* and \#hashes/
s/QQQ_ESCAPE2_QQQ/Path: \/usr\/local\/bin/

# ── Section 7: Horizontal Rules ──
s/QQQ_HRULE_QQQ/---/
s/QQQ_HRULE_TEXT_QQQ/Text after the horizontal rule/

# ── Section 8: Blockquotes ──
s/QQQ_BLOCKQUOTE1_QQQ/> This is a simple blockquote/
s/QQQ_BLOCKQUOTE2_QQQ/> The only way to do great work is to love what you do. — Steve Jobs/

# ── Section 9: Code Blocks ──
s/QQQ_CODEBLOCK1_QQQ/```javascript\nfunction greet(name) {\n  return \x60Hello, ${name}!\x60;\n}\n```/
s/QQQ_CODEBLOCK2_QQQ/```go\nfunc main() {\n  fmt.Println("Hello")\n}\n```/

# ── Section 10: Superscript & Subscript (legacy syntax) ──
s/QQQ_SUPER_WHOLE_QQQ/TM/
s/QQQ_SUPER_INLINE_QQQ/E = mc^{2}/
s/QQQ_SUPER_COMBO_QQQ/x^{2} + y^{2} = z^{2}/
s/QQQ_SUB_WHOLE_QQQ/~{0}/
s/QQQ_SUB_INLINE_QQQ/H~{2}O/
s/QQQ_SUB_MULTI_QQQ/C~{6}H~{12}O~{6}/

# ── Section 11: Superscript & Subscript (new {super=}/{sub=} syntax) ──
s/QQQ_SUPER_NEW_QQQ/{super=TM}/
s/QQQ_SUB_NEW_QQQ/{sub=0}/
s/QQQ_SUPER_INLINE_NEW_QQQ/E = mc{super=2}/
s/QQQ_SUB_INLINE_NEW_QQQ/H{sub=2}O/

# ── Section 12: Footnotes ──
s/QQQ_FOOTNOTE1_QQQ/[^This is a simple footnote]/
s/QQQ_FOOTNOTE2_QQQ/[^According to research published in Nature, 2024]/

# ── Section 13: Pipe Tables ──
s/QQQ_TABLE_PIPE_QQQ/| Col A | Col B |\n| Data 1 | Data 2 |\n| Data 3 | Data 4 |/
s/QQQ_TABLE_BOLD_QQQ/| **Feature** | **Status** | **Notes** |\n| Headings | Done | H1-H6 |\n| Tables | Done | Pipe syntax |\n| Regex | Done | All ops |/

# ── Section 14: Table Dimensions ──
s/QQQ_TABLE_DIM_QQQ/|3x4|/
s/QQQ_TABLE_HEADER_QQQ/|5x4:header|/
s/QQQ_TABLE_EMPTY_QQQ/|3x3|/

# ── Section 15: Image ──
s/QQQ_IMAGE_QQQ/![](https:\/\/www.google.com\/images\/branding\/googlelogo\/2x\/googlelogo_color_272x92dp.png){width=400}/

# ── Section 16: Commands (d/a/i/y) ──
s/QQQ_COMBO_HEAD_QQQ/## Results \& Summary/
s/QQQ_DELETE_ME_QQQ/DELETE THIS LINE/
d/DELETE THIS LINE/
s/QQQ_APPEND_TARGET_QQQ/Heading and Bold Test/
a/Heading and Bold Test/Line one of insert\nLine two of insert\nLine three of insert/
s/QQQ_INSERT_TARGET_QQQ/Energy/
i/Energy/**Energy**/
s/QQQ_XLAT_QQQ/xlAt AEIOU/

# ── Section 17: Style Attributes {key=value} ──

# Font only
s/QQQ_ATTR_FONT_QQQ/Font: Georgia text/{font=Georgia}

# Size only
s/QQQ_ATTR_SIZE_QQQ/Size: 20pt text/{size=20}

# Color only
s/QQQ_ATTR_COLOR_QQQ/Color: Red text/{color=#FF0000}

# Background only
s/QQQ_ATTR_BG_QQQ/Highlight: Yellow bg/{bg=#FFFF00}

# Multiple attrs combined
s/QQQ_ATTR_COMBO_QQQ/Combo: Blue Georgia 16pt/{font=Georgia size=16 color=#0000FF}

# Attrs + markdown bold
s/QQQ_ATTR_BOLD_FONT_QQQ/**Bold Montserrat 18pt**/{font=Montserrat size=18}

# Attrs + markdown heading
s/QQQ_ATTR_HEADING_FONT_QQQ/### Styled Heading/{font=Playfair+Display size=22 color=#333333}

# Page break
s/QQQ_ATTR_BREAK_QQQ/End of Page One/{break=page}
s/QQQ_ATTR_AFTER_BREAK_QQQ/Start of Page Two/

# Image with dimensions (already Pandoc-style)
s/QQQ_ATTR_IMG_QQQ/![](https:\/\/www.google.com\/images\/branding\/googlelogo\/2x\/googlelogo_color_272x92dp.png){width=200 height=68}/

# Section break
s/QQQ_ATTR_SECTION_QQQ/End Before Section Break/{break=section}
s/QQQ_ATTR_AFTER_SECTION_QQQ/New Section Content/
