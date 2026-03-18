package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/calendar/v3"
)

type CalendarWorkingLocationCmd struct {
	CalendarID  string `arg:"" name:"calendarId" help:"Calendar ID (default: primary)" default:"primary"`
	From        string `name:"from" required:"" help:"Start date (YYYY-MM-DD)"`
	To          string `name:"to" required:"" help:"End date (YYYY-MM-DD)"`
	Type        string `name:"type" required:"" help:"Location type: home, office, custom"`
	OfficeLabel string `name:"office-label" help:"Office name/label"`
	BuildingId  string `name:"building-id" help:"Building ID"`
	FloorId     string `name:"floor-id" help:"Floor ID"`
	DeskId      string `name:"desk-id" help:"Desk ID"`
	CustomLabel string `name:"custom-label" help:"Custom location label"`
}

func (c *CalendarWorkingLocationCmd) Run(ctx context.Context, flags *RootFlags) error {
	calendarID, err := prepareCalendarID(c.CalendarID, true)
	if err != nil {
		return err
	}
	props, err := c.buildWorkingLocationProperties()
	if err != nil {
		return err
	}

	summary := c.generateSummary()

	event := &calendar.Event{
		Summary:                   summary,
		Start:                     &calendar.EventDateTime{Date: strings.TrimSpace(c.From)},
		End:                       &calendar.EventDateTime{Date: strings.TrimSpace(c.To)},
		EventType:                 eventTypeWorkingLocation,
		Visibility:                visibilityPublic,
		Transparency:              transparencyTransparent,
		WorkingLocationProperties: props,
	}

	if dryRunErr := dryRunExit(ctx, flags, "calendar.working_location", map[string]any{
		"calendar_id": calendarID,
		"event":       event,
	}); dryRunErr != nil {
		return dryRunErr
	}

	mutation, err := newCalendarMutationContext(ctx, flags, calendarID)
	if err != nil {
		return err
	}

	created, err := mutation.insertEvent(ctx, event, calendarInsertOptions{})
	if err != nil {
		return err
	}
	return mutation.writeEvent(ctx, created)
}

func (c *CalendarWorkingLocationCmd) buildWorkingLocationProperties() (*calendar.EventWorkingLocationProperties, error) {
	return buildWorkingLocationProperties(workingLocationInput{
		Type:        c.Type,
		OfficeLabel: c.OfficeLabel,
		BuildingId:  c.BuildingId,
		FloorId:     c.FloorId,
		DeskId:      c.DeskId,
		CustomLabel: c.CustomLabel,
	})
}

func (c *CalendarWorkingLocationCmd) generateSummary() string {
	return workingLocationSummary(workingLocationInput{
		Type:        c.Type,
		OfficeLabel: c.OfficeLabel,
		CustomLabel: c.CustomLabel,
	})
}

type workingLocationInput struct {
	Type        string
	OfficeLabel string
	BuildingId  string
	FloorId     string
	DeskId      string
	CustomLabel string
}

func buildWorkingLocationProperties(input workingLocationInput) (*calendar.EventWorkingLocationProperties, error) {
	locType := strings.TrimSpace(strings.ToLower(input.Type))
	props := &calendar.EventWorkingLocationProperties{}

	switch locType {
	case "home":
		props.Type = "homeOffice"
		props.HomeOffice = map[string]any{}
	case "office":
		props.Type = "officeLocation"
		props.OfficeLocation = &calendar.EventWorkingLocationPropertiesOfficeLocation{
			Label:      strings.TrimSpace(input.OfficeLabel),
			BuildingId: strings.TrimSpace(input.BuildingId),
			FloorId:    strings.TrimSpace(input.FloorId),
			DeskId:     strings.TrimSpace(input.DeskId),
		}
	case "custom":
		if strings.TrimSpace(input.CustomLabel) == "" {
			return nil, fmt.Errorf("--custom-label is required for type=custom")
		}
		props.Type = "customLocation"
		props.CustomLocation = &calendar.EventWorkingLocationPropertiesCustomLocation{
			Label: strings.TrimSpace(input.CustomLabel),
		}
	default:
		return nil, fmt.Errorf("invalid location type: %q (must be home, office, or custom)", locType)
	}

	return props, nil
}

func workingLocationSummary(input workingLocationInput) string {
	locType := strings.TrimSpace(strings.ToLower(input.Type))
	switch locType {
	case "home":
		return "Working from home"
	case "office":
		if strings.TrimSpace(input.OfficeLabel) != "" {
			return fmt.Sprintf("Working from %s", strings.TrimSpace(input.OfficeLabel))
		}
		return "Working from office"
	case "custom":
		return fmt.Sprintf("Working from %s", strings.TrimSpace(input.CustomLabel))
	default:
		return "Working location"
	}
}
