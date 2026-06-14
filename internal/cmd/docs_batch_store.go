package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"google.golang.org/api/docs/v1"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/docsbatch"
)

const docsBatchBaseURLDefault = "https://docs.googleapis.com/v1"

var docsBatchBaseURL = docsBatchBaseURLDefault

type docsBatchWireBody struct {
	Requests     []json.RawMessage  `json:"requests"`
	WriteControl *docs.WriteControl `json:"writeControl,omitempty"`
}

func newDocsBatchStore(ctx context.Context) (*docsbatch.Repository, error) {
	store, err := openDocsBatchStore(ctx)
	if err != nil {
		return nil, err
	}
	if err := store.Ensure(); err != nil {
		return nil, err
	}

	return store, nil
}

func openDocsBatchStore(ctx context.Context) (*docsbatch.Repository, error) {
	layout, err := commandLayout(ctx, config.PathKindState)
	if err != nil {
		return nil, err
	}

	return newDocsBatchStoreAt(layout.BatchDir()), nil
}

func newDocsBatchStoreAt(dir string) *docsbatch.Repository {
	return docsbatch.New(dir, docsbatch.Options{})
}

func docsBatchWirePayload(state *docsbatch.State, entries []docsbatch.RequestEntry) docsBatchWireBody {
	requests := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		requests = append(requests, entry.Request)
	}
	body := docsBatchWireBody{Requests: requests}
	if state.RequiredRevisionID != "" {
		body.WriteControl = &docs.WriteControl{RequiredRevisionId: state.RequiredRevisionID}
	}

	return body
}

func submitDocsBatch(ctx context.Context, state *docsbatch.State, entries []docsbatch.RequestEntry) (*docs.BatchUpdateDocumentResponse, error) {
	ctx = authclient.WithClient(ctx, state.Client)
	client, err := docsHTTPClient(ctx, state.Account)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(docsBatchWirePayload(state, entries))
	if err != nil {
		return nil, fmt.Errorf("encode Docs batch: %w", err)
	}
	endpoint := docsBatchBaseURL + "/documents/" + url.PathEscape(state.DocumentID) + ":batchUpdate"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create Docs batch request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("submit Docs batch: %w", err)
	}
	defer response.Body.Close()

	if err := gapi.CheckResponse(response); err != nil {
		return nil, err
	}

	var result docs.BatchUpdateDocumentResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Docs batch response: %w", err)
	}

	return &result, nil
}

func marshalDocsBatchRequests(requests []*docs.Request) ([]json.RawMessage, error) {
	rawRequests := make([]json.RawMessage, 0, len(requests))
	for _, request := range requests {
		raw, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("marshal Docs request: %w", err)
		}
		rawRequests = append(rawRequests, raw)
	}

	return rawRequests, nil
}

func docsBatchResponseRevision(result *docs.BatchUpdateDocumentResponse) string {
	if result == nil || result.WriteControl == nil {
		return ""
	}

	return strings.TrimSpace(result.WriteControl.RequiredRevisionId)
}

func isDocsBatchBadRequest(err error) bool {
	var apiErr *gapi.Error

	return errors.As(err, &apiErr) && apiErr.Code == http.StatusBadRequest
}
