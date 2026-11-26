package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/docsedit"
	"github.com/openclaw/gogcli/internal/outfmt"
)

const docsSuggestionsInline = "SUGGESTIONS_INLINE"

type DocsSuggestFlags struct {
	Suggest  bool   `name:"suggest" help:"Propose this change instead of editing directly"`
	Revision string `name:"revision" help:"Require this Docs revision in suggestion mode (mandatory for explicit offsets)"`
}

func (s DocsSuggestFlags) validate(incompatible bool) error {
	if !s.Suggest && s.Revision != "" {
		return usage("--revision requires --suggest")
	}
	if s.Suggest && incompatible {
		return usage("--suggest is incompatible with the requested options; see the Docs suggestion-mode guide")
	}
	if s.Revision != "" && strings.TrimSpace(s.Revision) == "" {
		return usage("--revision must not be blank")
	}
	return nil
}

func (s DocsSuggestFlags) addPlan(payload map[string]any) {
	if s.Suggest {
		payload["writeMode"] = "SUGGEST"
		if s.Revision != "" {
			payload["requiredRevisionId"] = s.Revision
		}
	}
}

func (s DocsSuggestFlags) validatePlacement(p docsedit.Placement) error {
	if s.Suggest && s.Revision == "" && (p.Kind == docsedit.PlacementIndex || p.Kind == docsedit.PlacementRange) {
		return usage("--revision from the same inline read is required for explicit suggestion offsets")
	}
	return nil
}

func (s DocsSuggestFlags) load(ctx context.Context, svc *docs.Service, id, tab, segment string) (*docsLoadedTarget, error) {
	loaded, err := loadDocsTargetSegmentView(ctx, svc, id, tab, segment, docsSuggestionsInline)
	if err != nil {
		return nil, err
	}
	if s.Revision != "" && s.Revision != loaded.full.RevisionId {
		return nil, usage("document revision changed; reread it in inline suggestion view and recalculate offsets")
	}
	return loaded, nil
}

func findDocsSuggestionTab(tabs []*docs.Tab, query string) (*docs.Tab, error) {
	query = strings.TrimSpace(query)
	selected, err := findTab(tabs, query)
	if err != nil {
		return nil, err
	}
	if selected.TabProperties == nil || selected.TabProperties.TabId == "" {
		return nil, usagef("tab has no ID: %q", query)
	}
	if selected.TabProperties.TabId != query {
		for _, tab := range tabs {
			if tab != selected && tab.TabProperties != nil && strings.EqualFold(tab.TabProperties.Title, query) {
				return nil, usagef("ambiguous tab title %q; use an exact tab ID", query)
			}
		}
	}
	return selected, nil
}

func (s DocsSuggestFlags) resolve(ctx context.Context, svc *docs.Service, id, tab, segment string, placement docsedit.Placement) (docsResolvedPlacement, error) {
	if !s.Suggest {
		return resolveDocsPlacementTarget(ctx, svc, id, tab, segment, placement)
	}
	loaded, err := s.load(ctx, svc, id, tab, segment)
	if err != nil {
		return docsResolvedPlacement{}, err
	}
	facts := docsedit.PlacementFacts{
		TabID: loaded.tabID, SegmentID: loaded.segmentID, SegmentKind: loaded.segmentKind,
		EndIndex: docsDocumentEndIndex(loaded.target),
	}
	if placement.Kind == docsedit.PlacementAnchor {
		anchor, anchorErr := resolveDocsAnchorInTarget(loaded, docsAtAnchorFlags{
			At: placement.Anchor.Text, Occurrence: placement.Anchor.Occurrence, MatchCase: placement.Anchor.MatchCase,
		})
		if anchorErr != nil {
			return docsResolvedPlacement{}, anchorErr
		}
		facts.Anchor = &anchor.Match
	}
	resolved, err := docsedit.ResolvePlacement(placement, facts)
	if err != nil {
		return docsResolvedPlacement{}, err
	}
	if offsetErr := validateDocsSuggestionOffsets(resolved, loaded.target, facts.EndIndex); offsetErr != nil {
		return docsResolvedPlacement{}, offsetErr
	}
	resolved.RequiredRevisionID = loaded.full.RevisionId
	nextIndex := resolved.Index + 1
	if splitsDocsSurrogate(loaded.target.Body.Content, nextIndex) {
		nextIndex++
	}
	return docsResolvedPlacement{ResolvedPlacement: resolved, Document: loaded.full, NextIndex: nextIndex}, nil
}

func validateDocsSuggestionOffsets(resolved docsedit.ResolvedPlacement, target *docs.Document, end int64) error {
	if target == nil || target.Body == nil {
		return usage("inline snapshot did not contain the suggestion target")
	}
	minimum := int64(1)
	if resolved.SegmentID != "" {
		minimum = 0
	}
	last := resolved.Index
	if resolved.Range != nil {
		last = resolved.Range.End
	}
	if resolved.Index < minimum || last >= end {
		return usage("suggestion offsets are outside the editable range of the inline snapshot")
	}
	if splitsDocsSurrogate(target.Body.Content, resolved.Index) || (last != resolved.Index && splitsDocsSurrogate(target.Body.Content, last)) {
		return usage("suggestion offset splits a UTF-16 surrogate pair")
	}
	return nil
}

