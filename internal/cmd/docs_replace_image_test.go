package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

func TestSelectDocsReplaceImage(t *testing.T) {
	t.Parallel()

	items := []docsImageListItem{
		{ObjectID: "img-a", Alt: "Quarterly chart"},
		{ObjectID: "img-b", Alt: "Quarterly chart detail", Positioned: true},
	}
	tests := []struct {
		name      string
		objectID  string
		matchAlt  string
		wantID    string
		wantError string
		wantExit  int
	}{
		{name: "object id", objectID: "img-b", wantID: "img-b"},
		{name: "case insensitive alt", matchAlt: "DETAIL", wantID: "img-b"},
		{name: "ambiguous alt", matchAlt: "quarterly", wantError: "ambiguous --match-alt", wantExit: 2},
		{name: "ambiguous default", wantError: "document contains 2 images", wantExit: 2},
		{name: "missing object", objectID: "missing", wantError: "image object not found", wantExit: emptyResultsExitCode},
		{name: "missing alt", matchAlt: "missing", wantError: "no image alt text contains", wantExit: emptyResultsExitCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := selectDocsReplaceImage(items, tt.objectID, tt.matchAlt)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) || ExitCode(err) != tt.wantExit {
					t.Fatalf("error = %v, exit = %d; want %q, exit %d", err, ExitCode(err), tt.wantError, tt.wantExit)
				}
				return
			}
			if err != nil {
				t.Fatalf("select: %v", err)
			}
			if got.ObjectID != tt.wantID {
				t.Fatalf("object ID = %q, want %q", got.ObjectID, tt.wantID)
			}
		})
	}

	if _, err := selectDocsReplaceImage(nil, "", ""); err == nil || ExitCode(err) != emptyResultsExitCode {
		t.Fatalf("empty images error = %v, exit = %d", err, ExitCode(err))
	}
}

