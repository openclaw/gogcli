package googleapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type trackedRequestBody struct {
	io.Reader
	closes int
}

func (b *trackedRequestBody) Close() error {
	b.closes++
	return nil
}

type failingRequestReader struct{ err error }

func (r failingRequestReader) Read([]byte) (int, error) { return 0, r.err }

func TestRetryTransportClosesRejectedRequestBody(t *testing.T) {
	tests := []struct {
		name        string
		reader      io.Reader
		openCircuit bool
		wantErr     error
	}{
		{"open circuit", strings.NewReader("upload"), true, nil},
		{"buffer read error", failingRequestReader{io.ErrUnexpectedEOF}, false, io.ErrUnexpectedEOF},
		{"buffer limit", io.LimitReader(repeatingRequestReader{}, maxBufferedReplayBodyBytes+1), false, errRequestBodyTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := &trackedRequestBody{Reader: tt.reader}

			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/upload", body)
			if err != nil {
				t.Fatal(err)
			}
			req.ContentLength = 6
			rt := NewRetryTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("rejected request reached the base transport")
				return nil, errUnexpectedRequestBody
			}))

			if tt.openCircuit {
				for range CircuitBreakerThreshold {
					rt.CircuitBreaker.RecordFailure()
				}
			}

			resp, err := (&http.Client{Transport: rt}).Do(req)
			if resp != nil {
				_ = resp.Body.Close()

				t.Fatal("expected no response")
			}

			if err == nil || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}

			if tt.openCircuit && !IsCircuitBreakerError(err) {
				t.Fatalf("error = %v, want circuit breaker error", err)
			}

			if body.closes != 1 {
				t.Fatalf("request body closed %d times, want 1", body.closes)
			}
		})
	}
}

type repeatingRequestReader struct{}

func (repeatingRequestReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}

	return len(p), nil
}
