package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/ui"
	"github.com/steipete/gogcli/internal/zoom"
)

type zoomMeetingClient interface {
	CreateMeeting(context.Context, string, zoom.CreateMeetingRequest) (*zoom.Meeting, error)
	DeleteMeeting(context.Context, string) error
}

var newZoomMeetingClient = func(alias string) (zoomMeetingClient, error) {
	creds, err := zoom.LoadCredentials(alias)
	if err != nil {
		return nil, err
	}
	return zoom.NewClient(alias, creds)
}

func createZoomMeetingForEvent(ctx context.Context, event *calendar.Event) (*zoom.Meeting, error) {
	client, err := newZoomMeetingClient("")
	if err != nil {
		return nil, err
	}
	return client.CreateMeeting(ctx, "me", zoomMeetingRequestFromEvent(event))
}

func zoomMeetingRequestFromEvent(event *calendar.Event) zoom.CreateMeetingRequest {
	req := zoom.CreateMeetingRequest{Type: 2}
	if event == nil {
		return req
	}
	req.Topic = strings.TrimSpace(event.Summary)
	req.Agenda = strings.TrimSpace(event.Description)
	if event.Start != nil {
		req.Timezone = strings.TrimSpace(event.Start.TimeZone)
		if strings.TrimSpace(event.Start.DateTime) != "" {
			if start, err := time.Parse(time.RFC3339, event.Start.DateTime); err == nil {
				req.StartTime = start
			}
		}
	}
	if req.Timezone == "" {
		req.Timezone = tzUTC
	}
	req.Duration = eventDurationMinutes(event)
	return req
}

func eventDurationMinutes(event *calendar.Event) int {
	if event == nil || event.Start == nil || event.End == nil {
		return 0
	}
	start, startErr := time.Parse(time.RFC3339, event.Start.DateTime)
	end, endErr := time.Parse(time.RFC3339, event.End.DateTime)
	if startErr != nil || endErr != nil || !end.After(start) {
		return 0
	}
	return int(end.Sub(start).Round(time.Minute) / time.Minute)
}

func cancelZoomMeeting(ctx context.Context, meetingID, action string) error {
	client, err := newZoomMeetingClient("")
	if err != nil {
		return err
	}
	logZoomAudit(meetingID, action)
	return client.DeleteMeeting(ctx, meetingID)
}

func zoomMeetingID(meeting *zoom.Meeting) string {
	if meeting == nil {
		return ""
	}
	if meeting.ID != 0 {
		return fmt.Sprintf("%d", meeting.ID)
	}
	return strings.TrimSpace(meeting.UUID)
}

func logZoomAudit(meetingID, action string) {
	fmt.Fprintf(os.Stderr, "[zoom] meeting=%s action=%s ts=%s cmd=%s\n",
		meetingID,
		action,
		time.Now().UTC().Format(time.RFC3339),
		os.Args[0],
	)
}

var zoomMeetingIDPath = regexp.MustCompile(`/j/(\d+)`)

func extractZoomMeetingID(event *calendar.Event) (id string, ok bool) {
	if event == nil {
		return "", false
	}
	// Primary path: gog-managed Zoom block in the event description carries
	// the meeting ID in its start marker. This is the shape gog itself writes.
	if id, ok = extractZoomMeetingIDFromDescription(event.Description); ok {
		return id, true
	}
	// Legacy / interoperability path: events created by the Zoom for Google
	// Workspace add-on (or any tool that populated conferenceData directly)
	// expose the meeting ID via the join URL or addOn parameters.
	if event.ConferenceData == nil {
		return "", false
	}
	for _, ep := range event.ConferenceData.EntryPoints {
		if ep == nil {
			continue
		}
		u, err := url.Parse(strings.TrimSpace(ep.Uri))
		if err != nil {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if !strings.HasSuffix(host, "zoom.us") && !strings.HasSuffix(host, "zoomgov.com") {
			continue
		}
		m := zoomMeetingIDPath.FindStringSubmatch(u.Path)
		if len(m) == 2 {
			return m[1], true
		}
	}
	if params := event.ConferenceData.Parameters; params != nil && params.AddOnParameters != nil {
		if uuid := strings.TrimSpace(params.AddOnParameters.Parameters["meetingUuid"]); uuid != "" {
			return uuid, true
		}
	}
	return "", false
}

func eventConferenceProvider(event *calendar.Event) string {
	if event == nil {
		return ""
	}
	// Description-mode Zoom block takes precedence: gog writes its Zoom info
	// here and never sets conferenceData on the Zoom path.
	if descriptionHasZoomBlock(event.Description) {
		return conferenceProviderZoom
	}
	if event.ConferenceData == nil {
		if strings.TrimSpace(eventHangoutLink(event)) != "" {
			return conferenceProviderMeet
		}
		return ""
	}
	if isZoomConferenceData(event.ConferenceData) {
		return conferenceProviderZoom
	}
	if strings.TrimSpace(event.HangoutLink) != "" {
		return conferenceProviderMeet
	}
	if sol := event.ConferenceData.ConferenceSolution; sol != nil && sol.Key != nil && sol.Key.Type == "hangoutsMeet" {
		return conferenceProviderMeet
	}
	for _, ep := range event.ConferenceData.EntryPoints {
		if ep == nil {
			continue
		}
		uri := strings.ToLower(strings.TrimSpace(ep.Uri))
		if strings.Contains(uri, "meet.google.com") {
			return conferenceProviderMeet
		}
		if strings.Contains(uri, "zoom.us") || strings.Contains(uri, "zoomgov.com") {
			return conferenceProviderZoom
		}
	}
	if eventHasConferenceLink(event) {
		return "other"
	}
	return ""
}

func eventHangoutLink(event *calendar.Event) string {
	if event == nil {
		return ""
	}
	return event.HangoutLink
}

func isZoomConferenceData(data *calendar.ConferenceData) bool {
	if data == nil {
		return false
	}
	if sol := data.ConferenceSolution; sol != nil {
		if strings.EqualFold(strings.TrimSpace(sol.Name), "Zoom Meeting") {
			return true
		}
	}
	for _, ep := range data.EntryPoints {
		if ep == nil {
			continue
		}
		uri := strings.ToLower(strings.TrimSpace(ep.Uri))
		if strings.Contains(uri, "zoom.us") || strings.Contains(uri, "zoomgov.com") {
			return true
		}
	}
	return false
}

func redactEventZoomURLs(event *calendar.Event, includePasswords bool) {
	if includePasswords || event == nil {
		return
	}
	event.Description = redactZoomDescription(event.Description)
	if event.ConferenceData == nil {
		return
	}
	for _, ep := range event.ConferenceData.EntryPoints {
		if ep != nil {
			ep.Uri = zoom.RedactZoomURL(ep.Uri)
		}
	}
}

func redactCalendarEventForOutput(ctx context.Context, event *calendar.Event) {
	redactEventZoomURLs(event, zoomIncludePasswordsFromContext(ctx))
}

func redactCalendarEventsForOutput(ctx context.Context, events []*calendar.Event) {
	for _, event := range events {
		redactCalendarEventForOutput(ctx, event)
	}
}

func warnUnparseableZoomMeeting(u *ui.UI) {
	if u != nil {
		u.Err().Println("warning\tcould not find prior Zoom meeting ID; Calendar conference data will still be replaced")
		return
	}
	fmt.Fprintln(os.Stderr, "warning\tcould not find prior Zoom meeting ID; Calendar conference data will still be replaced")
}
