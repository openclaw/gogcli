package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	youtube "google.golang.org/api/youtube/v3"

	"github.com/steipete/gogcli/internal/errfmt"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const youtubeForceSSLOAuthScope = "https://www.googleapis.com/auth/youtube.force-ssl"

var youtubeVideoDefaultParts = []string{
	"snippet",
	"contentDetails",
	"statistics",
}

// youtubeVideoAllNonOwnerParts matches every videos.list part documented as
// readable for arbitrary videos. Owner-only parts remain available through an
// explicit --parts list for callers querying their own uploads.
var youtubeVideoAllNonOwnerParts = []string{
	"contentDetails",
	"id",
	"liveStreamingDetails",
	"localizations",
	"paidProductPlacementDetails",
	"player",
	"recordingDetails",
	"snippet",
	"statistics",
	"status",
	"topicDetails",
}

type YouTubeCmd struct {
	Activities    YouTubeActivitiesCmd    `cmd:"" name:"activities" aliases:"activity" help:"List channel activities"`
	Videos        YouTubeVideosCmd        `cmd:"" name:"videos" aliases:"video" help:"List or get videos"`
	Playlists     YouTubePlaylistsCmd     `cmd:"" name:"playlists" aliases:"playlist" help:"Manage playlists"`
	Comments      YouTubeCommentsCmd      `cmd:"" name:"comments" aliases:"comment" help:"List comment threads"`
	Channels      YouTubeChannelsCmd      `cmd:"" name:"channels" aliases:"channel" help:"List channels"`
	Search        YouTubeSearchCmd        `cmd:"" name:"search" aliases:"find" help:"Search YouTube for videos, channels, or playlists"`
	Subscriptions YouTubeSubscriptionsCmd `cmd:"" name:"subscriptions" aliases:"subscription" help:"Manage channel subscriptions"`
}

type YouTubeActivitiesCmd struct {
	List YouTubeActivitiesListCmd `cmd:"" name:"list" aliases:"ls" help:"List activities for a channel (or authenticated user)"`
}

