package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	gapi "google.golang.org/api/googleapi"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/docsbatch"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

const sheetsBatchMetadataFields = "sheets(properties(sheetId,title,index,gridProperties(rowCount,columnCount)),basicFilter(range),charts(chartId),bandedRanges(bandedRangeId),tables(tableId,name,range)),namedRanges(namedRangeId,name,range)"

var errSheetsMutationQueued = errors.New("sheets mutation queued")

type sheetsBatchContextKey struct{}

type sheetsBatchContext struct {
	id       string
	command  string
	identity docsbatch.Identity
	store    *docsbatch.Repository
}

func sheetsBatchFromContext(ctx context.Context) *sheetsBatchContext {
	if ctx == nil {
		return nil
	}
	batch, _ := ctx.Value(sheetsBatchContextKey{}).(*sheetsBatchContext)
	return batch
}

func validateSheetsBatchState(state *docsbatch.State) error {
	if state.Service != docsbatch.ServiceSheets || strings.TrimSpace(state.SpreadsheetID) == "" || state.DocumentID != "" || state.PresentationID != "" || state.FormID != "" {
		return usage("invalid stored Sheets batch target")
	}
	if strings.TrimSpace(state.Account) == "" || strings.TrimSpace(state.Client) == "" {
		return usage("Sheets batch is missing its bound account or OAuth client")
	}
	if state.RequiredRevisionID != "" {
		return usage("Sheets batches do not support revision locking")
	}
	return nil
}

func prepareSheetsBatch(ctx context.Context, flags *RootFlags, batchID, spreadsheetID, command string) (context.Context, error) {
	if batchID != "" && strings.TrimSpace(batchID) == "" {
		return nil, usage("--batch requires a non-empty batch ID; omit --batch to submit immediately")
	}
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return ctx, nil
	}
	if err := validateDocsBatchIDArg(batchID); err != nil {
		return nil, err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return nil, err
	}
	client, err := resolveClientForEmail(ctx, account, flags)
	if err != nil {
		return nil, err
	}
	store, err := openDocsBatchStore(ctx)
	if err != nil {
		return nil, err
	}
	state, err := store.Get(batchID)
	if err != nil {
		return nil, err
	}
	identity := docsbatch.Identity{Service: docsbatch.ServiceSheets, SpreadsheetID: spreadsheetID, Account: account, Client: client}
	if err := validateSheetsBatchIdentity(state, identity); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, sheetsBatchContextKey{}, &sheetsBatchContext{id: batchID, command: command, identity: identity, store: store}), nil
}

func validateSheetsBatchIdentity(state *docsbatch.State, identity docsbatch.Identity) error {
	if err := docsbatch.ValidateIdentity(state, identity); err != nil {
		return err
	}
	return validateSheetsBatchState(state)
}

// Queued planners read only their captured metadata; constructing a live SDK
// client here would unnecessarily require credentials for an already captured batch.
func sheetsMutationService(ctx context.Context, account string) (*sheets.Service, error) {
	if sheetsBatchFromContext(ctx) != nil {
		return nil, nil //nolint:nilnil // Queued planners use captured metadata, not an authenticated SDK service.
	}
	return sheetsService(ctx, account)
}

func requireSheetsMutationService(ctx context.Context, flags *RootFlags) (string, *sheets.Service, error) {
	return requireGoogleService(ctx, flags, sheetsMutationService)
}

func prepareSheetsMutation(ctx context.Context, flags *RootFlags, batchID, spreadsheetID, command string) (context.Context, *sheets.Service, error) {
	ctx, err := prepareSheetsBatch(ctx, flags, batchID, spreadsheetID, command)
	if err != nil {
		return nil, nil, err
	}
	_, svc, err := requireSheetsMutationService(ctx, flags)
	return ctx, svc, err
}

func confirmSheetsMutation(ctx context.Context, flags *RootFlags, batchID, action string) error {
	if batchID != "" {
		return nil // batch end confirms the complete persisted mutation.
	}
	return confirmDestructive(ctx, flags, action)
}

func sheetsMutationDryRun(ctx context.Context, flags *RootFlags, batchID, command string, payload map[string]any) error {
	if strings.TrimSpace(batchID) != "" {
		if err := validateDocsBatchIDArg(strings.TrimSpace(batchID)); err != nil {
			return err
		}
		payload["batch_id"] = strings.TrimSpace(batchID)
	}
	return dryRunExit(ctx, flags, command, payload)
}

