package cmd

import (
	"context"
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
