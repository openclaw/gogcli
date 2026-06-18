package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"
)

func TestSlidesTableCreate_BuildsCreateTableRequest(t *testing.T) {
	var captured slides.BatchUpdatePresentationRequest
	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"presentationId": "pres1",
				"replies": []any{
					map[string]any{"createTable": map[string]any{"objectId": "table_123"}},
				},
			})
		case strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet:
			resp := slidesPresGetResponse("", false)
			resp["revisionId"] = "rev1"
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer slidesSrv.Close()

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(), option.WithHTTPClient(slidesSrv.Client()), option.WithEndpoint(slidesSrv.URL+"/"))
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}

	var stdout bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &stdout, io.Discard), slidesSvc)
	cmd := &SlidesTableCreateCmd{
		PresentationID: "pres1",
		SlideID:        "existing_slide_1",
		ObjectID:       "table_123",
		Rows:           2,
		Cols:           3,
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(captured.Requests) != 1 || captured.Requests[0].CreateTable == nil {
		t.Fatalf("expected one createTable request, got %+v", captured.Requests)
	}
	create := captured.Requests[0].CreateTable
	if create.ObjectId != "table_123" || create.Rows != 2 || create.Columns != 3 {
		t.Fatalf("unexpected table request: %+v", create)
	}
	ep := create.ElementProperties
	if ep == nil || ep.PageObjectId != "existing_slide_1" {
		t.Fatalf("table not placed on target slide: %+v", ep)
	}
	if ep.Size != nil {
		t.Fatalf("createTable must omit provider-ignored size: %+v", ep.Size)
	}
	if ep.Transform != nil {
		t.Fatalf("createTable must omit provider-ignored transform: %+v", ep.Transform)
	}
	if captured.WriteControl == nil || captured.WriteControl.RequiredRevisionId != "rev1" {
		t.Fatalf("write control = %+v, want required revision rev1", captured.WriteControl)
	}
	if !strings.Contains(stdout.String(), "table\ttable_123") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestSlidesTableCreate_DryRunIncludesBatchUpdate(t *testing.T) {
	slidesFactory := func(context.Context, string) (*slides.Service, error) {
		t.Fatal("dry-run must not create a Slides service")
		return nil, context.Canceled
	}

	var stdout bytes.Buffer
	ctx := withSlidesTestServiceFactory(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), slidesFactory)
	err := runKong(t, &SlidesTableCreateCmd{}, []string{
		"pres1",
		"slide1",
		"--rows", "1",
		"--cols", "2",
	}, ctx, &RootFlags{Account: "a@b.com", DryRun: true})
	if err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		Op      string `json:"op"`
		Request struct {
			BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
		} `json:"request"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if got.Op != "slides.table.create" {
		t.Fatalf("op = %q", got.Op)
	}
	if len(got.Request.BatchUpdate.Requests) != 1 || got.Request.BatchUpdate.Requests[0].CreateTable == nil {
		t.Fatalf("unexpected dry-run batch: %+v", got.Request.BatchUpdate.Requests)
	}
}

func TestSlidesTableCreate_Validation(t *testing.T) {
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)
	cases := []struct {
		name string
		cmd  SlidesTableCreateCmd
		want string
	}{
		{name: "empty presentation", cmd: SlidesTableCreateCmd{SlideID: "s", Rows: 1, Cols: 1}, want: "empty presentationId"},
		{name: "empty slide", cmd: SlidesTableCreateCmd{PresentationID: "p", Rows: 1, Cols: 1}, want: "empty slideId"},
		{name: "bad rows", cmd: SlidesTableCreateCmd{PresentationID: "p", SlideID: "s", Rows: 0, Cols: 1}, want: "--rows must be >= 1"},
		{name: "bad cols", cmd: SlidesTableCreateCmd{PresentationID: "p", SlideID: "s", Rows: 1, Cols: 0}, want: "--cols must be >= 1"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Run(ctx, &RootFlags{Account: "a@b.com"})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}
