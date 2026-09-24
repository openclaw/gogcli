package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	gapi "google.golang.org/api/googleapi"
)

// Keep field values encoded: SDK structs omit explicit zero values and unknown
// fields, while decoding arbitrary numbers into float64 can lose precision.
func readRawObject(ctx context.Context, client *http.Client, endpoint string, query url.Values, resource string) (map[string]json.RawMessage, error) {
	if query == nil {
		query = make(url.Values)
	}
	query.Set("alt", "json")
	query.Set("prettyPrint", "false")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("create %s read request: %w", resource, err)
	}
	readClient := *client
	readClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := readClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", resource, err)
	}
	defer response.Body.Close()
	if checkErr := gapi.CheckResponse(response); checkErr != nil {
		return nil, checkErr
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", resource, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", resource, err)
	}
	if fields == nil {
		return nil, fmt.Errorf("%s not found", resource)
	}
	return fields, nil
}
