package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsTablePinHeaderZeroIsSerializedForAllTables(t *testing.T) {
	doc := docsTableOpsTestDocument(
		docsTableOpsTestElement(5, "First", 2),
		docsTableOpsTestElement(40, "Second", 3),
	)
	var got map[string]any
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(doc)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batch: %v", err)
			}
			_ = json.NewEncoder(w).Encode(&docs.BatchUpdateDocumentResponse{DocumentId: "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	cmd := &DocsTablePinHeaderCmd{}
	err := runKong(t, cmd, []string{"doc1", "--table", "*", "--rows", "0"}, newDocsTableOpsTestContext(t, docSvc), &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	writeControl, ok := got["writeControl"].(map[string]any)
	if !ok || writeControl["requiredRevisionId"] != "rev-1" {
		t.Fatalf("write control = %#v", got["writeControl"])
	}
	requests, ok := got["requests"].([]any)
	if !ok || len(requests) != 2 {
		t.Fatalf("requests = %#v", got["requests"])
	}
	wantStarts := []float64{40, 5}
	for i, rawRequest := range requests {
		request, ok := rawRequest.(map[string]any)
		if !ok {
			t.Fatalf("request %d = %#v", i, rawRequest)
		}
		pin, ok := request["pinTableHeaderRows"].(map[string]any)
		if !ok {
			t.Fatalf("pin request %d = %#v", i, request)
		}
		count, exists := pin["pinnedHeaderRowsCount"]
		if !exists || count != float64(0) {
			t.Fatalf("pinned count %d = %#v, exists=%v", i, count, exists)
		}
		location := pin["tableStartLocation"].(map[string]any)
		if location["index"] != wantStarts[i] {
			t.Fatalf("table start %d = %#v, want %v", i, location["index"], wantStarts[i])
		}
	}
}

func TestDocsTablePinHeaderRejectsTooManyRows(t *testing.T) {
	doc := docsTableOpsTestDocument(docsTableOpsTestElement(5, "Header", 2))
	postCount := 0
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(doc)
			return
		}
		if r.Method == http.MethodPost {
			postCount++
		}
		http.NotFound(w, r)
	})
	defer cleanup()

	cmd := &DocsTablePinHeaderCmd{}
	err := runKong(t, cmd, []string{"doc1", "--rows", "3"}, newDocsTableOpsTestContext(t, docSvc), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "table has 2 rows") {
		t.Fatalf("expected row-count error, got %v", err)
	}
	if postCount != 0 {
		t.Fatalf("post count = %d, want 0", postCount)
	}
}

func TestDocsTablePinHeaderRejectsNegativeRows(t *testing.T) {
	cmd := &DocsTablePinHeaderCmd{DocID: "doc1", Table: "1", Rows: -1}
	err := cmd.Run(newCmdRuntimeOutputContext(t, nil, nil), &RootFlags{Account: "a@b.com", DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "--rows must be >= 0") {
		t.Fatalf("expected rows error, got %v", err)
	}
}
