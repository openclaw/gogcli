package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/outfmt"
)

func TestSitesListAndSearchConstrainToGoogleSites(t *testing.T) {
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

	flags := &RootFlags{Account: "a@b.com"}
	var outBuf bytes.Buffer
	ctx := withSitesDriveTestService(newCmdRuntimeOutputContext(t, &outBuf, io.Discard), svc)

	if err := runKong(t, &SitesListCmd{}, []string{}, ctx, flags); err != nil {
		t.Fatalf("list: %v", err)
	}
	if err := runKong(t, &SitesSearchCmd{}, []string{"team"}, ctx, flags); err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(queries) != 2 {
		t.Fatalf("queries = %#v", queries)
	}
	for _, q := range queries {
		if !strings.Contains(q, googleSitesQuery) || !strings.Contains(q, "trashed = false") {
			t.Fatalf("query missing sites/trashed constraints: %q", q)
		}
	}
	if !strings.Contains(outBuf.String(), "Team Site") || !strings.Contains(outBuf.String(), "site") {
		t.Fatalf("unexpected output: %q", outBuf.String())
	}
}

func TestSitesListSearchInvalidMaxFailsBeforeService(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSitesDriveTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		unexpectedSitesDriveTestService(t, "expected max validation to fail before creating sites drive service"),
	)
	cases := []struct {
		name string
		cmd  any
		args []string
	}{
		{name: "list zero", cmd: &SitesListCmd{}, args: []string{"--max", "0"}},
		{name: "list negative", cmd: &SitesListCmd{}, args: []string{"--max=-1"}},
		{name: "search zero", cmd: &SitesSearchCmd{}, args: []string{"team", "--max", "0"}},
		{name: "search negative", cmd: &SitesSearchCmd{}, args: []string{"team", "--max=-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runKong(t, tc.cmd, tc.args, ctx, flags)
			if ExitCode(err) != 2 || !strings.Contains(err.Error(), "max must be > 0") {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestSitesGetAndURL(t *testing.T) {
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

	flags := &RootFlags{Account: "a@b.com"}
	var outBuf bytes.Buffer
	ctx := withSitesDriveTestService(newCmdRuntimeOutputContext(t, &outBuf, io.Discard), svc)

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

	t.Run("get json runtime output", func(t *testing.T) {
		var jsonOut bytes.Buffer
		jsonCtx := withSitesDriveTestService(newCmdRuntimeJSONOutputContext(t, &jsonOut, io.Discard), svc)
		if err := (&SitesGetCmd{SiteID: "site1"}).Run(jsonCtx, flags); err != nil {
			t.Fatalf("get JSON: %v", err)
		}
		var payload struct {
			Site struct {
				ID string `json:"id"`
			} `json:"site"`
		}
		if err := json.Unmarshal(jsonOut.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload.Site.ID != "site1" {
			t.Fatalf("site id = %q, want site1", payload.Site.ID)
		}
	})

	t.Run("url json runtime output", func(t *testing.T) {
		var jsonOut bytes.Buffer
		jsonCtx := withSitesDriveTestService(newCmdRuntimeJSONOutputContext(t, &jsonOut, io.Discard), svc)
		if err := (&SitesURLCmd{SiteIDs: []string{"site1"}}).Run(jsonCtx, flags); err != nil {
			t.Fatalf("url JSON: %v", err)
		}
		var payload struct {
			URLs []struct {
				ID  string `json:"id"`
				URL string `json:"url"`
			} `json:"urls"`
		}
		if err := json.Unmarshal(jsonOut.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(payload.URLs) != 1 || payload.URLs[0].ID != "site1" ||
			payload.URLs[0].URL != "https://sites.google.com/d/site1/edit" {
			t.Fatalf("unexpected URLs: %#v", payload.URLs)
		}
	})
}

func TestSitesGetRejectsNonSite(t *testing.T) {
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "doc1",
			"name":     "Doc",
			"mimeType": driveMimeGoogleDoc,
		})
	}))
	defer closeSrv()

	ctx := withSitesDriveTestService(
		outfmt.WithMode(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), outfmt.Mode{JSON: true}),
		svc,
	)
	err := (&SitesGetCmd{SiteID: "doc1"}).Run(ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "not a Google Site") {
		t.Fatalf("expected not-site error, got %v", err)
	}

	err = (&SitesURLCmd{SiteIDs: []string{"doc1"}}).Run(ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "not a Google Site") {
		t.Fatalf("expected url not-site error, got %v", err)
	}
}
