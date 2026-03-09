# Sedmat: Sed-like Document Formatting

**Sedmat** is a sed-inspired DSL for formatting Google Docs. It uses brace syntax `{key=value}` as its canonical format, with legacy Markdown shortcuts for convenience.

Full spec: [sedmat.org](https://sedmat.org)

```bash
gog docs sed <DOC_ID> '<expression>'             # single expression
gog docs sed <DOC_ID> -f expressions.sed          # batch from file
gog docs sed <DOC_ID> -f seed.txt -p              # paste seed content
gog docs sed <DOC_ID> '<expr>' --dry-run          # preview without applying
echo 's/foo/{b}bar/' | gog docs sed <DOC_ID>      # pipe from stdin
gog docs sed <DOC_ID> <<'EOF'                      # heredoc
s/title/{h=t}My Report/
s/draft/{b c=green}final/
EOF
```

---

## Brace Syntax (Canonical)

The brace DSL is the primary way to format text. Braces go **before** the text by default:

```bash
s/text/{b}hello/                     # braces before (default)
s/text/hello{b}/                     # braces after — same result
s/text/{c=red}warning{b}/           # before and after
```

### Boolean Flags

| Key | Long | Effect |
|-----|------|--------|
| `b` | `bold` | **Bold** |
| `i` | `italic` | *Italic* |
| `_` | `underline` | Underline |
| `-` | `strike` | ~~Strikethrough~~ |
| `#` | `code` | `Monospace` (Courier New + grey bg) |
| `^` | `sup` | Superscript |
| `,` | `sub` | Subscript |
| `w` | `smallcaps` | Small Caps |

Negate with `!`: `{!b}` removes bold, `{!i}` removes italic.

```bash
s/hello/{b}hello/                    # bold
s/world/{i _}world/                  # italic + underline
s/note/{b i}note/                    # bold + italic
s/draft/{-}draft/                    # strikethrough
s/code/{#}code/                      # monospace
s/TM/{^}TM/                         # superscript
s/H2O/H2O — {,}2/                   # subscript
s/Title/{w}Title/                    # small caps
s/loud/{!b}loud/                     # remove bold
```

### Key=Value Properties

| Key | Long | Value | Effect |
|-----|------|-------|--------|
| `c` | `color` | hex/name | Text color |
| `z` | `bg` | hex/name | Background/highlight |
| `f` | `font` | name | Font family |
| `s` | `size` | pt | Font size |
| `u` | `url` | URL | Hyperlink |
| `h` | `heading` | 1-6/t/s | Heading level (t=title, s=subtitle) |
| `a` | `align` | left/center/right | Paragraph alignment |
| `l` | `leading` | pt | Line spacing |
| `n` | `indent` | pt | Indentation |
| `o` | `opacity` | 0-100 | Text opacity |
| `k` | `kerning` | pt | Letter spacing |
| `p` | `spacing` | before,after | Paragraph spacing (pt) |
| `e` | `effect` | name | Text effect |
| `x` | `width` | px | Image width |
| `y` | `height` | px | Image height |

```bash
s/error/{c=red}error/                # red text
s/warning/{z=#FFFF00}warning/        # yellow highlight
s/title/{f=Georgia s=24}title/       # Georgia 24pt
s/heading/{h=2}heading/              # Heading 2
s/TITLE/{h=t}TITLE/                  # Title style
s/click here/{u=https:\/\/example.com}click here/  # hyperlink
s/para/{a=center}para/               # center align
s/note/{p=12,6}note/                 # 12pt before, 6pt after
s/fine/{o=50}fine/                   # 50% opacity
```

### Combos

Combine any flags and properties in one brace block:

```bash
s/heading/{b f=Montserrat s=18}heading/
s/title/{h=3 f=Playfair+Display s=22 c=#333333}title/
s/link/{b i u=https:\/\/example.com}link/
```

### Clear Formatting

```bash
s/messy/{0}messy/                    # strip all formatting
```

### Bookmarks & Internal Links

```bash
s/Chapter 1/{@=ch1}Chapter 1/       # create bookmark anchor
s/see chapter 1/{u=#ch1}see chapter 1/  # link to bookmark
```

---

## Structural Commands (Brace)

### Breaks

```bash
s/PAGEBREAK/{+=p}PAGEBREAK/         # page break
s/COLBREAK/{+=c}COLBREAK/           # column break
s/SECBREAK/{+=s}SECBREAK/           # section break
```

### Tables

```bash
s/placeholder/{T=4x3}placeholder/           # 4 rows × 3 cols
s/placeholder/{T=4x3:header}placeholder/    # with header row
```

### Checkboxes

```bash
s/task/{check}task/                  # unchecked checkbox
s/done/{check=y}done/               # checked
s/todo/{check=n}todo/               # explicitly unchecked
```

### Images

```bash
s/placeholder/{img=https:\/\/example.com\/photo.jpg}placeholder/
s/placeholder/{img=https:\/\/example.com\/photo.jpg x=400}placeholder/
s/placeholder/{img=https:\/\/example.com\/photo.jpg x=200 y=68}placeholder/
```

---

## Legacy Markdown Syntax

Markdown shortcuts are supported for convenience. The brace syntax is preferred.

### Headings

```bash
s/text/# Heading 1/
s/text/## Heading 2/
s/text/### Heading 3/
s/text/#### Heading 4/
s/text/##### Heading 5/
s/text/###### Heading 6/
```

### Inline Styles

```bash
s/text/**bold**/
s/text/*italic*/
s/text/***bold italic***/
s/text/~~strikethrough~~/
s/text/`monospace`/
s/text/__underline__/
s/text/^{superscript}/
s/text/~{subscript}/
```

### Links & Images

```bash
s/text/[link text](https:\/\/example.com)/
s/placeholder/![alt](https:\/\/example.com\/img.png)/
s/placeholder/![alt](https:\/\/example.com\/img.png =400x200)/
```

### Lists

```bash
s/text/- bullet item/
s/text/1. numbered item/
s/text/  - nested bullet (2 spaces per level)/
s/text/- [ ] unchecked checkbox/
s/text/- [x] checked checkbox/
```

### Blocks

```bash
s/text/---/                          # horizontal rule
s/text/> blockquote text/
```

### Code Blocks

````bash
s/text/```\ncode line 1\ncode line 2\n```/
````

### Footnotes

```bash
s/text/Some claim[^1]/
s/FOOTNOTE/[^1]: Source citation/
```

### Pipe Tables

```bash
s/placeholder/| Col A | Col B |\n| data1 | data2 |/
```

---

## Sed Commands

### Substitution

```bash
s/pattern/replacement/               # first match
s/pattern/replacement/g              # all matches (global)
s/pattern/replacement/i              # case-insensitive
s/pattern/replacement/2              # nth match only
s/pattern/replacement/m              # multiline (^/$ match line boundaries)
s/pattern/replacement/gi             # combine flags
```

### Delete, Append, Insert, Transliterate

```bash
d/pattern/                           # delete lines matching pattern
a/pattern/new text/                  # append text after matching lines
i/pattern/new text/                  # insert text before matching lines
y/abc/xyz/                           # transliterate a→x, b→y, c→z
```

### Back-references

```bash
s/(important)/{b}$1/                 # capture + format
s/([A-Z]{2,})/{b}$1/g               # bold all caps words
s/"([^"]+)"/{i}$1/g                 # italicize quoted text
s/hello/{b}&/                        # & = whole match
```

### Positional Insert

```bash
s/^$/Initial content/                # empty doc only
s/^/Prepended text\n/                # beginning of doc
s/$/\nAppended text/                 # end of doc
```

---

## Table Operations

Tables are numbered in document order (1-indexed). Use `|-1|` for last, `|*|` for all.

### Cell References

```bash
s/|1|[1,1]/{b}Header/               # row 1, col 1
s/|1|[2,3]/value/                    # row 2, col 3
s/|1|[1,*]/{b}&/                     # bold entire row (wildcard)
s/|1|[*,2]/data/                     # set entire column
```

### Add Rows & Columns

```bash
s/|1|[+1,0]//                        # append row
s/|1|[0,+1]//                        # append column
```

### Merge & Split

```bash
s/|1|[1,1:1,3]/merge/               # merge cells row1 col1-3
s/|1|[2,2]/split/                    # split merged cell
```

### Delete Tables

```bash
s/|1|//                              # delete first table
s/|*|//                              # delete all tables
```

---

## Image References

```bash
s/!(1)/!(https:\/\/new.png)/         # replace 1st image
s/!(-1)//                            # delete last image
s/!(*)/!(https:\/\/placeholder.png)/g  # replace all images
s/![logo]/!(https:\/\/new-logo.png)/ # match by alt text
```

---

## Paragraph Addressing

Target specific paragraphs by number using address prefixes. Use `gog docs structure` to see paragraph numbers.

```bash
# Introspection — see paragraph numbers, types, and content
gog docs structure <DOC_ID>              # show numbered structure
gog docs cat <DOC_ID> -N                 # cat with [N] prefixes

# Delete by paragraph number
gog docs sed <DOC_ID> '5d'              # delete paragraph 5
gog docs sed <DOC_ID> '3,7d'            # delete paragraphs 3-7
gog docs sed <DOC_ID> '$d'              # delete last paragraph

# Substitute within addressed paragraphs
gog docs sed <DOC_ID> '5s/.*/New text/' # replace all text in paragraph 5
gog docs sed <DOC_ID> '3,7s/old/new/g' # replace within paragraphs 3-7

# Insert/Append around addressed paragraphs
gog docs sed <DOC_ID> '5a/New line/'    # append after paragraph 5
gog docs sed <DOC_ID> '3i/Before text/' # insert before paragraph 3
gog docs sed <DOC_ID> '$a/Last line/'   # append after last paragraph
```

### Address Syntax

| Address | Meaning |
|---------|---------|
| `N` | Paragraph number N (1-based) |
| `N,M` | Range from paragraph N to M |
| `$` | Last paragraph |
| `N,$` | From paragraph N to end |

### Multi-Tab Support

```bash
gog docs structure <DOC_ID> --tab "Sheet1"
gog docs sed <DOC_ID> --tab "Sheet1" '3d'
```

---

## Batch Mode

Create a `.sed` file with one expression per line. Comments start with `#`.

```bash
# format-doc.sed
s/QQQ_TITLE/{h=t}Report Title/
s/QQQ_AUTHOR/{i c=gray}John Doe/
s/QQQ_DATE/2026-02-22/

# Apply
gog docs sed <DOC_ID> -a <account> -f format-doc.sed
```

### Seed + Format Workflow

1. Clear the doc: `gog docs clear <DOC_ID> -a <account>`
2. Insert seed via positional insert: `gog docs sed <DOC_ID> -f seed.sed -a <account>`
3. Format it: `gog docs sed <DOC_ID> -f format.sed -a <account>`

---

## Options

| Flag | Effect |
|------|--------|
| `-f <file>` | Read expressions from file (batch mode) |
| `-p` | Paste mode (insert file content as text) |
| `-n` / `--dry-run` | Preview without applying changes |
| `-a <account>` | Google account to use |

---

## See Also

- `gog docs clear` — Clear all content from a document
- `gog docs edit` — Simple find/replace without regex
- `gog docs get` — Export document content
- `gog docs images list` — List images in document
- [sedmat.org](https://sedmat.org) — Full Sedmat specification
