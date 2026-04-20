package cmd

// Shared string literals used across multiple command groups.
//
// Motivation: some linters (e.g. goconst) encourage consolidating repeated
// literals across the package; keeping them in one place avoids accidental
// coupling to unrelated semantic constants.
const (
	literalAll = "all"

	// literalMarkdownTripleDash is the three-dash token used for YAML
	// frontmatter delimiters, horizontal rules, and slide separators.
	literalMarkdownTripleDash = "---"
)
