package cmd

import (
	"fmt"
	"net/http"
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