func splitsDocsSurrogate(content []*docs.StructuralElement, index int64) bool {
	for _, element := range content {
		if element == nil || index <= element.StartIndex || index >= element.EndIndex {
			continue
		}
		switch {
		case element.Paragraph != nil:
			for _, run := range element.Paragraph.Elements {
				if run != nil && run.TextRun != nil && index > run.StartIndex && index < run.EndIndex && splitsDocsTextRun(run, index) {
					return true
				}
			}
		case element.Table != nil:
			if splitsDocsTableSurrogate(element.Table, index) {
				return true
			}
		case element.TableOfContents != nil:
			if splitsDocsSurrogate(element.TableOfContents.Content, index) {
				return true
			}
		}
	}
	return false
}

func splitsDocsTableSurrogate(table *docs.Table, index int64) bool {
	for _, row := range table.TableRows {
		if row == nil {
			continue
		}
		for _, cell := range row.TableCells {
			if cell != nil && splitsDocsSurrogate(cell.Content, index) {
				return true
			}
		}
	}
	return false
}

func splitsDocsTextRun(run *docs.ParagraphElement, index int64) bool {
	offset := run.StartIndex
	for _, r := range run.TextRun.Content {
		offset++
		if r > 0xFFFF {
			if offset == index {
				return true
			}
			offset++
		}
		if offset >= index {
			return false
		}
	}
	return false
}

func runDocsSuggestions(ctx context.Context, flags *RootFlags, action, id, revision string, requests []*docs.Request) error {
	if id == "" {
		return usage("empty docId")
	}
	if strings.TrimSpace(revision) == "" {
		return usage("--revision must not be empty")
	}
	if err := dryRunExit(ctx, flags, action, map[string]any{
		"documentId": id,
		"body":       docsSuggestionRequest(revision, requests),
	}); err != nil {
		return err
	}
	return submitDocsSuggestions(ctx, flags, id, revision, requests)
}

func docsSuggestionRequest(revision string, requests []*docs.Request) *docs.BatchUpdateDocumentRequest {
	return &docs.BatchUpdateDocumentRequest{
		WriteControl: &docs.WriteControl{WriteMode: "SUGGEST", RequiredRevisionId: revision},
		Requests:     requests,
	}
}

func submitDocsSuggestions(ctx context.Context, flags *RootFlags, id, revision string, requests []*docs.Request) error {
	if strings.TrimSpace(revision) == "" {
		return usage("a document revision is required to suggest changes")
	}
	if len(requests) == 0 || len(requests) > docsBatchUpdateRequestCap {
		return usagef("suggestion writes must contain 1 to %d requests in one batch", docsBatchUpdateRequestCap)
	}

	// One explicit SUGGEST write. Never enqueue, rebase, retry an uncertain POST,
	// or call the normal editing path. A thread-save failure can still leave
	// changes in the Docs model, so report it as unconfirmed, not rolled back.
	return postDocsSuggestions(ctx, flags, id, len(requests), docsSuggestionRequest(revision, requests))
}

func postDocsSuggestions(ctx context.Context, flags *RootFlags, id string, count int, payload any) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := docsHTTPClient(ctx, account)
	if err != nil {
		return err
	}
	var resp docs.BatchUpdateDocumentResponse
	err = postBatchUpdate(ctx, client, "Docs suggestion", "https://docs.googleapis.com/v1/documents/"+url.PathEscape(id)+":batchUpdate", payload, &resp)
	return writeDocsSuggestionResult(ctx, id, count, &resp, err)
}

func writeDocsSuggestionResult(ctx context.Context, id string, count int, resp *docs.BatchUpdateDocumentResponse, err error) error {
	if err != nil {
		if isDocsNotFound(err) {
			return docsNotFoundError(id, err)
		}
		return fmt.Errorf("suggestion request failed; inspect the document before retrying: %w", err)
	}
	ids := confirmedDocsSuggestionIDs(resp, id, count)
	if len(ids) == 0 {
		if resp != nil && resp.DocumentId == id && resp.CommentUpdateState != "" {
			var reported []string
			for _, row := range resp.SuggestionResponses {
				if row != nil {
					reported = append(reported, row.CreatedSuggestionIds...)
					reported = append(reported, row.UpdatedSummarySuggestionIds...)
				}
			}
			return fmt.Errorf("the response did not confirm creation or update of every suggestion (thread state %q; unconfirmed IDs %q); inspect the document before retrying", resp.CommentUpdateState, sortedUniqueStrings(reported))
		}
		return fmt.Errorf("the response did not confirm creation or update of every suggestion; inspect the document before retrying")
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"documentId": id, "suggestionIds": ids})
	}
	for _, suggestionID := range ids {
		if outfmt.IsPlain(ctx) {
			fmt.Fprintln(stdoutWriter(ctx), suggestionID)
		} else {
			fmt.Fprintf(stdoutWriter(ctx), "Pending suggestion: %s\n", suggestionID)
		}
	}
	return nil
}

func confirmedDocsSuggestionIDs(resp *docs.BatchUpdateDocumentResponse, id string, count int) []string {
	if resp == nil || resp.DocumentId != id || resp.CommentUpdateState != "ALL_SAVED" || len(resp.SuggestionResponses) != count {
		return nil
	}
	var ids []string
	for _, row := range resp.SuggestionResponses {
		if row == nil || len(row.AcceptedSuggestionIds)+len(row.RejectedSuggestionIds)+len(row.DeletedSuggestionIds) > 0 {
			return nil
		}
		confirmed := mergeDocsSuggestionIDs(row.CreatedSuggestionIds, row.UpdatedSummarySuggestionIds)
		if len(confirmed) == 0 {
			return nil
		}
		ids = append(ids, confirmed...)
	}
	return sortedUniqueStrings(ids)
}
