package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/classroom/v1"
	"google.golang.org/api/option"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestClassroomCoursesListCmd_Parse(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantRole string
		wantMax  int64
	}{
		{
			name:     "default values",
			args:     []string{},
			wantRole: "",
			wantMax:  100,
		},
		{
			name:     "teacher role",
			args:     []string{"--role", "teacher"},
			wantRole: "teacher",
			wantMax:  100,
		},
		{
			name:     "student role",
			args:     []string{"--role", "student"},
			wantRole: "student",
			wantMax:  100,
		},
		{
			name:     "custom max",
			args:     []string{"--max", "50"},
			wantRole: "",
			wantMax:  50,
		},
		{
			name:     "both flags",
			args:     []string{"--role", "teacher", "--max", "25"},
			wantRole: "teacher",
			wantMax:  25,
		},
		{
			name:     "alias limit",
			args:     []string{"--limit", "10"},
			wantRole: "",
			wantMax:  10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &ClassroomCoursesListCmd{}
			parser := newTestKong(t, cmd)
			_, err := parser.Parse(tt.args)
			if err != nil {
				t.Errorf("Parse() error = %v", err)
				return
			}
			if cmd.Role != tt.wantRole {
				t.Errorf("Role = %q, want %q", cmd.Role, tt.wantRole)
			}
			if cmd.Max != tt.wantMax {
				t.Errorf("Max = %d, want %d", cmd.Max, tt.wantMax)
			}
		})
	}
}

func TestClassroomCoursesGetCmd_Parse(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		wantID string
	}{
		{
			name:   "valid course id",
			args:   []string{"12345"},
			wantID: "12345",
		},
		{
			name:   "alphanumeric course id",
			args:   []string{"abc123def"},
			wantID: "abc123def",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &ClassroomCoursesGetCmd{}
			parser := newTestKong(t, cmd)
			_, err := parser.Parse(tt.args)
			if err != nil {
				t.Errorf("Parse() error = %v", err)
				return
			}
			if cmd.CourseID != tt.wantID {
				t.Errorf("CourseID = %q, want %q", cmd.CourseID, tt.wantID)
			}
		})
	}
}

func TestClassroomCoursesCreateCmd_Parse(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantName string
	}{
		{
			name:     "valid with name",
			args:     []string{"--name", "Math 101"},
			wantName: "Math 101",
		},
		{
			name:     "all optional fields",
			args:     []string{"--name", "Science", "--section", "Period 1", "--description", "Lab course", "--room", "Room 205"},
			wantName: "Science",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &ClassroomCoursesCreateCmd{}
			parser := newTestKong(t, cmd)
			_, err := parser.Parse(tt.args)
			if err != nil {
				t.Errorf("Parse() error = %v", err)
				return
			}
			if cmd.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", cmd.Name, tt.wantName)
			}
		})
	}
}

