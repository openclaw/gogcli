package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestSitesListAndSearchConstrainToGoogleSites(t *testing.T) {
	origNew := newSitesDriveService
	t.Cleanup(func() { newSitesDriveService = origNew })

	var queries []string
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}
		queries = append(queries, r.URL.Query().Get("q"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{{
				"id":           "site1",
				"name":         "Team Site",
				"mimeType":     driveMimeGoogleSite,
				"modifiedTime": "2026-05-09T10:00:00Z",
			}},
		})
	}))
	defer closeSrv()
	newSitesDriveService = stubDriveService(svc)

	flags := &RootFlags{Account: "a@b.com"}
	var outBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &outBuf, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	tableOut := captureStdout(t, func() {
		if err := runKong(t, &SitesListCmd{}, []string{}, ctx, flags); err != nil {
			t.Fatalf("list: %v", err)
		}
		if err := runKong(t, &SitesSearchCmd{}, []string{"team"}, ctx, flags); err != nil {
			t.Fatalf("search: %v", err)
		}
	})

	if len(queries) != 2 {
		t.Fatalf("queries = %#v", queries)
	}
	for _, q := range queries {
		if !strings.Contains(q, googleSitesQuery) || !strings.Contains(q, "trashed = false") {
			t.Fatalf("query missing sites/trashed constraints: %q", q)
		}
	}
	if !strings.Contains(tableOut, "Team Site") || !strings.Contains(tableOut, "site") {
		t.Fatalf("unexpected output: %q", tableOut)
	}
}

func TestSitesGetAndURL(t *testing.T) {
	origNew := newSitesDriveService
	t.Cleanup(func() { newSitesDriveService = origNew })

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/drive/v3/files/"), "/files/")
		if id != "site1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "site1",
			"name":         "Team Site",
			"mimeType":     driveMimeGoogleSite,
			"modifiedTime": "2026-05-09T10:00:00Z",
			"webViewLink":  "https://sites.google.com/d/site1/edit",
		})
	}))
	defer closeSrv()
	newSitesDriveService = stubDriveService(svc)

	flags := &RootFlags{Account: "a@b.com"}
	var outBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &outBuf, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &SitesGetCmd{}, []string{"https://sites.google.com/d/site1/edit"}, ctx, flags); err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := runKong(t, &SitesURLCmd{}, []string{"site1"}, ctx, flags); err != nil {
		t.Fatalf("url: %v", err)
	}
	got := outBuf.String()
	if !strings.Contains(got, "link\thttps://sites.google.com/d/site1/edit") ||
		!strings.Contains(got, "site1\thttps://sites.google.com/d/site1/edit") {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestSitesGetRejectsNonSite(t *testing.T) {
	origNew := newSitesDriveService
	t.Cleanup(func() { newSitesDriveService = origNew })

	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "doc1",
			"name":     "Doc",
			"mimeType": driveMimeGoogleDoc,
		})
	}))
	defer closeSrv()
	newSitesDriveService = stubDriveService(svc)

	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
	err := (&SitesGetCmd{SiteID: "doc1"}).Run(ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "not a Google Site") {
		t.Fatalf("expected not-site error, got %v", err)
	}

	err = (&SitesURLCmd{SiteIDs: []string{"doc1"}}).Run(ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "not a Google Site") {
		t.Fatalf("expected url not-site error, got %v", err)
	}
}
