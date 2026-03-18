package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesCreateFromTemplateCmd struct {
	TemplateID   string   `arg:"" name:"templateId" help:"Template presentation ID"`
	Title        string   `arg:"" name:"title" help:"New presentation title"`
	Replace      []string `name:"replace" help:"Text replacement in format 'key=value' (repeatable)"`
	Replacements string   `name:"replacements" help:"JSON file containing replacements" type:"existingfile"`
	Parent       string   `name:"parent" help:"Destination folder ID"`
	Exact        bool     `name:"exact" help:"Use exact string matching instead of {{key}} placeholders"`
}

func (c *SlidesCreateFromTemplateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	templateID := normalizeGoogleID(strings.TrimSpace(c.TemplateID))
	if templateID == "" {
		return usage("empty templateId")
	}

	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	// Parse replacements from both sources
	replacements, err := c.parseReplacements()
	if err != nil {
		return err
	}

	if len(replacements) == 0 {
		return usage("no replacements specified (use --replace or --replacements)")
	}

	parent := normalizeGoogleID(strings.TrimSpace(c.Parent))
	if dryRunErr := dryRunExit(ctx, flags, "slides.create-from-template", map[string]any{
		"template_id":  templateID,
		"title":        title,
		"parent":       parent,
		"exact":        c.Exact,
		"replacements": replacements,
	}); dryRunErr != nil {
		return dryRunErr
	}

	// Create Drive service to copy the template
	driveSvc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	// Copy template
	f := &drive.File{
		Name: title,
	}
	if parent != "" {
		f.Parents = []string{parent}
	}

	created, err := driveSvc.Files.Copy(templateID, f).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("failed to copy template: %w", err)
	}

	if created == nil {
		return errors.New("template copy failed")
	}

	// Verify it's a presentation
	if created.MimeType != "application/vnd.google-apps.presentation" {
		return fmt.Errorf("template is not a Google Slides presentation (got %s)", created.MimeType)
	}

	presentationID := created.Id

	// Create Slides service for text replacement
	slidesSvc, err := newSlidesService(ctx, account)
	if err != nil {
		return fmt.Errorf("failed to create slides service: %w", err)
	}

	requests := buildTemplateReplacementRequests(replacements, c.Exact)

	// Execute batch update
	result, err := slidesSvc.Presentations.BatchUpdate(presentationID, &slides.BatchUpdatePresentationRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		u.Err().Printf("Warning: presentation created but text replacement failed: %v", err)
		u.Err().Printf("Presentation ID: %s", presentationID)
		u.Err().Printf("You may need to manually edit or delete this presentation")
		return fmt.Errorf("text replacement failed: %w", err)
	}

	replacementStats := collectTemplateReplacementStats(requests, result.Replies)

	// Output results
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"presentationId": presentationID,
			"name":           created.Name,
			"link":           created.WebViewLink,
			"replacements":   replacementStats,
		})
	}

	u.Out().Printf("Created presentation from template")
	u.Out().Printf("id\t%s", presentationID)
	u.Out().Printf("name\t%s", created.Name)
	if created.WebViewLink != "" {
		u.Out().Printf("link\t%s", created.WebViewLink)
	}

	if len(replacementStats) > 0 {
		u.Out().Println("")
		u.Out().Println("Replacements:")
		for key, count := range replacementStats {
			if count > 0 {
				u.Out().Printf("  %s\t%d occurrences", key, count)
			} else {
				u.Out().Printf("  %s\tnot found", key)
			}
		}
	}

	return nil
}

// parseReplacements combines replacements from --replace flags and --replacements file
func (c *SlidesCreateFromTemplateCmd) parseReplacements() (map[string]string, error) {
	result := make(map[string]string)

	// Load from JSON file first
	if c.Replacements != "" {
		data, err := os.ReadFile(c.Replacements)
		if err != nil {
			return nil, fmt.Errorf("failed to read replacements file: %w", err)
		}

		var fileReplacements map[string]interface{}
		if err := json.Unmarshal(data, &fileReplacements); err != nil {
			return nil, fmt.Errorf("invalid JSON in replacements file: %w", err)
		}

		// Convert all values to strings
		for k, v := range fileReplacements {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}

			switch val := v.(type) {
			case string:
				result[k] = val
			case float64:
				result[k] = fmt.Sprintf("%g", val)
			case bool:
				result[k] = fmt.Sprintf("%t", val)
			case nil:
				result[k] = ""
			default:
				// Try to marshal back to JSON for complex types
				jsonVal, err := json.Marshal(v)
				if err != nil {
					return nil, fmt.Errorf("cannot convert value for key %q to string: %w", k, err)
				}
				result[k] = string(jsonVal)
			}
		}
	}

	// Process --replace flags (these override file values)
	for _, replacement := range c.Replace {
		parts := strings.SplitN(replacement, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid replacement format %q (expected key=value)", replacement)
		}

		key := strings.TrimSpace(parts[0])
		value := parts[1] // Don't trim value - it might be intentionally whitespace

		if key == "" {
			return nil, fmt.Errorf("empty key in replacement %q", replacement)
		}

		result[key] = value
	}

	return result, nil
}

func buildTemplateReplacementRequests(replacements map[string]string, exact bool) []*slides.Request {
	requests := make([]*slides.Request, 0, len(replacements))
	for key, value := range replacements {
		requests = append(requests, &slides.Request{
			ReplaceAllText: &slides.ReplaceAllTextRequest{
				ContainsText: &slides.SubstringMatchCriteria{
					Text:      templateReplacementSearchText(key, exact),
					MatchCase: true,
				},
				ReplaceText: value,
			},
		})
	}
	return requests
}

func collectTemplateReplacementStats(requests []*slides.Request, replies []*slides.Response) map[string]int64 {
	stats := make(map[string]int64)
	for i, reply := range replies {
		if reply == nil || reply.ReplaceAllText == nil || i >= len(requests) || requests[i] == nil || requests[i].ReplaceAllText == nil {
			continue
		}
		stats[templateReplacementDisplayKey(requests[i].ReplaceAllText.ContainsText.Text)] = reply.ReplaceAllText.OccurrencesChanged
	}
	return stats
}

func templateReplacementSearchText(key string, exact bool) string {
	if exact {
		return key
	}
	if strings.HasPrefix(key, "{{") && strings.HasSuffix(key, "}}") {
		return key
	}
	return fmt.Sprintf("{{%s}}", key)
}

func templateReplacementDisplayKey(searchText string) string {
	return strings.TrimSuffix(strings.TrimPrefix(searchText, "{{"), "}}")
}
