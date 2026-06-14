package gmailwatch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type hookDoer func(*http.Request) (*http.Response, error)

func (f hookDoer) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHookSenderSuccess(t *testing.T) {
	t.Parallel()

	sender := &HookSender{
		URL:   "https://example.com/hook",
		Token: "secret",
		Client: hookDoer(func(request *http.Request) (*http.Response, error) {
			if request.Header.Get("Authorization") != "Bearer secret" {
				t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
			}

			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	result := sender.Send(context.Background(), &Payload{HistoryID: "200"})
	if result.Err != nil || result.Status != DeliveryStatusOK || !result.Record {
		t.Fatalf("result = %#v", result)
	}
}

func TestHookSenderClassifiesFailures(t *testing.T) {
	t.Parallel()

	transportErr := errors.New("dial failed") //nolint:err113 // Test-only transport failure.
	sender := &HookSender{
		URL: "https://example.com/hook",
		Client: hookDoer(func(*http.Request) (*http.Response, error) {
			return nil, transportErr
		}),
	}

	result := sender.Send(context.Background(), &Payload{})
	if !errors.Is(result.Err, transportErr) || result.Status != DeliveryStatusError || !result.Record {
		t.Fatalf("transport result = %#v", result)
	}

	sender.Client = hookDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	result = sender.Send(context.Background(), &Payload{})

	var statusErr *HookStatusError
	if !errors.As(result.Err, &statusErr) || statusErr.StatusCode != http.StatusBadGateway ||
		result.Status != DeliveryStatusHTTPError || result.Note != "status 502" || !result.Record {
		t.Fatalf("HTTP result = %#v", result)
	}
}
