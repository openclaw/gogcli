package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type BatchCmd struct {
	Begin BatchBeginCmd `cmd:"" help:"Begin a persisted request batch"`
	List  BatchListCmd  `cmd:"" aliases:"ls" help:"List persisted request batches"`
	Show  BatchShowCmd  `cmd:"" help:"Show a persisted request batch"`
	End   BatchEndCmd   `cmd:"" aliases:"submit" help:"Submit and remove a request batch"`
	Abort BatchAbortCmd `cmd:"" aliases:"rm,delete" help:"Delete a request batch without submitting"`
	Prune BatchPruneCmd `cmd:"" help:"Delete stale request batches"`
}

type BatchBeginCmd struct {
	Service string `name:"service" help:"Google API service" enum:"docs" default:"docs"`
	DocID   string `name:"doc" required:"" help:"Google Doc ID"`
	Name    string `name:"name" help:"Optional batch label"`
}

func (c *BatchBeginCmd) Run(ctx context.Context, flags *RootFlags) error {
	documentID := strings.TrimSpace(c.DocID)
	if documentID == "" {
		return usage("empty --doc")
	}
	if err := dryRunExit(ctx, flags, "batch.begin", map[string]any{
		"service": c.Service,
		"doc_id":  documentID,
		"name":    strings.TrimSpace(c.Name),
	}); err != nil {
		return err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := resolveClientForEmail(account, flags)
	if err != nil {
		return err
	}
	store, err := newDocsBatchStore()
	if err != nil {
		return err
	}
	state, err := store.create(docsBatchState{
		Name:       strings.TrimSpace(c.Name),
		Service:    c.Service,
		DocumentID: documentID,
		Account:    account,
		Client:     client,
	})
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, state)
	}
	ui.FromContext(ctx).Out().Println(state.BatchID)

	return nil
}

type BatchListCmd struct{}

func (c *BatchListCmd) Run(ctx context.Context) error {
	store, err := newDocsBatchStore()
	if err != nil {
		return err
	}
	batches, err := store.list()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"batches": batches})
	}

	out := ui.FromContext(ctx).Out()
	for _, batch := range batches {
		out.Linef("%s\t%s\t%s\t%d\t%s", batch.BatchID, batch.Service, batch.DocumentID, batch.Requests, batch.UpdatedAt.Format(time.RFC3339))
	}

	return nil
}

type BatchShowCmd struct {
	BatchID string `arg:"" name:"batchId" help:"Batch ID"`
}

func (c *BatchShowCmd) Run(ctx context.Context) error {
	store, err := newDocsBatchStore()
	if err != nil {
		return err
	}
	state, err := store.get(strings.TrimSpace(c.BatchID))
	if err != nil {
		return err
	}
	payload := map[string]any{
		"batch":   state,
		"payload": docsBatchWirePayload(state, state.Requests),
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	ui.FromContext(ctx).Out().Println(string(data))

	return nil
}

type BatchAbortCmd struct {
	BatchID string `arg:"" name:"batchId" help:"Batch ID"`
}

func (c *BatchAbortCmd) Run(ctx context.Context, flags *RootFlags) error {
	batchID := strings.TrimSpace(c.BatchID)
	if err := validateDocsBatchID(batchID); err != nil {
		return err
	}
	if err := dryRunExit(ctx, flags, "batch.abort", map[string]any{"batch_id": batchID}); err != nil {
		return err
	}
	store, err := newDocsBatchStore()
	if err != nil {
		return err
	}
	state, err := store.delete(batchID)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"aborted":  true,
			"batch_id": state.BatchID,
			"requests": len(state.Requests),
		})
	}
	ui.FromContext(ctx).Out().Linef("aborted\t%s\t%d", state.BatchID, len(state.Requests))

	return nil
}

type BatchPruneCmd struct {
	OlderThan time.Duration `name:"older-than" help:"Delete batches not updated within this duration" default:"72h"`
}

func (c *BatchPruneCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.OlderThan <= 0 {
		return usage("--older-than must be greater than zero")
	}
	if err := dryRunExit(ctx, flags, "batch.prune", map[string]any{"older_than": c.OlderThan.String()}); err != nil {
		return err
	}
	store, err := newDocsBatchStore()
	if err != nil {
		return err
	}
	removed, err := store.prune(c.OlderThan)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"pruned":  len(removed),
			"batches": removed,
		})
	}
	ui.FromContext(ctx).Out().Linef("pruned\t%d", len(removed))

	return nil
}

type BatchEndCmd struct {
	BatchID         string `arg:"" name:"batchId" help:"Batch ID"`
	ContinueOnError bool   `name:"continue-on-error" help:"After an atomic validation failure, submit requests individually and retain failures"`
	AutoSplit       bool   `name:"auto-split" help:"Submit batches over 500 requests as ordered chunks (non-atomic)"`
}

type docsBatchEndResult struct {
	BatchID  string `json:"batch_id"`
	Requests int    `json:"requests"`
	Chunks   int    `json:"chunks"`
	Failed   int    `json:"failed"`
	Atomic   bool   `json:"atomic"`
	DryRun   bool   `json:"dry_run,omitempty"`
	Payload  any    `json:"payload,omitempty"`
}

