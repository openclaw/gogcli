package gmailwatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type HookSender struct {
	URL    string
	Token  string
	Client HTTPDoer
}

type HookStatusError struct {
	StatusCode int
}

func (e *HookStatusError) Error() string {
	return fmt.Sprintf("hook status %d", e.StatusCode)
}

func (s *HookSender) Send(ctx context.Context, payload *Payload) DeliveryResult {
	data, err := json.Marshal(payload)
	if err != nil {
		return DeliveryResult{Err: fmt.Errorf("encode hook payload: %w", err)}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(data))
	if err != nil {
		return DeliveryResult{Err: fmt.Errorf("create hook request: %w", err)}
	}

	request.Header.Set("Content-Type", "application/json")

	if s.Token != "" {
		request.Header.Set("Authorization", "Bearer "+s.Token)
	}

	response, err := s.Client.Do(request)
	if err != nil {
		return DeliveryResult{
			Status: DeliveryStatusError,
			Note:   err.Error(),
			Err:    err,
			Record: true,
		}
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		note := fmt.Sprintf("status %d", response.StatusCode)

		return DeliveryResult{
			Status: DeliveryStatusHTTPError,
			Note:   note,
			Err:    &HookStatusError{StatusCode: response.StatusCode},
			Record: true,
		}
	}

	return DeliveryResult{
		Status: DeliveryStatusOK,
		Record: true,
	}
}
