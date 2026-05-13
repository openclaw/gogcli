package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchFAIcon_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<svg/>")
	}))
	t.Cleanup(srv.Close)

	body, err := fetchFAIconFromURL(context.Background(), srv.Client(), srv.URL+"/x.svg")
	require.NoError(t, err)
	assert.Equal(t, "<svg/>", string(body))
}

func TestFetchFAIcon_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	_, err := fetchFAIconFromURL(context.Background(), srv.Client(), srv.URL+"/x.svg")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "404"))
}

func TestFASVGURL(t *testing.T) {
	cases := []struct {
		style, name, expected string
	}{
		{"solid", "truck-fast", "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/solid/truck-fast.svg"},
		{"brands", "github", "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/brands/github.svg"},
		{"regular", "clock", "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/regular/clock.svg"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, faSVGURL(tc.style, tc.name))
	}
}

func TestMMDCCommandArgs(t *testing.T) {
	args := mmdcCommandArgs("/usr/bin/mmdc", "/tmp/in.mmd", "/tmp/out.png")
	assert.Equal(t, []string{"/usr/bin/mmdc", "-i", "/tmp/in.mmd", "-o", "/tmp/out.png", "-b", "transparent", "--scale", "2"}, args)
}

func TestRenderMermaid_BinaryMissing(t *testing.T) {
	_, err := renderMermaidWithBinary(context.Background(), "/nonexistent/mmdc-binary", "graph TD\nA-->B")
	require.Error(t, err)
}

type fakeDriveUploader struct {
	uploaded []string // file IDs in upload order
	deleted  []string
}

func (f *fakeDriveUploader) UploadAsset(ctx context.Context, name, mime string, body []byte) (ImageRef, error) {
	id := fmt.Sprintf("file-%d", len(f.uploaded)+1)
	f.uploaded = append(f.uploaded, id)
	return ImageRef{DriveFileID: id, PublicURL: "https://drive.example/" + id}, nil
}
func (f *fakeDriveUploader) DeleteAsset(ctx context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func TestAssetPipeline_CollectsUniqueIcons(t *testing.T) {
	cfg := DefaultAssetPipelineConfig()
	cfg.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("<svg/>")), Header: http.Header{}}, nil
	})}
	cfg.MMDCPath = "" // disable mmdc; no diagrams in test

	uploader := &fakeDriveUploader{}
	p := &AssetPipeline{Config: cfg, Uploader: uploader}

	slides := []Slide{
		{Body: []Block{ParagraphBlock{Inlines: []Inline{
			IconRef{Style: "solid", Name: "truck-fast"},
			TextRun{Text: " hello "},
			IconRef{Style: "solid", Name: "truck-fast"}, // duplicate, should not re-upload
		}}}},
		{Body: []Block{IconRowsBlock{Kind: "boxes", Rows: []IconRow{
			{Icon: &IconRef{Style: "brands", Name: "github"}, Text: "GitHub"},
		}}}},
	}

	am, err := p.Resolve(context.Background(), slides)
	require.NoError(t, err)
	assert.Equal(t, 2, len(am.Icons), "two unique icons, no duplicates")
	assert.Equal(t, 2, len(uploader.uploaded), "exactly two Drive uploads")
}

func TestAssetPipeline_Cleanup(t *testing.T) {
	uploader := &fakeDriveUploader{}
	p := &AssetPipeline{Config: DefaultAssetPipelineConfig(), Uploader: uploader}
	uploader.uploaded = []string{"file-1", "file-2"}
	p.uploaded = []string{"file-1", "file-2"}

	require.NoError(t, p.Cleanup(context.Background()))
	assert.Equal(t, []string{"file-1", "file-2"}, uploader.deleted)
}

func TestDriveUploaderSatisfiesUploader(t *testing.T) {
	var _ Uploader = (*DriveUploader)(nil)
}

