package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
)

type OpenCmd struct {
	Target string `arg:"" name:"target" help:"Google URL or ID"`
	Type   string `name:"type" help:"Type hint (auto|drive|folder|docs|sheets|slides|gmail-thread)" default:"auto" enum:"auto,drive,folder,docs,sheets,slides,gmail-thread"`
}

func (c *OpenCmd) Run(ctx context.Context) error {
	// Always emit untransformed JSON, even if the caller enabled global JSON transforms.
	ctx = outfmt.WithJSONTransform(ctx, outfmt.JSONTransform{})

	target := strings.TrimSpace(c.Target)
	if target == "" {
		return usage("empty target")
	}

	kind := strings.ToLower(strings.TrimSpace(c.Type))
	if kind == "" {
		kind = colorAuto
	}

	url := bestEffortWebURL(kind, target)
	if strings.TrimSpace(url) == "" {
		url = target
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"input": target,
			"type":  kind,
			"url":   url,
		})
	}

	if outfmt.IsPlain(ctx) {
		_, _ = fmt.Fprintf(os.Stdout, "type\t%s\n", kind)
		_, _ = fmt.Fprintf(os.Stdout, "url\t%s\n", url)
		return nil
	}

	_, _ = fmt.Fprintln(os.Stdout, url)
	return nil
}

func bestEffortWebURL(kind string, input string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	id := normalizeGoogleID(input)

	switch kind {
	case "drive", colorAuto:
		// If it's a known Google URL already, prefer canonicalized forms.
		if u := parseMaybeURL(input); u != nil {
			host := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(u.Host), "www."))
			switch host {
			case "drive.google.com":
				// Preserve folder URLs when detected.
				if strings.Contains(u.Path, "/folders/") {
					if id != "" {
						return fmt.Sprintf("https://drive.google.com/drive/folders/%s", id)
					}
				}
				// Generic best-effort file open URL.
				if id != "" {
					return fmt.Sprintf("https://drive.google.com/open?id=%s", id)
				}
			case "docs.google.com":
				// Keep doc-type-specific canonical URLs when possible.
				if id == "" {
					return input
				}
				if strings.Contains(u.Path, "/document/") {
					return fmt.Sprintf("https://docs.google.com/document/d/%s/edit", id)
				}
				if strings.Contains(u.Path, "/spreadsheets/") {
					return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit", id)
				}
				if strings.Contains(u.Path, "/presentation/") {
					return fmt.Sprintf("https://docs.google.com/presentation/d/%s/edit", id)
				}
				return fmt.Sprintf("https://drive.google.com/open?id=%s", id)
			case "mail.google.com", "gmail.google.com":
				th := normalizeGmailThreadID(input)
				if th != "" && th != input {
					return fmt.Sprintf("https://mail.google.com/mail/u/0/#all/%s", th)
				}
				return input
			default:
				return input
			}
		}

		if id != "" {
			return fmt.Sprintf("https://drive.google.com/open?id=%s", id)
		}
		return input
	case "folder":
		if id != "" {
			return fmt.Sprintf("https://drive.google.com/drive/folders/%s", id)
		}
		return ""
	case "docs":
		if id != "" {
			return fmt.Sprintf("https://docs.google.com/document/d/%s/edit", id)
		}
		return ""
	case "sheets":
		if id != "" {
			return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit", id)
		}
		return ""
	case "slides":
		if id != "" {
			return fmt.Sprintf("https://docs.google.com/presentation/d/%s/edit", id)
		}
		return ""
	case "gmail-thread":
		th := normalizeGmailThreadID(input)
		if strings.TrimSpace(th) == "" {
			return ""
		}
		return fmt.Sprintf("https://mail.google.com/mail/u/0/#all/%s", th)
	default:
		return ""
	}
}
