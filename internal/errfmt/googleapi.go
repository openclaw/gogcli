package errfmt

import (
	"fmt"
	"regexp"
	"strings"

	ggoogleapi "google.golang.org/api/googleapi"
)

type googleAPIHint struct {
	API         string
	DisplayName string
	Service     string
}

var googleAPIHints = []googleAPIHint{
	{API: "admin.googleapis.com", DisplayName: "Admin SDK API", Service: "admin"},
	{API: "appsactivity.googleapis.com", DisplayName: "Drive Activity API", Service: "drive"},
	{API: "classroom.googleapis.com", DisplayName: "Classroom API", Service: "classroom"},
	{API: "cloudidentity.googleapis.com", DisplayName: "Cloud Identity API", Service: "groups"},
	{API: "docs.googleapis.com", DisplayName: "Docs API", Service: "docs"},
	{API: "drive.googleapis.com", DisplayName: "Drive API", Service: "drive"},
	{API: "forms.googleapis.com", DisplayName: "Forms API", Service: "forms"},
	{API: "gmail.googleapis.com", DisplayName: "Gmail API", Service: "gmail"},
	{API: "keep.googleapis.com", DisplayName: "Keep API", Service: "keep"},
	{API: "people.googleapis.com", DisplayName: "People API", Service: "contacts"},
	{API: "script.googleapis.com", DisplayName: "Apps Script API", Service: "appscript"},
	{API: "sheets.googleapis.com", DisplayName: "Sheets API", Service: "sheets"},
	{API: "slides.googleapis.com", DisplayName: "Slides API", Service: "slides"},
	{API: "tasks.googleapis.com", DisplayName: "Tasks API", Service: "tasks"},
}

var apiNamePattern = regexp.MustCompile(`(?i)\b([a-z][a-z0-9-]*\.googleapis\.com)\b`)

func formatGoogleAPIError(gerr *ggoogleapi.Error) string {
	reason := googleAPIErrorReason(gerr)
	if googleAPIIsDisabled(gerr, reason) {
		if hint, ok := googleAPIHintForError(gerr); ok {
			return fmt.Sprintf(
				"%s is not enabled for this OAuth project.\nEnable it at: %s\nThen retry the command. If you enabled it on a different OAuth client, re-authenticate with: gog auth add <account> --services %s",
				hint.DisplayName,
				googleAPIEnableURL(hint.API),
				hint.Service,
			)
		}
	}

	if reason != "" {
		return fmt.Sprintf("Google API error (%d %s): %s", gerr.Code, reason, gerr.Message)
	}

	return fmt.Sprintf("Google API error (%d): %s", gerr.Code, gerr.Message)
}

func googleAPIErrorReason(gerr *ggoogleapi.Error) string {
	if gerr == nil || len(gerr.Errors) == 0 {
		return ""
	}

	return strings.TrimSpace(gerr.Errors[0].Reason)
}

func googleAPIIsDisabled(gerr *ggoogleapi.Error, reason string) bool {
	if gerr == nil || gerr.Code != 403 {
		return false
	}
	reason = strings.ToLower(strings.TrimSpace(reason))
	message := strings.ToLower(gerr.Message)

	return reason == "accessnotconfigured" ||
		strings.Contains(message, "has not been used") ||
		strings.Contains(message, "it is disabled") ||
		strings.Contains(message, "api has not been used")
}

func googleAPIHintForError(gerr *ggoogleapi.Error) (googleAPIHint, bool) {
	if gerr == nil {
		return googleAPIHint{}, false
	}

	message := strings.ToLower(gerr.Message)
	for _, item := range gerr.Errors {
		message += " " + strings.ToLower(item.Message)
	}

	if match := apiNamePattern.FindStringSubmatch(message); len(match) == 2 {
		if hint, ok := googleAPIHintForAPI(match[1]); ok {
			return hint, true
		}
	}

	for _, hint := range googleAPIHints {
		if strings.Contains(message, hint.API) || strings.Contains(message, strings.ToLower(hint.DisplayName)) {
			return hint, true
		}
	}

	return googleAPIHint{}, false
}

func googleAPIHintForAPI(api string) (googleAPIHint, bool) {
	api = strings.ToLower(strings.TrimSpace(api))
	for _, hint := range googleAPIHints {
		if hint.API == api {
			return hint, true
		}
	}

	return googleAPIHint{}, false
}

func googleAPIEnableURL(api string) string {
	return "https://console.developers.google.com/apis/api/" + api + "/overview"
}
