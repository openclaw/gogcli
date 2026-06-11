package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/api/docs/v1"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/filelock"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/googleauth"
)

const (
	docsBatchService        = "docs"
	docsBatchLockTimeout    = 5 * time.Second
	docsBatchBaseURLDefault = "https://docs.googleapis.com/v1"
)

var (
	docsBatchNow           = time.Now
	docsBatchBaseURL       = docsBatchBaseURLDefault
	newDocsBatchHTTPClient = func(ctx context.Context, account string) (*http.Client, error) {
		return googleapi.NewHTTPClient(ctx, googleauth.ServiceDocs, account)
	}
)

type docsBatchRequestEntry struct {
	AppendedAt time.Time       `json:"appended_at"`
	Command    string          `json:"command"`
	Request    json.RawMessage `json:"request"`
}

type docsBatchState struct {
	BatchID            string                  `json:"batch_id"`
	Name               string                  `json:"name,omitempty"`
	Service            string                  `json:"service"`
	DocumentID         string                  `json:"doc_id"`
	Account            string                  `json:"account"`
	Client             string                  `json:"client"`
	CreatedAt          time.Time               `json:"created_at"`
	UpdatedAt          time.Time               `json:"updated_at"`
	RequiredRevisionID string                  `json:"required_revision_id,omitempty"`
	Requests           []docsBatchRequestEntry `json:"requests"`
}

