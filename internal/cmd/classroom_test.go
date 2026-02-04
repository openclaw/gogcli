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

	"github.com/steipete/gogcli/internal/ui"
)

// testCtxWithCurrentStdout creates a context with a UI that uses the current os.Stdout.
// This must be called inside captureStdout to capture output that goes through u.Out().Printf.
func testCtxWithCurrentStdout(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

// stubClassroomService creates a test server and returns a classroom service that uses it.
// The handler is expected to handle all classroom API routes.
func stubClassroomService(t *testing.T, handler http.Handler) (*classroom.Service, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	svc, err := classroom.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		srv.Close()
		t.Fatalf("NewService: %v", err)
	}
	return svc, srv.Close
}

// classroomTestHandler returns a comprehensive mock handler for classroom API endpoints.
func classroomTestHandler(t *testing.T) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON := func(data any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(data)
		}

		path := r.URL.Path
		switch {
		// Guardian invitations
		case strings.Contains(path, "/userProfiles/") && strings.Contains(path, "/guardianInvitations"):
			switch {
			case r.Method == http.MethodGet && strings.Contains(path, "/guardianInvitations/"):
				writeJSON(map[string]any{
					"invitationId":        "gi1",
					"studentId":           "s1",
					"invitedEmailAddress": "guardian@example.com",
					"state":               "PENDING",
					"creationTime":        "2024-01-01T00:00:00Z",
				})
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"guardianInvitations": []map[string]any{{
						"invitationId":        "gi1",
						"invitedEmailAddress": "guardian@example.com",
						"state":               "PENDING",
						"creationTime":        "2024-01-01T00:00:00Z",
					}},
				})
			case r.Method == http.MethodPost:
				writeJSON(map[string]any{
					"invitationId":        "gi2",
					"studentId":           "s1",
					"invitedEmailAddress": "new@example.com",
					"state":               "PENDING",
				})
			default:
				http.NotFound(w, r)
			}
		// Guardians
		case strings.Contains(path, "/userProfiles/") && strings.Contains(path, "/guardians"):
			switch {
			case r.Method == http.MethodGet && strings.Contains(path, "/guardians/"):
				writeJSON(map[string]any{
					"guardianId": "g1",
					"studentId":  "s1",
					"guardianProfile": map[string]any{
						"emailAddress": "guardian@example.com",
						"name":         map[string]any{"fullName": "Guardian One"},
					},
				})
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"guardians": []map[string]any{{
						"guardianId": "g1",
						"studentId":  "s1",
						"guardianProfile": map[string]any{
							"emailAddress": "guardian@example.com",
							"name":         map[string]any{"fullName": "Guardian One"},
						},
					}},
				})
			case r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				http.NotFound(w, r)
			}
		// User profile
		case strings.Contains(path, "/userProfiles/") && r.Method == http.MethodGet:
			writeJSON(map[string]any{
				"id":              "u1",
				"emailAddress":    "me@example.com",
				"name":            map[string]any{"fullName": "User One"},
				"verifiedTeacher": true,
			})
		// Invitations
		case strings.Contains(path, "/invitations"):
			switch {
			case strings.Contains(path, ":accept") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"accepted": true})
			case strings.Contains(path, "/invitations/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{"id": "i1", "courseId": "c1", "userId": "u1", "role": "STUDENT"})
			case strings.Contains(path, "/invitations/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"invitations": []map[string]any{{"id": "i1", "courseId": "c1", "userId": "u1", "role": "STUDENT"}},
				})
			case r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "i2", "courseId": "c1", "userId": "u2", "role": "TEACHER"})
			default:
				http.NotFound(w, r)
			}
		// Course work materials
		case strings.Contains(path, "/courseWorkMaterials"):
			switch {
			case strings.Contains(path, "/courseWorkMaterials/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"id":          "m1",
					"title":       "Material 1",
					"description": "Material description",
					"state":       "PUBLISHED",
					"topicId":     "t1",
					"updateTime":  "2024-01-01T00:00:00Z",
				})
			case strings.Contains(path, "/courseWorkMaterials/") && r.Method == http.MethodPatch:
				writeJSON(map[string]any{"id": "m1", "title": "Updated Material", "state": "PUBLISHED"})
			case strings.Contains(path, "/courseWorkMaterials/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			case strings.Contains(path, "/courseWorkMaterials") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "m3", "title": "New Material", "state": "DRAFT"})
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"courseWorkMaterial": []map[string]any{{
						"id":         "m1",
						"title":      "Material 1",
						"state":      "PUBLISHED",
						"topicId":    "t1",
						"updateTime": "2024-01-01T00:00:00Z",
					}},
				})
			default:
				http.NotFound(w, r)
			}
		// Coursework
		case strings.Contains(path, "/courseWork"):
			switch {
			case strings.Contains(path, ":modifyAssignees") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "cw1", "assigneeMode": "ALL_STUDENTS"})
			case strings.Contains(path, "/courseWork/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"id":            "cw1",
					"title":         "Assignment 1",
					"description":   "Do this work",
					"state":         "PUBLISHED",
					"workType":      "ASSIGNMENT",
					"topicId":       "t1",
					"maxPoints":     100,
					"alternateLink": "https://classroom.google.com/cw1",
					"dueDate":       map[string]int64{"year": 2024, "month": 12, "day": 25},
					"dueTime":       map[string]int64{"hours": 23, "minutes": 59},
					"scheduledTime": "2024-12-01T00:00:00Z",
				})
			case strings.Contains(path, "/courseWork/") && r.Method == http.MethodPatch:
				writeJSON(map[string]any{"id": "cw1", "title": "Updated Work", "state": "PUBLISHED"})
			case strings.Contains(path, "/courseWork/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			case strings.Contains(path, "/courseWork") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "cw3", "title": "New Work", "state": "DRAFT"})
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"courseWork": []map[string]any{{
						"id":       "cw1",
						"title":    "Assignment 1",
						"state":    "PUBLISHED",
						"workType": "ASSIGNMENT",
						"topicId":  "t1",
					}},
				})
			default:
				http.NotFound(w, r)
			}
		// Announcements
		case strings.Contains(path, "/announcements"):
			switch {
			case strings.Contains(path, ":modifyAssignees") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "a1", "assigneeMode": "INDIVIDUAL_STUDENTS"})
			case strings.Contains(path, "/announcements/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"id":            "a1",
					"text":          "Hello class!",
					"state":         "PUBLISHED",
					"scheduledTime": "2024-01-01T00:00:00Z",
					"alternateLink": "https://classroom.google.com/a1",
				})
			case strings.Contains(path, "/announcements/") && r.Method == http.MethodPatch:
				writeJSON(map[string]any{"id": "a1", "state": "PUBLISHED"})
			case strings.Contains(path, "/announcements/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			case strings.Contains(path, "/announcements") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "a3", "state": "DRAFT"})
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"announcements": []map[string]any{{
						"id":         "a1",
						"text":       "Hello class!",
						"state":      "PUBLISHED",
						"updateTime": "2024-01-01T00:00:00Z",
					}},
				})
			default:
				http.NotFound(w, r)
			}
		// Topics
		case strings.Contains(path, "/topics"):
			switch {
			case strings.Contains(path, "/topics/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{"topicId": "t1", "name": "Topic 1", "courseId": "c1"})
			case strings.Contains(path, "/topics/") && r.Method == http.MethodPatch:
				writeJSON(map[string]any{"topicId": "t1", "name": "Updated Topic"})
			case strings.Contains(path, "/topics/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			case strings.Contains(path, "/topics") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"topicId": "t3", "name": "New Topic"})
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"topic": []map[string]any{{"topicId": "t1", "name": "Topic 1"}},
				})
			default:
				http.NotFound(w, r)
			}
		// Students
		case strings.Contains(path, "/students"):
			switch {
			case strings.Contains(path, "/students/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"userId": "s1",
					"profile": map[string]any{
						"emailAddress": "student@example.com",
						"name":         map[string]any{"fullName": "Student One"},
					},
					"studentWorkFolder": map[string]any{"id": "folder1"},
				})
			case strings.Contains(path, "/students/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			case strings.Contains(path, "/students") && r.Method == http.MethodPost:
				writeJSON(map[string]any{
					"userId": "s1",
					"profile": map[string]any{
						"emailAddress": "student@example.com",
						"name":         map[string]any{"fullName": "Student One"},
					},
				})
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"students": []map[string]any{{
						"userId": "s1",
						"profile": map[string]any{
							"emailAddress": "student@example.com",
							"name":         map[string]any{"fullName": "Student One"},
						},
					}},
				})
			default:
				http.NotFound(w, r)
			}
		// Teachers
		case strings.Contains(path, "/teachers"):
			switch {
			case strings.Contains(path, "/teachers/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"userId": "t1",
					"profile": map[string]any{
						"emailAddress": "teacher@example.com",
						"name":         map[string]any{"fullName": "Teacher One"},
					},
				})
			case strings.Contains(path, "/teachers/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			case strings.Contains(path, "/teachers") && r.Method == http.MethodPost:
				writeJSON(map[string]any{
					"userId": "t1",
					"profile": map[string]any{
						"emailAddress": "teacher@example.com",
						"name":         map[string]any{"fullName": "Teacher One"},
					},
				})
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"teachers": []map[string]any{{
						"userId": "t1",
						"profile": map[string]any{
							"emailAddress": "teacher@example.com",
							"name":         map[string]any{"fullName": "Teacher One"},
						},
					}},
				})
			default:
				http.NotFound(w, r)
			}
		// Courses list
		case strings.HasSuffix(path, "/courses") && r.Method == http.MethodGet:
			writeJSON(map[string]any{
				"courses": []map[string]any{{
					"id":          "c1",
					"name":        "Biology 101",
					"section":     "Section A",
					"courseState": "ACTIVE",
					"ownerId":     "me",
				}},
			})
		// Courses create
		case strings.HasSuffix(path, "/courses") && r.Method == http.MethodPost:
			writeJSON(map[string]any{
				"id":             "c2",
				"name":           "New Course",
				"courseState":    "ACTIVE",
				"ownerId":        "me",
				"enrollmentCode": "abc123",
			})
		// Course operations
		case strings.Contains(path, "/courses/"):
			switch r.Method {
			case http.MethodGet:
				writeJSON(map[string]any{
					"id":                 "c1",
					"name":               "Biology 101",
					"section":            "Section A",
					"descriptionHeading": "Welcome",
					"description":        "Biology course",
					"room":               "Room 101",
					"courseState":        "ACTIVE",
					"ownerId":            "me",
					"enrollmentCode":     "abc123",
					"alternateLink":      "https://classroom.google.com/c/c1",
				})
			case http.MethodPatch:
				writeJSON(map[string]any{"id": "c1", "name": "Updated Course", "courseState": "ARCHIVED"})
			case http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	})
}

