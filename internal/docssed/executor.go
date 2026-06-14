package docssed

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"google.golang.org/api/docs/v1"
	gapi "google.golang.org/api/googleapi"
)

const (
	defaultMaxRetries = 5
	defaultBaseDelay  = time.Second
	defaultMaxDelay   = 30 * time.Second
)

var (
	errDocumentBackendRequired = errors.New("docs document backend is required")
	errDocsServiceRequired     = errors.New("docs service is required")
)

type DocumentBackend interface {
	Get(context.Context, string) (*docs.Document, error)
	BatchUpdate(context.Context, string, []*docs.Request) (*docs.BatchUpdateDocumentResponse, error)
}

type Executor struct {
	backend    DocumentBackend
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	jitter     func(time.Duration) time.Duration
	sleep      func(context.Context, time.Duration) error
}

type executorOptions struct {
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	jitter     func(time.Duration) time.Duration
	sleep      func(context.Context, time.Duration) error
}

func NewExecutor(backend DocumentBackend) *Executor {
	return newExecutor(backend, executorOptions{
		maxRetries: defaultMaxRetries,
		baseDelay:  defaultBaseDelay,
		maxDelay:   defaultMaxDelay,
		jitter:     cryptoJitter,
		sleep:      sleepContext,
	})
}

func NewServiceExecutor(service *docs.Service) *Executor {
	return NewExecutor(serviceDocumentBackend{service: service})
}

func newExecutor(backend DocumentBackend, opts executorOptions) *Executor {
	return &Executor{
		backend:    backend,
		maxRetries: opts.maxRetries,
		baseDelay:  opts.baseDelay,
		maxDelay:   opts.maxDelay,
		jitter:     opts.jitter,
		sleep:      opts.sleep,
	}
}

func (e *Executor) Get(ctx context.Context, documentID string) (*docs.Document, error) {
	if e == nil || e.backend == nil {
		return nil, errDocumentBackendRequired
	}

	var document *docs.Document
	err := e.retry(ctx, func() error {
		var getErr error

		document, getErr = e.backend.Get(ctx, documentID)
		if getErr != nil {
			return fmt.Errorf("get document from backend: %w", getErr)
		}

		return nil
	})

	return document, err
}

func (e *Executor) BatchUpdate(
	ctx context.Context,
	documentID string,
	requests []*docs.Request,
) (*docs.BatchUpdateDocumentResponse, error) {
	if len(requests) == 0 {
		return &docs.BatchUpdateDocumentResponse{DocumentId: documentID}, nil
	}

	if e == nil || e.backend == nil {
		return nil, errDocumentBackendRequired
	}

	var response *docs.BatchUpdateDocumentResponse
	err := e.retry(ctx, func() error {
		var updateErr error

		response, updateErr = e.backend.BatchUpdate(ctx, documentID, requests)
		if updateErr != nil {
			return fmt.Errorf("batch update document through backend: %w", updateErr)
		}

		return nil
	})

	return response, err
}

func (e *Executor) retry(ctx context.Context, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}
		lastErr = err

		if !isRetryableError(err) {
			return err
		}

		if attempt == e.maxRetries {
			return fmt.Errorf("after %d retries: %w", e.maxRetries, lastErr)
		}

		delay := e.baseDelay * time.Duration(1<<uint(attempt))
		if delay > e.maxDelay {
			delay = e.maxDelay
		}

		halfDelay := delay / 2
		if e.jitter != nil {
			delay = halfDelay + e.jitter(halfDelay)
		} else {
			delay = halfDelay
		}

		if err := e.sleep(ctx, delay); err != nil {
			return fmt.Errorf("wait before retry: %w", err)
		}
	}

	return lastErr
}

type serviceDocumentBackend struct {
	service *docs.Service
}

func (b serviceDocumentBackend) Get(ctx context.Context, documentID string) (*docs.Document, error) {
	if b.service == nil {
		return nil, errDocsServiceRequired
	}

	document, err := b.service.Documents.Get(documentID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get Docs document: %w", err)
	}

	return document, nil
}

func (b serviceDocumentBackend) BatchUpdate(
	ctx context.Context,
	documentID string,
	requests []*docs.Request,
) (*docs.BatchUpdateDocumentResponse, error) {
	if b.service == nil {
		return nil, errDocsServiceRequired
	}

	response, err := b.service.Documents.BatchUpdate(documentID, &docs.BatchUpdateDocumentRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("batch update Docs document: %w", err)
	}

	return response, nil
}

func cryptoJitter(limit time.Duration) time.Duration {
	if limit <= 0 {
		return 0
	}

	value, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0
	}

	return time.Duration(value.Int64())
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("retry canceled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *gapi.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case 429, 500, 502, 503:
			return true
		}
	}

	message := err.Error()

	return strings.Contains(message, "rateLimitExceeded") || strings.Contains(message, "429")
}
