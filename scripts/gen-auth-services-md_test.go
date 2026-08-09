package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func TestMainUpdatesReadme(t *testing.T) {
	orig, _ := os.Getwd()

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	t.Cleanup(func() { _ = os.Chdir(orig) })

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test\n"+startMarker+"\n"+endMarker+"\n"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}

	main()

	updated, err := os.ReadFile(readme)
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	text := string(updated)
	if !strings.Contains(text, startMarker) || !strings.Contains(text, endMarker) {
		t.Fatalf("missing markers: %q", text)
	}

	if !strings.Contains(text, "|") {
		t.Fatalf("expected markdown table: %q", text)
	}
}

func TestReadmeAuthServicesBlockIsFresh(t *testing.T) {
	readmePath := filepath.Join("..", "README.md")

	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %s: %v", readmePath, err)
	}

	got, ok := authServicesBlock(string(data))
	if !ok {
		t.Fatalf("missing auth services markers in %s", readmePath)
	}

	want := strings.TrimRight(googleauth.ServicesMarkdown(googleauth.ServicesInfo()), "\n")
	if got != want {
		t.Fatalf("README auth services block is stale; run go run ./scripts/gen-auth-services-md.go")
	}
}

func authServicesBlock(content string) (string, bool) {
	content = strings.ReplaceAll(content, "\r\n", "\n")

	start := strings.Index(content, startMarker)
	end := strings.Index(content, endMarker)

	if start == -1 || end == -1 || end < start {
		return "", false
	}

	start += len(startMarker)
	block := content[start:end]
	block = strings.TrimPrefix(block, "\n")
	block = strings.TrimSuffix(block, "\n")

	return block, true
}
