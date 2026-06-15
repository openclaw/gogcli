package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	driveChangesFields             = "nextPageToken,newStartPageToken,changes(kind,type,removed,time,fileId,driveId,file(id,name,mimeType,modifiedTime,trashed,webViewLink))"
	driveChangesWebhookSchemeHTTPS = "https"
	driveChangesServerSchemeHTTP   = "http"
)

type DriveChangesCmd struct {
	StartToken DriveChangesStartTokenCmd `cmd:"" name:"start-token" aliases:"token" help:"Get a Drive changes start page token"`
	List       DriveChangesListCmd       `cmd:"" name:"list" aliases:"ls" help:"List Drive changes since a page token"`
	Poll       DriveChangesPollCmd       `cmd:"" name:"poll" help:"Poll Drive changes with a persisted page token"`
	Serve      DriveChangesServeCmd      `cmd:"" name:"serve" help:"Receive Drive change notifications and run a local hook"`
	Watch      DriveChangesWatchCmd      `cmd:"" name:"watch" help:"Watch Drive changes with a webhook channel"`
	Stop       DriveChangesStopCmd       `cmd:"" name:"stop" help:"Stop a Drive changes webhook channel"`
}

type DriveChangesStartTokenCmd struct {
	DriveID string `name:"drive" aliases:"drive-id" help:"Shared drive ID for a shared-drive change log"`
}

func (c *DriveChangesStartTokenCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	startPageToken, err := getDriveChangesStartToken(ctx, svc, c.DriveID)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"startPageToken": startPageToken})
	}
	u.Out().Linef("startPageToken\t%s", startPageToken)
	return nil
}

type DriveChangesListCmd struct {
	Token          string `name:"token" required:"" help:"Start page token or next page token"`
	Max            int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page           string `name:"page" aliases:"cursor" help:"Alias for --token when continuing a page"`
	All            bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	IncludeRemoved bool   `name:"include-removed" help:"Include removed changes" default:"true" negatable:"_"`
	DriveID        string `name:"drive" aliases:"drive-id" help:"Shared drive ID for a shared-drive change log"`
	FailEmpty      bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no changes"`
}

