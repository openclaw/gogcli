package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
)

// AssetMap pairs parsed AST references with uploaded Drive ImageRefs.
// Icons is keyed by IconRef value (Style+Name); Diagrams is keyed by
// DiagramBlock.ID.
type AssetMap struct {
	Icons    map[IconRef]ImageRef
	Diagrams map[string]ImageRef
}

// NewAssetMap returns an empty initialized AssetMap.
func NewAssetMap() AssetMap {
	return AssetMap{
		Icons:    map[IconRef]ImageRef{},
		Diagrams: map[string]ImageRef{},
	}
}

// AssetPipelineConfig holds the runtime knobs for the pipeline.
type AssetPipelineConfig struct {
	HTTPClient     *http.Client
	MMDCPath       string
	Strict         bool
	KeepTempImages bool
	DefaultFAStyle string
}

// DefaultAssetPipelineConfig returns a config with sane defaults: 30s
// HTTP timeout, mmdc on PATH, non-strict, no image retention.
func DefaultAssetPipelineConfig() AssetPipelineConfig {
	return AssetPipelineConfig{
		HTTPClient:     &http.Client{Timeout: 30 * time.Second},
		MMDCPath:       "mmdc",
		Strict:         false,
		KeepTempImages: false,
		DefaultFAStyle: "solid",
	}
}

func faSVGURL(style, name string) string {
	return fmt.Sprintf(
		"https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/%s/%s.svg",
		style, name,
	)
}

func mmdcCommandArgs(mmdcPath, in, out string) []string {
	return []string{mmdcPath, "-i", in, "-o", out, "-b", "transparent", "--scale", "2"}
}

// renderMermaidWithBinary writes source to a temp .mmd, runs mmdc, and
// returns the rendered PNG bytes. The temp files are cleaned up.
func renderMermaidWithBinary(ctx context.Context, mmdcPath, source string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "gogcli-mermaid-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in.mmd")
	out := filepath.Join(dir, "out.png")
	if err := os.WriteFile(in, []byte(source), 0o600); err != nil {
		return nil, err
	}
	args := mmdcCommandArgs(mmdcPath, in, out)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) // #nosec G204 — args constructed from validated config
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Surface stderr so the user can see WHY mmdc failed (puppeteer
		// chromium download, mermaid syntax error, etc.) — bare exit codes
		// are useless on their own.
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return nil, fmt.Errorf("mmdc failed: %w: %s", err, trimmed)
		}
		return nil, fmt.Errorf("mmdc failed: %w", err)
	}
	return os.ReadFile(out)
}

func fetchFAIconFromURL(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Uploader abstracts the Drive operations the pipeline needs. Real impl
// (Task 14) wraps drive.Service; tests use fakeDriveUploader.
type Uploader interface {
	UploadAsset(ctx context.Context, name, mime string, body []byte) (ImageRef, error)
	DeleteAsset(ctx context.Context, fileID string) error
}

// AssetPipeline resolves all FA icon and mermaid diagram references in a
// slice of Slides into ImageRefs by fetching/rendering them and uploading
// to Drive via the Uploader.
type AssetPipeline struct {
	Config   AssetPipelineConfig
	Uploader Uploader

	// uploaded tracks Drive file IDs created by this pipeline so Cleanup
	// can delete them when --keep-temp-images is false.
	uploaded []string
}

// Resolve walks all slides, collects unique IconRefs and DiagramBlocks,
// fetches/renders/uploads each, and returns the resulting AssetMap.
//
// Per-asset failures are logged (warn-and-skip) unless Config.Strict.
func (p *AssetPipeline) Resolve(ctx context.Context, slides []Slide) (AssetMap, error) {
	am := NewAssetMap()

	icons := collectIconRefs(slides)
	diagrams := collectDiagrams(slides)

	for ref := range icons {
		body, resolvedStyle, err := fetchFAIconWithStyleFallback(ctx, p.Config.HTTPClient, ref)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(os.Stderr, "warning: skipping FA icon :%s-%s: %v\n", ref.Style, ref.Name, err)
			continue
		}
		ir, err := p.Uploader.UploadAsset(ctx, fmt.Sprintf("fa-%s-%s.svg", resolvedStyle, ref.Name), "image/svg+xml", body)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(os.Stderr, "warning: skipping FA icon :%s-%s: upload: %v\n", ref.Style, ref.Name, err)
			continue
		}
		am.Icons[ref] = ir
		p.uploaded = append(p.uploaded, ir.DriveFileID)
	}

	for blockID, source := range diagrams {
		if p.Config.MMDCPath == "" {
			fmt.Fprintf(os.Stderr, "warning: mmdc not configured; skipping mermaid diagram %s\n", blockID)
			continue
		}
		png, err := renderMermaidWithBinary(ctx, p.Config.MMDCPath, source)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(os.Stderr, "warning: skipping mermaid diagram %s: %v\n", blockID, err)
			continue
		}
		ir, err := p.Uploader.UploadAsset(ctx, blockID+".png", "image/png", png)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(os.Stderr, "warning: skipping mermaid diagram %s: upload: %v\n", blockID, err)
			continue
		}
		am.Diagrams[blockID] = ir
		p.uploaded = append(p.uploaded, ir.DriveFileID)
	}

	return am, nil
}