// Tests for ClassroomCoursesListCmd
func TestClassroomCoursesListCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCoursesListCmd{}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Biology 101") {
		t.Errorf("expected course name in output, got %q", out)
	}
	if !strings.Contains(out, "ACTIVE") {
		t.Errorf("expected course state in output, got %q", out)
	}
}

func TestClassroomCoursesListCmd_Run_EmptyList(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"courses": []any{}})
	})
	svc, cleanup := stubClassroomService(t, handler)
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCoursesListCmd{}
	flags := &RootFlags{Account: "a@b.com"}

	// Empty list should not error - just verify no crash
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// Tests for ClassroomCoursesGetCmd
func TestClassroomCoursesGetCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCoursesGetCmd{CourseID: "c1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	for _, expected := range []string{"c1", "Biology 101", "Section A", "Room 101"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in output, got %q", expected, out)
		}
	}
}

func TestClassroomCoursesGetCmd_Run_EmptyCourseID(t *testing.T) {
	cmd := &ClassroomCoursesGetCmd{CourseID: ""}
	flags := &RootFlags{Account: "a@b.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty courseId")
	}
	if !strings.Contains(err.Error(), "empty courseId") {
		t.Errorf("expected 'empty courseId' error, got %v", err)
	}
}

// Tests for ClassroomCoursesCreateCmd
func TestClassroomCoursesCreateCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCoursesCreateCmd{
		Name:    "New Course",
		OwnerID: "me",
		State:   "active",
	}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "c2") {
		t.Errorf("expected course id in output, got %q", out)
	}
}

