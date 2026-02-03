package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const driveMimeGoogleSite = "application/vnd.google-apps.site"

// SitesCmd manages Google Sites (via Drive API).
type SitesCmd struct {
	List   SitesListCmd   `cmd:"" name:"list" aliases:"ls" help:"List sites"`
	Delete SitesDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete a site"`
}

type SitesListCmd struct {
	User string `name:"user" help:"User email to list sites for"`
	Max  int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *SitesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	query := fmt.Sprintf("mimeType='%s' and trashed=false", driveMimeGoogleSite)
	call := svc.Files.List().
		Q(query).
		Fields("files(id,name,owners(emailAddress),webViewLink,createdTime),nextPageToken").
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true)
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Files) == 0 {
		u.Err().Println("No sites found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tOWNER\tURL\tCREATED")
	for _, file := range resp.Files {
		if file == nil {
			continue
		}
		owner := ""
		if len(file.Owners) > 0 {
			owner = file.Owners[0].EmailAddress
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			sanitizeTab(file.Id),
			sanitizeTab(file.Name),
			sanitizeTab(owner),
			sanitizeTab(file.WebViewLink),
			sanitizeTab(file.CreatedTime),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type SitesDeleteCmd struct {
	Site string `arg:"" name:"site" help:"Site ID or URL"`
}

func (c *SitesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	site := strings.TrimSpace(c.Site)
	if site == "" {
		return usage("site is required")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	siteID, err := resolveSiteID(ctx, svc, site)
	if err != nil {
		return err
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete site %s", siteID)); err != nil {
		return err
	}

	if err := svc.Files.Delete(siteID).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete site %s: %w", siteID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"siteId": siteID, "deleted": true})
	}

	u.Out().Printf("Deleted site: %s\n", siteID)
	return nil
}

func resolveSiteID(ctx context.Context, svc *drive.Service, input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", usage("site is required")
	}
	if !strings.Contains(trimmed, "/") && !strings.Contains(trimmed, "http") {
		return trimmed, nil
	}

	pageToken := ""
	query := fmt.Sprintf("mimeType='%s' and trashed=false", driveMimeGoogleSite)
	for {
		call := svc.Files.List().
			Q(query).
			Fields("files(id,name,webViewLink),nextPageToken").
			SupportsAllDrives(true).
			IncludeItemsFromAllDrives(true)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Context(ctx).Do()
		if err != nil {
			return "", fmt.Errorf("resolve site %s: %w", trimmed, err)
		}

		for _, file := range resp.Files {
			if file == nil {
				continue
			}
			if file.Id == trimmed {
				return file.Id, nil
			}
			if file.WebViewLink == trimmed || strings.Contains(file.WebViewLink, trimmed) {
				return file.Id, nil
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return "", fmt.Errorf("could not resolve site %q (use gog sites list to find the site ID)", trimmed)
}