func (c *DriveChangesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	token := strings.TrimSpace(c.Page)
	if token == "" {
		token = strings.TrimSpace(c.Token)
	}
	if token == "" {
		return usage("missing --token")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	changes, nextPageToken, err := loadDriveChanges(ctx, svc, token, driveChangesLoadOptions{
		max:            c.Max,
		includeRemoved: c.IncludeRemoved,
		driveID:        c.DriveID,
		all:            c.All,
	})
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return writePagedJSONResult(ctx, map[string]any{
			"changes":       changes,
			"nextPageToken": nextPageToken,
		}, len(changes), c.FailEmpty)
	}
	if len(changes) == 0 {
		u.Err().Println("No changes")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TIME\tTYPE\tFILE_ID\tNAME\tREMOVED")
	for _, change := range changes {
		name := ""
		if change.File != nil {
			name = change.File.Name
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\n", change.Time, change.Type, change.FileId, sanitizeTab(name), change.Removed)
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type driveChangesLoadOptions struct {
	max            int64
	includeRemoved bool
	driveID        string
	all            bool
}

type driveChangesPage struct {
	changes           []*drive.Change
	nextPageToken     string
	newStartPageToken string
}

func getDriveChangesStartToken(ctx context.Context, svc *drive.Service, driveID string) (string, error) {
	call := svc.Changes.GetStartPageToken().SupportsAllDrives(true).Context(ctx)
	if driveID = strings.TrimSpace(driveID); driveID != "" {
		call = call.DriveId(driveID)
	}
	resp, err := call.Do()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.StartPageToken) == "" {
		return "", fmt.Errorf("drive changes start-token response was empty")
	}
	return resp.StartPageToken, nil
}

func fetchDriveChangesPage(ctx context.Context, svc *drive.Service, pageToken string, opts driveChangesLoadOptions) (driveChangesPage, error) {
	call := svc.Changes.List(pageToken).
		PageSize(opts.max).
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		IncludeRemoved(opts.includeRemoved).
		Fields(gapi.Field(driveChangesFields)).
		Context(ctx)
	if driveID := strings.TrimSpace(opts.driveID); driveID != "" {
		call = call.DriveId(driveID)
	}
	resp, err := call.Do()
	if err != nil {
		return driveChangesPage{}, err
	}
	return driveChangesPage{
		changes:           resp.Changes,
		nextPageToken:     strings.TrimSpace(resp.NextPageToken),
		newStartPageToken: strings.TrimSpace(resp.NewStartPageToken),
	}, nil
}

func loadDriveChanges(ctx context.Context, svc *drive.Service, pageToken string, opts driveChangesLoadOptions) ([]*drive.Change, string, error) {
	pageToken = strings.TrimSpace(pageToken)
	if pageToken == "" {
		return nil, "", usage("missing --token")
	}
	if !opts.all {
		page, err := fetchDriveChangesPage(ctx, svc, pageToken, opts)
		if err != nil {
			return nil, "", err
		}
		next := page.nextPageToken
		if next == "" {
			next = page.newStartPageToken
		}
		return page.changes, next, nil
	}

	seen := make(map[string]struct{})
	var changes []*drive.Change
	for range 10_000 {
		if _, ok := seen[pageToken]; ok {
			return nil, "", fmt.Errorf("pagination loop: repeated page token %q", pageToken)
		}
		seen[pageToken] = struct{}{}

		page, err := fetchDriveChangesPage(ctx, svc, pageToken, opts)
		if err != nil {
			return nil, "", err
		}
		changes = append(changes, page.changes...)
		if page.nextPageToken != "" {
			pageToken = page.nextPageToken
			continue
		}
		if page.newStartPageToken == "" {
			return nil, "", fmt.Errorf("drive changes response ended without newStartPageToken")
		}
		return changes, page.newStartPageToken, nil
	}
	return nil, "", fmt.Errorf("pagination exceeded max pages")
}

type DriveChangesWatchCmd struct {
	Token        string `name:"token" required:"" help:"Start page token or next page token to watch from"`
	WebhookURL   string `name:"webhook-url" required:"" help:"HTTPS webhook URL for Drive change notifications"`
	ChannelID    string `name:"channel-id" help:"Webhook channel ID (default: generated)"`
	ChannelToken string `name:"channel-token" help:"Opaque token echoed by Google in webhook notifications"`
	ExpirationMS int64  `name:"expiration-ms" help:"Unix epoch milliseconds when the channel should expire"`
	DriveID      string `name:"drive" aliases:"drive-id" help:"Shared drive ID for a shared-drive change log"`
}

func (c *DriveChangesWatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	token := strings.TrimSpace(c.Token)
	webhookURL := strings.TrimSpace(c.WebhookURL)
	if token == "" {
		return usage("missing --token")
	}
	if webhookURL == "" {
		return usage("missing --webhook-url")
	}
	if err := validateDriveChangesWebhookURL(webhookURL); err != nil {
		return err
	}
	if c.ExpirationMS < 0 {
		return usage("--expiration-ms must be >= 0")
	}
	channelID := strings.TrimSpace(c.ChannelID)
	if channelID == "" {
		var err error
		channelID, err = randomChannelID()
		if err != nil {
			return err
		}
	}
	channel := &drive.Channel{
		Id:      channelID,
		Type:    "web_hook",
		Address: webhookURL,
		Token:   strings.TrimSpace(c.ChannelToken),
	}
	if c.ExpirationMS > 0 {
		channel.Expiration = c.ExpirationMS
	}
	driveID := strings.TrimSpace(c.DriveID)
	channelTokenState := ""
	if channel.Token != "" {
		channelTokenState = "provided"
	}

	if err := dryRunExit(ctx, flags, "drive.changes.watch", map[string]any{
		"token":         token,
		"webhook_url":   webhookURL,
		"channel_id":    channelID,
		"channel_token": channelTokenState,
		"expiration_ms": c.ExpirationMS,
		"drive_id":      driveID,
	}); err != nil {
		return err
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	call := svc.Changes.Watch(token, channel).
		SupportsAllDrives(true).
		Context(ctx)
	if driveID != "" {
		call = call.DriveId(driveID)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"channel": resp})
	}
	u.Out().Linef("id\t%s", resp.Id)
	u.Out().Linef("resourceId\t%s", resp.ResourceId)
	u.Out().Linef("resourceUri\t%s", resp.ResourceUri)
	u.Out().Linef("expiration\t%d", resp.Expiration)
	return nil
}

func validateDriveChangesWebhookURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != driveChangesWebhookSchemeHTTPS || u.Host == "" {
		return usage("--webhook-url must be an absolute HTTPS URL")
	}
	return nil
}

type DriveChangesStopCmd struct {
	ChannelID  string `arg:"" name:"channelId" help:"Webhook channel ID"`
	ResourceID string `arg:"" name:"resourceId" help:"Webhook resource ID returned by watch"`
}

func (c *DriveChangesStopCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	channelID := strings.TrimSpace(c.ChannelID)
	resourceID := strings.TrimSpace(c.ResourceID)
	if channelID == "" || resourceID == "" {
		return usage("required: channelId resourceId")
	}

	if err := dryRunExit(ctx, flags, "drive.changes.stop", map[string]any{
		"channel_id":  channelID,
		"resource_id": resourceID,
	}); err != nil {
		return err
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	if err := svc.Channels.Stop(&drive.Channel{Id: channelID, ResourceId: resourceID}).Context(ctx).Do(); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"stopped": true, "channelId": channelID, "resourceId": resourceID})
	}
	u.Out().Linef("stopped\ttrue")
	return nil
}

func randomChannelID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "gog-" + hex.EncodeToString(b[:]), nil
}
