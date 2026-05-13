package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
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
	if err := cmd.Run(); err != nil {
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