func TestClassroomCoursesCreateCmd_Run_EmptyName(t *testing.T) {
	cmd := &ClassroomCoursesCreateCmd{Name: "", OwnerID: "me"}
	flags := &RootFlags{Account: "a@b.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

// Tests for ClassroomCoursesUpdateCmd
func TestClassroomCoursesUpdateCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCoursesUpdateCmd{
		CourseID: "c1",
		Name:     "Updated Biology",
		State:    "archived",
	}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "c1") {
		t.Errorf("expected course id in output, got %q", out)
	}
}

func TestClassroomCoursesUpdateCmd_Run_NoUpdates(t *testing.T) {
	cmd := &ClassroomCoursesUpdateCmd{CourseID: "c1"}
	flags := &RootFlags{Account: "a@b.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for no updates")
	}
	if !strings.Contains(err.Error(), "no updates") {
		t.Errorf("expected 'no updates' error, got %v", err)
	}
}

// Tests for ClassroomCoursesJoinCmd
func TestClassroomCoursesJoinCmd_Run_Student(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCoursesJoinCmd{
		CourseID:       "c1",
		Role:           "student",
		UserID:         "me",
		EnrollmentCode: "abc",
	}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "s1") {
		t.Errorf("expected user id in output, got %q", out)
	}
}

func TestClassroomCoursesJoinCmd_Run_Teacher(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCoursesJoinCmd{
		CourseID: "c1",
		Role:     "teacher",
		UserID:   "me",
	}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "t1") {
		t.Errorf("expected user id in output, got %q", out)
	}
}

