package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/slides/v1"
)

func TestSlidesInfoCmd_IncludesNativeMetadata(t *testing.T) {
	driveSvc, closeDrive := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/files/pres1") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "pres1",
				"name":     "Deck",
				"mimeType": "application/vnd.google-apps.presentation",
			})
			return
		}
		http.NotFound(w, r)
	}), drive.NewService)
	t.Cleanup(closeDrive)

	slidesSvc, closeSlides := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/presentations/pres1") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"presentationId": "pres1",
				"title":          "Deck",
				"locale":         "en-US",
				"revisionId":     "rev1",
				"pageSize": map[string]any{
					"width":  map[string]any{"magnitude": 12192000, "unit": "EMU"},
					"height": map[string]any{"magnitude": 6858000, "unit": "EMU"},
				},
				"slides": []any{map[string]any{"objectId": "slide1"}, map[string]any{"objectId": "slide2"}},
				"masters": []any{map[string]any{
					"objectId":         "master1",
					"masterProperties": map[string]any{"displayName": "Simple Light"},
					"pageProperties": map[string]any{"colorScheme": map[string]any{"colors": []any{
						map[string]any{"type": "ACCENT1", "color": map[string]any{"red": 1.0, "green": 0.5, "blue": 0.0}},
					}}},
				}},
				"layouts": []any{map[string]any{
					"objectId": "layout1",
					"layoutProperties": map[string]any{
						"name": "TITLE_AND_BODY", "displayName": "Title and body", "masterObjectId": "master1",
					},
				}},
			})
			return
		}
		http.NotFound(w, r)
	}), slides.NewService)
	t.Cleanup(closeSlides)

	var out bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &out, io.Discard)
	ctx = withDriveTestService(ctx, driveSvc)
	ctx = withSlidesTestService(ctx, slidesSvc)
	require.NoError(t, (&SlidesInfoCmd{PresentationID: "pres1"}).Run(ctx, &RootFlags{Account: "a@b.com"}))

	var result struct {
		File struct {
			ID string `json:"id"`
		} `json:"file"`
		Presentation slidesPresentationInfo `json:"presentation"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &result))
	assert.Equal(t, "pres1", result.File.ID)
	assert.Equal(t, 2, result.Presentation.SlideCount)
	require.NotNil(t, result.Presentation.PageSize)
	assert.InDelta(t, 960, result.Presentation.PageSize.Width, 0.001)
	assert.InDelta(t, 540, result.Presentation.PageSize.Height, 0.001)
	require.Len(t, result.Presentation.Masters, 1)
	require.Len(t, result.Presentation.Masters[0].ThemeColors, 1)
	assert.Equal(t, "#FF8000", result.Presentation.Masters[0].ThemeColors[0].RGB)
	require.Len(t, result.Presentation.Layouts, 1)
	assert.Equal(t, "TITLE_AND_BODY", result.Presentation.Layouts[0].Name)
}
