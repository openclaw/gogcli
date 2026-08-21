package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"google.golang.org/api/slides/v1"
)

func TestSlidesSkipSlide(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	var out bytes.Buffer
	ctx := withSlidesTestService(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		newSlidesServiceFromServer(t, srv),
	)
	cmd := &SlidesSkipSlideCmd{PresentationID: "pres1", SlideID: "slide_1"}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(captured) != 1 || captured[0].UpdateSlideProperties == nil {
		t.Fatalf("expected one UpdateSlideProperties request, got %+v", captured)
	}
	update := captured[0].UpdateSlideProperties
	if update.ObjectId != "slide_1" || update.Fields != "isSkipped" {
		t.Fatalf("unexpected update request: %+v", update)
	}
	if update.SlideProperties == nil || !update.SlideProperties.IsSkipped {
		t.Fatalf("expected isSkipped=true, got %+v", update.SlideProperties)
	}

	var got struct {
		PresentationID string `json:"presentationId"`
		SlideObjectID  string `json:"slideObjectId"`
		IsSkipped      bool   `json:"isSkipped"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	if got.PresentationID != "pres1" || got.SlideObjectID != "slide_1" || !got.IsSkipped {
		t.Fatalf("unexpected output: %#v", got)
	}
}

func TestSlidesUnskipSlideSendsFalse(t *testing.T) {
	body := slidesVisibilityBatchUpdate("slide_1", false)
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if !strings.Contains(string(encoded), `"slideProperties":{"isSkipped":false}`) {
		t.Fatalf("request must explicitly send isSkipped=false: %s", encoded)
	}

	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	var out bytes.Buffer
	ctx := withSlidesTestService(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		newSlidesServiceFromServer(t, srv),
	)
	cmd := &SlidesUnskipSlideCmd{PresentationID: "pres1", SlideID: "slide_1"}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(captured) != 1 || captured[0].UpdateSlideProperties == nil {
		t.Fatalf("expected one UpdateSlideProperties request, got %+v", captured)
	}
	if strings.Contains(out.String(), `"isSkipped": true`) || !strings.Contains(out.String(), `"isSkipped": false`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestSlidesUnskipSlideDryRunSkipsService(t *testing.T) {
	var out bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created during dry-run")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesUnskipSlideCmd{PresentationID: "pres1", SlideID: "slide_1"}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", DryRun: true}); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			IsSkipped   bool                                  `json:"is_skipped"`
			BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, out.String())
	}
	if !got.DryRun || got.Op != "slides.unskip-slide" || got.Request.IsSkipped {
		t.Fatalf("unexpected dry-run output: %#v", got)
	}
	if len(got.Request.BatchUpdate.Requests) != 1 || got.Request.BatchUpdate.Requests[0].UpdateSlideProperties == nil {
		t.Fatalf("expected UpdateSlideProperties dry-run request, got %+v", got.Request.BatchUpdate.Requests)
	}
}

func TestSlidesVisibilityValidation(t *testing.T) {
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)

	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "skip empty presentation",
			run: func() error {
				return (&SlidesSkipSlideCmd{PresentationID: " ", SlideID: "slide_1"}).Run(ctx, &RootFlags{Account: "a@b.com"})
			},
			want: "empty presentationId",
		},
		{
			name: "unskip empty slide",
			run: func() error {
				return (&SlidesUnskipSlideCmd{PresentationID: "pres1", SlideID: " "}).Run(ctx, &RootFlags{Account: "a@b.com"})
			},
			want: "empty slideId",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			if got := ExitCode(err); got != 2 {
				t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
			}
		})
	}
}
