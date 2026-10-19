package cmd

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type DocsFetchCmd struct {
	IDsFile   string `name:"ids-file" required:"" help:"One Doc ID or URL per line; - for stdin" type:"path"`
	OutputDir string `name:"output-dir" required:"" help:"Local snapshot directory (contains document text)" type:"path"`
	Refresh   bool   `name:"refresh" help:"Refetch successful snapshots instead of resuming from them"`
}

type docsFetchedTab struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

type docsFetchedSnapshot struct {
	DocumentID string           `json:"documentId"`
	Account    string           `json:"account"`
	Client     string           `json:"client"`
	Title      string           `json:"title"`
	RevisionID string           `json:"revisionId"`
	FetchedAt  string           `json:"fetchedAt"`
	SourceURL  string           `json:"sourceUrl"`
	Tabs       []docsFetchedTab `json:"tabs"`
}

type docsFetchResult struct {
	DocumentID string `json:"documentId"`
	File       string `json:"file,omitempty"`
	Cached     bool   `json:"cached"`
	Error      string `json:"error,omitempty"`
}

var docsFetchIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

func (c *DocsFetchCmd) Run(ctx context.Context, flags *RootFlags) error {
	reader := stdinReader(ctx)
	if c.IDsFile != "-" {
		f, err := os.Open(c.IDsFile)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		reader = f
	}
	ids, err := readDocsFetchIDs(reader)
	if err != nil {
		return usage(err.Error())
	}
	if err := dryRunExit(ctx, flags, "docs.fetch", map[string]any{"documentIds": ids, "output_dir": c.OutputDir, "refresh": c.Refresh}); err != nil {
		return err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := resolveClientForEmail(ctx, account, flags)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.OutputDir, 0o700); err != nil {
		return err
	}
	root, err := os.OpenRoot(c.OutputDir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	results := make([]docsFetchResult, 0, len(ids))
	failed := 0
	var svc *docs.Service
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := id + ".json"
		result := docsFetchResult{DocumentID: id, File: name}
		if !c.Refresh && validDocsFetchCache(root, name, id, account, client) {
			result.Cached = true
		} else {
			if svc == nil {
				svc, err = requireDocsService(ctx, flags)
				if err != nil {
					return err
				}
			}
			doc, fetchErr := svc.Documents.Get(id).IncludeTabsContent(true).Context(ctx).Do()
			if fetchErr == nil {
				snapshot := makeDocsFetchSnapshot(doc, id, account, client)
				fetchErr = saveDocsFetchSnapshot(root, name, snapshot)
			}
			if fetchErr != nil {
				failed++
				result.Error = fetchErr.Error()
				result.File = ""
			}
		}
		results = append(results, result)
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"results": results, "failed": failed}); err != nil {
			return err
		}
	} else {
		for _, result := range results {
			ui.FromContext(ctx).Out().Linef("%s\t%s\tcached=%t\t%s", result.DocumentID, result.File, result.Cached, result.Error)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d document fetches failed; rerun the same command to reuse successful snapshots and retry failures", failed)
	}
	return nil
}

func readDocsFetchIDs(reader io.Reader) ([]string, error) {
	ids := []string{}
	seen := map[string]bool{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		id := normalizeGoogleID(line)
		if !docsFetchIDPattern.MatchString(id) {
			return nil, fmt.Errorf("invalid Doc ID or URL on line %d", len(ids)+1)
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no document IDs")
	}
	return ids, nil
}

func makeDocsFetchSnapshot(doc *docs.Document, id, account, client string) docsFetchedSnapshot {
	snapshot := docsFetchedSnapshot{DocumentID: id, Account: account, Client: client, Title: doc.Title, RevisionID: doc.RevisionId, FetchedAt: time.Now().UTC().Format(time.RFC3339), SourceURL: "https://docs.google.com/document/d/" + id + "/edit", Tabs: []docsFetchedTab{}}
	for _, tab := range flattenTabs(doc.Tabs) {
		if tab.TabProperties == nil || tab.DocumentTab == nil {
			continue
		}
		snapshot.Tabs = append(snapshot.Tabs, docsFetchedTab{ID: tab.TabProperties.TabId, Title: tab.TabProperties.Title, Text: docsPlainText(&docs.Document{Body: tab.DocumentTab.Body}, 0)})
	}
	if len(snapshot.Tabs) == 0 {
		snapshot.Tabs = append(snapshot.Tabs, docsFetchedTab{Title: doc.Title, Text: docsPlainText(doc, 0)})
	}
	return snapshot
}

func validDocsFetchCache(root *os.Root, name, id, account, client string) bool {
	f, err := root.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	var snapshot docsFetchedSnapshot
	if err := json.NewDecoder(io.LimitReader(f, 32<<20)).Decode(&snapshot); err != nil {
		return false
	}
	return snapshot.DocumentID == id && snapshot.Account == account && snapshot.Client == client && snapshot.FetchedAt != "" && len(snapshot.Tabs) > 0
}

func saveDocsFetchSnapshot(root *os.Root, name string, snapshot docsFetchedSnapshot) error {
	tmp := ".fetch-" + rand.Text()
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(tmp) }()
	encodeErr := json.NewEncoder(f).Encode(snapshot)
	closeErr := f.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return root.Rename(tmp, name)
}
