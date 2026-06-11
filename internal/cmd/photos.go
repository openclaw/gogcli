package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newPhotosClient = func(ctx context.Context, email string) (*googleapi.PhotosClient, error) {
	return googleapi.NewPhotosClientForAccount(ctx, email, googleapi.WithPhotosBaseURL(os.Getenv("GOG_PHOTOS_BASE_URL")))
}

type PhotosCmd struct {
	List     PhotosListCmd     `cmd:"" name:"list" aliases:"ls" help:"List app-created media items"`
	Search   PhotosSearchCmd   `cmd:"" name:"search" aliases:"find" help:"Search app-created media items"`
	Get      PhotosGetCmd      `cmd:"" name:"get" aliases:"info,show" help:"Get an app-created media item"`
	Download PhotosDownloadCmd `cmd:"" name:"download" aliases:"dl" help:"Download an app-created media item"`
	Picker   PhotosPickerCmd   `cmd:"" name:"picker" help:"Access user-selected media with the Photos Picker API"`
}

type PhotosListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results (max 100)" default:"25"`
	Page string `name:"page" aliases:"cursor" help:"Page token"`
}

func (c *PhotosListCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := validatePhotosMax(c.Max); err != nil {
		return err
	}
	client, err := requirePhotosClient(ctx, flags)
	if err != nil {
		return err
	}
	resp, err := client.ListMediaItems(ctx, googleapi.PhotosListOptions{
		PageSize:  normalizePhotosPageSize(c.Max),
		PageToken: c.Page,
	})
	if err != nil {
		return err
	}
	return writePhotosMediaItems(ctx, resp, "No media items")
}

type PhotosSearchCmd struct {
	AlbumID         string `name:"album" aliases:"album-id" help:"App-created album ID"`
	MediaType       string `name:"media-type" help:"Media type: PHOTO|VIDEO|ALL_MEDIA" enum:"PHOTO,VIDEO,ALL_MEDIA" default:"ALL_MEDIA"`
	From            string `name:"from" help:"Start date YYYY-MM-DD"`
	To              string `name:"to" help:"End date YYYY-MM-DD"`
	IncludeArchived bool   `name:"include-archived" help:"Include archived media"`
	Order           string `name:"order" help:"Creation time order: desc|asc" enum:"desc,asc" default:"desc"`
	Max             int64  `name:"max" aliases:"limit" help:"Max results (max 100)" default:"25"`
	Page            string `name:"page" aliases:"cursor" help:"Page token"`
}

func (c *PhotosSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := validatePhotosMax(c.Max); err != nil {
		return err
	}
	start, err := parsePhotosDateFlag(c.From, "--from")
	if err != nil {
		return err
	}
	end, err := parsePhotosDateFlag(c.To, "--to")
	if err != nil {
		return err
	}
	client, err := requirePhotosClient(ctx, flags)
	if err != nil {
		return err
	}
	orderBy := "MediaMetadata.creation_time desc"
	if c.Order == "asc" {
		orderBy = "MediaMetadata.creation_time"
	}
	resp, err := client.SearchMediaItems(ctx, googleapi.PhotosSearchOptions{
		PageSize:             normalizePhotosPageSize(c.Max),
		PageToken:            c.Page,
		AlbumID:              c.AlbumID,
		MediaType:            c.MediaType,
		StartDate:            start,
		EndDate:              end,
		IncludeArchivedMedia: c.IncludeArchived,
		OrderBy:              orderBy,
	})
	if err != nil {
		return err
	}
	return writePhotosMediaItems(ctx, resp, "No media items")
}

type PhotosGetCmd struct {
	MediaItemID string `arg:"" name:"mediaItemId" help:"Media item ID"`
}

func (c *PhotosGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	mediaItemID := strings.TrimSpace(c.MediaItemID)
	if mediaItemID == "" {
		return usage("empty mediaItemId")
	}
	client, err := requirePhotosClient(ctx, flags)
	if err != nil {
		return err
	}
	item, err := client.GetMediaItem(ctx, mediaItemID)
	if err != nil {
		return err
	}
	return writePhotosMediaItem(ctx, item)
}

type PhotosDownloadCmd struct {
	MediaItemID string `arg:"" name:"mediaItemId" help:"Media item ID"`
	Out         string `name:"out" help:"Output path, directory, or '-' for stdout"`
	Video       bool   `name:"video" help:"Download video bytes with =dv (default auto when metadata says video)"`
	Overwrite   bool   `name:"overwrite" help:"Overwrite an existing output file"`
}