func TestClassroomCoursesJoinCmd_Run_InvalidRole(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCoursesJoinCmd{
		CourseID: "c1",
		Role:     "invalid",
		UserID:   "me",
	}
	flags := &RootFlags{Account: "a@b.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
	if !strings.Contains(err.Error(), "invalid role") {
		t.Errorf("expected 'invalid role' error, got %v", err)
	}
}

// Tests for ClassroomCoursesLeaveCmd
func TestClassroomCoursesLeaveCmd_Run_Student(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCoursesLeaveCmd{
		CourseID: "c1",
		Role:     "student",
		UserID:   "s1",
	}
	flags := &RootFlags{Account: "a@b.com", Force: true}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "removed") && !strings.Contains(out, "true") {
		t.Errorf("expected removal confirmation, got %q", out)
	}
}

// Tests for ClassroomCoursesURLCmd
func TestClassroomCoursesURLCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCoursesURLCmd{CourseIDs: []string{"c1"}}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "classroom.google.com") {
		t.Errorf("expected classroom URL in output, got %q", out)
	}
}

func TestClassroomCoursesURLCmd_Run_Empty(t *testing.T) {
	cmd := &ClassroomCoursesURLCmd{CourseIDs: []string{}}
	flags := &RootFlags{Account: "a@b.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing courseId")
	}
}

// Tests for ClassroomCourseworkListCmd
func TestClassroomCourseworkListCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCourseworkListCmd{CourseID: "c1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Assignment 1") {
		t.Errorf("expected coursework title in output, got %q", out)
	}
}

