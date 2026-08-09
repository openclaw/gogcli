package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestEmbeddedTZData verifies that the time/tzdata import in main.go
// successfully embeds the IANA timezone database. On macOS/Linux this
// passes regardless, but on Windows (where Go has no system tz database)
// it validates the actual fix. The test also guards against accidental
// removal of the tzembed import.
func TestEmbeddedTZData(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	if !strings.Contains(string(source), `_ "github.com/openclaw/gogcli/internal/tzembed"`) {
		t.Fatalf("main.go must blank-import internal/tzembed")
	}

	zones := []string{
		"America/New_York",
		"America/Los_Angeles",
		"Europe/Berlin",
		"Europe/London",
		"Asia/Tokyo",
		"Australia/Sydney",
		"Pacific/Auckland",
		"UTC",
	}

	for _, zone := range zones {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Errorf("time.LoadLocation(%q) failed: %v (is time/tzdata imported?)", zone, err)
			continue
		}
		if loc == nil {
			t.Errorf("time.LoadLocation(%q) returned nil location", zone)
		}
	}
}
