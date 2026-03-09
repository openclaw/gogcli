package cmd

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/api/classroom/v1"
)

func TestWrapClassroomError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantNil  bool
		contains string
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			wantNil: true,
		},
		{
			name:     "accessNotConfigured wraps with enable link",
			err:      errors.New("accessNotConfigured: Classroom API has not been used"),
			contains: "console.developers.google.com",
		},
		{
			name:     "Classroom API has not been used wraps with enable link",
			err:      errors.New("Classroom API has not been used in project"),
			contains: "classroom.googleapis.com",
		},
		{
			name:     "insufficientPermissions wraps with re-auth hint",
			err:      errors.New("insufficientPermissions: Request had insufficient auth"),
			contains: "gog auth add",
		},
		{
			name:     "insufficient authentication scopes wraps with re-auth hint",
			err:      errors.New("insufficient authentication scopes"),
			contains: "--services classroom",
		},
		{
			name:     "other errors pass through",
			err:      errors.New("some other error"),
			contains: "some other error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapClassroomError(tt.err)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil error")
			}
			if tt.contains != "" && !strings.Contains(got.Error(), tt.contains) {
				t.Errorf("error %q does not contain %q", got.Error(), tt.contains)
			}
		})
	}
}

func TestFormatClassroomDate(t *testing.T) {
	tests := []struct {
		name string
		date *classroom.Date
		want string
	}{
		{
			name: "nil date returns empty",
			date: nil,
			want: "",
		},
		{
			name: "zero values return empty",
			date: &classroom.Date{Year: 0, Month: 0, Day: 0},
			want: "",
		},
		{
			name: "partial values return empty",
			date: &classroom.Date{Year: 2024, Month: 0, Day: 15},
			want: "",
		},
		{
			name: "valid date formats correctly",
			date: &classroom.Date{Year: 2024, Month: 3, Day: 15},
			want: "2024-03-15",
		},
		{
			name: "single digit month and day get padded",
			date: &classroom.Date{Year: 2024, Month: 1, Day: 5},
			want: "2024-01-05",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatClassroomDate(tt.date)
			if got != tt.want {
				t.Errorf("formatClassroomDate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatClassroomTime(t *testing.T) {
	tests := []struct {
		name string
		time *classroom.TimeOfDay
		want string
	}{
		{
			name: "nil time returns empty",
			time: nil,
			want: "",
		},
		{
			name: "hours and minutes only",
			time: &classroom.TimeOfDay{Hours: 14, Minutes: 30},
			want: "14:30",
		},
		{
			name: "with seconds",
			time: &classroom.TimeOfDay{Hours: 14, Minutes: 30, Seconds: 45},
			want: "14:30:45",
		},
		{
			name: "with nanos (shows seconds)",
			time: &classroom.TimeOfDay{Hours: 14, Minutes: 30, Nanos: 1},
			want: "14:30:00",
		},
		{
			name: "single digit hours and minutes get padded",
			time: &classroom.TimeOfDay{Hours: 9, Minutes: 5},
			want: "09:05",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatClassroomTime(tt.time)
			if got != tt.want {
				t.Errorf("formatClassroomTime() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatClassroomDue(t *testing.T) {
	tests := []struct {
		name string
		date *classroom.Date
		time *classroom.TimeOfDay
		want string
	}{
		{
			name: "nil date and time returns empty",
			date: nil,
			time: nil,
			want: "",
		},
		{
			name: "date only",
			date: &classroom.Date{Year: 2024, Month: 3, Day: 15},
			time: nil,
			want: "2024-03-15",
		},
		{
			name: "time only",
			date: nil,
			time: &classroom.TimeOfDay{Hours: 14, Minutes: 30},
			want: "14:30",
		},
		{
			name: "date and time",
			date: &classroom.Date{Year: 2024, Month: 3, Day: 15},
			time: &classroom.TimeOfDay{Hours: 14, Minutes: 30},
			want: "2024-03-15 14:30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatClassroomDue(tt.date, tt.time)
			if got != tt.want {
				t.Errorf("formatClassroomDue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseClassroomDate(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
		want    *classroom.Date
	}{
		{
			name:    "empty value errors",
			value:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only errors",
			value:   "   ",
			wantErr: true,
		},
		{
			name:    "invalid format errors",
			value:   "2024/03/15",
			wantErr: true,
		},
		{
			name:    "valid date parses correctly",
			value:   "2024-03-15",
			wantErr: false,
			want:    &classroom.Date{Year: 2024, Month: 3, Day: 15},
		},
		{
			name:    "whitespace trimmed",
			value:   "  2024-03-15  ",
			wantErr: false,
			want:    &classroom.Date{Year: 2024, Month: 3, Day: 15},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseClassroomDate(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Year != tt.want.Year || got.Month != tt.want.Month || got.Day != tt.want.Day {
				t.Errorf("parseClassroomDate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseClassroomTime(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
		want    *classroom.TimeOfDay
	}{
		{
			name:    "empty value errors",
			value:   "",
			wantErr: true,
		},
		{
			name:    "invalid format errors",
			value:   "14:30:45:00",
			wantErr: true,
		},
		{
			name:    "HH:MM format parses",
			value:   "14:30",
			wantErr: false,
			want:    &classroom.TimeOfDay{Hours: 14, Minutes: 30, Seconds: 0},
		},
		{
			name:    "HH:MM:SS format parses",
			value:   "14:30:45",
			wantErr: false,
			want:    &classroom.TimeOfDay{Hours: 14, Minutes: 30, Seconds: 45},
		},
		{
			name:    "whitespace trimmed",
			value:   "  14:30  ",
			wantErr: false,
			want:    &classroom.TimeOfDay{Hours: 14, Minutes: 30, Seconds: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseClassroomTime(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Hours != tt.want.Hours || got.Minutes != tt.want.Minutes || got.Seconds != tt.want.Seconds {
				t.Errorf("parseClassroomTime() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseClassroomDue(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		wantErr  bool
		wantDate *classroom.Date
		wantTime *classroom.TimeOfDay
	}{
		{
			name:     "empty value returns nil",
			value:    "",
			wantErr:  false,
			wantDate: nil,
			wantTime: nil,
		},
		{
			name:     "RFC3339 format parses",
			value:    "2024-03-15T14:30:00Z",
			wantErr:  false,
			wantDate: &classroom.Date{Year: 2024, Month: 3, Day: 15},
			wantTime: &classroom.TimeOfDay{Hours: 14, Minutes: 30, Seconds: 0},
		},
		{
			name:     "YYYY-MM-DD HH:MM format parses",
			value:    "2024-03-15 14:30",
			wantErr:  false,
			wantDate: &classroom.Date{Year: 2024, Month: 3, Day: 15},
			wantTime: &classroom.TimeOfDay{Hours: 14, Minutes: 30, Seconds: 0},
		},
		{
			name:     "YYYY-MM-DDTHH:MM format parses",
			value:    "2024-03-15T14:30",
			wantErr:  false,
			wantDate: &classroom.Date{Year: 2024, Month: 3, Day: 15},
			wantTime: &classroom.TimeOfDay{Hours: 14, Minutes: 30, Seconds: 0},
		},
		{
			name:     "date only parses",
			value:    "2024-03-15",
			wantErr:  false,
			wantDate: &classroom.Date{Year: 2024, Month: 3, Day: 15},
			wantTime: nil,
		},
		{
			name:    "invalid format errors",
			value:   "not-a-date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDate, gotTime, err := parseClassroomDue(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantDate == nil {
				if gotDate != nil {
					t.Errorf("expected nil date, got %+v", gotDate)
				}
			} else {
				if gotDate == nil {
					t.Fatal("expected non-nil date")
					return
				}
				if gotDate.Year != tt.wantDate.Year || gotDate.Month != tt.wantDate.Month || gotDate.Day != tt.wantDate.Day {
					t.Errorf("date = %+v, want %+v", gotDate, tt.wantDate)
				}
			}

			if tt.wantTime == nil {
				if gotTime != nil {
					t.Errorf("expected nil time, got %+v", gotTime)
				}
			} else {
				if gotTime == nil {
					t.Fatal("expected non-nil time")
					return
				}
				if gotTime.Hours != tt.wantTime.Hours || gotTime.Minutes != tt.wantTime.Minutes {
					t.Errorf("time = %+v, want %+v", gotTime, tt.wantTime)
				}
			}
		})
	}
}

func TestUpdateMask(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		want   string
	}{
		{
			name:   "empty slice returns empty",
			fields: []string{},
			want:   "",
		},
		{
			name:   "single field",
			fields: []string{"name"},
			want:   "name",
		},
		{
			name:   "multiple fields joined with comma",
			fields: []string{"name", "description", "state"},
			want:   "name,description,state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateMask(tt.fields)
			if got != tt.want {
				t.Errorf("updateMask() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeAssigneeMode(t *testing.T) {
	tests := []struct {
		name          string
		mode          string
		add           []string
		remove        []string
		wantMode      string
		wantOpts      bool
		wantAdd       []string
		wantRemove    []string
		wantErrSubstr string
	}{
		{
			name:     "no mode or students returns empty",
			wantMode: "",
			wantOpts: false,
		},
		{
			name:     "mode only uppercases",
			mode:     "all_students",
			wantMode: "ALL_STUDENTS",
			wantOpts: false,
		},
		{
			name:       "students default mode",
			add:        []string{"a", "b"},
			remove:     []string{"c"},
			wantMode:   "INDIVIDUAL_STUDENTS",
			wantOpts:   true,
			wantAdd:    []string{"a", "b"},
			wantRemove: []string{"c"},
		},
		{
			name:     "students with explicit mode",
			mode:     "INDIVIDUAL_STUDENTS",
			add:      []string{"a"},
			wantMode: "INDIVIDUAL_STUDENTS",
			wantOpts: true,
			wantAdd:  []string{"a"},
		},
		{
			name:          "students with invalid mode errors",
			mode:          "ALL_STUDENTS",
			add:           []string{"a"},
			wantErrSubstr: "INDIVIDUAL_STUDENTS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMode, gotOpts, err := normalizeAssigneeMode(tt.mode, tt.add, tt.remove)
			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotMode != tt.wantMode {
				t.Errorf("mode = %q, want %q", gotMode, tt.wantMode)
			}
			if (gotOpts != nil) != tt.wantOpts {
				t.Fatalf("opts nil = %v, want %v", gotOpts == nil, !tt.wantOpts)
			}
			if tt.wantOpts {
				if strings.Join(gotOpts.AddStudentIds, ",") != strings.Join(tt.wantAdd, ",") {
					t.Errorf("add = %v, want %v", gotOpts.AddStudentIds, tt.wantAdd)
				}
				if strings.Join(gotOpts.RemoveStudentIds, ",") != strings.Join(tt.wantRemove, ",") {
					t.Errorf("remove = %v, want %v", gotOpts.RemoveStudentIds, tt.wantRemove)
				}
			}
		})
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
		want    float64
	}{
		{
			name:    "empty value errors",
			value:   "",
			wantErr: true,
		},
		{
			name:    "invalid format errors",
			value:   "not-a-number",
			wantErr: true,
		},
		{
			name:    "integer parses",
			value:   "42",
			wantErr: false,
			want:    42.0,
		},
		{
			name:    "decimal parses",
			value:   "3.14",
			wantErr: false,
			want:    3.14,
		},
		{
			name:    "whitespace trimmed",
			value:   "  3.14  ",
			wantErr: false,
			want:    3.14,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFloat(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseFloat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProfileName(t *testing.T) {
	tests := []struct {
		name    string
		profile *classroom.UserProfile
		want    string
	}{
		{
			name:    "nil profile returns empty",
			profile: nil,
			want:    "",
		},
		{
			name:    "nil name returns empty",
			profile: &classroom.UserProfile{Name: nil},
			want:    "",
		},
		{
			name:    "full name preferred",
			profile: &classroom.UserProfile{Name: &classroom.Name{FullName: "John Doe", GivenName: "John", FamilyName: "Doe"}},
			want:    "John Doe",
		},
		{
			name:    "falls back to given + family name",
			profile: &classroom.UserProfile{Name: &classroom.Name{GivenName: "John", FamilyName: "Doe"}},
			want:    "John Doe",
		},
		{
			name:    "handles missing family name",
			profile: &classroom.UserProfile{Name: &classroom.Name{GivenName: "John"}},
			want:    "John",
		},
		{
			name:    "handles missing given name",
			profile: &classroom.UserProfile{Name: &classroom.Name{FamilyName: "Doe"}},
			want:    "Doe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := profileName(tt.profile)
			if got != tt.want {
				t.Errorf("profileName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProfileEmail(t *testing.T) {
	tests := []struct {
		name    string
		profile *classroom.UserProfile
		want    string
	}{
		{
			name:    "nil profile returns empty",
			profile: nil,
			want:    "",
		},
		{
			name:    "returns email address",
			profile: &classroom.UserProfile{EmailAddress: "test@example.com"},
			want:    "test@example.com",
		},
		{
			name:    "empty email returns empty",
			profile: &classroom.UserProfile{EmailAddress: ""},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := profileEmail(tt.profile)
			if got != tt.want {
				t.Errorf("profileEmail() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatFloatValue(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  string
	}{
		{
			name:  "integer value",
			value: 100.0,
			want:  "100",
		},
		{
			name:  "single decimal place",
			value: 85.5,
			want:  "85.5",
		},
		{
			name:  "two decimal places",
			value: 85.75,
			want:  "85.75",
		},
		{
			name:  "trailing zeros removed",
			value: 85.10,
			want:  "85.1",
		},
		{
			name:  "zero",
			value: 0.0,
			want:  "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFloatValue(tt.value)
			if got != tt.want {
				t.Errorf("formatFloatValue() = %q, want %q", got, tt.want)
			}
		})
	}
}
