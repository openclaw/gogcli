## feat(docs): add sedmat — sed-like document formatting DSL

### What is sedmat?

**Sedmat** is a stream-editor (sed) inspired DSL for formatting Google Docs programmatically. It lets you apply text styling, create tables, insert images, and restructure documents using concise expressions — either one at a time or in batch from `.sed` files.

Think `sed` but for Google Docs formatting instead of text transformation.

### Key Features

- **Brace syntax** — `{b i c=red}` for bold, italic, red text; negation with `{!b}`
- **Text styling** — font, size, color, background, alignment, headings, spacing
- **Tables** — create, merge cells, resize columns, apply per-cell formatting
- **Images** — insert from URL, isolation mode, inline placement
- **Batch mode** — process `.sed` files with hundreds of expressions
- **Dry-run** — preview changes without modifying the document
- **Regex flags** — `/g` global, `/i` case-insensitive, `/1` first-match-only
- **Positional inserts** — `i/pos/text/` to insert at specific document positions

### Usage Examples

```bash
# Bold all occurrences of "important"
gog docs sed <DOC_ID> 's/important/{b}important/g'

# Apply heading style and color from a sed file
gog docs sed <DOC_ID> -f format.sed

# Preview changes without applying
gog docs sed <DOC_ID> 's/draft/{- c=gray}draft/' --dry-run

# Seed a document with content, then format it
gog docs sed <DOC_ID> -f content.txt -p
gog docs sed <DOC_ID> -f styling.sed
```

### Test Coverage

- **17 test files** covering unit, integration, edge cases, and fuzz testing
- Tests run with: `go test ./internal/cmd/... -run Sed -count=1`
- Key function coverage: `Run` 95%, `runBatch` 75%, `classifyExprForBatch` 87%, `runNative` 82%

### File Structure

```
internal/cmd/
├── docs_sed.go                  # Main entry point, batch/single execution
├── docs_sed_brace.go            # Brace expression parser
├── docs_sed_brace_format.go     # Brace → Google Docs formatting requests
├── docs_sed_brace_pattern.go    # Pattern matching and classification
├── docs_sed_brace_structural.go # Structural formatting (headings, alignment)
├── docs_sed_commands.go         # Command registration (cobra)
├── docs_sed_dryrun.go           # Dry-run preview engine
├── docs_sed_helpers.go          # Shared utilities
├── docs_sed_images.go           # Image insertion and isolation
├── docs_sed_insert.go           # Positional insert engine
├── docs_sed_manual.go           # Built-in manual/help text
├── docs_sed_nesting.go          # Nested expression handling
├── docs_sed_parse.go            # Expression parser (s/pat/repl/flags)
├── docs_sed_retry.go            # Retry with exponential backoff
├── docs_sed_table_cells.go      # Table cell operations
├── docs_sed_table_create.go     # Table creation
├── docs_sed_table_ops.go        # Table merge/resize operations
├── docs_sed_tables.go           # Table expression routing
├── docs_sed_*_test.go           # 17 test files
docs/
└── sedmat.md                    # Full DSL reference documentation
testdata/
├── v7-v12_*.sed                 # Test expression files
└── v7-v12_seed.txt              # Test seed documents
```

### Commits

1. **`feat(docs): add sedmat — sed-like document formatting DSL`** — core implementation (18 source files)
2. **`test(sed): comprehensive test suite (~75% overall package coverage, near-100% on core sed functions)`** — 17 test files
3. **`docs(sed): sedmat v3.5 reference documentation`** — full DSL reference
