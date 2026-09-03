package discoveryapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	discovery "google.golang.org/api/discovery/v1"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestDescriptionFallsBackToServiceHostedDiscovery(t *testing.T) {
	var requests []string
	client := Client{HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.URL.String())
		status := http.StatusNotFound
		body := `{"error":"not found"}`

		if request.URL.String() == "https://meet.googleapis.com/$discovery/rest?version=v2" {
			status = http.StatusOK
			body = `{
				"name":"meet",
				"version":"v2",
				"rootUrl":"https://meet.googleapis.com/",
				"servicePath":"v2/",
				"resources":{"conferenceRecords":{"methods":{"list":{
					"id":"meet.conferenceRecords.list",
					"httpMethod":"GET",
					"path":"conferenceRecords"
				}}}}
			}`
		}

		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}}

	description, err := client.Description(context.Background(), "meet", "v2")
	if err != nil {
		t.Fatal(err)
	}

	method, err := FindMethod(description, "meet.conferenceRecords.list")
	if err != nil {
		t.Fatal(err)
	}

	requestURL, err := BuildURL(description, method, nil)
	if err != nil {
		t.Fatal(err)
	}

	if requestURL != "https://meet.googleapis.com/v2/conferenceRecords" {
		t.Fatalf("URL = %q", requestURL)
	}

	wantRequests := []string{
		"https://www.googleapis.com/discovery/v1/apis/meet/v2/rest",
		"https://meet.googleapis.com/$discovery/rest?version=v2",
	}
	if strings.Join(requests, "\n") != strings.Join(wantRequests, "\n") {
		t.Fatalf("requests = %q, want %q", requests, wantRequests)
	}
}

func TestDescriptionDoesNotFallbackForCustomBaseURL(t *testing.T) {
	for _, baseURL := range []string{"https://discovery.example.test/root", DefaultBaseURL} {
		t.Run(baseURL, func(t *testing.T) {
			requestCount := 0
			client := Client{
				BaseURL: baseURL,
				HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					requestCount++

					return &http.Response{
						StatusCode: http.StatusNotFound,
						Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
						Header:     make(http.Header),
					}, nil
				})},
			}

			_, err := client.Description(context.Background(), "meet", "v2")
			if err == nil {
				t.Fatal("Description unexpectedly succeeded")
			}

			if !errors.Is(err, ErrDiscoveryRequest) {
				t.Fatalf("error = %v, want ErrDiscoveryRequest", err)
			}

			if requestCount != 1 {
				t.Fatalf("request count = %d, want 1", requestCount)
			}
		})
	}
}

func TestDescriptionFallbackRequiresDefaultNotFoundAndSafeServiceName(t *testing.T) {
	for _, test := range []struct {
		name       string
		api        string
		statusCode int
	}{
		{name: "server error", api: "meet", statusCode: http.StatusInternalServerError},
		{name: "unsafe service name", api: "meet.example", statusCode: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			requestCount := 0
			client := Client{HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				requestCount++

				return &http.Response{
					StatusCode: test.statusCode,
					Body:       io.NopCloser(strings.NewReader(`{"error":"request failed"}`)),
					Header:     make(http.Header),
				}, nil
			})}}

			if _, err := client.Description(context.Background(), test.api, "v2"); err == nil {
				t.Fatal("Description unexpectedly succeeded")
			}

			if requestCount != 1 {
				t.Fatalf("request count = %d, want 1", requestCount)
			}
		})
	}
}

func TestMethodsAndBuildURL(t *testing.T) {
	description := &discovery.RestDescription{
		RootUrl:     "https://example.test/",
		ServicePath: "gmail/v1/",
		Parameters:  map[string]discovery.JsonSchema{"fields": {Location: "query"}},
		Resources: map[string]discovery.RestResource{
			"users": {
				Resources: map[string]discovery.RestResource{
					"labels": {
						Methods: map[string]discovery.RestMethod{
							"list": {
								Id:         "gmail.users.labels.list",
								HttpMethod: "GET",
								Path:       "users/{userId}/labels",
								Parameters: map[string]discovery.JsonSchema{
									"userId": {Location: "path", Required: true},
									"max":    {Location: "query"},
								},
							},
						},
					},
				},
			},
		},
	}

	method, err := FindMethod(description, "users.labels.list")
	if err != nil {
		t.Fatal(err)
	}

	requestURL, err := BuildURL(description, method, map[string]any{"userId": "me", "max": 5, "fields": "labels/id"})
	if err != nil {
		t.Fatal(err)
	}

	if requestURL != "https://example.test/gmail/v1/users/me/labels?fields=labels%2Fid&max=5" {
		t.Fatalf("URL = %q", requestURL)
	}

	if _, err := BuildURL(description, method, map[string]any{}); err == nil || !strings.Contains(err.Error(), "userId") {
		t.Fatalf("missing parameter error = %v", err)
	}

	if _, err := BuildURL(description, method, map[string]any{"userId": "me", "typo": true}); err == nil || !strings.Contains(err.Error(), "unknown parameter") {
		t.Fatalf("unknown parameter error = %v", err)
	}
}

func TestValidateGoogleAPIURL(t *testing.T) {
	for _, requestURL := range []string{
		"https://www.googleapis.com/gmail/v1/users/me/labels",
		"https://gmail.googleapis.com/gmail/v1/users/me/labels",
	} {
		if err := ValidateGoogleAPIURL(requestURL); err != nil {
			t.Fatalf("ValidateGoogleAPIURL(%q): %v", requestURL, err)
		}
	}

	for _, requestURL := range []string{
		"http://www.googleapis.com/gmail/v1/users/me/labels",
		"https://googleapis.com.example.test/steal",
		"https://example.test/steal",
	} {
		if err := ValidateGoogleAPIURL(requestURL); err == nil {
			t.Fatalf("ValidateGoogleAPIURL(%q) unexpectedly succeeded", requestURL)
		}
	}
}