// Cleanup deletes every Drive file the pipeline uploaded, unless
// Config.KeepTempImages is true.
func (p *AssetPipeline) Cleanup(ctx context.Context) error {
	if p.Config.KeepTempImages {
		return nil
	}
	var firstErr error
	for _, id := range p.uploaded {
		if err := p.Uploader.DeleteAsset(ctx, id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// collectIconRefs walks all slides, deduping IconRef values.
func collectIconRefs(slides []Slide) map[IconRef]struct{} {
	out := map[IconRef]struct{}{}
	var walkBlocks func([]Block)
	walkBlocks = func(blocks []Block) {
		for _, b := range blocks {
			switch v := b.(type) {
			case ParagraphBlock:
				for _, in := range v.Inlines {
					if r, ok := in.(IconRef); ok {
						out[r] = struct{}{}
					}
				}
			case BulletsBlock:
				for _, item := range v.Items {
					for _, in := range item.Inlines {
						if r, ok := in.(IconRef); ok {
							out[r] = struct{}{}
						}
					}
				}
			case HeadingBlock:
				for _, in := range v.Inlines {
					if r, ok := in.(IconRef); ok {
						out[r] = struct{}{}
					}
				}
			case ColumnsBlock:
				for _, col := range v.Columns {
					walkBlocks(col)
				}
			case IconRowsBlock:
				for _, row := range v.Rows {
					if row.Icon != nil {
						out[*row.Icon] = struct{}{}
					}
				}
			}
		}
	}
	for _, s := range slides {
		walkBlocks(s.Body)
	}
	return out
}

// collectDiagrams walks all slides for DiagramBlocks, returning {ID: source}.
func collectDiagrams(slides []Slide) map[string]string {
	out := map[string]string{}
	var walkBlocks func([]Block)
	walkBlocks = func(blocks []Block) {
		for _, b := range blocks {
			switch v := b.(type) {
			case DiagramBlock:
				out[v.ID] = v.Source
			case ColumnsBlock:
				for _, col := range v.Columns {
					walkBlocks(col)
				}
			}
		}
	}
	for _, s := range slides {
		walkBlocks(s.Body)
	}
	return out
}

// fetchFAIconWithStyleFallback fetches the SVG for ref. If the requested
// style returns 404 (common for users who write `:fa-dev:` when "dev" is
// only published under brands/), it tries the other free-tier styles in a
// fixed order: brands, regular, solid. Returns the body, the style that
// actually served, and the final error.
func fetchFAIconWithStyleFallback(ctx context.Context, client *http.Client, ref IconRef) ([]byte, string, error) {
	tried := map[string]bool{}
	order := []string{ref.Style, "brands", "regular", "solid"}
	var lastErr error
	for _, style := range order {
		if style == "" || tried[style] {
			continue
		}
		tried[style] = true
		body, err := fetchFAIconFromURL(ctx, client, faSVGURL(style, ref.Name))
		if err == nil {
			return body, style, nil
		}
		lastErr = err
		// Only fall through on 404; other errors (network, 5xx) shouldn't
		// trigger style guessing.
		if !strings.Contains(err.Error(), "HTTP 404") {
			return nil, ref.Style, err
		}
	}
	return nil, ref.Style, lastErr
}

// DriveUploader implements Uploader by writing temporary files to Drive,
// granting public read access, and reading the WebContentLink. Mirrors
// the pattern in slides_add_slide.go.
type DriveUploader struct {
	Svc *drive.Service
}

func (d *DriveUploader) UploadAsset(ctx context.Context, name, mime string, body []byte) (ImageRef, error) {
	created, err := d.Svc.Files.Create(&drive.File{
		Name:     name,
		MimeType: mime,
	}).Media(bytes.NewReader(body)).Fields("id, webContentLink").Context(ctx).Do()
	if err != nil {
		return ImageRef{}, fmt.Errorf("upload %s: %w", name, err)
	}
	if _, err := d.Svc.Permissions.Create(created.Id, &drive.Permission{
		Type: "anyone",
		Role: "reader",
	}).Context(ctx).Do(); err != nil {
		// Best-effort cleanup so a permission failure doesn't orphan the upload.
		_ = d.Svc.Files.Delete(created.Id).Context(ctx).Do()
		return ImageRef{}, fmt.Errorf("permission %s: %w", created.Id, err)
	}
	url := created.WebContentLink
	if url == "" {
		got, err := d.Svc.Files.Get(created.Id).Fields("webContentLink").Context(ctx).Do()
		if err != nil {
			_ = d.Svc.Files.Delete(created.Id).Context(ctx).Do()
			return ImageRef{}, fmt.Errorf("get url for %s: %w", created.Id, err)
		}
		url = got.WebContentLink
	}
	return ImageRef{DriveFileID: created.Id, PublicURL: url}, nil
}

func (d *DriveUploader) DeleteAsset(ctx context.Context, fileID string) error {
	return d.Svc.Files.Delete(fileID).Context(ctx).Do()
}
