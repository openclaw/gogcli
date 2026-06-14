package docssed

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"google.golang.org/api/docs/v1"
	gapi "google.golang.org/api/googleapi"
)

var errExecutorTestPermanent = errors.New("permanent")

type fakeDocumentBackend struct {
	getErrors        []error
	updateErrors     []error
	getCalls         int
	updateCalls      int
	getDocumentID    string
	updateDocumentID string
	updateRequests   []*docs.Request
	document         *docs.Document
	response         *docs.BatchUpdateDocumentResponse
}

func (b *fakeDocumentBackend) Get(_ context.Context, documentID string) (*docs.Document, error) {
	b.getDocumentID = documentID
	err := queuedError(b.getErrors, b.getCalls)
	b.getCalls++

	if err != nil {
		return nil, err
	}

	return b.document, nil
}

func (b *fakeDocumentBackend) BatchUpdate(
	_ context.Context,
	documentID string,
	requests []*docs.Request,
) (*docs.BatchUpdateDocumentResponse, error) {
	b.updateDocumentID = documentID
	b.updateRequests = requests
	err := queuedError(b.updateErrors, b.updateCalls)
	b.updateCalls++

	if err != nil {
		return nil, err
	}

	return b.response, nil
}

func TestExecutorGetRetriesWithDeterministicBackoff(t *testing.T) {
	t.Parallel()

	backend := &fakeDocumentBackend{
		getErrors: []error{
			&gapi.Error{Code: 429},
			&gapi.Error{Code: 503},
			nil,
		},
		document: &docs.Document{DocumentId: "doc"},
	}
	var delays []time.Duration
	executor := newExecutor(backend, executorOptions{
		maxRetries: 5,
		baseDelay:  2 * time.Second,
		maxDelay:   3 * time.Second,
		jitter:     func(time.Duration) time.Duration { return 0 },
		sleep: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
	})

	document, err := executor.Get(context.Background(), "doc")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if document.DocumentId != "doc" || backend.getCalls != 3 {
		t.Fatalf("document/calls = %#v/%d", document, backend.getCalls)
	}

	if backend.getDocumentID != "doc" {
		t.Fatalf("document ID = %q, want doc", backend.getDocumentID)
	}

	if want := []time.Duration{time.Second, 1500 * time.Millisecond}; !reflect.DeepEqual(delays, want) {
		t.Fatalf("delays = %#v, want %#v", delays, want)
	}
}

func TestExecutorReturnsNonRetryableErrorImmediately(t *testing.T) {
	t.Parallel()

	backend := &fakeDocumentBackend{getErrors: []error{errExecutorTestPermanent}}
	executor := NewExecutor(backend)

	_, err := executor.Get(context.Background(), "doc")
	if !errors.Is(err, errExecutorTestPermanent) {
		t.Fatalf("error = %v, want permanent", err)
	}

	if backend.getCalls != 1 {
		t.Fatalf("get calls = %d, want 1", backend.getCalls)
	}
}

func TestExecutorReturnsFinalRetryCause(t *testing.T) {
	t.Parallel()

	finalErr := &gapi.Error{Code: 503, Message: "still unavailable"}
	backend := &fakeDocumentBackend{getErrors: []error{
		&gapi.Error{Code: 500},
		finalErr,
	}}
	executor := newExecutor(backend, executorOptions{
		maxRetries: 1,
		baseDelay:  time.Nanosecond,
		maxDelay:   time.Nanosecond,
		jitter:     func(time.Duration) time.Duration { return 0 },
		sleep:      func(context.Context, time.Duration) error { return nil },
	})

	_, err := executor.Get(context.Background(), "doc")
	if !errors.Is(err, finalErr) {
		t.Fatalf("error = %v, want final cause", err)
	}

	if backend.getCalls != 2 {
		t.Fatalf("get calls = %d, want 2", backend.getCalls)
	}
}

