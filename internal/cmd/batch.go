package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/docsbatch"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
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
	Service        string `name:"service" help:"Google API service: docs, slides, forms, or sheets (inferred from target)"`
	DocID          string `name:"doc" help:"Google Doc ID; choose exactly one batch target"`
	PresentationID string `name:"presentation" help:"Google Slides presentation ID; choose exactly one batch target"`
	FormID         string `name:"form" help:"Google Form ID or URL; choose exactly one batch target"`
	SpreadsheetID  string `name:"spreadsheet" help:"Google Sheets spreadsheet ID; choose exactly one batch target"`
	Name           string `name:"name" help:"Optional batch label"`
}

func (c *BatchBeginCmd) Run(ctx context.Context, flags *RootFlags) error {
	documentID := strings.TrimSpace(c.DocID)
	presentationID := strings.TrimSpace(c.PresentationID)
	formID := strings.TrimSpace(normalizeGoogleID(c.FormID))
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	targets := 0
	for _, target := range []string{documentID, presentationID, formID, spreadsheetID} {
		if target != "" {
			targets++
		}
	}
	if targets != 1 {
		return usage("provide exactly one of --doc, --presentation, --form, or --spreadsheet")
	}
	service := docsbatch.ServiceDocs
	if presentationID != "" {
		service = docsbatch.ServiceSlides
	} else if formID != "" {
		service = docsbatch.ServiceForms
	}
	if spreadsheetID != "" {
		service = docsbatch.ServiceSheets
	}
	if c.Service != "" && c.Service != service {
		return usagef("--service %s does not match the %s target", c.Service, service)
	}
	preview := map[string]any{
		"service": service,
		"name":    strings.TrimSpace(c.Name),
	}
	switch service {
	case docsbatch.ServiceDocs:
		preview["doc_id"] = documentID
	case docsbatch.ServiceSlides:
		preview["presentation_id"] = presentationID
	case docsbatch.ServiceForms:
		preview["form_id"] = formID
	case docsbatch.ServiceSheets:
		preview["spreadsheet_id"] = spreadsheetID
	}
	if err := dryRunExit(ctx, flags, "batch.begin", preview); err != nil {
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
	store, err := newDocsBatchStore(ctx)
	if err != nil {
		return err
	}
	state, err := store.Create(docsbatch.State{
		Name:           strings.TrimSpace(c.Name),
		Service:        service,
		DocumentID:     documentID,
		PresentationID: presentationID,
		FormID:         formID,
		SpreadsheetID:  spreadsheetID,
		Account:        account,
		Client:         client,
	})
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), state)
	}
	ui.FromContext(ctx).Out().Println(state.BatchID)

	return nil
}

type BatchListCmd struct{}

func (c *BatchListCmd) Run(ctx context.Context) error {
	store, err := openDocsBatchStore(ctx)
	if err != nil {
		return err
	}
	batches, err := store.List()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"batches": batches})
	}

	out := ui.FromContext(ctx).Out()
	for _, batch := range batches {
		targetID := batch.DocumentID
		switch batch.Service {
		case docsbatch.ServiceSlides:
			targetID = batch.PresentationID
		case docsbatch.ServiceForms:
			targetID = batch.FormID
		case docsbatch.ServiceSheets:
			targetID = batch.SpreadsheetID
		}
		out.Linef("%s\t%s\t%s\t%d\t%s", batch.BatchID, batch.Service, targetID, batch.Requests, batch.UpdatedAt.Format(time.RFC3339))
	}

	return nil
}

type BatchShowCmd struct {
	BatchID string `arg:"" name:"batchId" help:"Batch ID"`
}

