package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const googleSitesQuery = "mimeType = '" + driveMimeGoogleSite + "'"

var newSitesDriveService = googleapi.NewSitesDrive

type SitesCmd struct {
	List   SitesListCmd   `cmd:"" name:"list" aliases:"ls" help:"List Google Sites visible in Drive"`
	Search SitesSearchCmd `cmd:"" name:"search" aliases:"find" help:"Search Google Sites by text or Drive query"`
	Get    SitesGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get Google Site metadata"`
	URL    SitesURLCmd    `cmd:"" name:"url" aliases:"open" help:"Print Google Site editor URLs"`
}

type SitesListCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"20"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	Query     string `name:"query" help:"Additional Drive query filter"`
	AllDrives bool   `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
	Drive     string `name:"drive" aliases:"drive-id" help:"Scope list to a specific shared drive (uses corpora=drive with driveId). Mutually exclusive with --no-all-drives."`
}

type SitesSearchCmd struct {
	Query     []string `arg:"" name:"query" help:"Search query"`
	RawQuery  bool     `name:"raw-query" aliases:"raw" help:"Treat query as Drive query language (pass through; will still be constrained to Google Sites)"`
	Max       int64    `name:"max" aliases:"limit" help:"Max results" default:"20"`
	Page      string   `name:"page" aliases:"cursor" help:"Page token"`
	AllDrives bool     `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
	Drive     string   `name:"drive" aliases:"drive-id" help:"Scope search to a specific shared drive (uses corpora=drive with driveId). Mutually exclusive with --no-all-drives."`
}

type SitesGetCmd struct {
	SiteID string `arg:"" name:"siteId" help:"Site Drive file ID or sites.google.com editor URL"`
	Fields string `name:"fields" help:"Drive API field mask (overrides the default set; e.g. 'id,name,webViewLink')"`
}

type SitesURLCmd struct {
	SiteIDs []string `arg:"" name:"siteId" help:"Site Drive file IDs or sites.google.com editor URLs"`
}

func (c *SitesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	if strings.TrimSpace(c.Drive) != "" && !c.AllDrives {
		return usage("--drive cannot be combined with --no-all-drives")
	}
	svc, err := requireSitesDriveService(ctx, flags)
	if err != nil {
		return err
	}

	resp, err := listDriveFiles(ctx, svc, driveFileListOptions{
		query:     buildSitesQuery(c.Query),
		max:       c.Max,
		page:      c.Page,
		allDrives: c.AllDrives,
		driveID:   strings.TrimSpace(c.Drive),
	})
	if err != nil {
		return err
	}
	return writeDriveFileList(ctx, resp, "No sites")
}

func (c *SitesSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	query := strings.TrimSpace(strings.Join(c.Query, " "))
	if query == "" {
		return usage("missing query")
	}
	if strings.TrimSpace(c.Drive) != "" && !c.AllDrives {
		return usage("--drive cannot be combined with --no-all-drives")
	}
	svc, err := requireSitesDriveService(ctx, flags)
	if err != nil {
		return err
	}

	resp, err := listDriveFiles(ctx, svc, driveFileListOptions{
		query:     buildSitesSearchQuery(query, c.RawQuery),
		max:       c.Max,
		page:      c.Page,
		allDrives: c.AllDrives,
		driveID:   strings.TrimSpace(c.Drive),
	})
	if err != nil {
		return err
	}
	return writeDriveFileList(ctx, resp, "No sites")
}

func (c *SitesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	svc, err := requireSitesDriveService(ctx, flags)
	if err != nil {
		return err
	}
	siteID := normalizeGoogleID(strings.TrimSpace(c.SiteID))
	if siteID == "" {
		return usage("empty siteId")
	}

	mask := driveFileGetFields
	if strings.TrimSpace(c.Fields) != "" {
		mask = c.Fields
	}
	f, err := svc.Files.Get(siteID).
		SupportsAllDrives(true).
		Fields(gapi.Field(mask)).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if f.MimeType != "" && f.MimeType != driveMimeGoogleSite {
		return fmt.Errorf("file %s is not a Google Site (mimeType=%q)", siteID, f.MimeType)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"site": f})
	}

	u.Out().Printf("id\t%s", f.Id)
	u.Out().Printf("name\t%s", f.Name)
	u.Out().Printf("type\t%s", f.MimeType)
	u.Out().Printf("modified\t%s", f.ModifiedTime)
	if f.WebViewLink != "" {
		u.Out().Printf("link\t%s", f.WebViewLink)
	}
	return nil
}

func (c *SitesURLCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	svc, err := requireSitesDriveService(ctx, flags)
	if err != nil {
		return err
	}
	if len(c.SiteIDs) == 0 {
		return usage("missing siteId")
	}

	urls := make([]map[string]string, 0, len(c.SiteIDs))
	for _, rawID := range c.SiteIDs {
		siteID := normalizeGoogleID(strings.TrimSpace(rawID))
		if siteID == "" {
			return usage("empty siteId")
		}
		link, linkErr := siteWebLink(ctx, svc, siteID)
		if linkErr != nil {
			return linkErr
		}
		urls = append(urls, map[string]string{"id": siteID, "url": link})
		if !outfmt.IsJSON(ctx) {
			u.Out().Printf("%s\t%s", siteID, link)
		}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"urls": urls})
	}
	return nil
}

func buildSitesSearchQuery(query string, rawQuery bool) string {
	if rawQuery {
		return buildSitesQuery(query)
	}
	if looksLikeDriveQueryLanguage(query) {
		return buildSitesQuery(query)
	}
	return buildSitesQuery(fmt.Sprintf("fullText contains '%s'", escapeDriveQueryString(query)))
}

func buildSitesQuery(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		q = googleSitesQuery
	} else {
		q = fmt.Sprintf("(%s) and %s", q, googleSitesQuery)
	}
	if !hasDriveTrashedPredicate(q) {
		q += " and " + driveQueryNotTrashed
	}
	return q
}

func requireSitesDriveService(ctx context.Context, flags *RootFlags) (*drive.Service, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return nil, err
	}
	return newSitesDriveService(ctx, account)
}

func siteWebLink(ctx context.Context, svc *drive.Service, siteID string) (string, error) {
	f, err := svc.Files.Get(siteID).
		SupportsAllDrives(true).
		Fields("mimeType, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return "", err
	}
	if f.MimeType != "" && f.MimeType != driveMimeGoogleSite {
		return "", fmt.Errorf("file %s is not a Google Site (mimeType=%q)", siteID, f.MimeType)
	}
	if f.WebViewLink != "" {
		return f.WebViewLink, nil
	}
	return fmt.Sprintf("https://sites.google.com/d/%s/edit", siteID), nil
}
