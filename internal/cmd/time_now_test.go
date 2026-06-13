package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
)

func TestTimeNowCmd_JSON(t *testing.T) {
	var output bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &output, io.Discard)
	if err := runKong(t, &TimeNowCmd{}, []string{"--timezone", "UTC"}, ctx, &RootFlags{}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	var parsed struct {
		Timezone    string `json:"timezone"`
		UTCOffset   string `json:"utc_offset"`
		CurrentTime string `json:"current_time"`
	}
	if err := json.Unmarshal(output.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.Timezone != "UTC" {
		t.Fatalf("unexpected timezone: %q", parsed.Timezone)
	}
	if parsed.UTCOffset != "+00:00" {
		t.Fatalf("unexpected offset: %q", parsed.UTCOffset)
	}
	if parsed.CurrentTime == "" {
		t.Fatalf("expected current_time")
	}
}

func TestTimeNowCmd_InvalidTimezone(t *testing.T) {
	err := runKong(t, &TimeNowCmd{}, []string{"--timezone", "Nope/Zone"}, newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &RootFlags{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}
}

func TestTimeNowCmd_UsesRuntimeDefaultTimezone(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "")

	ambientLayout := config.Layout{ConfigDir: t.TempDir(), ExplicitConfig: true}
	if err := config.NewConfigStore(ambientLayout).Write(config.File{DefaultTimezone: "UTC"}); err != nil {
		t.Fatalf("write ambient config: %v", err)
	}
	t.Setenv("GOG_CONFIG_DIR", ambientLayout.ConfigDir)

	runtimeLayout := config.Layout{ConfigDir: t.TempDir(), ExplicitConfig: true}
	runtimeStore := config.NewConfigStore(runtimeLayout)
	if err := runtimeStore.Write(config.File{DefaultTimezone: "Europe/London"}); err != nil {
		t.Fatalf("write runtime config: %v", err)
	}

	result := executeWithTestRuntime(t, []string{"--json", "time", "now"}, &app.Runtime{
		Layout: runtimeLayout,
		Config: runtimeStore,
	})
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%s", result.err, result.stderr)
	}

	var parsed struct {
		Timezone string `json:"timezone"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nstdout=%s", err, result.stdout)
	}
	if parsed.Timezone != "Europe/London" {
		t.Fatalf("timezone = %q, want Europe/London", parsed.Timezone)
	}
}
