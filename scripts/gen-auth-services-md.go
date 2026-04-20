package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/googleauth"
)

const (
	readmePath  = "README.md"
	startMarker = "<!-- auth-services:start -->"
	endMarker   = "<!-- auth-services:end -->"
)

func main() {
	data, err := os.ReadFile(readmePath)
	if err != nil {
		fatalf("read README: %v", err)
	}

	content := string(data)
	start := strings.Index(content, startMarker)
	end := strings.Index(content, endMarker)

	if start == -1 || end == -1 || end < start {
		fatalf("missing markers %q ... %q in %s", startMarker, endMarker, readmePath)
	}

	table := googleauth.ServicesMarkdown(googleauth.ServicesInfo())
	if table == "" {
		fatalf("empty services table")
	}
	table = strings.TrimRight(table, "\n")

	replacement := startMarker + "\n" + table + "\n" + endMarker
	updated := content[:start] + replacement + content[end+len(endMarker):]

	if updated == content {
		return
	}

	if err := os.WriteFile(readmePath, []byte(updated), 0o600); err != nil { //nolint:gosec // path is a repo-local generated docs target
		fatalf("write README: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