func (c *BatchEndCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.ContinueOnError && c.AutoSplit {
		return usage("--continue-on-error and --auto-split are mutually exclusive")
	}
	store, err := newDocsBatchStore()
	if err != nil {
		return err
	}

	var result docsBatchEndResult
	err = store.withLockedState(strings.TrimSpace(c.BatchID), func(state *docsBatchState) error {
		if len(state.Requests) == 0 {
			return errors.New("batch has no requests")
		}
		result.BatchID = state.BatchID
		result.Requests = len(state.Requests)
		result.Atomic = !c.AutoSplit
		if flags != nil && flags.DryRun {
			result.DryRun = true
			result.Payload = docsBatchWirePayload(state, state.Requests)
			return nil
		}
		if len(state.Requests) > docsBatchUpdateRequestCap && !c.AutoSplit {
			return usagef("batch has %d requests; Docs allows at most %d per atomic update (use --auto-split for non-atomic submission)", len(state.Requests), docsBatchUpdateRequestCap)
		}
		if c.AutoSplit {
			return c.submitSplit(ctx, store, state, &result)
		}

		_, submitErr := submitDocsBatch(ctx, state, state.Requests)
		if submitErr == nil {
			result.Chunks = 1
			state.Requests = nil
			return store.persistOrDeleteUnlocked(state)
		}
		if !c.ContinueOnError || !isDocsBatchBadRequest(submitErr) {
			return submitErr
		}

		return c.submitIndividually(ctx, store, state, &result)
	})
	if err != nil {
		return err
	}

	return writeDocsBatchEndResult(ctx, result)
}

func (c *BatchEndCmd) submitSplit(ctx context.Context, store *docsBatchStore, state *docsBatchState, result *docsBatchEndResult) error {
	result.Atomic = false
	for len(state.Requests) > 0 {
		count := min(len(state.Requests), docsBatchUpdateRequestCap)
		response, err := submitDocsBatch(ctx, state, state.Requests[:count])
		if err != nil {
			return err
		}
		result.Chunks++
		state.Requests = state.Requests[count:]
		if len(state.Requests) > 0 {
			revision := docsBatchResponseRevision(response)
			if revision == "" {
				return errors.New("docs response omitted the revision required to continue split submission")
			}
			state.RequiredRevisionID = revision
		}
		if err := store.persistOrDeleteUnlocked(state); err != nil {
			return err
		}
	}

	return nil
}

func (c *BatchEndCmd) submitIndividually(ctx context.Context, store *docsBatchStore, state *docsBatchState, result *docsBatchEndResult) error {
	result.Atomic = false
	failed := make([]docsBatchRequestEntry, 0)
	for index, entry := range state.Requests {
		response, err := submitDocsBatch(ctx, state, []docsBatchRequestEntry{entry})
		if err != nil {
			failed = append(failed, entry)
			ui.FromContext(ctx).Err().Linef("batch request %d failed: %v", index+1, err)
			continue
		}
		result.Chunks++
		revision := docsBatchResponseRevision(response)
		if index < len(state.Requests)-1 && revision == "" {
			return errors.New("docs response omitted the revision required to continue individual submission")
		}
		if revision != "" {
			state.RequiredRevisionID = revision
		}
	}
	state.Requests = failed
	result.Failed = len(failed)

	return store.persistOrDeleteUnlocked(state)
}

func writeDocsBatchEndResult(ctx context.Context, result docsBatchEndResult) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, result)
	}
	out := ui.FromContext(ctx).Out()
	out.Linef("batch_id\t%s", result.BatchID)
	out.Linef("requests\t%d", result.Requests)
	out.Linef("chunks\t%d", result.Chunks)
	out.Linef("failed\t%d", result.Failed)
	out.Linef("atomic\t%t", result.Atomic)
	if result.DryRun {
		data, err := json.Marshal(result.Payload)
		if err != nil {
			return fmt.Errorf("encode dry-run payload: %w", err)
		}
		out.Linef("dry_run\ttrue")
		out.Linef("payload_json\t%s", data)
	}

	return nil
}

func validateDocsBatchTarget(flags *RootFlags, batchID, documentID string) error {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return nil
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := resolveClientForEmail(account, flags)
	if err != nil {
		return err
	}
	store, err := newDocsBatchStore()
	if err != nil {
		return err
	}
	state, err := store.get(batchID)
	if err != nil {
		return err
	}

	return validateDocsBatchIdentity(state, strings.TrimSpace(documentID), account, client)
}

func captureDocsBatchRevision(ctx context.Context, svc *docs.Service, batchID, documentID string) (string, error) {
	if strings.TrimSpace(batchID) == "" {
		return "", nil
	}
	document, err := svc.Documents.Get(documentID).
		Fields("revisionId").
		Context(ctx).
		Do()
	if err != nil {
		return "", err
	}
	if document == nil || strings.TrimSpace(document.RevisionId) == "" {
		return "", errors.New("docs response omitted document revision")
	}

	return document.RevisionId, nil
}

func queueDocsBatchRequests(ctx context.Context, flags *RootFlags, batchID, documentID, command, revisionID string, requests []*docs.Request, requireEmpty bool) (bool, error) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return false, nil
	}
	if len(requests) == 0 {
		return true, errors.New("no Docs requests to append")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return true, err
	}
	client, err := resolveClientForEmail(account, flags)
	if err != nil {
		return true, err
	}
	store, err := newDocsBatchStore()
	if err != nil {
		return true, err
	}
	total, err := store.appendRequests(batchID, command, documentID, account, client, revisionID, requests, requireEmpty)
	if err != nil {
		return true, err
	}
	if outfmt.IsJSON(ctx) {
		err = outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"batch_id": batchID,
			"queued":   len(requests),
			"requests": total,
		})
	} else {
		out := ui.FromContext(ctx).Out()
		out.Linef("batch_id\t%s", batchID)
		out.Linef("queued\t%d", len(requests))
		out.Linef("requests\t%d", total)
	}

	return true, err
}