type YouTubeActivitiesListCmd struct {
	ChannelID string `name:"channel-id" help:"Channel ID"`
	Mine      bool   `name:"mine" help:"Use authenticated user's channel (requires -a account)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YouTubeActivitiesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	channelID := strings.TrimSpace(c.ChannelID)
	svc, err := getYouTubeChannelListService(ctx, flags, channelID, c.Mine)
	if err != nil {
		return err
	}

	call := svc.Activities.List([]string{"snippet", "contentDetails"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if channelID != "" {
		call = call.ChannelId(channelID)
	} else {
		call = call.Mine(true)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	return writeYouTubeList(ctx, resp.Items, resp.NextPageToken, "No activities", youtubeActivityColumns())
}

type YouTubeVideosCmd struct {
	List YouTubeVideosListCmd `cmd:"" name:"list" aliases:"ls" help:"List videos by ID, chart, or your rating"`
}

type YouTubeVideosListCmd struct {
	ID       string `name:"id" help:"Comma-separated video IDs"`
	Chart    string `name:"chart" help:"Chart: mostPopular (regionCode required)"`
	Region   string `name:"region" help:"Region code (e.g. US) for chart"`
	MyRating string `name:"my-rating" help:"Your rated videos: like (liked videos) or dislike (requires -a account)"`
	Parts    string `name:"parts" help:"Comma-separated videos.list parts or 'all' (default: snippet,contentDetails,statistics)"`
	Max      int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page     string `name:"page" help:"Page token"`
}

func (c *YouTubeVideosListCmd) resolveParts() ([]string, error) {
	parts := splitCSV(c.Parts)
	if len(parts) == 0 {
		return append([]string(nil), youtubeVideoDefaultParts...), nil
	}
	all := false
	for _, part := range parts {
		if part == literalAll {
			all = true
			break
		}
	}
	if !all {
		return parts, nil
	}
	if len(parts) != 1 {
		return nil, usage("--parts all cannot be combined with explicit parts")
	}
	return append([]string(nil), youtubeVideoAllNonOwnerParts...), nil
}

func (c *YouTubeVideosListCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	ids := splitCSV(c.ID)
	chart := strings.TrimSpace(c.Chart)
	region := strings.TrimSpace(c.Region)
	myRating := strings.TrimSpace(c.MyRating)

	modes := 0
	if len(ids) > 0 {
		modes++
	}
	if chart != "" {
		modes++
	}
	if myRating != "" {
		modes++
	}
	if modes == 0 {
		return usage("set --id VIDEO_IDS, --chart mostPopular, or --my-rating like")
	}
	if modes > 1 {
		return usage("use only one of --id, --chart, or --my-rating")
	}
	if chart != "" && chart != "mostPopular" {
		return usage("--chart must be mostPopular")
	}
	if chart == "mostPopular" && region == "" {
		return usage("--chart mostPopular requires --region (e.g. US)")
	}
	if myRating != "" && myRating != "like" && myRating != "dislike" {
		return usage("--my-rating must be like or dislike")
	}
	parts, err := c.resolveParts()
	if err != nil {
		return err
	}

	var svc *youtube.Service
	if myRating != "" {
		// myRating reads are per-user and require OAuth, not an API key.
		account, accErr := requireAccount(flags)
		if accErr != nil {
			return accErr
		}
		svc, err = getYouTubeServiceForAccount(ctx, account)
	} else {
		svc, err = getYouTubeReadService(ctx, flags)
	}
	if err != nil {
		return err
	}

	call := svc.Videos.List(parts).
		MaxResults(c.Max).
		PageToken(c.Page)
	switch {
	case len(ids) > 0:
		call = call.Id(ids...)
	case chart != "":
		call = call.Chart(chart).RegionCode(region)
	default:
		call = call.MyRating(myRating)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	return writeYouTubeList(ctx, resp.Items, resp.NextPageToken, "No videos", youtubeVideoColumns())
}

type YouTubePlaylistsCmd struct {
	List   YouTubePlaylistsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List playlists by channel or authenticated user"`
	Items  YouTubePlaylistsItemsCmd  `cmd:"" name:"items" aliases:"item" help:"List the videos inside a playlist"`
	Create YouTubePlaylistsCreateCmd `cmd:"" name:"create" help:"Create a new playlist"`
	Add    YouTubePlaylistsAddCmd    `cmd:"" name:"add" help:"Add a video to a playlist"`
	Remove YouTubePlaylistsRemoveCmd `cmd:"" name:"remove" aliases:"rm" help:"Remove a video from a playlist"`
	Delete YouTubePlaylistsDeleteCmd `cmd:"" name:"delete" aliases:"del" help:"Delete a playlist"`
}

type YouTubePlaylistsListCmd struct {
	ChannelID string `name:"channel-id" help:"Channel ID"`
	Mine      bool   `name:"mine" help:"Use authenticated user (requires -a account)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YouTubePlaylistsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	channelID := strings.TrimSpace(c.ChannelID)
	svc, err := getYouTubeChannelListService(ctx, flags, channelID, c.Mine)
	if err != nil {
		return err
	}

	call := svc.Playlists.List([]string{"snippet", "contentDetails"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if channelID != "" {
		call = call.ChannelId(channelID)
	} else {
		call = call.Mine(true)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	return writeYouTubeList(ctx, resp.Items, resp.NextPageToken, "No playlists", youtubePlaylistColumns())
}

type YouTubePlaylistsItemsCmd struct {
	List YouTubePlaylistsItemsListCmd `cmd:"" name:"list" aliases:"ls" help:"List the videos inside a playlist"`
}

type YouTubePlaylistsItemsListCmd struct {
	PlaylistID string `name:"playlist-id" help:"Playlist ID (use LL for your liked videos; LL/private playlists require -a account)"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results per page" default:"50"`
	Page       string `name:"page" aliases:"cursor" help:"Page token"`
	All        bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
}

func (c *YouTubePlaylistsItemsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	playlistID := strings.TrimSpace(c.PlaylistID)
	if playlistID == "" {
		return usage("set --playlist-id ID (use LL for your liked videos; LL/private playlists require -a account)")
	}

	var svc *youtube.Service
	var err error
	if playlistID == "LL" {
		account, accErr := requireAccount(flags)
		if accErr != nil {
			return accErr
		}
		svc, err = getYouTubeServiceForAccount(ctx, account)
	} else {
		svc, err = getYouTubeReadService(ctx, flags)
	}
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*youtube.PlaylistItem, string, error) {
		resp, callErr := svc.PlaylistItems.List([]string{"snippet", "contentDetails"}).
			PlaylistId(playlistID).
			MaxResults(c.Max).
			PageToken(pageToken).
			Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return youtubeItemsOrEmpty(resp.Items), resp.NextPageToken, nil
	}
	items, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(items),
			"nextPageToken": nextPageToken,
		})
	}
	if len(items) == 0 {
		u.Err().Println("No playlist items")
		return nil
	}
	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactYouTubeRows(items),
		youtubePlaylistItemColumns(),
	); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type YouTubePlaylistsCreateCmd struct {
	Title       string `name:"title" required:"" help:"Playlist title"`
	Description string `name:"description" help:"Playlist description"`
	Privacy     string `name:"privacy" help:"Privacy: public, unlisted, private" default:"private" enum:"public,unlisted,private"`
}

func (c *YouTubePlaylistsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("--title is required")
	}
	description := strings.TrimSpace(c.Description)
	if err := dryRunExit(ctx, flags, "youtube.playlists.create", map[string]any{
		"title":       title,
		"description": description,
		"privacy":     c.Privacy,
	}); err != nil {
		return err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := getYouTubeWriteServiceForAccount(ctx, account)
	if err != nil {
		return err
	}

	pl, err := svc.Playlists.Insert([]string{"snippet", "status"}, &youtube.Playlist{
		Snippet: &youtube.PlaylistSnippet{
			Title:       title,
			Description: description,
		},
		Status: &youtube.PlaylistStatus{
			PrivacyStatus: c.Privacy,
		},
	}).Do()
	if err != nil {
		return wrapYouTubeWriteError(err, flags)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"playlist": pl})
	}
	if outfmt.IsPlain(ctx) {
		u.Out().Linef("id\t%s", pl.Id)
		u.Out().Linef("title\t%s", title)
		u.Out().Linef("privacy\t%s", c.Privacy)
		return nil
	}
	u.Out().Printf("Created playlist: %s (ID: %s)\n", title, pl.Id)
	return nil
}

type YouTubePlaylistsAddCmd struct {
	PlaylistID string `name:"playlist-id" required:"" help:"Playlist ID"`
	VideoID    string `name:"video-id" required:"" help:"Video ID to add"`
	Position   int64  `name:"position" help:"Position in playlist (0-based); appends if not set" default:"-1"`
}

func (c *YouTubePlaylistsAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	playlistID := strings.TrimSpace(c.PlaylistID)
	videoID := strings.TrimSpace(c.VideoID)
	if playlistID == "" {
		return usage("--playlist-id is required")
	}
	if videoID == "" {
		return usage("--video-id is required")
	}
	if c.Position < -1 {
		return usage("--position must be >= 0 when set")
	}
	request := map[string]any{
		"playlistId": playlistID,
		"videoId":    videoID,
	}
	if c.Position >= 0 {
		request["position"] = c.Position
	}
	if err := dryRunExit(ctx, flags, "youtube.playlists.add", request); err != nil {
		return err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := getYouTubeWriteServiceForAccount(ctx, account)
	if err != nil {
		return err
	}

	item := &youtube.PlaylistItem{
		Snippet: &youtube.PlaylistItemSnippet{
			PlaylistId: playlistID,
			ResourceId: &youtube.ResourceId{
				Kind:    "youtube#video",
				VideoId: videoID,
			},
		},
	}
	if c.Position >= 0 {
		item.Snippet.Position = c.Position
		item.Snippet.ForceSendFields = []string{"Position"}
	}

	result, err := svc.PlaylistItems.Insert([]string{"snippet"}, item).Do()
	if err != nil {
		return wrapYouTubeWriteError(err, flags)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"playlistItem": result})
	}
	if outfmt.IsPlain(ctx) {
		u.Out().Linef("item_id\t%s", result.Id)
		u.Out().Linef("playlist_id\t%s", playlistID)
		u.Out().Linef("video_id\t%s", videoID)
		return nil
	}
	u.Out().Printf("Added video %s to playlist %s (item ID: %s)\n", videoID, playlistID, result.Id)
	return nil
}

