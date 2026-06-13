package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	appconfig "github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	maxPhotosPickerPageSize  = 100
	maxPhotosPickerItemCount = 2000
)

var (
	errPhotosPickerWaitTimeout          = errors.New("session wait timed out")
	errPhotosPickerRepeatedPageToken    = errors.New("repeated page token from Photos Picker")
	errPhotosPickerPollingConfigMissing = errors.New("response is missing Photos Picker pollingConfig")
)

func openPhotosPickerBrowser(ctx context.Context, uri string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.CommandContext(ctx, "open", uri) //nolint:gosec // executable is fixed; arg is a Google-generated Picker URI
	case literalWindows:
		command = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", uri) //nolint:gosec // executable is fixed
	default:
		command = exec.CommandContext(ctx, "xdg-open", uri) //nolint:gosec // executable is fixed; arg is a Google-generated Picker URI
	}
	return command.Start()
}

type PhotosPickerCmd struct {
	Create   PhotosPickerCreateCmd   `cmd:"" name:"create" aliases:"new,start" help:"Create a photo-picking session"`
	Get      PhotosPickerGetCmd      `cmd:"" name:"get" aliases:"info,show" help:"Get a photo-picking session"`
	Wait     PhotosPickerWaitCmd     `cmd:"" name:"wait" aliases:"poll" help:"Wait until the user finishes picking media"`
	List     PhotosPickerListCmd     `cmd:"" name:"list" aliases:"ls,items" help:"List media selected in a session"`
	Download PhotosPickerDownloadCmd `cmd:"" name:"download" aliases:"dl" help:"Download selected media bytes"`
	Delete   PhotosPickerDeleteCmd   `cmd:"" name:"delete" aliases:"rm,close" help:"Delete a photo-picking session"`
}

type PhotosPickerCreateCmd struct {
	MaxItems int64 `name:"max-items" help:"Maximum items the user may select (max 2000; 0 uses the API default)" default:"0"`
	Open     bool  `name:"open" aliases:"browser" help:"Open the Picker URI in the default browser"`
}

func (c *PhotosPickerCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := validatePhotosPickerMaxItems(c.MaxItems); err != nil {
		return err
	}
	if dryRunErr := dryRunExit(ctx, flags, "photos.picker.sessions.create", map[string]any{
		"max_items": c.MaxItems,
		"open":      c.Open,
	}); dryRunErr != nil {
		return dryRunErr
	}
	client, err := requirePhotosPickerClient(ctx, flags)
	if err != nil {
		return err
	}
	session, err := client.CreateSession(ctx, c.MaxItems)
	if err != nil {
		return err
	}
	if err := writePhotosPickerSession(ctx, session); err != nil {
		return err
	}
	if c.Open && strings.TrimSpace(session.PickerURI) != "" {
		return openURL(ctx, session.PickerURI)
	}
	return nil
}

type PhotosPickerGetCmd struct {
	SessionID string `arg:"" name:"sessionId" help:"Photos Picker session ID"`
}

func (c *PhotosPickerGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	sessionID, err := requirePhotosPickerSessionID(c.SessionID)
	if err != nil {
		return err
	}
	client, err := requirePhotosPickerClient(ctx, flags)
	if err != nil {
		return err
	}
	session, err := client.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	return writePhotosPickerSession(ctx, session)
}

type PhotosPickerWaitCmd struct {
	SessionID string        `arg:"" name:"sessionId" help:"Photos Picker session ID"`
	Timeout   time.Duration `name:"timeout" help:"Maximum local wait; 0 uses the API-provided timeout" default:"0s"`
}

func (c *PhotosPickerWaitCmd) Run(ctx context.Context, flags *RootFlags) error {
	sessionID, err := requirePhotosPickerSessionID(c.SessionID)
	if err != nil {
		return err
	}
	if c.Timeout < 0 {
		return usage("--timeout must be non-negative")
	}
	client, err := requirePhotosPickerClient(ctx, flags)
	if err != nil {
		return err
	}
	session, err := waitForPhotosPickerSession(
		ctx,
		client,
		sessionID,
		c.Timeout,
		defaultPhotosPickerWaitRuntime(),
	)
	if err != nil {
		return err
	}
	return writePhotosPickerSession(ctx, session)
}

type PhotosPickerListCmd struct {
	SessionID string `arg:"" name:"sessionId" help:"Photos Picker session ID"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results per page (max 100)" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" help:"Fetch all pages"`
}

