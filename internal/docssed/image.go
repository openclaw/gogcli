//nolint:wsl_v5 // Compact parser stages stay adjacent to the state they consume.
package docssed

import (
	"strconv"
	"strings"
)

// ImageSpec describes an inline image replacement.
type ImageSpec struct {
	URL     string
	Alt     string
	Caption string
	Width   int
	Height  int
}

// ParseImageSyntax parses Markdown image syntax with optional Pandoc dimensions.
func ParseImageSyntax(text string) *ImageSpec {
	if !strings.HasPrefix(text, "![") {
		return nil
	}

	altEnd := strings.Index(text, "](")
	if altEnd == -1 {
		return nil
	}
	rest := text[altEnd+2:]
	urlEnd := -1
	for index, char := range rest {
		if char == '"' || char == ')' || char == '{' {
			urlEnd = index
			break
		}
	}
	if urlEnd == -1 {
		if !strings.HasSuffix(rest, ")") {
			return nil
		}
		urlEnd = len(rest) - 1
	}

	spec := &ImageSpec{
		URL: strings.TrimSpace(rest[:urlEnd]),
		Alt: text[2:altEnd],
	}
	rest = rest[urlEnd:]
	if strings.HasPrefix(rest, " \"") || strings.HasPrefix(rest, "\"") {
		rest = strings.TrimPrefix(rest, " ")
		if titleEnd := strings.Index(rest[1:], "\""); titleEnd != -1 {
			spec.Caption = rest[1 : titleEnd+1]
			rest = rest[titleEnd+2:]
		}
	}

	rest = strings.TrimPrefix(rest, ")")
	if strings.HasPrefix(rest, "{") {
		if attrEnd := strings.Index(rest, "}"); attrEnd != -1 {
			spec.Width, spec.Height = parseImageDimensions(rest[1:attrEnd])
		}
	}
	return spec
}

func parseImageDimensions(attributes string) (width, height int) {
	for _, field := range strings.Fields(attributes) {
		key, value, found := strings.Cut(field, "=")
		if !found {
			continue
		}
		value = strings.TrimSuffix(strings.TrimSuffix(value, "px"), "%")
		number, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		switch key {
		case "width", "w":
			width = number
		case "height", "h":
			height = number
		}
	}
	return width, height
}
