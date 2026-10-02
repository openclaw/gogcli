package cmd

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

const photosUploadBatchSize = 50

var errPhotosUploadFailed = errors.New("one or more Photos uploads failed")

// Google Photos accepts only photos and videos. Refuse anything else locally so
// an arbitrary file's bytes are never sent to the upload endpoint.
var photosUploadExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".jpe": true, ".jfif": true, ".png": true, ".gif": true,
	".webp": true, ".heic": true, ".heif": true, ".avif": true, ".bmp": true, ".tif": true,
	".tiff": true, ".ico": true, ".dng": true, ".cr2": true, ".nef": true, ".arw": true,
	".orf": true, ".rw2": true, ".raf": true,
	".mp4": true, ".m4v": true, ".mov": true, ".3gp": true, ".3g2": true, ".avi": true,
	".mkv": true, ".mpg": true, ".mpeg": true, ".mts": true, ".m2ts": true, ".wmv": true,
	".asf": true, ".mod": true, ".tod": true,
}

type PhotosUploadCmd struct {
	Files       []string `arg:"" name:"file" help:"Photo or video files to upload" type:"existingfile"`
	AlbumID     string   `name:"album" aliases:"album-id" help:"App-created album ID to add the new media items to"`
	Description string   `name:"description" help:"Description applied to every uploaded item (max 1000 chars); Google Photos shows it under Info > Other (searchable), not as the user-editable caption"`
}

type photosUploadResult struct {
	File        string `json:"file"`
	MediaItemID string `json:"mediaItemId,omitempty"`
	ProductURL  string `json:"productUrl,omitempty"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

func (c *PhotosUploadCmd) Run(ctx context.Context, flags *RootFlags) error {
	if len(c.Files) == 0 {
		return usage("at least one file is required")
	}
	description := strings.TrimSpace(c.Description)
	if len([]rune(description)) > 1000 {
		return usage("description must be at most 1000 characters")
	}
	paths := make([]string, 0, len(c.Files))
	for _, f := range c.Files {
		p, err := config.ExpandPath(strings.TrimSpace(f))
		if err != nil {
			return err
		}
		if !photosUploadExtensions[strings.ToLower(filepath.Ext(p))] {
			return usagef("%s: not a photo or video file type Google Photos accepts", p)
		}
		paths = append(paths, p)
	}
	if dryRunErr := dryRunExit(ctx, flags, "photos.upload", map[string]any{
		"files":       paths,
		"album_id":    strings.TrimSpace(c.AlbumID),
		"description": description,
	}); dryRunErr != nil {
		return dryRunErr
	}

	client, err := requirePhotosClient(ctx, flags)
	if err != nil {
		return err
	}

	results := make([]photosUploadResult, 0, len(paths))
	for start := 0; start < len(paths); start += photosUploadBatchSize {
		end := min(start+photosUploadBatchSize, len(paths))
		batch, err := uploadPhotosBatch(ctx, client, paths[start:end], strings.TrimSpace(c.AlbumID), description)
		results = append(results, batch...)
		if err != nil {
			return writePhotosUploadResults(ctx, results, err)
		}
	}

	var failed error
	for _, r := range results {
		if r.Status != "ok" {
			failed = errPhotosUploadFailed
			break
		}
	}
	return writePhotosUploadResults(ctx, results, failed)
}

func uploadPhotosBatch(
	ctx context.Context,
	client *googleapi.PhotosClient,
	paths []string,
	albumID string,
	description string,
) ([]photosUploadResult, error) {
	results := make([]photosUploadResult, len(paths))
	items := make([]googleapi.PhotosNewMediaItem, 0, len(paths))
	index := make([]int, 0, len(paths))
	for i, p := range paths {
		results[i] = photosUploadResult{File: p, Status: "error"}
		token, err := uploadPhotosFile(ctx, client, p)
		if err != nil {
			results[i].Error = photosScopeHint(err.Error())
			continue
		}
		items = append(items, googleapi.PhotosNewMediaItem{
			UploadToken: token,
			FileName:    filepath.Base(p),
			Description: description,
		})
		index = append(index, i)
	}
	if len(items) == 0 {
		return results, nil
	}
	resp, err := client.BatchCreateMediaItems(ctx, albumID, items)
	if err != nil {
		for _, i := range index {
			results[i].Error = err.Error()
		}
		return results, err
	}
	for n, i := range index {
		if n >= len(resp.NewMediaItemResults) || resp.NewMediaItemResults[n] == nil {
			results[i].Error = "no result returned for this item"
			continue
		}
		r := resp.NewMediaItemResults[n]
		if r.MediaItem != nil && r.MediaItem.ID != "" && (r.Status == nil || r.Status.Code == 0) {
			results[i].Status = "ok"
			results[i].MediaItemID = r.MediaItem.ID
			results[i].ProductURL = r.MediaItem.ProductURL
			continue
		}
		if r.Status != nil {
			results[i].Error = strings.TrimSpace(r.Status.Message)
		}
		if results[i].Error == "" {
			results[i].Error = "media item not created"
		}
	}
	return results, nil
}

func uploadPhotosFile(ctx context.Context, client *googleapi.PhotosClient, path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // user-selected upload path
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	return client.UploadBytes(ctx, f, info.Size(), photosMimeType(path), filepath.Base(path))
}

// photosScopeHint points at the opt-in scope when Google refuses for lack of it.
func photosScopeHint(msg string) string {
	if strings.Contains(msg, "403") || strings.Contains(strings.ToLower(msg), "insufficient") {
		return msg + " (uploads need: gog auth add <email> --services photos --photos-scope=append)"
	}
	return msg
}

// photosMimeType maps a file extension to the upload content type. Unknown
// types are sent without one and Google Photos sniffs the bytes.
func photosMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".jfif", ".jpe":
		return "image/jpeg"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	case ".webp":
		return "image/webp"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		if semi := strings.IndexByte(t, ';'); semi >= 0 {
			t = t[:semi]
		}
		return t
	}
	return ""
}

func writePhotosUploadResults(ctx context.Context, results []photosUploadResult, runErr error) error {
	u := ui.FromContext(ctx)
	ok := 0
	for _, r := range results {
		if r.Status == "ok" {
			ok++
		}
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"results":  results,
			"uploaded": ok,
			"failed":   len(results) - ok,
		}); err != nil {
			return err
		}
		return runErr
	}
	w, flush := tableWriter(ctx)
	fmt.Fprintln(w, "STATUS\tFILE\tMEDIA_ITEM_ID\tERROR")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Status, sanitizeTab(r.File), r.MediaItemID, sanitizeTab(r.Error))
	}
	flush()
	u.Err().Printf("uploaded %d of %d\n", ok, len(results))
	return runErr
}

type PhotosAlbumsCmd struct {
	List   PhotosAlbumsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List app-created albums"`
	Create PhotosAlbumsCreateCmd `cmd:"" name:"create" help:"Create an app-created album (uploads can only target these)"`
}

type PhotosAlbumsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results (max 50)" default:"25"`
	Page string `name:"page" aliases:"cursor" help:"Page token"`
}

func (c *PhotosAlbumsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.Max <= 0 || c.Max > 50 {
		return usage("max must be between 1 and 50")
	}
	client, err := requirePhotosClient(ctx, flags)
	if err != nil {
		return err
	}
	resp, err := client.ListAlbums(ctx, c.Max, c.Page)
	if err != nil {
		return err
	}
	u := ui.FromContext(ctx)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"albums":        resp.Albums,
			"albumCount":    len(resp.Albums),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Albums) == 0 {
		u.Err().Println("No app-created albums")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tITEMS\tPRODUCT_URL")
	for _, a := range resp.Albums {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.ID, sanitizeTab(a.Title), a.MediaItemsCount, a.ProductURL)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type PhotosAlbumsCreateCmd struct {
	Title string `arg:"" name:"title" help:"Album title"`
}

func (c *PhotosAlbumsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty album title")
	}
	if dryRunErr := dryRunExit(ctx, flags, "photos.albums.create", map[string]any{"title": title}); dryRunErr != nil {
		return dryRunErr
	}
	client, err := requirePhotosClient(ctx, flags)
	if err != nil {
		return err
	}
	album, err := client.CreateAlbum(ctx, title)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"album": album})
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("id\t%s", album.ID)
	u.Out().Linef("title\t%s", album.Title)
	if album.ProductURL != "" {
		u.Out().Linef("product_url\t%s", album.ProductURL)
	}
	return nil
}