func (c *PhotosPickerListCmd) Run(ctx context.Context, flags *RootFlags) error {
	sessionID, err := requirePhotosPickerSessionID(c.SessionID)
	if err != nil {
		return err
	}
	if validationErr := validatePhotosPickerPageSize(c.Max); validationErr != nil {
		return validationErr
	}
	client, err := requirePhotosPickerClient(ctx, flags)
	if err != nil {
		return err
	}
	items := make([]*googleapi.PhotosPickerMediaItem, 0)
	pageToken := strings.TrimSpace(c.Page)
	seenPageTokens := map[string]struct{}{}
	for {
		resp, listErr := client.ListMediaItems(ctx, sessionID, c.Max, pageToken)
		if listErr != nil {
			return listErr
		}
		items = append(items, resp.MediaItems...)
		nextPageToken := strings.TrimSpace(resp.NextPageToken)
		if !c.All || nextPageToken == "" {
			return writePhotosPickerMediaItems(ctx, sessionID, items, nextPageToken)
		}
		if _, exists := seenPageTokens[nextPageToken]; exists {
			return errPhotosPickerRepeatedPageToken
		}
		seenPageTokens[nextPageToken] = struct{}{}
		pageToken = nextPageToken
	}
}

type PhotosPickerDownloadCmd struct {
	SessionID   string `arg:"" name:"sessionId" help:"Photos Picker session ID"`
	MediaItemID string `arg:"" name:"mediaItemId" help:"Selected media item ID"`
	Out         string `name:"out" help:"Output path, directory, or '-' for stdout"`
	Overwrite   bool   `name:"overwrite" help:"Overwrite an existing output file"`
}

func (c *PhotosPickerDownloadCmd) Run(ctx context.Context, flags *RootFlags) error {
	sessionID, err := requirePhotosPickerSessionID(c.SessionID)
	if err != nil {
		return err
	}
	mediaItemID := strings.TrimSpace(c.MediaItemID)
	if mediaItemID == "" {
		return usage("empty mediaItemId")
	}
	outPathFlag := strings.TrimSpace(c.Out)
	if outPathFlag != "" {
		var expandErr error
		outPathFlag, expandErr = appconfig.ExpandPath(outPathFlag)
		if expandErr != nil {
			return expandErr
		}
	}
	defaultDir := ""
	if outPathFlag == "" {
		layout, layoutErr := commandLayout(ctx, appconfig.PathKindConfig)
		if layoutErr != nil {
			return layoutErr
		}
		defaultDir = layout.DriveDownloadsDir()
	}
	if dryRunErr := dryRunExit(ctx, flags, "photos.picker.download", map[string]any{
		"session_id":            sessionID,
		"media_item_id":         mediaItemID,
		"out":                   outPathFlag,
		"default_downloads_dir": defaultDir,
		"overwrite":             c.Overwrite,
	}); dryRunErr != nil {
		return dryRunErr
	}

	client, err := requirePhotosPickerClient(ctx, flags)
	if err != nil {
		return err
	}
	item, err := client.FindMediaItem(ctx, sessionID, mediaItemID)
	if err != nil {
		return err
	}
	resp, err := client.DownloadMedia(ctx, item)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if isStdoutPath(outPathFlag) {
		_, err = io.Copy(stdoutWriter(ctx), resp.Body)
		return err
	}
	filename := ""
	if item.MediaFile != nil {
		filename = item.MediaFile.Filename
	}
	dest, err := resolvePhotosDownloadDestPathParts(item.ID, filename, outPathFlag, defaultDir)
	if err != nil {
		return err
	}
	file, actual, err := openUserOutputFile(dest, outputFileOptions{
		Overwrite: c.Overwrite,
		FileMode:  0o600,
		DirMode:   0o700,
	})
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return writeResult(ctx, ui.FromContext(ctx),
		kv("sessionId", sessionID),
		kv("mediaItemId", item.ID),
		kv("path", actual),
		kv("bytes", written),
	)
}

type PhotosPickerDeleteCmd struct {
	SessionID string `arg:"" name:"sessionId" help:"Photos Picker session ID"`
}

func (c *PhotosPickerDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	sessionID, err := requirePhotosPickerSessionID(c.SessionID)
	if err != nil {
		return err
	}
	if dryRunErr := dryRunExit(ctx, flags, "photos.picker.sessions.delete", map[string]any{
		"session_id": sessionID,
	}); dryRunErr != nil {
		return dryRunErr
	}
	client, err := requirePhotosPickerClient(ctx, flags)
	if err != nil {
		return err
	}
	if err := client.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	return writeResult(ctx, ui.FromContext(ctx),
		kv("deleted", true),
		kv("sessionId", sessionID),
	)
}

type photosPickerWaitRuntime struct {
	now  func() time.Time
	wait func(context.Context, time.Duration) error
}