// Tests for ClassroomCourseworkGetCmd
func TestClassroomCourseworkGetCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCourseworkGetCmd{CourseID: "c1", CourseworkID: "cw1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	for _, expected := range []string{"cw1", "Assignment 1", "ASSIGNMENT", "100"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in output, got %q", expected, out)
		}
	}
}

func TestClassroomCourseworkGetCmd_Run_Validation(t *testing.T) {
	tests := []struct {
		name     string
		courseID string
		workID   string
		wantErr  string
	}{
		{"empty courseId", "", "cw1", "empty courseId"},
		{"empty courseworkId", "c1", "", "empty courseworkId"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &ClassroomCourseworkGetCmd{CourseID: tt.courseID, CourseworkID: tt.workID}
			err := cmd.Run(testContext(t), &RootFlags{Account: "a@b.com"})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// Tests for ClassroomCourseworkCreateCmd
func TestClassroomCourseworkCreateCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCourseworkCreateCmd{
		CourseID:    "c1",
		Title:       "New Assignment",
		Description: "Do your homework",
		WorkType:    "assignment",
		MaxPoints:   100,
	}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "cw3") {
		t.Errorf("expected coursework id in output, got %q", out)
	}
}

func TestClassroomCourseworkCreateCmd_Run_DueTimeWithoutDate(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomCourseworkCreateCmd{
		CourseID: "c1",
		Title:    "Assignment",
		DueTime:  "14:00",
	}
	flags := &RootFlags{Account: "a@b.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for due time without date")
	}
	if !strings.Contains(err.Error(), "due time requires") {
		t.Errorf("expected 'due time requires' error, got %v", err)
	}
}

// Tests for ClassroomCourseworkAssigneesCmd
func TestClassroomCourseworkAssigneesCmd_Run_NoChanges(t *testing.T) {
	cmd := &ClassroomCourseworkAssigneesCmd{
		CourseID:     "c1",
		CourseworkID: "cw1",
	}
	flags := &RootFlags{Account: "a@b.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for no assignee changes")
	}
}

// Tests for ClassroomGuardiansListCmd
func TestClassroomGuardiansListCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomGuardiansListCmd{StudentID: "s1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "guardian@example.com") {
		t.Errorf("expected guardian email in output, got %q", out)
	}
}

func TestClassroomGuardiansListCmd_Run_EmptyStudentID(t *testing.T) {
	cmd := &ClassroomGuardiansListCmd{StudentID: ""}
	err := cmd.Run(testContext(t), &RootFlags{Account: "a@b.com"})
	if err == nil {
		t.Fatal("expected error for empty studentId")
	}
}

// Tests for ClassroomGuardiansGetCmd
func TestClassroomGuardiansGetCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomGuardiansGetCmd{StudentID: "s1", GuardianID: "g1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	for _, expected := range []string{"g1", "s1", "guardian@example.com"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in output, got %q", expected, out)
		}
	}
}

// Tests for ClassroomGuardiansDeleteCmd
func TestClassroomGuardiansDeleteCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomGuardiansDeleteCmd{StudentID: "s1", GuardianID: "g1"}
	flags := &RootFlags{Account: "a@b.com", Force: true}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "deleted") {
		t.Errorf("expected 'deleted' in output, got %q", out)
	}
}

// Tests for ClassroomGuardianInvitesListCmd
func TestClassroomGuardianInvitesListCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomGuardianInvitesListCmd{StudentID: "s1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "guardian@example.com") {
		t.Errorf("expected guardian email in output, got %q", out)
	}
}

// Tests for ClassroomGuardianInvitesGetCmd
func TestClassroomGuardianInvitesGetCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomGuardianInvitesGetCmd{StudentID: "s1", InvitationID: "gi1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	for _, expected := range []string{"gi1", "guardian@example.com", "PENDING"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in output, got %q", expected, out)
		}
	}
}

