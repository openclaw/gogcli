package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/classroom/v1"

	"github.com/steipete/gogcli/internal/timeparse"
)

func wrapClassroomError(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	if strings.Contains(errStr, "accessNotConfigured") ||
		strings.Contains(errStr, "Classroom API has not been used") {
		return fmt.Errorf("classroom API is not enabled; enable it at: https://console.developers.google.com/apis/api/classroom.googleapis.com/overview (%w)", err)
	}
	if strings.Contains(errStr, "insufficientPermissions") ||
		strings.Contains(errStr, "insufficient authentication scopes") {
		return fmt.Errorf("insufficient permissions for Classroom API; re-authenticate with: gog auth add <account> --services classroom\n\nOriginal error: %w", err)
	}
	return err
}

func formatClassroomDate(d *classroom.Date) string {
	if d == nil || d.Year == 0 || d.Month == 0 || d.Day == 0 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

func formatClassroomTime(t *classroom.TimeOfDay) string {
	if t == nil {
		return ""
	}
	if t.Seconds != 0 || t.Nanos != 0 {
		return fmt.Sprintf("%02d:%02d:%02d", t.Hours, t.Minutes, t.Seconds)
	}
	return fmt.Sprintf("%02d:%02d", t.Hours, t.Minutes)
}

func formatClassroomDue(d *classroom.Date, t *classroom.TimeOfDay) string {
	date := formatClassroomDate(d)
	clock := formatClassroomTime(t)
	if date == "" && clock == "" {
		return ""
	}
	if clock == "" {
		return date
	}
	if date == "" {
		return clock
	}
	return fmt.Sprintf("%s %s", date, clock)
}

func parseClassroomDate(value string) (*classroom.Date, error) {
	parsed, err := timeparse.ParseDate(value)
	if err != nil {
		if errors.Is(err, timeparse.ErrEmptyDate) {
			return nil, err
		}
		return nil, fmt.Errorf("invalid date %q (expected YYYY-MM-DD)", strings.TrimSpace(value))
	}
	return &classroom.Date{Year: int64(parsed.Year()), Month: int64(parsed.Month()), Day: int64(parsed.Day())}, nil
}

func parseClassroomTime(value string) (*classroom.TimeOfDay, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty time")
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		parsed, err = time.Parse("15:04:05", value)
		if err != nil {
			return nil, fmt.Errorf("invalid time %q (expected HH:MM or HH:MM:SS)", value)
		}
	}
	return &classroom.TimeOfDay{
		Hours:   int64(parsed.Hour()),
		Minutes: int64(parsed.Minute()),
		Seconds: int64(parsed.Second()),
	}, nil
}

func parseClassroomDue(value string) (*classroom.Date, *classroom.TimeOfDay, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil, nil
	}

	parsed, err := timeparse.ParseDateTimeOrDate(value, time.UTC)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid due value %q (expected RFC3339 or YYYY-MM-DD [HH:MM])", value)
	}

	utc := parsed.Time.UTC()
	date := &classroom.Date{Year: int64(utc.Year()), Month: int64(utc.Month()), Day: int64(utc.Day())}
	if !parsed.HasTime {
		return date, nil, nil
	}

	clock := &classroom.TimeOfDay{
		Hours:   int64(utc.Hour()),
		Minutes: int64(utc.Minute()),
		Seconds: int64(utc.Second()),
	}
	return date, clock, nil
}

func updateMask(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, ",")
}

func normalizeAssigneeMode(mode string, addStudents, removeStudents []string) (string, *classroom.ModifyIndividualStudentsOptions, error) {
	mode = strings.TrimSpace(mode)
	hasChanges := len(addStudents) > 0 || len(removeStudents) > 0
	if hasChanges {
		if mode == "" {
			mode = "INDIVIDUAL_STUDENTS"
		}
		mode = strings.ToUpper(mode)
		if mode != "INDIVIDUAL_STUDENTS" {
			return "", nil, fmt.Errorf("assignee mode must be INDIVIDUAL_STUDENTS when modifying individual students")
		}
		return mode, &classroom.ModifyIndividualStudentsOptions{
			AddStudentIds:    addStudents,
			RemoveStudentIds: removeStudents,
		}, nil
	}
	if mode == "" {
		return "", nil, nil
	}
	return strings.ToUpper(mode), nil, nil
}

func parseFloat(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty value")
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", value)
	}
	return parsed, nil
}

func profileName(profile *classroom.UserProfile) string {
	if profile == nil || profile.Name == nil {
		return ""
	}
	if profile.Name.FullName != "" {
		return profile.Name.FullName
	}
	return strings.TrimSpace(strings.TrimSpace(profile.Name.GivenName + " " + profile.Name.FamilyName))
}

func profileEmail(profile *classroom.UserProfile) string {
	if profile == nil {
		return ""
	}
	return profile.EmailAddress
}

func formatFloatValue(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

func scanClassroomTopicPages[T any](topicFilter string, pageToken string, scanPages int, fetch func(string) ([]T, string, error), topicID func(T) string) ([]T, string, error) {
	topicFilter = strings.TrimSpace(topicFilter)
	if scanPages <= 0 {
		scanPages = 1
	}

	for page := 0; ; page++ {
		items, nextPageToken, err := fetch(pageToken)
		if err != nil {
			return nil, "", err
		}

		if topicFilter == "" {
			return items, nextPageToken, nil
		}

		filtered := make([]T, 0, len(items))
		for _, item := range items {
			if topicID(item) == topicFilter {
				filtered = append(filtered, item)
			}
		}

		if len(filtered) > 0 {
			return filtered, nextPageToken, nil
		}

		if nextPageToken == "" || page+1 >= scanPages {
			return filtered, nextPageToken, nil
		}

		pageToken = nextPageToken
	}
}
