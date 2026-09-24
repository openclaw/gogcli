package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"

	gapi "google.golang.org/api/googleapi"
	"google.golang.org/api/slides/v1"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/docsbatch"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

var slidesBatchBaseURL = "https://slides.googleapis.com/v1"

type slidesBatchWireBody struct {
	Requests     []json.RawMessage    `json:"requests"`
	WriteControl *slides.WriteControl `json:"writeControl"`
}

func slidesBatchWirePayload(state *docsbatch.State, entries []docsbatch.RequestEntry) slidesBatchWireBody {
	requests := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		requests = append(requests, entry.Request)
	}
	return slidesBatchWireBody{
		Requests:     requests,
		WriteControl: &slides.WriteControl{RequiredRevisionId: state.RequiredRevisionID},
	}
}

func submitSlidesBatch(ctx context.Context, state *docsbatch.State, entries []docsbatch.RequestEntry) (string, error) {
	if googleapi.ReadOnly(ctx) {
		return "", googleapi.ErrReadOnly
	}
	ctx = authclient.WithClient(ctx, state.Client)
	client, err := slidesHTTPClient(ctx, state.Account)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(slidesBatchWirePayload(state, entries))
	if err != nil {
		return "", fmt.Errorf("encode Slides batch: %w", err)
	}
	request, err := http.NewRequestWithContext(googleapi.WithoutRetries(ctx), http.MethodPost,
		slidesBatchBaseURL+"/presentations/"+url.PathEscape(state.PresentationID)+":batchUpdate", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create Slides batch request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	// Never replay a possibly completed mutation, including through an HTTP redirect.
	batchClient := *client
	batchClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := batchClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("submit Slides batch: %w", err)
	}
	defer response.Body.Close()
	if err := gapi.CheckResponse(response); err != nil {
		return "", err
	}
	var result slides.BatchUpdatePresentationResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode Slides batch response: %w", err)
	}
	if result.WriteControl == nil {
		return "", nil
	}
	return strings.TrimSpace(result.WriteControl.RequiredRevisionId), nil
}

func queueSlidesBatchRequests(ctx context.Context, flags *RootFlags, batchID, presentationID, command string, requests []*slides.Request, output map[string]any) (bool, error) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return false, nil
	}
	if err := validateDocsBatchIDArg(batchID); err != nil {
		return true, err
	}
	if len(requests) == 0 {
		return true, errors.New("no Slides requests to append")
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
	state, err := store.Get(batchID)
	if err != nil {
		return true, err
	}
	identity := docsbatch.Identity{
		Service: docsbatch.ServiceSlides, PresentationID: presentationID, Account: account, Client: client,
	}
	if identityErr := docsbatch.ValidateIdentity(state, identity); identityErr != nil {
		return true, identityErr
	}
	revision := state.RequiredRevisionID
	if revision == "" {
		svc, serviceErr := slidesService(ctx, account)
		if serviceErr != nil {
			return true, serviceErr
		}
		presentation, getErr := svc.Presentations.Get(presentationID).Fields("revisionId").Context(ctx).Do()
		if getErr != nil {
			return true, getErr
		}
		if presentation == nil || strings.TrimSpace(presentation.RevisionId) == "" {
			return true, errors.New("slides response omitted presentation revision; editor access is required")
		}
		revision = presentation.RevisionId
	}
	// Capture the first revision once; Google validates it and all queued objects at submission.
	rawRequests := make([]json.RawMessage, 0, len(requests))
	for _, request := range requests {
		raw, marshalErr := json.Marshal(request)
		if marshalErr != nil {
			return true, fmt.Errorf("encode queued Slides request: %w", marshalErr)
		}
		rawRequests = append(rawRequests, raw)
	}
	total, err := store.Append(docsbatch.AppendOptions{
		BatchID: batchID, Command: command, Identity: identity, RevisionID: revision, Requests: rawRequests,
	})
	if err != nil {
		return true, err
	}
	result := maps.Clone(output)
	if result == nil {
		result = make(map[string]any)
	}
	delete(result, "deleted") // Queuing a deletion does not mean it has executed.
	result["batch_id"] = batchID
	result["queued"] = len(requests)
	result["requests"] = total
	if outfmt.IsJSON(ctx) {
		return true, outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}
	out := ui.FromContext(ctx).Out()
	out.Linef("batch_id\t%s", batchID)
	out.Linef("queued\t%d", len(requests))
	out.Linef("requests\t%d", total)
	for _, key := range []string{"slideObjectId", "objectId", "tableObjectId", "groupObjectId"} {
		if id, ok := result[key].(string); ok && id != "" {
			out.Linef("%s\t%s", key, id)
		}
	}
	return true, nil
}
