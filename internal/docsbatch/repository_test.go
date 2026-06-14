package docsbatch

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
)

const testBatchID = "018f47b5-7b5e-7cc0-9a78-4a5bb1886251"

var errInjectedWrite = errors.New("injected write failure")

func TestRepositoryLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	repository := New(t.TempDir(), Options{
		Now:   func() time.Time { return now },
		NewID: func() (string, error) { return testBatchID, nil },
	})

	state, err := repository.Create(State{
		Service:    ServiceDocs,
		DocumentID: "doc1",
		Account:    "user@example.com",
		Client:     "default",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if state.BatchID != testBatchID || !state.CreatedAt.Equal(now) || !state.UpdatedAt.Equal(now) {
		t.Fatalf("state = %#v", state)
	}

	now = now.Add(time.Minute)

	total, err := repository.Append(AppendOptions{
		BatchID:    state.BatchID,
		Command:    "docs.insert",
		Identity:   testIdentity("doc1"),
		RevisionID: "rev1",
		Requests:   []json.RawMessage{json.RawMessage(`{"insertText":{"text":"x"}}`)},
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}

	loaded, err := repository.Get(state.BatchID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if loaded.RequiredRevisionID != "rev1" || len(loaded.Requests) != 1 || !loaded.UpdatedAt.Equal(now) {
		t.Fatalf("loaded = %#v", loaded)
	}

	listed, err := repository.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(listed) != 1 || listed[0].Requests != 1 {
		t.Fatalf("listed = %#v", listed)
	}

	deleted, err := repository.Delete(state.BatchID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if len(deleted.Requests) != 1 {
		t.Fatalf("deleted = %#v", deleted)
	}
}

func TestRepositoryAppendValidation(t *testing.T) {
	t.Parallel()

	repository := testRepository(t, nil)

	state, err := repository.Create(State{
		Service:    ServiceDocs,
		DocumentID: "doc1",
		Account:    "user@example.com",
		Client:     "default",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	request := []json.RawMessage{json.RawMessage(`{"insertText":{"text":"x"}}`)}

	tests := []struct {
		name    string
		options AppendOptions
	}{
		{
			name: "document",
			options: AppendOptions{
				BatchID: state.BatchID, Command: "docs.insert", Identity: testIdentity("other"),
				RevisionID: "rev1", Requests: request,
			},
		},
		{
			name: "empty revision",
			options: AppendOptions{
				BatchID: state.BatchID, Command: "docs.insert", Identity: testIdentity("doc1"),
				Requests: request,
			},
		},
	}
	for _, tt := range tests {
		if _, appendErr := repository.Append(tt.options); appendErr == nil {
			t.Fatalf("%s: Append succeeded", tt.name)
		}
	}

	first := AppendOptions{
		BatchID: state.BatchID, Command: "docs.insert", Identity: testIdentity("doc1"),
		RevisionID: "rev1", Requests: request,
	}
	if _, err := repository.Append(first); err != nil {
		t.Fatalf("first Append: %v", err)
	}

	revisionMismatch := first

	revisionMismatch.RevisionID = "rev2"
	if _, err := repository.Append(revisionMismatch); err == nil {
		t.Fatal("revision mismatch succeeded")
	}

	requireEmpty := first

	requireEmpty.RequireEmpty = true
	if _, err := repository.Append(requireEmpty); err == nil {
		t.Fatal("RequireEmpty append succeeded")
	}
}

func TestRepositoryConcurrentAppend(t *testing.T) {
	t.Parallel()

	repository := testRepository(t, nil)

	state, err := repository.Create(State{
		Service:    ServiceDocs,
		DocumentID: "doc1",
		Account:    "user@example.com",
		Client:     "default",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const appendCount = 20
	var wg sync.WaitGroup

	errs := make(chan error, appendCount)
	for index := range appendCount {
		wg.Add(1)

		go func() {
			defer wg.Done()

			request, marshalErr := json.Marshal(map[string]int{"index": index})
			if marshalErr != nil {
				errs <- marshalErr

				return
			}

			_, appendErr := repository.Append(AppendOptions{
				BatchID:    state.BatchID,
				Command:    "docs.insert",
				Identity:   testIdentity("doc1"),
				RevisionID: "rev1",
				Requests:   []json.RawMessage{request},
			})
			errs <- appendErr
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	loaded, err := repository.Get(state.BatchID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if len(loaded.Requests) != appendCount {
		t.Fatalf("requests = %d, want %d", len(loaded.Requests), appendCount)
	}
}

func TestRepositoryTransactionPersistsAndDeletes(t *testing.T) {
	t.Parallel()

	repository := testRepository(t, nil)

	state, err := repository.Create(State{
		Service:    ServiceDocs,
		DocumentID: "doc1",
		Account:    "user@example.com",
		Client:     "default",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, appendErr := repository.Append(AppendOptions{
		BatchID: state.BatchID, Command: "docs.insert", Identity: testIdentity("doc1"),
		RevisionID: "rev1", Requests: []json.RawMessage{json.RawMessage(`{"one":1}`), json.RawMessage(`{"two":2}`)},
	}); appendErr != nil {
		t.Fatalf("Append: %v", appendErr)
	}

	if transactionErr := repository.WithState(state.BatchID, func(transaction *Transaction) error {
		transaction.State().Requests = transaction.State().Requests[1:]
		transaction.State().RequiredRevisionID = "rev2"

		return transaction.PersistOrDelete()
	}); transactionErr != nil {
		t.Fatalf("persist transaction: %v", transactionErr)
	}

	loaded, err := repository.Get(state.BatchID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if len(loaded.Requests) != 1 || loaded.RequiredRevisionID != "rev2" {
		t.Fatalf("loaded = %#v", loaded)
	}

	if transactionErr := repository.WithState(state.BatchID, func(transaction *Transaction) error {
		transaction.State().Requests = nil

		return transaction.PersistOrDelete()
	}); transactionErr != nil {
		t.Fatalf("delete transaction: %v", transactionErr)
	}

	if _, getErr := repository.Get(state.BatchID); getErr == nil {
		t.Fatal("deleted batch still exists")
	}
}

func TestRepositoryPrune(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	ids := []string{
		"018f47b5-7b5e-7cc0-9a78-4a5bb1886251",
		"018f47b5-7b5e-7cc0-9a78-4a5bb1886252",
	}
	repository := New(t.TempDir(), Options{
		Now: func() time.Time { return now },
		NewID: func() (string, error) {
			id := ids[0]
			ids = ids[1:]

			return id, nil
		},
	})

	now = now.Add(-4 * time.Hour)

	stale, err := repository.Create(State{Service: ServiceDocs, DocumentID: "old", Account: "a", Client: "default"})
	if err != nil {
		t.Fatalf("create stale: %v", err)
	}

	now = now.Add(4 * time.Hour)

	current, err := repository.Create(State{Service: ServiceDocs, DocumentID: "new", Account: "a", Client: "default"})
	if err != nil {
		t.Fatalf("create current: %v", err)
	}

	removed, err := repository.Prune(3 * time.Hour)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if len(removed) != 1 || removed[0].BatchID != stale.BatchID {
		t.Fatalf("removed = %#v", removed)
	}

	if _, err := repository.Get(current.BatchID); err != nil {
		t.Fatalf("current batch: %v", err)
	}
}

func TestRepositoryWriteFailurePreservesState(t *testing.T) {
	t.Parallel()

	failWrites := false
	repository := testRepository(t, func(path string, data []byte, mode os.FileMode) error {
		if failWrites {
			return errInjectedWrite
		}

		return config.WriteFileAtomic(path, data, mode)
	})

	state, err := repository.Create(State{
		Service: ServiceDocs, DocumentID: "doc1", Account: "user@example.com", Client: "default",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	failWrites = true

	_, err = repository.Append(AppendOptions{
		BatchID: state.BatchID, Command: "docs.insert", Identity: testIdentity("doc1"),
		RevisionID: "rev1", Requests: []json.RawMessage{json.RawMessage(`{"insertText":{}}`)},
	})
	if err == nil {
		t.Fatal("Append succeeded")
	}

	failWrites = false

	loaded, err := repository.Get(state.BatchID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if len(loaded.Requests) != 0 || loaded.RequiredRevisionID != "" {
		t.Fatalf("state changed after failed write: %#v", loaded)
	}
}

func TestRepositoryRejectsCorruptAndMismatchedState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	repository := New(dir, Options{})
	if err := os.WriteFile(filepath.Join(dir, testBatchID+".json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	if _, err := repository.Get(testBatchID); err == nil {
		t.Fatal("corrupt state loaded")
	}

	otherID := "018f47b5-7b5e-7cc0-9a78-4a5bb1886252"

	payload, err := json.Marshal(State{BatchID: otherID})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, testBatchID+".json"), payload, 0o600); err != nil {
		t.Fatalf("write mismatched state: %v", err)
	}

	if _, err := repository.Get(testBatchID); err == nil {
		t.Fatal("mismatched state loaded")
	}
}

func TestValidateIDRejectsTraversal(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"../batch", "not-a-uuid", "018F47B5-7B5E-7CC0-9A78-4A5BB1886251"} {
		if err := ValidateID(value); err == nil {
			t.Fatalf("ValidateID(%q) succeeded", value)
		}
	}
}

func testRepository(t *testing.T, writer func(string, []byte, os.FileMode) error) *Repository {
	t.Helper()

	return New(t.TempDir(), Options{
		NewID:     func() (string, error) { return testBatchID, nil },
		WriteFile: writer,
	})
}

func testIdentity(documentID string) Identity {
	return Identity{
		Service:    ServiceDocs,
		DocumentID: documentID,
		Account:    "user@example.com",
		Client:     "default",
	}
}