type YouTubePlaylistsRemoveCmd struct {
	PlaylistID string `name:"playlist-id" help:"Playlist ID (required with --video-id)"`
	VideoID    string `name:"video-id" help:"Video ID to remove"`
	ItemID     string `name:"item-id" help:"Playlist item ID to remove directly"`
}

func (c *YouTubePlaylistsRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	playlistID := strings.TrimSpace(c.PlaylistID)
	videoID := strings.TrimSpace(c.VideoID)
	itemID := strings.TrimSpace(c.ItemID)
	if videoID == "" && itemID == "" {
		return usage("set --video-id or --item-id")
	}
	if videoID != "" && itemID != "" {
		return usage("use either --video-id or --item-id, not both")
	}
	if videoID != "" && playlistID == "" {
		return usage("--playlist-id is required with --video-id")
	}

	if itemID != "" {
		if err := dryRunAndConfirmDestructive(ctx, flags, "youtube.playlists.remove", map[string]any{
			"itemId": itemID,
		}, fmt.Sprintf("remove playlist item %s", itemID)); err != nil {
			return err
		}
	} else if flags != nil && flags.DryRun {
		return dryRunAndConfirmDestructive(ctx, flags, "youtube.playlists.remove", map[string]any{
			"playlistId": playlistID,
			"videoId":    videoID,
		}, fmt.Sprintf("remove video %s from playlist %s", videoID, playlistID))
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := getYouTubeWriteServiceForAccount(ctx, account)
	if err != nil {
		return err
	}

	if videoID != "" {
		resp, lookupErr := svc.PlaylistItems.List([]string{"id"}).
			PlaylistId(playlistID).
			VideoId(videoID).
			MaxResults(1).
			Do()
		if lookupErr != nil {
			return wrapYouTubeWriteError(lookupErr, flags)
		}
		if len(resp.Items) == 0 {
			return fmt.Errorf("video %s not found in playlist %s", videoID, playlistID)
		}
		itemID = resp.Items[0].Id
		if err := dryRunAndConfirmDestructive(ctx, flagsWithoutDryRun(flags), "youtube.playlists.remove", map[string]any{
			"itemId": itemID,
		}, fmt.Sprintf("remove playlist item %s", itemID)); err != nil {
			return err
		}
	}

	if err := svc.PlaylistItems.Delete(itemID).Do(); err != nil {
		return wrapYouTubeWriteError(err, flags)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"removed": true, "itemId": itemID})
	}
	if outfmt.IsPlain(ctx) {
		u.Out().Linef("removed\ttrue")
		u.Out().Linef("item_id\t%s", itemID)
		return nil
	}
	u.Out().Printf("Removed playlist item %s\n", itemID)
	return nil
}

type YouTubePlaylistsDeleteCmd struct {
	PlaylistID string `arg:"" name:"playlist-id" help:"Playlist ID to delete"`
}

