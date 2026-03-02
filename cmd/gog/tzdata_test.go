package main

import (
	"testing"
	"time"

	_ "github.com/steipete/gogcli/internal/tzembed" // Ensure tz database is embedded
)

// TestEmbeddedTZData verifies that the time/tzdata import in main.go
// successfully embeds the IANA timezone database. On macOS/Linux this
// passes regardless, but on Windows (where Go has no system tz database)
// it validates the actual fix. The test also guards against accidental
// removal of the time/tzdata import.
func TestEmbeddedTZData(t *testing.T) {
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