func TestExecutorUsesSixAttemptsByDefault(t *testing.T) {
	t.Parallel()

	finalErr := &gapi.Error{Code: 503, Message: "still unavailable"}
	backend := &fakeDocumentBackend{getErrors: []error{
		finalErr,
		finalErr,
		finalErr,
		finalErr,
		finalErr,
		finalErr,
	}}
	executor := newExecutor(backend, executorOptions{
		maxRetries: defaultMaxRetries,
		baseDelay:  time.Nanosecond,
		maxDelay:   time.Nanosecond,
		jitter:     func(time.Duration) time.Duration { return 0 },
		sleep:      func(context.Context, time.Duration) error { return nil },
	})

	_, err := executor.Get(context.Background(), "doc")
	if !errors.Is(err, finalErr) {
		t.Fatalf("error = %v, want final cause", err)
	}

	if backend.getCalls != defaultMaxRetries+1 {
		t.Fatalf("get calls = %d, want %d", backend.getCalls, defaultMaxRetries+1)
	}
}

func TestExecutorCancellationInterruptsRetryDelay(t *testing.T) {
	t.Parallel()

	backend := &fakeDocumentBackend{getErrors: []error{&gapi.Error{Code: 429}}}
	executor := newExecutor(backend, executorOptions{
		maxRetries: 5,
		baseDelay:  time.Hour,
		maxDelay:   time.Hour,
		jitter:     func(time.Duration) time.Duration { return 0 },
		sleep:      sleepContext,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := executor.Get(ctx, "doc")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want canceled", err)
	}

	if backend.getCalls != 1 {
		t.Fatalf("get calls = %d, want 1", backend.getCalls)
	}
}

func TestExecutorBatchUpdateForwardsRequestsInOrder(t *testing.T) {
	t.Parallel()

	first := &docs.Request{InsertText: &docs.InsertTextRequest{Text: "first"}}
	second := &docs.Request{InsertText: &docs.InsertTextRequest{Text: "second"}}
	backend := &fakeDocumentBackend{
		response: &docs.BatchUpdateDocumentResponse{DocumentId: "doc"},
	}

	response, err := NewExecutor(backend).BatchUpdate(
		context.Background(),
		"doc",
		[]*docs.Request{first, second},
	)
	if err != nil {
		t.Fatalf("BatchUpdate: %v", err)
	}

	if response.DocumentId != "doc" || backend.updateCalls != 1 {
		t.Fatalf("response/calls = %#v/%d", response, backend.updateCalls)
	}

	if backend.updateDocumentID != "doc" {
		t.Fatalf("document ID = %q, want doc", backend.updateDocumentID)
	}

	if !reflect.DeepEqual(backend.updateRequests, []*docs.Request{first, second}) {
		t.Fatalf("requests = %#v, want original order", backend.updateRequests)
	}
}

func TestExecutorBatchUpdateSkipsEmptyRequests(t *testing.T) {
	t.Parallel()

	backend := &fakeDocumentBackend{}

	response, err := NewExecutor(backend).BatchUpdate(context.Background(), "doc", nil)
	if err != nil {
		t.Fatalf("BatchUpdate: %v", err)
	}

	if response.DocumentId != "doc" || backend.updateCalls != 0 {
		t.Fatalf("response/calls = %#v/%d", response, backend.updateCalls)
	}
}

func TestExecutorRequiresBackendAndService(t *testing.T) {
	t.Parallel()

	_, err := NewExecutor(nil).Get(context.Background(), "doc")
	if !errors.Is(err, errDocumentBackendRequired) {
		t.Fatalf("backend error = %v", err)
	}

	_, err = NewServiceExecutor(nil).Get(context.Background(), "doc")
	if !errors.Is(err, errDocsServiceRequired) {
		t.Fatalf("service error = %v", err)
	}
}

func TestIsRetryableError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		want bool
	}{
		{err: nil, want: false},
		{err: errExecutorTestPermanent, want: false},
		{err: &gapi.Error{Code: 429}, want: true},
		{err: &gapi.Error{Code: 500}, want: true},
		{err: &gapi.Error{Code: 502}, want: true},
		{err: &gapi.Error{Code: 503}, want: true},
		{err: &gapi.Error{Code: 404}, want: false},
		{err: errors.New("rateLimitExceeded"), want: true}, //nolint:err113 // Static test fixture.
		{err: errors.New("error 429"), want: true},         //nolint:err113 // Static test fixture.
	}
	for _, tc := range tests {
		if got := isRetryableError(tc.err); got != tc.want {
			t.Fatalf("isRetryableError(%v) = %t, want %t", tc.err, got, tc.want)
		}
	}
}

func queuedError(errs []error, call int) error {
	if call >= len(errs) {
		return nil
	}

	return errs[call]
}