func (c *PhotosDownloadCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	mediaItemID := strings.TrimSpace(c.MediaItemID)
	if mediaItemID == "" {
		return usage("empty mediaItemId")
	}
	client, err := requirePhotosClient(ctx, flags)
	if err != nil {
		return err
	}
	item, err := client.GetMediaItem(ctx, mediaItemID)
	if err != nil {
		return err
	}
	baseURL := strings.TrimSpace(item.BaseURL)
	if baseURL == "" {
		return fmt.Errorf("media item %s has no baseUrl", item.ID)
	}
	suffix := "=d"
	if c.Video || (item.MediaMetadata != nil && item.MediaMetadata.Video != nil) {
		suffix = "=dv"
	}
	downloadURL := baseURL + suffix

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("build media download request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download media item: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return fmt.Errorf("download media item: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	if isStdoutPath(c.Out) {
		_, err = io.Copy(os.Stdout, resp.Body)
		return err
	}
	dest, err := resolvePhotosDownloadDestPath(item, c.Out)
	if err != nil {
		return err
	}
	f, actual, err := openUserOutputFile(dest, outputFileOptions{
		Overwrite: c.Overwrite,
		FileMode:  0o600,
		DirMode:   0o700,
	})
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return writeResult(ctx, u,
		kv("mediaItemId", item.ID),
		kv("path", actual),
		kv("bytes", n),
	)
}

func requirePhotosClient(ctx context.Context, flags *RootFlags) (*googleapi.PhotosClient, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return nil, err
	}
	client, err := newPhotosClient(ctx, account)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func normalizePhotosPageSize(n int64) int64 {
	if n <= 0 {
		return 25
	}
	if n > 100 {
		return 100
	}
	return n
}

func validatePhotosMax(n int64) error {
	if n <= 0 {
		return usage("max must be > 0")
	}
	if n > 100 {
		return usage("max must be <= 100")
	}
	return nil
}

func parsePhotosDateFlag(raw string, flag string) (*googleapi.PhotosDate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil //nolint:nilnil // optional flag
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, usage(fmt.Sprintf("%s must be YYYY-MM-DD", flag))
	}
	return &googleapi.PhotosDate{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}, nil
}

func writePhotosMediaItems(ctx context.Context, resp *googleapi.PhotosMediaItemsResponse, emptyMessage string) error {
	u := ui.FromContext(ctx)
	if resp == nil {
		resp = &googleapi.PhotosMediaItemsResponse{}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"mediaItems":     resp.MediaItems,
			"mediaItemCount": len(resp.MediaItems),
			"nextPageToken":  resp.NextPageToken,
		})
	}
	if len(resp.MediaItems) == 0 {
		u.Err().Println(emptyMessage)
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tFILENAME\tMIME\tCREATED\tPRODUCT_URL")
	for _, item := range resp.MediaItems {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			item.ID,
			sanitizeTab(item.Filename),
			item.MimeType,
			photosCreationTime(item),
			item.ProductURL,
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

func writePhotosMediaItem(ctx context.Context, item *googleapi.PhotosMediaItem) error {
	u := ui.FromContext(ctx)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"mediaItem": item})
	}
	if item == nil {
		u.Err().Println("No media item")
		return nil
	}
	u.Out().Linef("id\t%s", item.ID)
	u.Out().Linef("filename\t%s", item.Filename)
	u.Out().Linef("mime\t%s", item.MimeType)
	if created := photosCreationTime(item); created != "" {
		u.Out().Linef("created\t%s", created)
	}
	if item.ProductURL != "" {
		u.Out().Linef("product_url\t%s", item.ProductURL)
	}
	return nil
}

func photosCreationTime(item *googleapi.PhotosMediaItem) string {
	if item == nil || item.MediaMetadata == nil {
		return ""
	}
	return item.MediaMetadata.CreationTime
}

func resolvePhotosDownloadDestPath(item *googleapi.PhotosMediaItem, outPathFlag string) (string, error) {
	if item == nil {
		return "", fmt.Errorf("missing media item metadata")
	}
	return resolvePhotosDownloadDestPathParts(item.ID, item.Filename, outPathFlag)
}

func resolvePhotosDownloadDestPathParts(itemID string, itemFilename string, outPathFlag string) (string, error) {
	filename := strings.TrimSpace(itemFilename)
	if filename == "" {
		filename = strings.TrimSpace(itemID)
	}
	if filename == "" {
		filename = "media-item"
	}
	safeName := filepath.Base(filename)
	if safeName == "." || safeName == ".." || safeName == "" {
		safeName = "media-item"
	}
	destPath := strings.TrimSpace(outPathFlag)
	if destPath != "" {
		expanded, err := config.ExpandPath(destPath)
		if err != nil {
			return "", err
		}
		destPath = expanded
	}
	if destPath == "" {
		dir, err := config.EnsureDriveDownloadsDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, safeName), nil
	}
	if st, err := os.Stat(destPath); err == nil && st.IsDir() {
		return filepath.Join(destPath, safeName), nil
	}
	return destPath, nil
}
