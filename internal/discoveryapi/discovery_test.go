package discoveryapi

import (
	"strings"
	"testing"

	discovery "google.golang.org/api/discovery/v1"
)

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
