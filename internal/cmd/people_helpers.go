package cmd

import (
	"fmt"
	"strings"
)

const peopleAPIEnableURL = "https://console.developers.google.com/apis/api/people.googleapis.com/overview"

func normalizePeopleResource(raw string) string {
	resource := strings.TrimSpace(raw)
	if resource == "" {
		return ""
	}
	if resource == "me" {
		return "people/me"
	}
	if strings.HasPrefix(resource, "people/") {
		return resource
	}
	return "people/" + resource
}

func wrapPeopleAPIError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "accessNotConfigured") ||
		strings.Contains(err.Error(), "People API has not been used") {
		return fmt.Errorf("people API is not enabled; enable it at: %s (%w)", peopleAPIEnableURL, err)
	}
	return err
}
