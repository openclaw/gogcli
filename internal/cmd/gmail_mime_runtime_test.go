package cmd

import (
	"context"
	"encoding/base64"
	"net/mail"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
)

func TestBuildGmailMessage_DateHeaderUsesRuntimeTimezone(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "")

	ambientLayout := config.Layout{ConfigDir: t.TempDir(), ExplicitConfig: true}
	if err := config.NewConfigStore(ambientLayout).Write(config.File{DefaultTimezone: "UTC"}); err != nil {
		t.Fatalf("write ambient config: %v", err)
	}
	t.Setenv("GOG_CONFIG_DIR", ambientLayout.ConfigDir)

	runtimeLayout := config.Layout{ConfigDir: t.TempDir(), ExplicitConfig: true}
	runtimeStore := config.NewConfigStore(runtimeLayout)
	if err := runtimeStore.Write(config.File{DefaultTimezone: "Asia/Tokyo"}); err != nil {
		t.Fatalf("write runtime config: %v", err)
	}
	ctx := app.WithRuntime(context.Background(), &app.Runtime{
		Layout: runtimeLayout,
		Config: runtimeStore,
	})

	msg, err := buildGmailMessage(ctx, sendMessageOptions{
		FromAddr: "a@b.com",
		Subject:  "Hi",
		Body:     "Hello",
	}, sendBatch{To: []string{"c@d.com"}}, false)
	if err != nil {
		t.Fatalf("buildGmailMessage: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(msg.Raw)
	if err != nil {
		t.Fatalf("decode message: %v", err)
	}
	parsed, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if dateHeader := parsed.Header.Get("Date"); !strings.HasSuffix(dateHeader, "+0900") {
		t.Fatalf("expected Asia/Tokyo Date header, got %q", dateHeader)
	}
}