// Tests for ClassroomGuardianInvitesCreateCmd
func TestClassroomGuardianInvitesCreateCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomGuardianInvitesCreateCmd{StudentID: "s1", Email: "new@example.com"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "gi2") {
		t.Errorf("expected invitation id in output, got %q", out)
	}
}

// Tests for ClassroomInvitationsListCmd
func TestClassroomInvitationsListCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomInvitationsListCmd{CourseID: "c1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "STUDENT") {
		t.Errorf("expected role in output, got %q", out)
	}
}

// Tests for ClassroomInvitationsGetCmd
func TestClassroomInvitationsGetCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomInvitationsGetCmd{InvitationID: "i1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	for _, expected := range []string{"i1", "c1", "u1", "STUDENT"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in output, got %q", expected, out)
		}
	}
}

// Tests for ClassroomInvitationsCreateCmd
func TestClassroomInvitationsCreateCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomInvitationsCreateCmd{
		CourseID: "c1",
		UserID:   "u2",
		Role:     "teacher",
	}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "i2") {
		t.Errorf("expected invitation id in output, got %q", out)
	}
}

func TestClassroomInvitationsCreateCmd_Run_Validation(t *testing.T) {
	tests := []struct {
		name     string
		courseID string
		userID   string
		role     string
		wantErr  string
	}{
		{"empty courseId", "", "u1", "teacher", "empty courseId"},
		{"empty userId", "c1", "", "teacher", "empty userId"},
		{"empty role", "c1", "u1", "", "empty role"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &ClassroomInvitationsCreateCmd{CourseID: tt.courseID, UserID: tt.userID, Role: tt.role}
			err := cmd.Run(testContext(t), &RootFlags{Account: "a@b.com"})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// Tests for ClassroomInvitationsAcceptCmd
func TestClassroomInvitationsAcceptCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomInvitationsAcceptCmd{InvitationID: "i1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "accepted") {
		t.Errorf("expected 'accepted' in output, got %q", out)
	}
}

// Tests for ClassroomInvitationsDeleteCmd
func TestClassroomInvitationsDeleteCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomInvitationsDeleteCmd{InvitationID: "i1"}
	flags := &RootFlags{Account: "a@b.com", Force: true}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "deleted") {
		t.Errorf("expected 'deleted' in output, got %q", out)
	}
}

// Tests for ClassroomAnnouncementsListCmd
func TestClassroomAnnouncementsListCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomAnnouncementsListCmd{CourseID: "c1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Hello class!") {
		t.Errorf("expected announcement text in output, got %q", out)
	}
}

// Tests for ClassroomAnnouncementsGetCmd
func TestClassroomAnnouncementsGetCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomAnnouncementsGetCmd{CourseID: "c1", AnnouncementID: "a1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	for _, expected := range []string{"a1", "Hello class!", "PUBLISHED"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in output, got %q", expected, out)
		}
	}
}

// Tests for ClassroomAnnouncementsCreateCmd
func TestClassroomAnnouncementsCreateCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomAnnouncementsCreateCmd{
		CourseID: "c1",
		Text:     "New announcement",
		State:    "draft",
	}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "a3") {
		t.Errorf("expected announcement id in output, got %q", out)
	}
}

// Tests for ClassroomAnnouncementsDeleteCmd
func TestClassroomAnnouncementsDeleteCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomAnnouncementsDeleteCmd{CourseID: "c1", AnnouncementID: "a1"}
	flags := &RootFlags{Account: "a@b.com", Force: true}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "deleted") {
		t.Errorf("expected 'deleted' in output, got %q", out)
	}
}

// Tests for ClassroomMaterialsListCmd
func TestClassroomMaterialsListCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomMaterialsListCmd{CourseID: "c1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Material 1") {
		t.Errorf("expected material title in output, got %q", out)
	}
}

