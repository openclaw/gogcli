package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	gapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func postBatchUpdate(ctx context.Context, client *http.Client, service, endpoint string, payload, result any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s batch: %w", service, err)
	}
	request, err := http.NewRequestWithContext(googleapi.WithoutRetries(ctx), http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("create %s batch request: %w", service, err)
	}
	request.Header.Set("Content-Type", "application/json")
	// Never replay a possibly completed mutation, including through a redirect.
	batchClient := *client
	batchClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := batchClient.Do(request)
	if err != nil {
		return fmt.Errorf("submit %s batch: %w", service, err)
	}
	defer response.Body.Close()
	if responseErr := gapi.CheckResponse(response); responseErr != nil {
		return responseErr
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read %s batch response: %w", service, err)
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return fmt.Errorf("decode %s batch response: expected a JSON object", service)
	}
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("decode %s batch response: %w", service, err)
	}
	return nil
}