func (c *BatchShowCmd) Run(ctx context.Context) error {
	batchID := strings.TrimSpace(c.BatchID)
	if err := validateDocsBatchIDArg(batchID); err != nil {
		return err
	}
	store, err := openDocsBatchStore(ctx)
	if err != nil {
		return err
	}
	state, err := store.Get(batchID)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"batch":   state,
		"payload": batchWirePayload(state, state.Requests),
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
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
	if err := validateDocsBatchIDArg(batchID); err != nil {
		return err
	}
	if err := dryRunExit(ctx, flags, "batch.abort", map[string]any{"batch_id": batchID}); err != nil {
		return err
	}
	store, err := newDocsBatchStore(ctx)
	if err != nil {
		return err
	}
	state, err := store.Delete(batchID)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
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
	store, err := newDocsBatchStore(ctx)
	if err != nil {
		return err
	}
	removed, err := store.Prune(c.OlderThan)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
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

const persistedBatchRequestCap = 500

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
	batchID := strings.TrimSpace(c.BatchID)
	if err := validateDocsBatchIDArg(batchID); err != nil {
		return err
	}
	if flags != nil && flags.DryRun {
		store, err := openDocsBatchStore(ctx)
		if err != nil {
			return err
		}
		state, err := store.Get(batchID)
		if err != nil {
			return err
		}
		if len(state.Requests) == 0 {
			return errors.New("batch has no requests")
		}
		if err := c.validateFormsMode(state); err != nil {
			return err
		}
		if err := validateBatchSubmission(flags, state); err != nil {
			return err
		}

		return writeDocsBatchEndResult(ctx, docsBatchEndResult{
			BatchID:  state.BatchID,
			Requests: len(state.Requests),
			Atomic:   !c.AutoSplit,
			DryRun:   true,
			Payload:  batchWirePayload(state, state.Requests),
		})
	}
	store, err := newDocsBatchStore(ctx)
	if err != nil {
		return err
	}

	var result docsBatchEndResult
	err = store.WithState(batchID, func(transaction *docsbatch.Transaction) error {
		state := transaction.State()
		if modeErr := c.validateFormsMode(state); modeErr != nil {
			return modeErr
		}
		if validationErr := validateBatchSubmission(flags, state); validationErr != nil {
			return validationErr
		}
		if len(state.Requests) == 0 {
			return errors.New("batch has no requests")
		}
		result.BatchID = state.BatchID
		result.Requests = len(state.Requests)
		result.Atomic = !c.AutoSplit
		if len(state.Requests) > persistedBatchRequestCap && !c.AutoSplit {
			return usagef("batch has %d requests; gog submits at most %d per atomic update (use --auto-split for non-atomic submission)", len(state.Requests), persistedBatchRequestCap)
		}
		if state.Service == docsbatch.ServiceSheets {
			if googleapi.ReadOnly(ctx) || (flags != nil && flags.ReadOnly) {
				return googleapi.ErrReadOnly
			}
			if confirmErr := confirmDestructive(ctx, flags, "submit Sheets batch (may delete data)"); confirmErr != nil {
				return confirmErr
			}
		}
		if c.AutoSplit {
			return c.submitSplit(ctx, transaction, state, &result)
		}

		_, submitErr := submitPersistedBatch(ctx, state, state.Requests)
		if submitErr == nil {
			result.Chunks = 1
			state.Requests = nil

			return transaction.PersistOrDelete()
		}
		if !c.ContinueOnError || !isDocsBatchBadRequest(submitErr) {
			return submitErr
		}

		return c.submitIndividually(ctx, transaction, state, &result)
	})
	if err != nil {
		return err
	}

	if err := writeDocsBatchEndResult(ctx, result); err != nil {
		return err
	}
	if result.Failed > 0 {
		return fmt.Errorf("%d of %d requests failed; batch %s retains the failed requests", result.Failed, result.Requests, result.BatchID)
	}
	return nil
}

func (c *BatchEndCmd) validateFormsMode(state *docsbatch.State) error {
	if state.Service != docsbatch.ServiceForms {
		return nil
	}
	if c.AutoSplit || c.ContinueOnError {
		return usage("Forms batches support atomic submission only; omit --auto-split and --continue-on-error")
	}
	if len(state.Requests) > persistedBatchRequestCap {
		return usagef("Forms batch has %d requests; gog submits at most %d per atomic update", len(state.Requests), persistedBatchRequestCap)
	}
	return nil
}

