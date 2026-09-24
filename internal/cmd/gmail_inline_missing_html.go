package cmd

import (
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"strings"
	"unicode/utf8"

	nethtml "golang.org/x/net/html"
)

func missingInlineImageID(value string, missing map[string]string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 4 || !strings.EqualFold(value[:4], "cid:") {
		return "", nil
	}
	id := value[4:]
	if decoded, err := url.PathUnescape(id); err == nil {
		id = decoded
	}
	key := canonicalContentID(id)
	if _, ok := missing[key]; ok {
		if _, err := strictInlineContentID(id); err != nil {
			return "", err
		}
		return key, nil
	}
	return "", nil
}

func replaceMissingInlineImages(body, messageID string, missing map[string]string) (string, []inlineImageWarning, []string, error) {
	if err := validateMissingImageContainers(body, missing); err != nil {
		return "", nil, nil, err
	}
	tokenizer := nethtml.NewTokenizer(strings.NewReader(body))
	var output strings.Builder
	var warnings []inlineImageWarning
	var markers []string
	warningIndexes := make(map[string]int)
	for {
		kind := tokenizer.Next()
		raw := string(tokenizer.Raw())
		if kind == nethtml.ErrorToken {
			if err := tokenizer.Err(); !errors.Is(err, io.EOF) {
				return "", nil, nil, fmt.Errorf("parse quoted HTML: %w", err)
			}
			output.WriteString(raw)
			break
		}
		if kind != nethtml.StartTagToken && kind != nethtml.SelfClosingTagToken && kind != nethtml.EndTagToken {
			output.WriteString(raw)
			continue
		}
		token := tokenizer.Token()
		name := strings.ToLower(token.Data)
		if name != "img" || kind == nethtml.EndTagToken {
			output.WriteString(raw)
			continue
		}
		id, marker, err := missingInlineImagePlaceholder(token, missing)
		if err != nil {
			return "", nil, nil, err
		}
		if id == "" {
			output.WriteString(raw)
			continue
		}
		if err := validateReplacementImageTag(raw); err != nil {
			return "", nil, nil, err
		}
		output.WriteString(html.EscapeString(marker))
		markers = append(markers, marker)
		if index, exists := warningIndexes[id]; exists {
			warnings[index].Occurrences++
		} else {
			warningIndexes[id] = len(warnings)
			warnings = append(warnings, inlineImageWarning{
				Code: "missing_inline_image", SourceMessageID: messageID, ContentID: missing[id],
				Occurrences: 1, Replacement: missingInlineImagesPlaceholder,
			})
		}
	}
	updated := output.String()
	for _, id := range referencedContentIDs(updated) {
		if _, unresolved := missing[canonicalContentID(id)]; unresolved {
			return "", nil, nil, fmt.Errorf("missing CID %q is used outside a supported image tag", id)
		}
	}
	if len(warnings) != len(missing) {
		return "", nil, nil, fmt.Errorf("missing CID references could not be replaced unambiguously")
	}
	return updated, warnings, markers, nil
}

func validateMissingImageContainers(body string, missing map[string]string) error {
	doc, err := nethtml.Parse(strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("parse quoted HTML: %w", err)
	}
	var walk func(*nethtml.Node, bool) error
	walk = func(node *nethtml.Node, protected bool) error {
		if node.Type == nethtml.ElementNode {
			switch strings.ToLower(node.Data) {
			case "picture", "svg", "math", "template":
				protected = true
			}
			if protected && node.Data == "img" {
				for _, attr := range node.Attr {
					if attr.Key == "src" {
						id, idErr := missingInlineImageID(attr.Val, missing)
						if idErr != nil {
							return idErr
						}
						if id != "" {
							return fmt.Errorf("missing inline image is inside an unsupported HTML container")
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := walk(child, protected); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(doc, false)
}

func missingInlineImagePlaceholder(token nethtml.Token, missing map[string]string) (string, string, error) {
	id, alt := "", ""
	for _, attr := range token.Attr {
		switch strings.ToLower(attr.Key) {
		case "src":
			key, err := missingInlineImageID(attr.Val, missing)
			if err != nil {
				return "", "", err
			}
			if key != "" {
				id = key
			}
		case "alt":
			alt = attr.Val
		}
	}
	if id == "" {
		return "", "", nil
	}
	for _, attr := range token.Attr {
		if (isCIDURLAttribute(attr) && cidURLAttributeName(attr) != "src") ||
			(strings.EqualFold(attr.Key, "style") && (strings.ContainsAny(attr.Val, "(\\") || strings.Contains(attr.Val, "/*"))) {
			return "", "", fmt.Errorf("missing inline image has unsupported resource or style attributes")
		}
	}
	if !utf8.ValidString(missing[id]) || !utf8.ValidString(alt) {
		return "", "", fmt.Errorf("missing inline image has invalid text")
	}
	marker := "[Inline image unavailable: " + missing[id]
	if strings.TrimSpace(alt) != "" {
		marker += " — " + alt
	}
	return id, marker + "]", nil
}

// The HTML tokenizer drops duplicate attributes. Check the original image tag
// before replacing it so an ambiguous src or discarded resource is not hidden.
func validateReplacementImageTag(raw string) error {
	space := func(char byte) bool {
		return char == ' ' || char == '\t' || char == '\n' || char == '\r' || char == '\f'
	}
	i := 1
	for i < len(raw) && !space(raw[i]) && raw[i] != '>' && raw[i] != '/' {
		i++
	}
	seen := make(map[string]bool)
	for i < len(raw) {
		for i < len(raw) && space(raw[i]) {
			i++
		}
		if i == len(raw) || raw[i] == '>' || raw[i:] == "/>" {
			return nil
		}
		start := i
		for i < len(raw) && !space(raw[i]) && raw[i] != '=' && raw[i] != '>' && raw[i] != '/' {
			if raw[i] == 0 || raw[i] == '<' || raw[i] == '\'' || raw[i] == '"' {
				return fmt.Errorf("missing inline image has malformed attributes")
			}
			i++
		}
		name := strings.ToLower(raw[start:i])
		if name == "" || seen[name] {
			return fmt.Errorf("missing inline image has ambiguous attributes")
		}
		seen[name] = true
		for i < len(raw) && space(raw[i]) {
			i++
		}
		if i == len(raw) || raw[i] != '=' {
			continue
		}
		i++
		for i < len(raw) && space(raw[i]) {
			i++
		}
		if i < len(raw) && (raw[i] == '\'' || raw[i] == '"') {
			quote := raw[i]
			i++
			for i < len(raw) && raw[i] != quote {
				i++
			}
			if i == len(raw) {
				return fmt.Errorf("missing inline image has an unclosed attribute")
			}
			i++
		} else {
			for i < len(raw) && !space(raw[i]) && raw[i] != '>' {
				if strings.ContainsRune("\x00<\"'`=", rune(raw[i])) {
					return fmt.Errorf("missing inline image has malformed attributes")
				}
				i++
			}
		}
	}
	return nil
}