func fetchSheetsMutationMetadata(ctx context.Context, svc *sheets.Service, spreadsheetID string, fields gapi.Field) (*sheets.Spreadsheet, error) {
	if batch := sheetsBatchFromContext(ctx); batch != nil {
		if batch.identity.SpreadsheetID != spreadsheetID {
			return nil, usage("metadata target does not match the Sheets batch")
		}
		return batch.metadata(ctx)
	}
	call := svc.Spreadsheets.Get(spreadsheetID).Fields(fields)
	if ctx != nil {
		call = call.Context(ctx)
	}
	return call.Do()
}

func (b *sheetsBatchContext) metadata(ctx context.Context) (*sheets.Spreadsheet, error) {
	state, loadErr := b.store.Get(b.id)
	if loadErr != nil {
		return nil, loadErr
	}
	if err := validateSheetsBatchIdentity(state, b.identity); err != nil {
		return nil, err
	}
	raw := state.SheetsBaseMetadata
	if len(raw) == 0 {
		captureErr := b.store.WithState(b.id, func(transaction *docsbatch.Transaction) error {
			state := transaction.State()
			if err := validateSheetsBatchIdentity(state, b.identity); err != nil {
				return err
			}
			if len(state.SheetsBaseMetadata) > 0 {
				raw = state.SheetsBaseMetadata
				return nil
			}
			boundCtx := authclient.WithClient(ctx, state.Client)
			client, err := sheetsHTTPClient(boundCtx, state.Account)
			if err != nil {
				return err
			}
			metadata, err := readRawObject(boundCtx, client,
				"https://sheets.googleapis.com/v4/spreadsheets/"+url.PathEscape(state.SpreadsheetID),
				url.Values{"fields": {sheetsBatchMetadataFields}}, "spreadsheet metadata")
			if err != nil {
				return err
			}
			raw, err = json.Marshal(metadata)
			if err != nil {
				return fmt.Errorf("encode Sheets base metadata: %w", err)
			}
			if _, decodeErr := decodeSheetsBaseMetadata(raw); decodeErr != nil {
				return decodeErr
			}
			state.SheetsBaseMetadata = raw
			return transaction.Persist()
		})
		if captureErr != nil {
			return nil, captureErr
		}
	}
	return decodeSheetsBaseMetadata(raw)
}

func decodeSheetsBaseMetadata(raw json.RawMessage) (*sheets.Spreadsheet, error) {
	var metadata *sheets.Spreadsheet
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("decode Sheets base metadata: %w", err)
	}
	if metadata == nil {
		return nil, errors.New("stored Sheets base metadata is empty")
	}
	return metadata, nil
}

func queueSheetsBatchRequests(ctx context.Context, requests []*sheets.Request) (bool, error) {
	if sheetsBatchFromContext(ctx) == nil {
		return false, nil
	}
	raw := make([]json.RawMessage, 0, len(requests))
	for _, request := range requests {
		encoded, err := json.Marshal(request)
		if err != nil {
			return true, fmt.Errorf("encode queued Sheets request: %w", err)
		}
		raw = append(raw, encoded)
	}
	return true, queueSheetsRawBatchRequests(ctx, raw)
}

func queueSheetsRawBatchRequests(ctx context.Context, requests []json.RawMessage) error {
	batch := sheetsBatchFromContext(ctx)
	if batch == nil {
		return errors.New("missing Sheets batch context")
	}
	if len(requests) == 0 {
		return errors.New("no Sheets requests to append")
	}
	total, err := batch.store.Append(docsbatch.AppendOptions{
		BatchID: batch.id, Command: batch.command, Identity: batch.identity, Requests: requests,
	})
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"batch_id": batch.id, "spreadsheetId": batch.identity.SpreadsheetID, "queued": len(requests), "requests": total,
		})
	}
	out := ui.FromContext(ctx).Out()
	out.Linef("batch_id\t%s", batch.id)
	out.Linef("queued\t%d", len(requests))
	out.Linef("requests\t%d", total)
	return nil
}

func sheetsBatchWirePayload(entries []docsbatch.RequestEntry) sheetsBatchRequestBody {
	requests := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		requests = append(requests, entry.Request)
	}
	return sheetsBatchRequestBody{Requests: requests}
}

func submitSheetsBatch(ctx context.Context, state *docsbatch.State, entries []docsbatch.RequestEntry) error {
	ctx = authclient.WithClient(ctx, state.Client)
	_, err := submitSheetsStructuralRequests(ctx, state.Account, state.SpreadsheetID, sheetsBatchWirePayload(entries))
	return err
}