// Tests for ClassroomMaterialsGetCmd
func TestClassroomMaterialsGetCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomMaterialsGetCmd{CourseID: "c1", MaterialID: "m1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	for _, expected := range []string{"m1", "Material 1", "PUBLISHED"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in output, got %q", expected, out)
		}
	}
}

// Tests for ClassroomMaterialsCreateCmd
func TestClassroomMaterialsCreateCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomMaterialsCreateCmd{
		CourseID: "c1",
		Title:    "New Material",
		State:    "draft",
	}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "m3") {
		t.Errorf("expected material id in output, got %q", out)
	}
}

// Tests for ClassroomMaterialsUpdateCmd
func TestClassroomMaterialsUpdateCmd_Run_NoUpdates(t *testing.T) {
	cmd := &ClassroomMaterialsUpdateCmd{CourseID: "c1", MaterialID: "m1"}
	err := cmd.Run(testContext(t), &RootFlags{Account: "a@b.com"})
	if err == nil {
		t.Fatal("expected error for no updates")
	}
}

// Tests for ClassroomMaterialsDeleteCmd
func TestClassroomMaterialsDeleteCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomMaterialsDeleteCmd{CourseID: "c1", MaterialID: "m1"}
	flags := &RootFlags{Account: "a@b.com", Force: true}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "deleted") {
		t.Errorf("expected 'deleted' in output, got %q", out)
	}
}

// Tests for ClassroomRosterCmd
func TestClassroomRosterCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomRosterCmd{CourseID: "c1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "teacher") || !strings.Contains(out, "student") {
		t.Errorf("expected roles in output, got %q", out)
	}
}

func TestClassroomRosterCmd_Run_StudentsOnly(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomRosterCmd{CourseID: "c1", Students: true}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if strings.Contains(out, "Teacher One") {
		t.Errorf("expected no teachers in output, got %q", out)
	}
}

// Tests for ClassroomStudentsListCmd
func TestClassroomStudentsListCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomStudentsListCmd{CourseID: "c1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Student One") {
		t.Errorf("expected student name in output, got %q", out)
	}
}

// Tests for ClassroomStudentsGetCmd
func TestClassroomStudentsGetCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomStudentsGetCmd{CourseID: "c1", UserID: "s1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	for _, expected := range []string{"s1", "student@example.com", "folder1"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in output, got %q", expected, out)
		}
	}
}

// Tests for ClassroomTeachersListCmd
func TestClassroomTeachersListCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomTeachersListCmd{CourseID: "c1"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Teacher One") {
		t.Errorf("expected teacher name in output, got %q", out)
	}
}

// Tests for ClassroomProfileGetCmd
func TestClassroomProfileGetCmd_Run_Text(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomProfileGetCmd{UserID: "me"}
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	for _, expected := range []string{"u1", "me@example.com", "User One", "true"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in output, got %q", expected, out)
		}
	}
}

func TestClassroomProfileGetCmd_Run_DefaultUser(t *testing.T) {
	origNew := newClassroomService
	t.Cleanup(func() { newClassroomService = origNew })

	svc, cleanup := stubClassroomService(t, classroomTestHandler(t))
	defer cleanup()
	newClassroomService = func(context.Context, string) (*classroom.Service, error) { return svc, nil }

	cmd := &ClassroomProfileGetCmd{} // empty UserID should default to "me"
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testCtxWithCurrentStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "me@example.com") {
		t.Errorf("expected profile output, got %q", out)
	}
}

// Test helper function coverage
func TestTruncateClassroomText(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"empty string", "", 10, ""},
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"zero max", "hello", 0, "hello"},
		{"negative max", "hello", -1, "hello"},
		{"whitespace trimmed", "  hello  ", 10, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateClassroomText(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateClassroomText(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// Test error cases for missing required account
func TestClassroomCoursesListCmd_Run_MissingAccount(t *testing.T) {
	cmd := &ClassroomCoursesListCmd{}
	err := cmd.Run(testContext(t), &RootFlags{})
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}
