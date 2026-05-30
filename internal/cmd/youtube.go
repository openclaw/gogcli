package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	youtube "google.golang.org/api/youtube/v3"

	"github.com/steipete/gogcli/internal/errfmt"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const youtubeCommentsOAuthScope = "https://www.googleapis.com/auth/youtube.force-ssl"

type YouTubeCmd struct {
	Activities YouTubeActivitiesCmd `cmd:"" name:"activities" aliases:"activity" help:"List channel activities"`
	Videos     YouTubeVideosCmd     `cmd:"" name:"videos" aliases:"video" help:"List or get videos"`
	Playlists  YouTubePlaylistsCmd  `cmd:"" name:"playlists" aliases:"playlist" help:"List playlists"`
	Comments   YouTubeCommentsCmd   `cmd:"" name:"comments" aliases:"comment" help:"List comment threads"`
	Channels   YouTubeChannelsCmd   `cmd:"" name:"channels" aliases:"channel" help:"List channels"`
}

type YouTubeActivitiesCmd struct {
	List YouTubeActivitiesListCmd `cmd:"" name:"list" aliases:"ls" help:"List activities for a channel (or authenticated user)"`
}

type YouTubeActivitiesListCmd struct {
	ChannelID string `name:"channel-id" help:"Channel ID (use with API key)"`
	Mine      bool   `name:"mine" help:"Use authenticated user's channel (requires -a account)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YouTubeActivitiesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	if c.ChannelID == "" && !c.Mine {
		return usage("set --channel-id ID or --mine (--mine requires -a account)")
	}
	if c.ChannelID != "" && c.Mine {
		return usage("use either --channel-id or --mine, not both")
	}

	var svc *youtube.Service
	var err error
	if c.Mine {
		account, accErr := requireAccount(flags)
		if accErr != nil {
			return accErr
		}
		svc, err = getYouTubeServiceForAccount(ctx, account)
	} else {
		svc, err = getYouTubeServiceWithAPIKey(ctx)
	}
	if err != nil {
		return err
	}

	call := svc.Activities.List([]string{"snippet", "contentDetails"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if c.ChannelID != "" {
		call = call.ChannelId(c.ChannelID)
	} else {
		call = call.Mine(true)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"items":         resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No activities")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "KIND\tVIDEO_ID\tTITLE\tPUBLISHED_AT")
	for _, a := range resp.Items {
		vidID := ""
		if a.ContentDetails != nil && a.ContentDetails.Upload != nil {
			vidID = a.ContentDetails.Upload.VideoId
		}
		title := ""
		if a.Snippet != nil {
			title = a.Snippet.Title
		}
		pubAt := ""
		if a.Snippet != nil && a.Snippet.PublishedAt != "" {
			pubAt = a.Snippet.PublishedAt
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.Kind, sanitizeTab(vidID), sanitizeTab(title), sanitizeTab(pubAt))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubeVideosCmd struct {
	List YouTubeVideosListCmd `cmd:"" name:"list" aliases:"ls" help:"List videos by ID or chart"`
}

type YouTubeVideosListCmd struct {
	ID     string `name:"id" help:"Comma-separated video IDs"`
	Chart  string `name:"chart" help:"Chart: mostPopular (regionCode required)"`
	Region string `name:"region" help:"Region code (e.g. US) for chart"`
	Max    int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page   string `name:"page" help:"Page token"`
}

func (c *YouTubeVideosListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	if c.ID == "" && c.Chart == "" {
		return usage("set --id VIDEO_IDS or --chart mostPopular")
	}
	if c.ID != "" && c.Chart != "" {
		return usage("use either --id or --chart, not both")
	}
	if c.Chart != "" && c.Chart != "mostPopular" {
		return usage("--chart must be mostPopular")
	}
	if c.Chart == "mostPopular" && c.Region == "" {
		return usage("--chart mostPopular requires --region (e.g. US)")
	}

	svc, err := getYouTubeReadService(ctx, flags)
	if err != nil {
		return err
	}

	call := svc.Videos.List([]string{"snippet", "contentDetails", "statistics"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if c.ID != "" {
		call = call.Id(splitCSV(c.ID)...)
	} else {
		call = call.Chart(c.Chart).RegionCode(c.Region)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"items":         resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No videos")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tCHANNEL\tVIEWS\tPUBLISHED_AT")
	for _, v := range resp.Items {
		title := ""
		ch := ""
		views := ""
		pubAt := ""
		if v.Snippet != nil {
			title = v.Snippet.Title
			ch = v.Snippet.ChannelTitle
			pubAt = v.Snippet.PublishedAt
		}
		if v.Statistics != nil {
			views = fmt.Sprintf("%d", v.Statistics.ViewCount)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", v.Id, sanitizeTab(title), sanitizeTab(ch), views, sanitizeTab(pubAt))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubePlaylistsCmd struct {
	List YouTubePlaylistsListCmd `cmd:"" name:"list" aliases:"ls" help:"List playlists by channel or authenticated user"`
}

type YouTubePlaylistsListCmd struct {
	ChannelID string `name:"channel-id" help:"Channel ID (use with API key)"`
	Mine      bool   `name:"mine" help:"Use authenticated user (requires -a account)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YouTubePlaylistsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	if c.ChannelID == "" && !c.Mine {
		return usage("set --channel-id ID or --mine (--mine requires -a account)")
	}
	if c.ChannelID != "" && c.Mine {
		return usage("use either --channel-id or --mine, not both")
	}

	var svc *youtube.Service
	var err error
	if c.Mine {
		account, accErr := requireAccount(flags)
		if accErr != nil {
			return accErr
		}
		svc, err = getYouTubeServiceForAccount(ctx, account)
	} else {
		svc, err = getYouTubeServiceWithAPIKey(ctx)
	}
	if err != nil {
		return err
	}

	call := svc.Playlists.List([]string{"snippet", "contentDetails"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if c.ChannelID != "" {
		call = call.ChannelId(c.ChannelID)
	} else {
		call = call.Mine(true)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"items":         resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No playlists")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tCHANNEL\tVIDEO_COUNT\tPUBLISHED_AT")
	for _, p := range resp.Items {
		title := ""
		ch := ""
		pubAt := ""
		count := int64(0)
		if p.Snippet != nil {
			title = p.Snippet.Title
			ch = p.Snippet.ChannelTitle
			pubAt = p.Snippet.PublishedAt
		}
		if p.ContentDetails != nil {
			count = p.ContentDetails.ItemCount
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", p.Id, sanitizeTab(title), sanitizeTab(ch), count, sanitizeTab(pubAt))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubeCommentsCmd struct {
	List YouTubeCommentsListCmd `cmd:"" name:"list" aliases:"ls" help:"List comment threads for a video or channel"`
}

type YouTubeCommentsListCmd struct {
	VideoID   string `name:"video-id" help:"Video ID (list top-level comments for this video)"`
	ChannelID string `name:"channel-id" help:"Channel ID (list comments that mention the channel)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YouTubeCommentsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	if c.VideoID == "" && c.ChannelID == "" {
		return usage("set --video-id ID or --channel-id ID")
	}
	if c.VideoID != "" && c.ChannelID != "" {
		return usage("use either --video-id or --channel-id, not both")
	}

	svc, err := getYouTubeCommentsService(ctx, flags)
	if err != nil {
		return err
	}

	call := svc.CommentThreads.List([]string{"snippet"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if c.VideoID != "" {
		call = call.VideoId(c.VideoID)
	} else {
		call = call.ChannelId(c.ChannelID)
	}
	resp, err := call.Do()
	if err != nil {
		return wrapYouTubeCommentsError(err, flags)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"items":         resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No comment threads")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tAUTHOR\tTEXT\tLIKE_COUNT\tPUBLISHED_AT")
	for _, t := range resp.Items {
		id := t.Id
		author := ""
		text := ""
		likes := int64(0)
		pubAt := ""
		if t.Snippet != nil && t.Snippet.TopLevelComment != nil && t.Snippet.TopLevelComment.Snippet != nil {
			s := t.Snippet.TopLevelComment.Snippet
			author = s.AuthorDisplayName
			text = s.TextDisplay
			likes = s.LikeCount
			pubAt = s.PublishedAt
		}
		text = strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")
		if len(text) > 60 {
			text = text[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", id, sanitizeTab(author), sanitizeTab(text), likes, sanitizeTab(pubAt))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubeChannelsCmd struct {
	List YouTubeChannelsListCmd `cmd:"" name:"list" aliases:"ls" help:"List channels by ID or authenticated user"`
}

type YouTubeChannelsListCmd struct {
	ID   string `name:"id" help:"Comma-separated channel IDs (use with API key)"`
	Mine bool   `name:"mine" help:"Use authenticated user (requires -a account)"`
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page string `name:"page" help:"Page token"`
}

func (c *YouTubeChannelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	if c.ID == "" && !c.Mine {
		return usage("set --id CHANNEL_IDS or --mine (--mine requires -a account)")
	}
	if c.ID != "" && c.Mine {
		return usage("use either --id or --mine, not both")
	}

	var svc *youtube.Service
	var err error
	if c.Mine {
		account, accErr := requireAccount(flags)
		if accErr != nil {
			return accErr
		}
		svc, err = getYouTubeServiceForAccount(ctx, account)
	} else {
		svc, err = getYouTubeServiceWithAPIKey(ctx)
	}
	if err != nil {
		return err
	}

	call := svc.Channels.List([]string{"snippet", "statistics", "contentDetails"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if c.ID != "" {
		call = call.Id(splitCSV(c.ID)...)
	} else {
		call = call.Mine(true)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"items":         resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No channels")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tSUBS\tVIDEOS\tVIEWS\tPUBLISHED_AT")
	for _, ch := range resp.Items {
		title := ""
		pubAt := ""
		subs := ""
		videos := ""
		views := ""
		if ch.Snippet != nil {
			title = ch.Snippet.Title
			pubAt = ch.Snippet.PublishedAt
		}
		if ch.Statistics != nil {
			subs = fmt.Sprintf("%d", ch.Statistics.SubscriberCount)
			videos = fmt.Sprintf("%d", ch.Statistics.VideoCount)
			views = fmt.Sprintf("%d", ch.Statistics.ViewCount)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", ch.Id, sanitizeTab(title), subs, videos, views, sanitizeTab(pubAt))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

func validateYouTubeMax(limit int64) error {
	if limit < 1 || limit > 50 {
		return usage("--max must be between 1 and 50")
	}
	return nil
}

func getYouTubeReadService(ctx context.Context, flags *RootFlags) (*youtube.Service, error) {
	if youtubeAccountSelectorPresent(flags) {
		account, err := requireAccount(flags)
		if err != nil {
			return nil, err
		}
		return getYouTubeServiceForAccount(ctx, account)
	}
	return getYouTubeServiceWithAPIKey(ctx)
}

func getYouTubeCommentsService(ctx context.Context, flags *RootFlags) (*youtube.Service, error) {
	if youtubeAccountSelectorPresent(flags) {
		account, err := requireAccount(flags)
		if err != nil {
			return nil, err
		}
		return getYouTubeCommentsServiceForAccount(ctx, account)
	}
	return getYouTubeServiceWithAPIKey(ctx)
}

func youtubeAccountSelectorPresent(flags *RootFlags) bool {
	return flagAccount(flags) != "" || strings.TrimSpace(os.Getenv("GOG_ACCOUNT")) != "" || hasDirectAccessToken(flags)
}

func wrapYouTubeCommentsError(err error, flags *RootFlags) error {
	if err == nil {
		return nil
	}
	errText := err.Error()
	if !strings.Contains(errText, "insufficientPermissions") &&
		!strings.Contains(errText, "insufficient authentication scopes") &&
		!strings.Contains(errText, "ACCESS_TOKEN_SCOPE_INSUFFICIENT") {
		return err
	}
	if !youtubeAccountSelectorPresent(flags) {
		return err
	}
	account, accountErr := requireAccount(flags)
	if accountErr != nil {
		return err
	}
	return errfmt.NewUserFacingError(
		fmt.Sprintf("youtube comments OAuth requires %s; re-authenticate with: gog auth add %s --services youtube --extra-scopes %s --force-consent", youtubeCommentsOAuthScope, account, youtubeCommentsOAuthScope),
		err,
	)
}