func defaultPhotosPickerWaitRuntime() photosPickerWaitRuntime {
	return photosPickerWaitRuntime{
		now: time.Now,
		wait: func(ctx context.Context, duration time.Duration) error {
			timer := time.NewTimer(duration)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func waitForPhotosPickerSession(
	ctx context.Context,
	client *googleapi.PhotosPickerClient,
	sessionID string,
	maxWait time.Duration,
	waitRuntime photosPickerWaitRuntime,
) (*googleapi.PhotosPickerSession, error) {
	session, err := client.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.MediaItemsSet {
		return session, nil
	}
	if waitRuntime.now == nil || waitRuntime.wait == nil {
		waitRuntime = defaultPhotosPickerWaitRuntime()
	}
	apiTimeout, err := photosPickerDuration(session.PollingConfig, true)
	if err != nil {
		return nil, err
	}
	timeout := apiTimeout
	if maxWait > 0 && timeout > 0 && maxWait < timeout {
		timeout = maxWait
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("%w: session %s", errPhotosPickerWaitTimeout, sessionID)
	}
	deadline := waitRuntime.now().Add(timeout)

	for {
		interval, intervalErr := photosPickerDuration(session.PollingConfig, false)
		if intervalErr != nil {
			return nil, intervalErr
		}
		remaining := deadline.Sub(waitRuntime.now())
		if remaining <= 0 {
			return nil, fmt.Errorf("%w: session %s", errPhotosPickerWaitTimeout, sessionID)
		}
		if interval > remaining {
			interval = remaining
		}
		if waitErr := waitRuntime.wait(ctx, interval); waitErr != nil {
			return nil, waitErr
		}
		session, err = client.GetSession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if session.MediaItemsSet {
			return session, nil
		}
	}
}

func photosPickerDuration(config *googleapi.PhotosPickerPollingConfig, timeout bool) (time.Duration, error) {
	if config == nil {
		return 0, errPhotosPickerPollingConfigMissing
	}
	raw := strings.TrimSpace(config.PollInterval)
	field := "pollInterval"
	if timeout {
		raw = strings.TrimSpace(config.TimeoutIn)
		field = "timeoutIn"
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid Photos Picker %s %q: %w", field, raw, err)
	}
	if duration < 0 || (!timeout && duration == 0) {
		return 0, fmt.Errorf("invalid Photos Picker %s %q", field, raw)
	}
	return duration, nil
}

func requirePhotosPickerClient(ctx context.Context, flags *RootFlags) (*googleapi.PhotosPickerClient, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return nil, err
	}
	return photosPickerService(ctx, account)
}

func requirePhotosPickerSessionID(raw string) (string, error) {
	sessionID := strings.TrimSpace(raw)
	if sessionID == "" {
		return "", usage("empty sessionId")
	}
	return sessionID, nil
}

func validatePhotosPickerMaxItems(maxItems int64) error {
	if maxItems < 0 {
		return usage("--max-items must be non-negative")
	}
	if maxItems > maxPhotosPickerItemCount {
		return usage("--max-items must be <= 2000")
	}
	return nil
}

func validatePhotosPickerPageSize(maxResults int64) error {
	if maxResults <= 0 {
		return usage("max must be > 0")
	}
	if maxResults > maxPhotosPickerPageSize {
		return usage("max must be <= 100")
	}
	return nil
}

func writePhotosPickerSession(ctx context.Context, session *googleapi.PhotosPickerSession) error {
	if session == nil {
		session = &googleapi.PhotosPickerSession{}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"session": session})
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("id\t%s", session.ID)
	if session.PickerURI != "" {
		u.Out().Linef("picker_uri\t%s", session.PickerURI)
	}
	if session.ExpireTime != "" {
		u.Out().Linef("expire_time\t%s", session.ExpireTime)
	}
	u.Out().Linef("media_items_set\t%t", session.MediaItemsSet)
	if session.PollingConfig != nil {
		u.Out().Linef("poll_interval\t%s", session.PollingConfig.PollInterval)
		u.Out().Linef("timeout_in\t%s", session.PollingConfig.TimeoutIn)
	}
	return nil
}

func writePhotosPickerMediaItems(
	ctx context.Context,
	sessionID string,
	items []*googleapi.PhotosPickerMediaItem,
	nextPageToken string,
) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"sessionId":      sessionID,
			"mediaItems":     items,
			"mediaItemCount": len(items),
			"nextPageToken":  nextPageToken,
		})
	}
	u := ui.FromContext(ctx)
	if len(items) == 0 {
		u.Err().Println("No picked media items")
		return nil
	}
	writer, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(writer, "ID\tTYPE\tFILENAME\tMIME\tCREATED\tWIDTH\tHEIGHT")
	for _, item := range items {
		if item == nil {
			continue
		}
		filename, mimeType := "", ""
		var width, height int64
		if item.MediaFile != nil {
			filename = item.MediaFile.Filename
			mimeType = item.MediaFile.MimeType
			if item.MediaFile.MediaFileMetadata != nil {
				width = item.MediaFile.MediaFileMetadata.Width
				height = item.MediaFile.MediaFileMetadata.Height
			}
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
			item.ID,
			item.Type,
			sanitizeTab(filename),
			mimeType,
			item.CreateTime,
			width,
			height,
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}
