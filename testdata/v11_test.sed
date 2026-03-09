# SEDMAT Comprehensive Test v11
# =============================
# Full brace syntax {key=value} as canonical DSL.
# Markdown still works but braces are preferred.

# ── Section 1: Headings (brace syntax) ──
s/QQQ_TITLE_QQQ/SEDMAT Comprehensive Test v11{h=t}/
s/QQQ_SUBTITLE_QQQ/Text Formatting \& Styles{h=s}/
s/QQQ_H3_QQQ/Heading Level Three{h=3}/
s/QQQ_H4_QQQ/Heading Level Four{h=4}/
s/QQQ_H5_QQQ/Heading Level Five{h=5}/
s/QQQ_H6_QQQ/Heading Level Six{h=6}/

# ── Section 2: Inline Styles (brace syntax) ──
s/QQQ_BOLD_QQQ/This text is bold{b}/
s/QQQ_ITALIC_QQQ/This text is italic{i}/
s/QQQ_BOLDITALIC_QQQ/Bold and italic combined{b i}/
s/QQQ_STRIKE_QQQ/Strikethrough text{-}/
s/QQQ_CODE_QQQ/inline code snippet{#}/
s/QQQ_UNDERLINE_QQQ/Underlined text here{_}/
s/QQQ_SMALLCAPS_QQQ/Small Caps Text{w}/
s/QQQ_LINK_QQQ/Visit Deft.md{u=https:\/\/deft.md}/

# ── Section 3: Lists (markdown — convenience) ──
s/QQQ_BULLET1_QQQ/- First bullet point/
s/QQQ_BULLET2_QQQ/- Second bullet point/
s/QQQ_BULLET3_QQQ/- Third bullet point/
s/QQQ_NUM1_QQQ/1. First numbered item/
s/QQQ_NUM2_QQQ/1. Second numbered item/
s/QQQ_NUM3_QQQ/1. Third numbered item/

# ── Section 3b: Checkboxes (brace syntax) ──
s/QQQ_CHECK1_QQQ/Unchecked task one{check}/
s/QQQ_CHECK2_QQQ/Checked task two{check=y}/
s/QQQ_CHECK3_QQQ/Unchecked task three{check=n}/

# ── Section 4: Nested Lists (markdown) ──
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
s/banana/banana{b}/
s/QQQ_NAME_QQQ/Name: John Smith/
s/Name: (\w+) (\w+)/Name: \2, \1/
s/QQQ_AMP_QQQ/Amp: MATCHME/
s/MATCHME/&{b}/
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

# ── Section 10: Superscript & Subscript (brace syntax) ──
s/QQQ_SUPER_QQQ/TM{^}/
s/QQQ_SUB_QQQ/0{,}/
s/QQQ_SUPER_INLINE_QQQ/E = mc{^=2}/
s/QQQ_SUB_INLINE_QQQ/H{,=2}O/
s/QQQ_FORMULA_QQQ/x{^=2} + y{^=2} = z{^=2}/
s/QQQ_CHEMISTRY_QQQ/C{,=6}H{,=12}O{,=6}/

# ── Section 11: Footnotes (markdown) ──
s/QQQ_FOOTNOTE1_QQQ/[^This is a simple footnote]/
s/QQQ_FOOTNOTE2_QQQ/[^According to research published in Nature, 2024]/

# ── Section 12: Pipe Tables (markdown — convenience) ──
s/QQQ_TABLE_PIPE_QQQ/| Col A | Col B |\n| Data 1 | Data 2 |\n| Data 3 | Data 4 |/
s/QQQ_TABLE_BOLD_QQQ/| **Feature** | **Status** | **Notes** |\n| Headings | Done | H1-H6 |\n| Tables | Done | Pipe syntax |\n| Regex | Done | All ops |/

# ── Section 13: Table Dimensions (old pipe + new brace) ──
s/QQQ_TABLE_DIM_QQQ/|3x4|/
s/QQQ_TABLE_HEADER_QQQ/|5x4:header|/
s/QQQ_TABLE_EMPTY_QQQ/|3x3|/
s/QQQ_TABLE_BRACE_QQQ/{T=4x3:header}/

# ── Section 14: Image ──
s/QQQ_IMAGE_QQQ/![](https:\/\/www.google.com\/images\/branding\/googlelogo\/2x\/googlelogo_color_272x92dp.png){x=400}/

# ── Section 15: Commands (d/a/i/y) ──
s/QQQ_COMBO_HEAD_QQQ/Results \& Summary{h=2}/
s/QQQ_DELETE_ME_QQQ/DELETE THIS LINE/
d/DELETE THIS LINE/
s/QQQ_APPEND_TARGET_QQQ/Heading and Bold Test/
a/Heading and Bold Test/Line one of insert\nLine two of insert\nLine three of insert/
s/QQQ_INSERT_TARGET_QQQ/Energy/
i/Energy/Energy{b}/
s/QQQ_XLAT_QQQ/xlAt AEIOU/

# ── Section 16: Style Attributes (brace syntax) ──

# Font only
s/QQQ_FONT_QQQ/Font: Georgia text{f=Georgia}/

# Size only
s/QQQ_SIZE_QQQ/Size: 20pt text{s=20}/

# Color (hex)
s/QQQ_COLOR_QQQ/Color: Red text{c=#FF0000}/

# Color (named)
s/QQQ_COLOR_NAMED_QQQ/Color: Blue named{c=blue}/

# Background (hex)
s/QQQ_BG_QQQ/Highlight: Yellow bg{z=#FFFF00}/

# Background (named)
s/QQQ_BG_NAMED_QQQ/Highlight: Green bg{z=green}/

# Multiple attrs combined
s/QQQ_COMBO_STYLE_QQQ/Combo: Blue Georgia 16pt{f=Georgia s=16 c=blue}/

# Brace bold + font
s/QQQ_BOLD_FONT_QQQ/Bold Montserrat 18pt{b f=Montserrat s=18}/

# Heading + font styling
s/QQQ_HEAD_FONT_QQQ/Styled Heading{h=3 f=Playfair+Display s=22 c=#333333}/

# Page break (brace)
s/QQQ_BREAK_PAGE_QQQ/End of Page One{+=p}/
s/QQQ_AFTER_BREAK_QQQ/Start of Page Two/

# Section break (brace)
s/QQQ_BREAK_SECTION_QQQ/End Before Section Break{+=s}/
s/QQQ_AFTER_SECTION_QQQ/New Section Content/

# Image with dimensions (brace)
s/QQQ_IMG_DIM_QQQ/![](https:\/\/www.google.com\/images\/branding\/googlelogo\/2x\/googlelogo_color_272x92dp.png){x=200 y=68}/

# ── Section 17: New v3.5 Features ──

# Alignment
s/QQQ_ALIGN_CENTER_QQQ/Centered text here{a=center}/
s/QQQ_ALIGN_RIGHT_QQQ/Right-aligned text{a=right}/

# Indent
s/QQQ_INDENT_QQQ/Indented paragraph{n=2}/

# Paragraph spacing
s/QQQ_SPACING_QQQ/Spaced paragraph{p=12,6}/

# Line spacing / leading
s/QQQ_LEADING_QQQ/Double spaced text{l=2}/

# Inline scoping
s/QQQ_INLINE_BOLD_QQQ/The word {b=Warning} is bold here/
s/QQQ_INLINE_SUP_QQQ/10{^=th} percentile/
s/QQQ_INLINE_MULTI_QQQ/H{,=2}SO{,=4} is sulfuric acid/

# Reset
s/QQQ_RESET_QQQ/Plain text after reset{0}/

# Negation
s/QQQ_NEGATE_QQQ/Not bold anymore{!b}/

# Bookmark
s/QQQ_BOOKMARK_QQQ/Chapter One Begins{@=ch1}/
# TODO: bookmark link requires bookmark to exist first (separate batch)
# s/QQQ_BOOKMARK_LINK_QQQ/Jump to Chapter One{u=#ch1}/
s/QQQ_BOOKMARK_LINK_QQQ/Jump to Chapter One{u=https:\/\/example.com}/

# Opacity
s/QQQ_OPACITY_QQQ/Faded text{o=50}/