func TestDocsReplaceImageURLRun(t *testing.T) {
	t.Parallel()

	doc := docsReplaceImageTestDocument()
	var got docs.BatchUpdateDocumentRequest
	docsSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(doc)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batch: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	driveFactory := func(context.Context, string) (*drive.Service, error) {
		t.Fatal("URL replacement must not create a Drive service")
		return nil, errors.New("unexpected Drive service call")
	}
	var stdout, stderr bytes.Buffer
	ctx := withDriveTestServiceFactory(newCmdRuntimeJSONOutputContext(t, &stdout, &stderr), driveFactory)
	ctx = withDocsTestService(ctx, docsSvc)
	err := runKong(t, &DocsReplaceImageCmd{}, []string{
		"doc1",
		"--url", "https://example.com/replacement.png?sig=abc",
		"--object-id", "img-positioned",
	}, ctx, &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("replace-image: %v", err)
	}
	if len(got.Requests) != 1 || got.Requests[0].ReplaceImage == nil {
		t.Fatalf("requests = %#v", got.Requests)
	}
	replace := got.Requests[0].ReplaceImage
	if replace.ImageObjectId != "img-positioned" || replace.Uri != "https://example.com/replacement.png?sig=abc" ||
		replace.ImageReplaceMethod != docsImageReplaceMethodCenterCrop {
		t.Fatalf("replace request = %#v", replace)
	}
	if got.WriteControl == nil || got.WriteControl.RequiredRevisionId != "rev-images" {
		t.Fatalf("write control = %#v", got.WriteControl)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if payload["objectId"] != "img-positioned" || payload["positioned"] != true || payload["replaced"] != true {
		t.Fatalf("output = %#v", payload)
	}
	if payload["width"] != 320.0 || payload["height"] != 180.0 || payload["url"] != "https://example.com/replacement.png?sig=abc" {
		t.Fatalf("output metadata = %#v", payload)
	}
}

func TestDocsReplaceImageFileUploadAndCleanup(t *testing.T) {
	t.Parallel()

	imagePath := filepath.Join(t.TempDir(), "replacement.png")
	if err := os.WriteFile(imagePath, []byte("test-png"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}

	var got docs.BatchUpdateDocumentRequest
	docsSvc, docsCleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(docsReplaceImageSingleInlineDocument())
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batch: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer docsCleanup()

	var uploads, shares, revokes int
	driveSvc, driveCleanup := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/upload/drive/v3/files"):
			uploads++
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "uploaded-1", "name": "replacement.png"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/files/uploaded-1/permissions"):
			shares++
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "permission-1", "type": "anyone", "role": "reader"})
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/files/uploaded-1/permissions/permission-1"):
			revokes++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveCleanup()

	var stdout, stderr bytes.Buffer
	ctx := withDriveTestService(newCmdRuntimeJSONOutputContext(t, &stdout, &stderr), driveSvc)
	ctx = withDocsTestService(ctx, docsSvc)
	err := runKong(t, &DocsReplaceImageCmd{}, []string{"doc1", "--file", imagePath}, ctx, &RootFlags{
		Account: "a@b.com",
		Force:   true,
		NoInput: true,
	})
	if err != nil {
		t.Fatalf("replace-image --file: %v", err)
	}
	if uploads != 1 || shares != 1 || revokes != 1 {
		t.Fatalf("drive calls: uploads=%d shares=%d revokes=%d", uploads, shares, revokes)
	}
	if len(got.Requests) != 1 || got.Requests[0].ReplaceImage == nil ||
		!strings.Contains(got.Requests[0].ReplaceImage.Uri, "uploaded-1") {
		t.Fatalf("replace request = %#v", got.Requests)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if payload["uploadedFileId"] != "uploaded-1" || payload["permissionId"] != "permission-1" || payload["revoked"] != true {
		t.Fatalf("output = %#v", payload)
	}
}

func TestDocsReplaceImageDryRunSkipsServices(t *testing.T) {
	t.Parallel()

	docsFactory := func(context.Context, string) (*docs.Service, error) {
		t.Fatal("dry-run must not create a Docs service")
		return nil, errors.New("unexpected Docs service call")
	}
	driveFactory := func(context.Context, string) (*drive.Service, error) {
		t.Fatal("dry-run must not create a Drive service")
		return nil, errors.New("unexpected Drive service call")
	}
	var stdout, stderr bytes.Buffer
	ctx := withDriveTestServiceFactory(newCmdRuntimeJSONOutputContext(t, &stdout, &stderr), driveFactory)
	ctx = withDocsTestServiceFactory(ctx, docsFactory)
	err := runKong(t, &DocsReplaceImageCmd{}, []string{
		"doc1",
		"--url", "https://example.com/replacement.png",
		"--match-alt", "chart",
	}, ctx, &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("dry-run error = %v", err)
	}
	var payload struct {
		Op      string `json:"op"`
		Request struct {
			MatchAlt string `json:"matchAlt"`
			Method   string `json:"method"`
		} `json:"request"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if payload.Op != "docs.replace-image" || payload.Request.MatchAlt != "chart" || payload.Request.Method != docsImageReplaceMethodCenterCrop {
		t.Fatalf("output = %#v", payload)
	}
}

func TestDocsReplaceImageSelectorConflict(t *testing.T) {
	t.Parallel()

	err := runKong(t, &DocsReplaceImageCmd{}, []string{
		"doc1", "--url", "https://example.com/replacement.png", "--object-id", "img-1", "--match-alt", "chart",
	}, newCmdRuntimeOutputContext(t, nil, nil), &RootFlags{Account: "a@b.com", DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error = %v", err)
	}
}

func docsReplaceImageSingleInlineDocument() *docs.Document {
	doc := docsReplaceImageTestDocument()
	doc.Body.Content[0].Paragraph.PositionedObjectIds = nil
	doc.PositionedObjects = nil
	return doc
}

func docsReplaceImageTestDocument() *docs.Document {
	return &docs.Document{
		DocumentId: "doc1",
		RevisionId: "rev-images",
		Body: &docs.Body{Content: []*docs.StructuralElement{{
			StartIndex: 1,
			EndIndex:   2,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{{
					StartIndex: 1,
					EndIndex:   2,
					InlineObjectElement: &docs.InlineObjectElement{
						InlineObjectId: "img-inline",
					},
				}},
				PositionedObjectIds: []string{"img-positioned"},
			},
		}}},
		InlineObjects: map[string]docs.InlineObject{
			"img-inline": {
				InlineObjectProperties: &docs.InlineObjectProperties{EmbeddedObject: docsReplaceEmbeddedObject("Inline chart", 200, 100)},
			},
		},
		PositionedObjects: map[string]docs.PositionedObject{
			"img-positioned": {
				PositionedObjectProperties: &docs.PositionedObjectProperties{EmbeddedObject: docsReplaceEmbeddedObject("Positioned diagram", 320, 180)},
			},
		},
	}
}

func docsReplaceEmbeddedObject(title string, width, height float64) *docs.EmbeddedObject {
	return &docs.EmbeddedObject{
		Title:           title,
		ImageProperties: &docs.ImageProperties{},
		Size: &docs.Size{
			Width:  &docs.Dimension{Magnitude: width, Unit: "PT"},
			Height: &docs.Dimension{Magnitude: height, Unit: "PT"},
		},
	}
}