func (c *YouTubePlaylistsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	playlistID := strings.TrimSpace(c.PlaylistID)
	if playlistID == "" {
		return usage("playlist-id is required")
	}
	if err := dryRunAndConfirmDestructive(ctx, flags, "youtube.playlists.delete", map[string]any{
		"playlistId": playlistID,
	}, fmt.Sprintf("delete playlist %s", playlistID)); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := getYouTubeWriteServiceForAccount(ctx, account)
	if err != nil {
		return err
	}
	if err := svc.Playlists.Delete(playlistID).Do(); err != nil {
		return wrapYouTubeWriteError(err, flags)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"deleted": true, "playlistId": playlistID})
	}
	if outfmt.IsPlain(ctx) {
		u.Out().Linef("deleted\ttrue")
		u.Out().Linef("playlist_id\t%s", playlistID)
		return nil
	}
	u.Out().Printf("Deleted playlist %s\n", playlistID)
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
	videoID := strings.TrimSpace(c.VideoID)
	channelID := strings.TrimSpace(c.ChannelID)
	if videoID == "" && channelID == "" {
		return usage("set --video-id ID or --channel-id ID")
	}
	if videoID != "" && channelID != "" {
		return usage("use either --video-id or --channel-id, not both")
	}

	svc, err := getYouTubeCommentsService(ctx, flags)
	if err != nil {
		return err
	}

	call := svc.CommentThreads.List([]string{"snippet"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if videoID != "" {
		call = call.VideoId(videoID)
	} else {
		call = call.ChannelId(channelID)
	}
	resp, err := call.Do()
	if err != nil {
		return wrapYouTubeCommentsError(err, flags)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(resp.Items),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No comment threads")
		return nil
	}
	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactYouTubeRows(resp.Items),
		youtubeCommentColumns(),
	); err != nil {
		return err
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubeChannelsCmd struct {
	List YouTubeChannelsListCmd `cmd:"" name:"list" aliases:"ls" help:"List channels by ID or authenticated user"`
}

type YouTubeChannelsListCmd struct {
	ID   string `name:"id" help:"Comma-separated channel IDs"`
	Mine bool   `name:"mine" help:"Use authenticated user (requires -a account)"`
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page string `name:"page" help:"Page token"`
}

func (c *YouTubeChannelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	ids := splitCSV(c.ID)
	if len(ids) == 0 && !c.Mine {
		return usage("set --id CHANNEL_IDS or --mine (--mine requires -a account)")
	}
	if len(ids) > 0 && c.Mine {
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
		svc, err = getYouTubeReadService(ctx, flags)
	}
	if err != nil {
		return err
	}

	call := svc.Channels.List([]string{"snippet", "statistics", "contentDetails"}).
		MaxResults(c.Max).
		PageToken(c.Page)
	if len(ids) > 0 {
		call = call.Id(ids...)
	} else {
		call = call.Mine(true)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(resp.Items),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No channels")
		return nil
	}
	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactYouTubeRows(resp.Items),
		youtubeChannelColumns(),
	); err != nil {
		return err
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubeSearchCmd struct {
	List YouTubeSearchListCmd `cmd:"" name:"list" aliases:"ls" help:"Search for videos, channels, or playlists"`
}

type YouTubeSearchListCmd struct {
	Query     string `arg:"" help:"Search query"`
	Type      string `name:"type" help:"Resource type: video, channel, playlist (comma-separated)" default:"video"`
	Order     string `name:"order" help:"Sort order: relevance, date, rating, title, videoCount, viewCount" default:"relevance" enum:"relevance,date,rating,title,videoCount,viewCount"`
	ChannelID string `name:"channel-id" help:"Restrict results to a specific channel"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
	Page      string `name:"page" help:"Page token"`
}

func (c *YouTubeSearchListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	query := strings.TrimSpace(c.Query)
	if query == "" {
		return usage("search query is required")
	}

	types := splitCSV(c.Type)
	if len(types) == 0 {
		return usage("--type must be video, channel, or playlist (comma-separated)")
	}
	for _, t := range types {
		switch t {
		case "video", "channel", "playlist":
		default:
			return usage("--type must be video, channel, or playlist (comma-separated)")
		}
	}

	svc, err := getYouTubeReadService(ctx, flags)
	if err != nil {
		return err
	}

	call := svc.Search.List([]string{"snippet"}).
		Q(query).
		Type(types...).
		Order(c.Order).
		MaxResults(c.Max).
		PageToken(c.Page)
	if channelID := strings.TrimSpace(c.ChannelID); channelID != "" {
		call = call.ChannelId(channelID)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}
	resp.Items = filterYouTubeSearchItemsByType(resp.Items, types)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(resp.Items),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No results")
		return nil
	}
	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactYouTubeRows(resp.Items),
		youtubeSearchColumns(),
	); err != nil {
		return err
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type YouTubeSubscriptionsCmd struct {
	List        YouTubeSubscriptionsListCmd        `cmd:"" name:"list" aliases:"ls" help:"List subscriptions for authenticated user"`
	Subscribe   YouTubeSubscriptionsSubscribeCmd   `cmd:"" name:"subscribe" help:"Subscribe to a channel"`
	Unsubscribe YouTubeSubscriptionsUnsubscribeCmd `cmd:"" name:"unsubscribe" help:"Unsubscribe from a channel"`
}

type YouTubeSubscriptionsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results per page" default:"50"`
	Page string `name:"page" aliases:"cursor" help:"Page token"`
	All  bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
}

func (c *YouTubeSubscriptionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateYouTubeMax(c.Max); err != nil {
		return err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := getYouTubeServiceForAccount(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*youtube.Subscription, string, error) {
		resp, callErr := svc.Subscriptions.List([]string{"snippet"}).
			Mine(true).
			MaxResults(c.Max).
			PageToken(pageToken).
			Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return youtubeItemsOrEmpty(resp.Items), resp.NextPageToken, nil
	}
	items, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(items),
			"nextPageToken": nextPageToken,
		})
	}
	if len(items) == 0 {
		u.Err().Println("No subscriptions")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tCHANNEL_ID\tTITLE\tSUBSCRIBED_AT")
	for _, s := range items {
		printSubscriptionRow(w, s)
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")

	return nil
}

func printSubscriptionRow(w io.Writer, s *youtube.Subscription) {
	channelID := ""
	title := ""
	subscribedAt := ""
	if s.Snippet != nil {
		title = s.Snippet.Title
		subscribedAt = s.Snippet.PublishedAt
		if s.Snippet.ResourceId != nil {
			channelID = s.Snippet.ResourceId.ChannelId
		}
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Id, sanitizeTab(channelID), sanitizeTab(title), sanitizeTab(subscribedAt))
}

type YouTubeSubscriptionsSubscribeCmd struct {
	ChannelID string `name:"channel-id" help:"Channel ID to subscribe to"`
}

func (c *YouTubeSubscriptionsSubscribeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	channelID := strings.TrimSpace(c.ChannelID)
	if channelID == "" {
		return usage("--channel-id is required")
	}
	if err := dryRunExit(ctx, flags, "youtube.subscriptions.subscribe", map[string]any{
		"channelId": channelID,
	}); err != nil {
		return err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := getYouTubeWriteServiceForAccount(ctx, account)
	if err != nil {
		return err
	}

	sub, err := svc.Subscriptions.Insert([]string{"snippet"}, &youtube.Subscription{
		Snippet: &youtube.SubscriptionSnippet{
			ResourceId: &youtube.ResourceId{
				Kind:      "youtube#channel",
				ChannelId: channelID,
			},
		},
	}).Do()
	if err != nil {
		return wrapYouTubeWriteError(err, flags)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"subscription": sub})
	}
	if outfmt.IsPlain(ctx) {
		u.Out().Linef("id\t%s", sub.Id)
		u.Out().Linef("channel_id\t%s", channelID)
		return nil
	}
	u.Out().Printf("Subscribed: %s (subscription ID: %s)\n", channelID, sub.Id)
	return nil
}

type YouTubeSubscriptionsUnsubscribeCmd struct {
	ID        string `name:"id" help:"Subscription ID (from subscriptions list)"`
	ChannelID string `name:"channel-id" help:"Channel ID (looked up to find subscription ID)"`
}

func (c *YouTubeSubscriptionsUnsubscribeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	subID := strings.TrimSpace(c.ID)
	channelID := strings.TrimSpace(c.ChannelID)
	if subID == "" && channelID == "" {
		return usage("set --id or --channel-id")
	}
	if subID != "" && channelID != "" {
		return usage("use either --id or --channel-id, not both")
	}

	// For --id we have everything needed to confirm/dry-run before any I/O.
	if subID != "" {
		if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "youtube.subscriptions.unsubscribe", map[string]any{"id": subID}, fmt.Sprintf("unsubscribe (subscription ID: %s)", subID)); confirmErr != nil {
			return confirmErr
		}
	} else if flags != nil && flags.DryRun {
		return dryRunAndConfirmDestructive(ctx, flags, "youtube.subscriptions.unsubscribe", map[string]any{"channelId": channelID}, fmt.Sprintf("unsubscribe from channel %s", channelID))
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := getYouTubeWriteServiceForAccount(ctx, account)
	if err != nil {
		return err
	}

	if channelID != "" {
		resp, lookupErr := svc.Subscriptions.List([]string{"id"}).
			Mine(true).
			ForChannelId(channelID).
			MaxResults(1).
			Do()
		if lookupErr != nil {
			return wrapYouTubeWriteError(lookupErr, flags)
		}
		if len(resp.Items) == 0 {
			return fmt.Errorf("not subscribed to channel %s", channelID)
		}
		subID = resp.Items[0].Id
		if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "youtube.subscriptions.unsubscribe", map[string]any{"id": subID}, fmt.Sprintf("unsubscribe (subscription ID: %s)", subID)); confirmErr != nil {
			return confirmErr
		}
	}

	if err := svc.Subscriptions.Delete(subID).Do(); err != nil {
		return wrapYouTubeWriteError(err, flags)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"unsubscribed": true, "subscriptionId": subID})
	}
	if outfmt.IsPlain(ctx) {
		u.Out().Linef("unsubscribed\ttrue")
		u.Out().Linef("subscription_id\t%s", subID)
		return nil
	}
	u.Out().Printf("Unsubscribed (subscription ID: %s)\n", subID)
	return nil
}

