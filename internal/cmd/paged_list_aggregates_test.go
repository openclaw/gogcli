package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/outfmt"
)

func TestWritePagedJSONResultAggregates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []string
		token string
		count int
		more  bool
	}{
		{"more", []string{"1", "2"}, "tok", 2, true},
		{"last", []string{"1"}, "", 1, false},
		{"empty", []string{}, "", 0, false},
		{"nil", nil, "", 0, false},
		{"empty intermediate", nil, "tok", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
			ctx = app.WithRuntime(ctx, &app.Runtime{IO: app.IO{Out: &buf}})
			payload := map[string]any{"messages": tc.items, "nextPageToken": tc.token, "warnings": []string{"auxiliary"}}
			if err := writePagedJSONResult(ctx, payload, len(tc.items), false); err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got["count"] != float64(tc.count) || got["has_more"] != tc.more || got["nextPageToken"] != tc.token {
				t.Fatalf("payload = %s", buf.String())
			}
		})
	}
}

func TestWriteDriveListAggregatesAndProjection(t *testing.T) {
	for _, resultsOnly := range []bool{false, true} {
		var buf bytes.Buffer
		ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
		ctx = app.WithRuntime(ctx, &app.Runtime{IO: app.IO{Out: &buf}})
		ctx = outfmt.WithJSONTransform(ctx, outfmt.JSONTransform{ResultsOnly: resultsOnly})
		err := writeDriveFileList(ctx, &drive.FileList{Files: []*drive.File{{Id: "one"}}, NextPageToken: "two"}, "empty", "")
		if err != nil {
			t.Fatal(err)
		}
		if resultsOnly {
			var got []map[string]any
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil || len(got) != 1 {
				t.Fatalf("projection: %s; %v", buf.String(), err)
			}
		} else {
			var got map[string]any
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got["count"] != float64(1) || got["has_more"] != true || got["nextPageToken"] != "two" {
				t.Fatalf("payload: %s", buf.String())
			}
		}
	}
}

func TestPagedAggregatesPreserveExplicitMetadata(t *testing.T) {
	payload := map[string]any{"count": 123, "has_more": true}
	addPagedAggregates(payload, 1)
	if payload["count"] != 123 || payload["has_more"] != true {
		t.Fatalf("metadata overwritten: %#v", payload)
	}
}