type docsBatchSummary struct {
	BatchID    string    `json:"batch_id"`
	Name       string    `json:"name,omitempty"`
	Service    string    `json:"service"`
	DocumentID string    `json:"doc_id"`
	Account    string    `json:"account"`
	Client     string    `json:"client"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Requests   int       `json:"requests"`
}

type docsBatchWireBody struct {
	Requests     []json.RawMessage  `json:"requests"`
	WriteControl *docs.WriteControl `json:"writeControl,omitempty"`
}

type docsBatchStore struct {
	dir  string
	lock *filelock.Lock
}

func newDocsBatchStore() (*docsBatchStore, error) {
	dir, err := config.EnsureBatchDir()
	if err != nil {
		return nil, err
	}

	return newDocsBatchStoreAt(dir), nil
}

func newDocsBatchStoreAt(dir string) *docsBatchStore {
	return &docsBatchStore{
		dir:  dir,
		lock: filelock.Shared(filepath.Join(dir, ".lock"), docsBatchLockTimeout),
	}
}

func (s *docsBatchStore) create(state docsBatchState) (*docsBatchState, error) {
	var created *docsBatchState
	err := s.lock.WithExclusive(func() error {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("create batch ID: %w", err)
		}

		now := docsBatchNow().UTC()
		state.BatchID = id.String()
		state.CreatedAt = now
		state.UpdatedAt = now
		state.Requests = []docsBatchRequestEntry{}
		if err := s.writeUnlocked(&state); err != nil {
			return err
		}
		created = &state

		return nil
	})

	return created, err
}

func (s *docsBatchStore) list() ([]docsBatchSummary, error) {
	var summaries []docsBatchSummary
	err := s.lock.WithExclusive(func() error {
		states, err := s.listStatesUnlocked()
		if err != nil {
			return err
		}

		summaries = make([]docsBatchSummary, 0, len(states))
		for _, state := range states {
			summaries = append(summaries, docsBatchSummary{
				BatchID:    state.BatchID,
				Name:       state.Name,
				Service:    state.Service,
				DocumentID: state.DocumentID,
				Account:    state.Account,
				Client:     state.Client,
				CreatedAt:  state.CreatedAt,
				UpdatedAt:  state.UpdatedAt,
				Requests:   len(state.Requests),
			})
		}

		return nil
	})

	return summaries, err
}

func (s *docsBatchStore) get(batchID string) (*docsBatchState, error) {
	var state *docsBatchState
	err := s.lock.WithExclusive(func() error {
		loaded, err := s.readUnlocked(batchID)
		if err != nil {
			return err
		}
		state = loaded

		return nil
	})

	return state, err
}

func (s *docsBatchStore) appendRequests(batchID, command, documentID, account, client, revisionID string, requests []*docs.Request, requireEmpty bool) (int, error) {
	total := 0
	err := s.lock.WithExclusive(func() error {
		state, err := s.readUnlocked(batchID)
		if err != nil {
			return err
		}
		if err := validateDocsBatchIdentity(state, documentID, account, client); err != nil {
			return err
		}
		if revisionID == "" {
			return errors.New("document revision is empty")
		}
		// An empty batch has no revision boundary because no request positions have
		// been resolved yet. The first queued operation establishes the baseline.
		if state.RequiredRevisionID != "" && state.RequiredRevisionID != revisionID {
			return fmt.Errorf("document revision changed since the first request was queued (batch=%s current=%s)", state.RequiredRevisionID, revisionID)
		}
		if requireEmpty && len(state.Requests) > 0 {
			return errors.New("this operation must be the first request in a batch")
		}

		now := docsBatchNow().UTC()
		for _, request := range requests {
			raw, marshalErr := json.Marshal(request)
			if marshalErr != nil {
				return fmt.Errorf("marshal Docs request: %w", marshalErr)
			}
			state.Requests = append(state.Requests, docsBatchRequestEntry{
				AppendedAt: now,
				Command:    command,
				Request:    raw,
			})
		}
		state.RequiredRevisionID = revisionID
		state.UpdatedAt = now
		if err := s.writeUnlocked(state); err != nil {
			return err
		}
		total = len(state.Requests)

		return nil
	})

	return total, err
}

func (s *docsBatchStore) delete(batchID string) (*docsBatchState, error) {
	var deleted *docsBatchState
	err := s.lock.WithExclusive(func() error {
		state, err := s.readUnlocked(batchID)
		if err != nil {
			return err
		}
		if err := os.Remove(s.path(state.BatchID)); err != nil {
			return fmt.Errorf("remove batch: %w", err)
		}
		deleted = state

		return nil
	})

	return deleted, err
}

func (s *docsBatchStore) prune(olderThan time.Duration) ([]docsBatchSummary, error) {
	var removed []docsBatchSummary
	err := s.lock.WithExclusive(func() error {
		states, err := s.listStatesUnlocked()
		if err != nil {
			return err
		}

		cutoff := docsBatchNow().UTC().Add(-olderThan)
		for _, state := range states {
			if state.UpdatedAt.After(cutoff) {
				continue
			}
			if err := os.Remove(s.path(state.BatchID)); err != nil {
				return fmt.Errorf("remove batch %s: %w", state.BatchID, err)
			}
			removed = append(removed, docsBatchSummary{
				BatchID:    state.BatchID,
				Name:       state.Name,
				Service:    state.Service,
				DocumentID: state.DocumentID,
				Account:    state.Account,
				Client:     state.Client,
				CreatedAt:  state.CreatedAt,
				UpdatedAt:  state.UpdatedAt,
				Requests:   len(state.Requests),
			})
		}

		return nil
	})

	return removed, err
}

func (s *docsBatchStore) withLockedState(batchID string, fn func(*docsBatchState) error) error {
	return s.lock.WithExclusive(func() error {
		state, err := s.readUnlocked(batchID)
		if err != nil {
			return err
		}

		return fn(state)
	})
}

func (s *docsBatchStore) persistOrDeleteUnlocked(state *docsBatchState) error {
	if len(state.Requests) == 0 {
		if err := os.Remove(s.path(state.BatchID)); err != nil {
			return fmt.Errorf("remove completed batch: %w", err)
		}
		return nil
	}

	state.UpdatedAt = docsBatchNow().UTC()
	return s.writeUnlocked(state)
}

func (s *docsBatchStore) listStatesUnlocked() ([]*docsBatchState, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read batch directory: %w", err)
	}

	states := make([]*docsBatchState, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		state, readErr := s.readPathUnlocked(filepath.Join(s.dir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].UpdatedAt.After(states[j].UpdatedAt)
	})

	return states, nil
}

func (s *docsBatchStore) readUnlocked(batchID string) (*docsBatchState, error) {
	if err := validateDocsBatchID(batchID); err != nil {
		return nil, err
	}

	return s.readPathUnlocked(s.path(batchID))
}

func (s *docsBatchStore) readPathUnlocked(path string) (*docsBatchState, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the private batch directory and a validated UUID.
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("batch not found: %s", strings.TrimSuffix(filepath.Base(path), ".json"))
		}
		return nil, fmt.Errorf("read batch: %w", err)
	}

	var state docsBatchState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode batch %s: %w", filepath.Base(path), err)
	}
	if err := validateDocsBatchID(state.BatchID); err != nil {
		return nil, fmt.Errorf("invalid stored batch: %w", err)
	}

	return &state, nil
}

func (s *docsBatchStore) writeUnlocked(state *docsBatchState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode batch: %w", err)
	}
	data = append(data, '\n')
	if err := config.WriteFileAtomic(s.path(state.BatchID), data, 0o600); err != nil {
		return fmt.Errorf("write batch: %w", err)
	}

	return nil
}

func (s *docsBatchStore) path(batchID string) string {
	return filepath.Join(s.dir, batchID+".json")
}

func validateDocsBatchID(batchID string) error {
	batchID = strings.TrimSpace(batchID)
	parsed, err := uuid.Parse(batchID)
	if err != nil || parsed.String() != batchID {
		return fmt.Errorf("invalid batch ID: %s", batchID)
	}

	return nil
}

func validateDocsBatchIdentity(state *docsBatchState, documentID, account, client string) error {
	switch {
	case state.Service != docsBatchService:
		return fmt.Errorf("batch service is %s, not docs", state.Service)
	case state.DocumentID != documentID:
		return fmt.Errorf("batch targets doc %s, not %s", state.DocumentID, documentID)
	case !strings.EqualFold(state.Account, account):
		return fmt.Errorf("batch uses account %s, not %s", state.Account, account)
	case state.Client != client:
		return fmt.Errorf("batch uses OAuth client %s, not %s", state.Client, client)
	default:
		return nil
	}
}

func docsBatchWirePayload(state *docsBatchState, entries []docsBatchRequestEntry) docsBatchWireBody {
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

func submitDocsBatch(ctx context.Context, state *docsBatchState, entries []docsBatchRequestEntry) (*docs.BatchUpdateDocumentResponse, error) {
	ctx = authclient.WithClient(ctx, state.Client)
	client, err := newDocsBatchHTTPClient(ctx, state.Account)
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
