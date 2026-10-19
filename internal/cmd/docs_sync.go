package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type DocsSyncCmd struct {
	DocID           string `arg:"" name:"docId" help:"Existing Google Doc ID or URL"`
	Manifest        string `name:"manifest" required:"" help:"JSON file containing tabs: [{file, tab}]; paths are relative to this file" type:"path"`
	State           string `name:"state" help:"Sync state path (default: manifest path plus .state.json)" type:"path"`
	Apply           bool   `name:"apply" help:"Apply the previewed sync; default only previews"`
	CreateTabs      bool   `name:"create-tabs" help:"Create missing tabs by title; never rename or delete tabs"`
	OverwriteRemote bool   `name:"overwrite-remote" help:"Explicitly replace managed tabs edited since the previous successful sync"`
}

type docsSyncInput struct {
	File string `json:"file"`
	Tab  string `json:"tab"`
}
type docsSyncEntry struct {
	SourceHash string `json:"sourceHash"`
	RemoteHash string `json:"remoteHash"`
}
type docsSyncState struct {
	DocumentID string                   `json:"documentId"`
	Account    string                   `json:"account"`
	Client     string                   `json:"client"`
	Entries    map[string]docsSyncEntry `json:"entries"`
}
type docsSyncAction struct {
	File       string `json:"file"`
	Tab        string `json:"tab"`
	TabID      string `json:"tabId,omitempty"`
	Action     string `json:"action"`
	content    string
	sourceHash string
	remoteHash string
}

func (c *DocsSyncCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if id == "" {
		return usage("empty docId")
	}
	inputs, contents, err := readDocsSyncManifest(c.Manifest)
	if err != nil {
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
	statePath := c.State
	if statePath == "" {
		statePath = c.Manifest + ".state.json"
	}
	state, err := readDocsSyncState(statePath, id, account, client)
	if err != nil {
		return err
	}
	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	doc, err := svc.Documents.Get(id).IncludeTabsContent(true).Context(ctx).Do()
	if err != nil {
		return err
	}
	actions, err := planDocsSync(doc, inputs, contents, state, c.CreateTabs, c.OverwriteRemote)
	if err != nil {
		return err
	}
	apply := c.Apply && (flags == nil || !flags.DryRun)
	if apply {
		for i := range actions {
			action := &actions[i]
			if action.Action == "skip" {
				continue
			}
			if action.Action == "create" {
				resp, createErr := svc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{Requests: []*docs.Request{{AddDocumentTab: &docs.AddDocumentTabRequest{TabProperties: &docs.TabProperties{Title: action.Tab}}}}}).Context(ctx).Do()
				if createErr != nil {
					return fmt.Errorf("create tab %q (earlier actions may have completed): %w", action.Tab, createErr)
				}
				if len(resp.Replies) == 0 || resp.Replies[0].AddDocumentTab == nil || resp.Replies[0].AddDocumentTab.TabProperties == nil {
					return fmt.Errorf("tab creation response lacks ID; inspect the document before retrying")
				}
				action.TabID = resp.Replies[0].AddDocumentTab.TabProperties.TabId
			} else {
				latest, readErr := svc.Documents.Get(id).IncludeTabsContent(true).Context(ctx).Do()
				if readErr != nil {
					return readErr
				}
				tab, tabErr := findTab(flattenTabs(latest.Tabs), action.TabID)
				if tabErr != nil || hashDocsSyncTab(tab) != action.remoteHash {
					return fmt.Errorf("managed tab %q changed during planning; stopped before replacing it", action.Tab)
				}
			}
			writer := &DocsWriteCmd{Tab: action.TabID, Markdown: true, Replace: true, CheckOrphans: true, quiet: true}
			if err := writer.writeMarkdown(ctx, flags, id, action.content); err != nil {
				return fmt.Errorf("sync tab %q (earlier writes may have completed): %w", action.Tab, err)
			}
			updated, readErr := svc.Documents.Get(id).IncludeTabsContent(true).Context(ctx).Do()
			if readErr != nil {
				return fmt.Errorf("tab written but state readback failed: %w", readErr)
			}
			tab, tabErr := findTab(flattenTabs(updated.Tabs), action.TabID)
			if tabErr != nil {
				return tabErr
			}
			state.Entries[action.TabID] = docsSyncEntry{SourceHash: action.sourceHash, RemoteHash: hashDocsSyncTab(tab)}
			if err := saveDocsSyncState(statePath, state); err != nil {
				return fmt.Errorf("tab written but saving sync state failed: %w", err)
			}
		}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"documentId": id, "applied": apply, "actions": actions, "state": statePath})
	}
	for _, action := range actions {
		ui.FromContext(ctx).Out().Linef("%s\t%s\t%s\t%s", action.Action, action.File, action.Tab, action.TabID)
	}
	return nil
}