func (c *BatchEndCmd) submitSplit(
	ctx context.Context,
	transaction *docsbatch.Transaction,
	state *docsbatch.State,
	result *docsBatchEndResult,
) error {
	result.Atomic = false
	for len(state.Requests) > 0 {
		count := min(len(state.Requests), persistedBatchRequestCap)
		revision, err := submitPersistedBatch(ctx, state, state.Requests[:count])
		if err != nil {
			return err
		}
		result.Chunks++
		state.Requests = state.Requests[count:]
		missingRevision := false
		if len(state.Requests) > 0 && batchUsesRevisions(state.Service) {
			if revision == "" {
				missingRevision = true
			} else {
				state.RequiredRevisionID = revision
			}
		}
		if err := transaction.PersistOrDelete(); err != nil {
			return err
		}
		if missingRevision {
			return fmt.Errorf("%s response omitted the revision required to continue split submission", state.Service)
		}
	}

	return nil
}

func (c *BatchEndCmd) submitIndividually(
	ctx context.Context,
	transaction *docsbatch.Transaction,
	state *docsbatch.State,
	result *docsBatchEndResult,
) error {
	result.Atomic = false
	failed := make([]docsbatch.RequestEntry, 0)
	pending := append([]docsbatch.RequestEntry(nil), state.Requests...)
	for index := 0; len(pending) > 0; index++ {
		entry := pending[0]
		pending = pending[1:]
		revision, err := submitPersistedBatch(ctx, state, []docsbatch.RequestEntry{entry})
		missingRevision := false
		if err != nil {
			failed = append(failed, entry)
			ui.FromContext(ctx).Err().Linef("batch request %d failed: %v", index+1, err)
		} else {
			result.Chunks++
			if (len(failed) > 0 || len(pending) > 0) && batchUsesRevisions(state.Service) && revision == "" {
				missingRevision = true
			} else if revision != "" {
				state.RequiredRevisionID = revision
			}
		}

		state.Requests = append(append([]docsbatch.RequestEntry(nil), failed...), pending...)
		result.Failed = len(failed)
		if persistErr := transaction.PersistOrDelete(); persistErr != nil {
			return persistErr
		}
		if state.Service == docsbatch.ServiceSheets && err != nil && !isDocsBatchBadRequest(err) {
			return fmt.Errorf("sheets batch stopped; inspect the spreadsheet before retrying retained requests: %w", err)
		}
		if missingRevision {
			return fmt.Errorf("%s response omitted the revision required to continue individual submission", state.Service)
		}
	}

	return nil
}

func writeDocsBatchEndResult(ctx context.Context, result docsBatchEndResult) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
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

func validateDocsBatchTarget(ctx context.Context, flags *RootFlags, batchID, documentID string) error {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return nil
	}
	if err := validateDocsBatchIDArg(batchID); err != nil {
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
	store, err := openDocsBatchStore(ctx)
	if err != nil {
		return err
	}
	state, err := store.Get(batchID)
	if err != nil {
		return err
	}

	return docsbatch.ValidateIdentity(state, docsbatch.Identity{
		Service:    docsbatch.ServiceDocs,
		DocumentID: strings.TrimSpace(documentID),
		Account:    account,
		Client:     client,
	})
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
	if err := validateDocsBatchIDArg(batchID); err != nil {
		return true, err
	}
	if len(requests) == 0 {
		return true, errors.New("no Docs requests to append")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return true, err
	}
	client, err := resolveClientForEmail(ctx, account, flags)
	if err != nil {
		return true, err
	}
	store, err := newDocsBatchStore(ctx)
	if err != nil {
		return true, err
	}
	rawRequests, err := marshalDocsBatchRequests(requests)
	if err != nil {
		return true, err
	}
	total, err := store.Append(docsbatch.AppendOptions{
		BatchID: batchID,
		Command: command,
		Identity: docsbatch.Identity{
			Service:    docsbatch.ServiceDocs,
			DocumentID: documentID,
			Account:    account,
			Client:     client,
		},
		RevisionID:   revisionID,
		Requests:     rawRequests,
		RequireEmpty: requireEmpty,
	})
	if err != nil {
		return true, err
	}
	if outfmt.IsJSON(ctx) {
		err = outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
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

func validateDocsBatchIDArg(batchID string) error {
	return newUsageError(docsbatch.ValidateID(batchID))
}
