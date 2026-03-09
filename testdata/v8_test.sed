# v8 — Horizontal Rules
s/HRULE_BEFORE/---/
s/HRULE_AFTER/Some text after the rule/

# v8 — Blockquotes
s/BLOCKQUOTE_SIMPLE/> This is a simple blockquote/
s/BLOCKQUOTE_LONG/> The only way to do great work is to love what you do. — Steve Jobs/

# v8 — Code Blocks
s/CODEBLOCK_JS/```js\nfunction greet(name) {\n  return `Hello, \${name}!`;\n}\n```/
s/CODEBLOCK_GO/```go\nfunc main() {\n  fmt.Println("Hello")\n}\n```/

# v8 — Nested Lists (bullets)
s/NESTED_BULLET_L0/- Top level item/
s/NESTED_BULLET_L1/  - First nested item/
s/NESTED_BULLET_L2/    - Second nested item/

# v8 — Nested Lists (numbered)
s/NESTED_NUM_L0/1. First item/
s/NESTED_NUM_L1/  1. Nested numbered/

# v8 — Superscript (whole replacement)
s/SUPER_WHOLE/^{TM}/

# v8 — Superscript (inline)
s/SUPER_INLINE/E = mc^{2}/
s/SUPER_MULTI/x^{2} + y^{2} = z^{2}/

# v8 — Subscript (whole replacement)
s/SUB_WHOLE/~{0}/

# v8 — Subscript (inline)
s/SUB_INLINE/H~{2}O/
s/SUB_MULTI/C~{6}H~{12}O~{6}/

# v8 — Footnotes
s/FOOTNOTE_SIMPLE/[^This is a simple footnote]/
s/FOOTNOTE_LONG/[^According to research published in Nature, 2024]/

# v8 — Combos: bold + inline super
s/COMBO_BOLD_SUPER/**Energy**/
s/COMBO_HEADING_RULE/## Section Divider/
s/COMBO_BLOCKQUOTE_NESTED/> To be or not to be/

# v8 — Code block standalone
s/COMBO_CODE_BLOCK/```\nconst x = 42;\n```/

s/FINAL_LINE/--- v8 test complete ---/
