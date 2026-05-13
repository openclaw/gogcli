package cmd

// ParseOptions configures the markdown parser. Filled in Task 8.
type ParseOptions struct {
	DefaultFAStyle string // "solid"|"regular"|"brands"; empty → "solid"
}

// ParseMarkdownToSlides is implemented in Task 8.
func ParseMarkdownToSlides(_ string, _ ParseOptions) ([]Slide, error) {
	return nil, nil
}
