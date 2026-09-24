package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	formsapi "google.golang.org/api/forms/v1"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/docsbatch"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

var formsBatchBaseURL = "https://forms.googleapis.com/v1"

type formsBatchWireBody struct {
	Requests     []json.RawMessage      `json:"requests"`
	WriteControl *formsapi.WriteControl `json:"writeControl"`
}

func formsBatchWirePayload(state *docsbatch.State, entries []docsbatch.RequestEntry) formsBatchWireBody {
	requests := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		requests = append(requests, entry.Request)
	}
	return formsBatchWireBody{Requests: requests, WriteControl: &formsapi.WriteControl{RequiredRevisionId: state.RequiredRevisionID}}
}

func submitFormsBatch(ctx context.Context, state *docsbatch.State, entries []docsbatch.RequestEntry) (string, error) {
	if googleapi.ReadOnly(ctx) {
		return "", googleapi.ErrReadOnly
	}
	ctx = authclient.WithClient(ctx, state.Client)
	client, err := formsHTTPClient(ctx, state.Account)
	if err != nil {
		return "", err
	}
	var result formsapi.BatchUpdateFormResponse
	endpoint := formsBatchBaseURL + "/forms/" + url.PathEscape(state.FormID) + ":batchUpdate"
	if err := postBatchUpdate(ctx, client, "Forms", endpoint, formsBatchWirePayload(state, entries), &result); err != nil {
		return "", err
	}
	if result.WriteControl == nil {
		return "", nil
	}
	return strings.TrimSpace(result.WriteControl.RequiredRevisionId), nil
}

func validateOptionalFormsBatch(batchID string) error {
	if batchID = strings.TrimSpace(batchID); batchID != "" {
		return validateDocsBatchIDArg(batchID)
	}
	return nil
}

func formsBatchPreview(batchID string, payload map[string]any) map[string]any {
	if batchID = strings.TrimSpace(batchID); batchID != "" {
		payload["batch_id"] = batchID
	}
	return payload
}

func queueFormsBatchRequests(ctx context.Context, flags *RootFlags, batchID, formID, command string, build func(int) ([]*formsapi.Request, error)) (bool, error) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return false, nil
	}
	if err := validateDocsBatchIDArg(batchID); err != nil {
		return true, err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return true, err
	}
	client, err := resolveClientForEmail(ctx, account, flags)
	if err != nil {
		return true, err
	}
	store, err := openDocsBatchStore(ctx)
	if err != nil {
		return true, err
	}
	identity := docsbatch.Identity{Service: docsbatch.ServiceForms, FormID: formID, Account: account, Client: client}
	var queued, total int
	err = store.WithState(batchID, func(transaction *docsbatch.Transaction) error {
		state := transaction.State()
		if identityErr := docsbatch.ValidateIdentity(state, identity); identityErr != nil {
			return identityErr
		}
		if state.RequiredRevisionID == "" && state.InitialFormItems == nil && len(state.Requests) == 0 {
			svc, serviceErr := formsService(ctx, account)
			if serviceErr != nil {
				return serviceErr
			}
			form, getErr := svc.Forms.Get(formID).Fields("revisionId,items(itemId)").Context(ctx).Do()
			if getErr != nil {
				return getErr
			}
			if form == nil || strings.TrimSpace(form.RevisionId) == "" {
				return errors.New("forms response omitted form revision; editor access is required")
			}
			state.RequiredRevisionID = form.RevisionId
			count := len(form.Items)
			state.InitialFormItems = &count
		}
		if state.InitialFormItems == nil || *state.InitialFormItems < 0 || strings.TrimSpace(state.RequiredRevisionID) == "" {
			return errors.New("forms batch is missing its initial item count or revision")
		}
		count := *state.InitialFormItems
		for _, entry := range state.Requests {
			var request formsapi.Request
			if decodeErr := json.Unmarshal(entry.Request, &request); decodeErr != nil {
				return fmt.Errorf("decode queued Forms request: %w", decodeErr)
			}
			count, err = formsBatchItemCount(count, &request)
			if err != nil {
				return err
			}
		}
		requests, buildErr := build(count)
		if buildErr != nil {
			return buildErr
		}
		if len(requests) == 0 {
			return errors.New("no Forms requests to append")
		}
		rawRequests := make([]json.RawMessage, 0, len(requests))
		for _, request := range requests {
			count, err = formsBatchItemCount(count, request)
			if err != nil {
				return err
			}
			raw, marshalErr := json.Marshal(request)
			if marshalErr != nil {
				return fmt.Errorf("encode Forms request: %w", marshalErr)
			}
			rawRequests = append(rawRequests, raw)
		}
		queued = len(requests)
		total, err = transaction.Append(docsbatch.AppendOptions{
			BatchID: batchID, Command: command, Identity: identity, RevisionID: state.RequiredRevisionID, Requests: rawRequests,
		})
		return err
	})
	if err != nil {
		return true, err
	}
	if outfmt.IsJSON(ctx) {
		return true, outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"batch_id": batchID, "form_id": formID, "queued": queued, "requests": total,
		})
	}
	out := ui.FromContext(ctx).Out()
	out.Linef("batch_id\t%s", batchID)
	out.Linef("queued\t%d", queued)
	out.Linef("requests\t%d", total)
	return true, nil
}

// Only item counts are projected; Google resolves ordered content changes at submission.
func formsBatchItemCount(count int, request *formsapi.Request) (int, error) {
	if request == nil {
		return count, usage("empty Forms batch request")
	}
	valid := func(location *formsapi.Location, upper int64) bool {
		return location != nil && location.Index >= 0 && location.Index < upper
	}
	switch {
	case request.CreateItem != nil:
		if !valid(request.CreateItem.Location, int64(count)+1) {
			return count, usagef("insertion index out of range (queued form has %d items)", count)
		}
		return count + 1, nil
	case request.DeleteItem != nil:
		if !valid(request.DeleteItem.Location, int64(count)) {
			return count, usagef("deletion index out of range (queued form has %d items)", count)
		}
		return count - 1, nil
	case request.MoveItem != nil:
		if !valid(request.MoveItem.OriginalLocation, int64(count)) || !valid(request.MoveItem.NewLocation, int64(count)) {
			return count, usagef("move index out of range (queued form has %d items)", count)
		}
	case request.UpdateFormInfo != nil, request.UpdateSettings != nil, request.UpdateItem != nil:
	default:
		return count, usage("cannot project item count for an unsupported Forms batch request")
	}
	return count, nil
}