func readDocsSyncManifest(name string) ([]docsSyncInput, []string, error) {
	b, err := os.ReadFile(name) // #nosec G304 -- The manifest path is explicitly selected by the CLI caller.
	if err != nil {
		return nil, nil, err
	}
	var manifest struct {
		Tabs []docsSyncInput `json:"tabs"`
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		return nil, nil, usagef("invalid manifest: %v", err)
	}
	if len(manifest.Tabs) == 0 {
		return nil, nil, usage("manifest has no tabs")
	}
	contents := make([]string, len(manifest.Tabs))
	for i, input := range manifest.Tabs {
		if strings.TrimSpace(input.File) == "" || strings.TrimSpace(input.Tab) == "" {
			return nil, nil, usage("each manifest tab requires file and tab")
		}
		path := input.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(name), path)
		}
		content, readErr := os.ReadFile(path) // #nosec G304 -- Source paths come from the caller's local manifest, not Google content.
		if readErr != nil {
			return nil, nil, readErr
		}
		if strings.TrimSpace(string(content)) == "" {
			return nil, nil, usagef("empty Markdown file %s", input.File)
		}
		contents[i] = string(content)
	}
	return manifest.Tabs, contents, nil
}

func readDocsSyncState(path, id, account, client string) (docsSyncState, error) {
	var state docsSyncState
	b, err := os.ReadFile(path) // #nosec G304 -- The state path is an explicitly selected local file.
	if errors.Is(err, os.ErrNotExist) {
		return docsSyncState{DocumentID: id, Account: account, Client: client, Entries: map[string]docsSyncEntry{}}, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(b, &state); err != nil {
		return state, fmt.Errorf("invalid sync state: %w", err)
	}
	if state.DocumentID != id || state.Account != account || state.Client != client || state.Entries == nil {
		return state, fmt.Errorf("sync state belongs to a different document, account, or client")
	}
	for tab, entry := range state.Entries {
		if tab == "" || entry.SourceHash == "" || entry.RemoteHash == "" {
			return state, fmt.Errorf("invalid sync state: incomplete baseline for tab %q", tab)
		}
	}
	return state, nil
}

func planDocsSync(doc *docs.Document, inputs []docsSyncInput, contents []string, state docsSyncState, create, overwrite bool) ([]docsSyncAction, error) {
	actions := make([]docsSyncAction, 0, len(inputs))
	seen := map[string]bool{}
	tabs := flattenTabs(doc.Tabs)
	for i, input := range inputs {
		action := docsSyncAction{File: input.File, Tab: input.Tab, Action: "create", content: contents[i], sourceHash: hashDocsSyncBytes([]byte(contents[i]))}
		var matches []*docs.Tab
		for _, tab := range tabs {
			if tab.TabProperties != nil && tab.TabProperties.TabId == input.Tab {
				matches = []*docs.Tab{tab}
				break
			}
		}
		if len(matches) == 0 {
			for _, tab := range tabs {
				if tab.TabProperties != nil && tab.TabProperties.Title == input.Tab {
					matches = append(matches, tab)
				}
			}
		}
		if len(matches) > 1 {
			return nil, fmt.Errorf("ambiguous tab title %q; use its ID", input.Tab)
		}
		identity := "title:" + input.Tab
		if len(matches) == 1 {
			tab := matches[0]
			action.TabID = tab.TabProperties.TabId
			action.remoteHash = hashDocsSyncTab(tab)
			action.Action = "update"
			identity = "id:" + action.TabID
			if previous, ok := state.Entries[action.TabID]; ok {
				if previous.RemoteHash != action.remoteHash && !overwrite {
					return nil, fmt.Errorf("tab %q was edited remotely; review it or explicitly use --overwrite-remote", input.Tab)
				}
				if previous.RemoteHash == action.remoteHash && previous.SourceHash == action.sourceHash {
					action.Action = "skip"
				}
			}
		} else if !create {
			return nil, fmt.Errorf("tab %q not found; add it first or use --create-tabs", input.Tab)
		}
		if seen[identity] {
			return nil, fmt.Errorf("multiple files target the same tab %q", input.Tab)
		}
		seen[identity] = true
		actions = append(actions, action)
	}
	return actions, nil
}

func hashDocsSyncTab(tab *docs.Tab) string {
	if tab == nil {
		return ""
	}
	b, _ := json.Marshal(tab.DocumentTab)
	var content any
	_ = json.Unmarshal(b, &content)
	normalizeDocsSyncImages(content)
	b, _ = json.Marshal(content)
	if tab.DocumentTab != nil {
		var normalized docs.DocumentTab
		_ = json.Unmarshal(b, &normalized)
		b, _ = json.Marshal(&normalized)
	}
	return hashDocsSyncBytes(b)
}

func hashDocsSyncBytes(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func saveDocsSyncState(path string, state docsSyncState) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".gog-docs-sync-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	encodeErr := json.NewEncoder(f).Encode(state)
	closeErr := f.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

// ContentUri is a temporary provider URL, not a content or formatting change.
func normalizeDocsSyncImages(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "imageProperties" {
				if image, ok := child.(map[string]any); ok {
					delete(image, "contentUri")
				}
			}
			normalizeDocsSyncImages(child)
		}
	case []any:
		for _, child := range v {
			normalizeDocsSyncImages(child)
		}
	}
}
