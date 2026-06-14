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

	"github.com/steipete/gogcli/internal/slidesmarkdown"
)

// AssetMap pairs parsed AST references with uploaded Drive ImageRefs.
// Icons is keyed by slidesmarkdown.IconRef value (Style+Name); Diagrams is
// keyed by slidesmarkdown.DiagramBlock.ID.
type AssetMap struct {
	Icons    map[slidesmarkdown.IconRef]ImageRef
	Diagrams map[string]ImageRef
}

// ImageRef is the result of uploading an asset to Drive.
type ImageRef struct {
	DriveFileID string
	PublicURL   string
}

// NewAssetMap returns an empty initialized AssetMap.
func NewAssetMap() AssetMap {
	return AssetMap{
		Icons:    map[slidesmarkdown.IconRef]ImageRef{},
		Diagrams: map[string]ImageRef{},
	}
}

// AssetPipelineConfig holds the runtime knobs for the pipeline.
type AssetPipelineConfig struct {
	HTTPClient        *http.Client
	MMDCPath          string
	SVGRasterizerPath string
	Strict            bool
	KeepTempImages    bool
	DefaultFAStyle    string
}

// DefaultAssetPipelineConfig returns a config with sane defaults: 30s
// HTTP timeout, mmdc on PATH, non-strict, no image retention.
func DefaultAssetPipelineConfig() AssetPipelineConfig {
	return AssetPipelineConfig{
		HTTPClient:        &http.Client{Timeout: 30 * time.Second},
		MMDCPath:          "mmdc",
		SVGRasterizerPath: "",
		Strict:            false,
		KeepTempImages:    false,
		DefaultFAStyle:    "solid",
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

func svgRasterizerArgs(binary, in, out string) []string {
	switch filepath.Base(binary) {
	case "magick":
		return []string{binary, in, "-background", "none", "-resize", "128x128", out}
	case "convert":
		return []string{binary, in, "-background", "none", "-resize", "128x128", out}
	default:
		return []string{binary, "-w", "128", "-h", "128", "-f", "png", "-o", out, in}
	}
}

func findSVGRasterizer() (string, error) {
	for _, candidate := range []string{"rsvg-convert", "magick", "convert"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("SVG rasterizer not found (install librsvg rsvg-convert or ImageMagick)")
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
	if writeErr := os.WriteFile(in, []byte(source), 0o600); writeErr != nil {
		return nil, writeErr
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
	return os.ReadFile(out) // #nosec G304 -- output path is inside a freshly-created temp directory.
}

func rasterizeSVGToPNG(ctx context.Context, svg []byte) ([]byte, error) {
	rasterizer, err := findSVGRasterizer()
	if err != nil {
		return nil, err
	}
	return rasterizeSVGToPNGWith(ctx, svg, rasterizer)
}

func rasterizeSVGToPNGWithOptional(ctx context.Context, svg []byte, rasterizer string) ([]byte, error) {
	if rasterizer != "" {
		return rasterizeSVGToPNGWith(ctx, svg, rasterizer)
	}
	return rasterizeSVGToPNG(ctx, svg)
}

func rasterizeSVGToPNGWith(ctx context.Context, svg []byte, rasterizer string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "gogcli-svg-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in.svg")
	out := filepath.Join(dir, "out.png")
	if writeErr := os.WriteFile(in, svg, 0o600); writeErr != nil {
		return nil, writeErr
	}
	args := svgRasterizerArgs(rasterizer, in, out)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) // #nosec G204 -- tool path and args are controlled by local config.
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return nil, fmt.Errorf("rasterize SVG: %w: %s", err, trimmed)
		}
		return nil, fmt.Errorf("rasterize SVG: %w", err)
	}
	return os.ReadFile(out) // #nosec G304 -- output path is inside a freshly-created temp directory.
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
func (p *AssetPipeline) Resolve(ctx context.Context, slides []slidesmarkdown.Slide) (AssetMap, error) {
	am := NewAssetMap()

	icons := collectIconRefs(slides)
	diagrams := collectDiagrams(slides)

	for ref := range icons {
		body, resolvedStyle, err := fetchFAIconWithStyleFallback(ctx, p.Config.HTTPClient, ref)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(stderrWriter(ctx), "warning: skipping FA icon :%s-%s: %v\n", ref.Style, ref.Name, err)
			continue
		}
		png, err := rasterizeSVGToPNGWithOptional(ctx, body, p.Config.SVGRasterizerPath)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(stderrWriter(ctx), "warning: skipping FA icon :%s-%s: %v\n", ref.Style, ref.Name, err)
			continue
		}
		ir, err := p.Uploader.UploadAsset(ctx, fmt.Sprintf("fa-%s-%s.png", resolvedStyle, ref.Name), "image/png", png)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(stderrWriter(ctx), "warning: skipping FA icon :%s-%s: upload: %v\n", ref.Style, ref.Name, err)
			continue
		}
		am.Icons[ref] = ir
		p.uploaded = append(p.uploaded, ir.DriveFileID)
	}

	for blockID, source := range diagrams {
		if p.Config.MMDCPath == "" {
			if p.Config.Strict {
				return am, fmt.Errorf("mmdc not configured; cannot render mermaid diagram %s", blockID)
			}
			fmt.Fprintf(stderrWriter(ctx), "warning: mmdc not configured; skipping mermaid diagram %s\n", blockID)
			continue
		}
		png, err := renderMermaidWithBinary(ctx, p.Config.MMDCPath, source)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(stderrWriter(ctx), "warning: skipping mermaid diagram %s: %v\n", blockID, err)
			continue
		}
		ir, err := p.Uploader.UploadAsset(ctx, blockID+".png", "image/png", png)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(stderrWriter(ctx), "warning: skipping mermaid diagram %s: upload: %v\n", blockID, err)
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
func collectIconRefs(slides []slidesmarkdown.Slide) map[slidesmarkdown.IconRef]struct{} {
	out := map[slidesmarkdown.IconRef]struct{}{}
	var walkBlocks func([]slidesmarkdown.Block)
	walkBlocks = func(blocks []slidesmarkdown.Block) {
		for _, b := range blocks {
			switch v := b.(type) {
			case slidesmarkdown.ParagraphBlock:
				if r, ok := leadingIcon(v.Inlines); ok {
					out[r] = struct{}{}
				}
			case slidesmarkdown.BulletsBlock:
				for _, item := range v.Items {
					if r, ok := leadingIcon(item.Inlines); ok {
						out[r] = struct{}{}
					}
				}
			case slidesmarkdown.HeadingBlock:
				if r, ok := leadingIcon(v.Inlines); ok {
					out[r] = struct{}{}
				}
			case slidesmarkdown.ColumnsBlock:
				for _, col := range v.Columns {
					walkBlocks(col)
				}
			case slidesmarkdown.IconRowsBlock:
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

func leadingIcon(inlines []slidesmarkdown.Inline) (slidesmarkdown.IconRef, bool) {
	if len(inlines) == 0 {
		return slidesmarkdown.IconRef{}, false
	}
	ref, ok := inlines[0].(slidesmarkdown.IconRef)
	return ref, ok
}

// collectDiagrams walks all slides for DiagramBlocks, returning {ID: source}.
func collectDiagrams(slides []slidesmarkdown.Slide) map[string]string {
	out := map[string]string{}
	var walkBlocks func([]slidesmarkdown.Block)
	walkBlocks = func(blocks []slidesmarkdown.Block) {
		for _, b := range blocks {
			switch v := b.(type) {
			case slidesmarkdown.DiagramBlock:
				out[v.ID] = v.Source
			case slidesmarkdown.ColumnsBlock:
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
func fetchFAIconWithStyleFallback(ctx context.Context, client *http.Client, ref slidesmarkdown.IconRef) ([]byte, string, error) {
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
	return ImageRef{DriveFileID: created.Id, PublicURL: driveImageDownloadURL(created.Id)}, nil
}

func (d *DriveUploader) DeleteAsset(ctx context.Context, fileID string) error {
	return d.Svc.Files.Delete(fileID).Context(ctx).Do()
}