func validateYouTubeMax(limit int64) error {
	if limit < 1 || limit > 50 {
		return usage("--max must be between 1 and 50")
	}
	return nil
}

func getYouTubeChannelListService(ctx context.Context, flags *RootFlags, channelID string, mine bool) (*youtube.Service, error) {
	if channelID == "" && !mine {
		return nil, usage("set --channel-id ID or --mine (--mine requires -a account)")
	}
	if channelID != "" && mine {
		return nil, usage("use either --channel-id or --mine, not both")
	}
	if !mine {
		return getYouTubeReadService(ctx, flags)
	}
	account, err := requireAccount(flags)
	if err != nil {
		return nil, err
	}
	return getYouTubeServiceForAccount(ctx, account)
}

func writeYouTubeList[T any](ctx context.Context, items []*T, nextPageToken, emptyMessage string, columns []outfmt.Column[*T]) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":         youtubeItemsOrEmpty(items),
			"nextPageToken": nextPageToken,
		})
	}
	u := ui.FromContext(ctx)
	if len(items) == 0 {
		u.Err().Println(emptyMessage)
		return nil
	}
	if err := outfmt.WriteTable(ctx, stdoutWriter(ctx), compactYouTubeRows(items), columns); err != nil {
		return err
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

func youtubeItemsOrEmpty[T any](items []*T) []*T {
	if items == nil {
		return []*T{}
	}
	return items
}

func filterYouTubeSearchItemsByType(items []*youtube.SearchResult, allowed []string) []*youtube.SearchResult {
	if len(items) == 0 || len(allowed) == 0 {
		return items
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, typ := range allowed {
		allowedSet[typ] = true
	}
	filtered := items[:0]
	for _, item := range items {
		if allowedSet[youtubeSearchResultType(item)] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func youtubeSearchResultType(item *youtube.SearchResult) string {
	if item == nil || item.Id == nil {
		return ""
	}
	switch {
	case item.Id.VideoId != "":
		return "video"
	case item.Id.ChannelId != "":
		return "channel"
	case item.Id.PlaylistId != "":
		return "playlist"
	default:
		return ""
	}
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
	return wrapYouTubeForceSSLError(err, flags, "youtube comments")
}

func wrapYouTubeWriteError(err error, flags *RootFlags) error {
	return wrapYouTubeForceSSLError(err, flags, "youtube mutations")
}

func wrapYouTubeForceSSLError(err error, flags *RootFlags, operation string) error {
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
		fmt.Sprintf("%s require OAuth scope %s; re-authenticate with: gog auth add %s --services youtube --extra-scopes %s --force-consent", operation, youtubeForceSSLOAuthScope, account, youtubeForceSSLOAuthScope),
		err,
	)
}