func TestClassroomCoursesListCmd_Run_JSON(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		if path != "/courses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"courses": []map[string]any{
				{"id": "c1", "name": "Math 101", "section": "Period 1", "courseState": "ACTIVE"},
				{"id": "c2", "name": "Science", "section": "Period 2", "courseState": "ACTIVE"},
			},
		})
	}))
	defer srv.Close()

	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &ClassroomCoursesListCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	var parsed struct {
		Courses []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Section     string `json:"section"`
			CourseState string `json:"courseState"`
		} `json:"courses"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Courses) != 2 {
		t.Fatalf("expected 2 courses, got %d", len(parsed.Courses))
	}
	if parsed.Courses[0].ID != "c1" || parsed.Courses[0].Name != "Math 101" {
		t.Fatalf("unexpected first course: %#v", parsed.Courses[0])
	}
}

func TestClassroomCoursesListCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		if path != "/courses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"courses": []map[string]any{
				{"id": "c1", "name": "Math 101", "section": "Period 1", "enrollmentCode": "abc123", "courseState": "ACTIVE"},
			},
		})
	}))
	defer srv.Close()

	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &ClassroomCoursesListCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	if !strings.Contains(out, "COURSE_ID") || !strings.Contains(out, "c1") || !strings.Contains(out, "Math 101") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestClassroomCoursesListCmd_Run_Plain(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		if path != "/courses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"courses": []map[string]any{
				{"id": "c1", "name": "Math 101", "section": "Period 1", "enrollmentCode": "abc123", "courseState": "ACTIVE"},
			},
		})
	}))
	defer srv.Close()

	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{Plain: true})

	cmd := &ClassroomCoursesListCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	if !strings.Contains(out, "c1") || !strings.Contains(out, "Math 101") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestClassroomCoursesListCmd_Run_FilterByRole(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	sawTeacherId := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		if path != "/courses" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("teacherId") == "me" {
			sawTeacherId = true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"courses": []map[string]any{
				{"id": "c1", "name": "Math 101", "courseState": "ACTIVE"},
			},
		})
	}))
	defer srv.Close()

	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &ClassroomCoursesListCmd{}
	_ = captureStdout(t, func() {
		if err := runKong(t, cmd, []string{"--role", "teacher"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	if !sawTeacherId {
		t.Fatalf("expected teacherId=me query parameter")
	}
}

func TestClassroomCoursesGetCmd_Run_JSON(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		if path != "/courses/c123" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "c123",
			"name":          "Math 101",
			"section":       "Period 1",
			"room":          "Room 205",
			"ownerId":       "user123",
			"courseState":   "ACTIVE",
			"description":   "Introduction to mathematics",
			"alternateLink": "https://classroom.google.com/c/c123",
		})
	}))
	defer srv.Close()

	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &ClassroomCoursesGetCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{"c123"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed["id"] != "c123" {
		t.Fatalf("unexpected id: %v", parsed["id"])
	}
}

func TestClassroomCoursesGetCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		if path != "/courses/c456" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "c456",
			"name":          "Science",
			"section":       "Period 2",
			"room":          "Lab 101",
			"ownerId":       "user456",
			"courseState":   "ACTIVE",
			"description":   "Science lab course",
			"alternateLink": "https://classroom.google.com/c/c456",
		})
	}))
	defer srv.Close()

	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &ClassroomCoursesGetCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{"c456"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	if !strings.Contains(out, "Name:") || !strings.Contains(out, "Science") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "Room:") || !strings.Contains(out, "Lab 101") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestClassroomCoursesCreateCmd_Run_JSON(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		if path != "/courses" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body["name"] != "New Course" {
			http.Error(w, "expected name New Course", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "new123",
			"name":        "New Course",
			"section":     "Period 3",
			"courseState": "ACTIVE",
		})
	}))
	defer srv.Close()

	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &ClassroomCoursesCreateCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{"--name", "New Course", "--section", "Period 3"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed["id"] != "new123" {
		t.Fatalf("unexpected id: %v", parsed["id"])
	}
}

func TestClassroomCoursesCreateCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		if path != "/courses" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "created123",
			"name": "Created Course",
		})
	}))
	defer srv.Close()

	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &ClassroomCoursesCreateCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{"--name", "Created Course"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	if !strings.Contains(out, "Created course: created123") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestClassroomCoursesURLCmd_Run(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		if path != "/courses/url123" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "url123",
			"alternateLink": "https://classroom.google.com/c/url123",
		})
	}))
	defer srv.Close()

	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &ClassroomCoursesURLCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{"url123"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	if !strings.Contains(out, "https://classroom.google.com/c/url123") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestClassroomCoursesURLCmd_Run_JSON(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		if path != "/courses/json123" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "json123",
			"alternateLink": "https://classroom.google.com/c/json123",
		})
	}))
	defer srv.Close()

	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &ClassroomCoursesURLCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{"json123"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed["url"] != "https://classroom.google.com/c/json123" {
		t.Fatalf("unexpected url: %v", parsed["url"])
	}
}

func TestClassroomCoursesUpdateCmd_Run(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1")
		if path != "/courses/upd123" || r.Method != http.MethodPatch {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "upd123",
			"name": "Updated Name",
		})
	}))
	defer srv.Close()

	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &ClassroomCoursesUpdateCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{"upd123", "--name", "Updated Name"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	if !strings.Contains(out, "Updated course: upd123") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func newTestKong(t *testing.T, cmd any) *kong.Kong {
	t.Helper()
	parser, err := kong.New(
		cmd,
		kong.Writers(io.Discard, io.Discard),
		kong.Exit(func(code int) { panic(exitPanic{code: code}) }),
	)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	return parser
}
