package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/classroom/v1"
	"google.golang.org/api/option"
)

func TestExecute_ClassroomMoreCommands_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON := func(data any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(data)
		}

		path := r.URL.Path
		switch {
		case strings.Contains(path, "/userProfiles/") && strings.Contains(path, "/guardianInvitations"):
			switch {
			case r.Method == http.MethodGet && strings.Contains(path, "/guardianInvitations/"):
				writeJSON(map[string]any{
					"invitationId":        "gi1",
					"invitedEmailAddress": "guardian@example.com",
					"state":               "PENDING",
					"creationTime":        "2024-01-01T00:00:00Z",
				})
				return
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"guardianInvitations": []map[string]any{{
						"invitationId":        "gi1",
						"invitedEmailAddress": "guardian@example.com",
						"state":               "PENDING",
						"creationTime":        "2024-01-01T00:00:00Z",
					}},
					"nextPageToken": "gip",
				})
				return
			case r.Method == http.MethodPost:
				writeJSON(map[string]any{
					"invitationId":        "gi2",
					"invitedEmailAddress": "new@example.com",
					"state":               "PENDING",
				})
				return
			}
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
				return
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
					"nextPageToken": "gp",
				})
				return
			case r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
				return
			}
		case strings.Contains(path, "/userProfiles/") && r.Method == http.MethodGet:
			writeJSON(map[string]any{
				"id":              "u1",
				"emailAddress":    "me@example.com",
				"name":            map[string]any{"fullName": "User One"},
				"verifiedTeacher": true,
			})
			return
		case strings.Contains(path, "/invitations"):
			switch {
			case strings.Contains(path, ":accept") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"accepted": true})
				return
			case strings.Contains(path, "/invitations/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{"id": "i1", "courseId": "c1", "userId": "u1", "role": "STUDENT"})
				return
			case strings.Contains(path, "/invitations/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
				return
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"invitations":   []map[string]any{{"id": "i1", "courseId": "c1", "userId": "u1", "role": "STUDENT"}},
					"nextPageToken": "ip",
				})
				return
			case r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "i2", "courseId": "c1", "userId": "u2", "role": "TEACHER"})
				return
			}
		case strings.Contains(path, "/studentSubmissions"):
			switch {
			case strings.Contains(path, ":turnIn") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"ok": true})
				return
			case strings.Contains(path, ":reclaim") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"ok": true})
				return
			case strings.Contains(path, ":return") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"ok": true})
				return
			case strings.Contains(path, "/studentSubmissions/") && r.Method == http.MethodPatch:
				if got := r.URL.Query().Get("updateMask"); got != "draftGrade,assignedGrade" {
					t.Fatalf("expected updateMask draftGrade,assignedGrade, got %q", got)
				}
				writeJSON(map[string]any{"id": "s1", "draftGrade": 5, "assignedGrade": 10})
				return
			case strings.Contains(path, "/studentSubmissions/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"id":            "s1",
					"userId":        "u1",
					"state":         "TURNED_IN",
					"late":          false,
					"draftGrade":    5,
					"assignedGrade": 10,
					"updateTime":    "2024-01-01T00:00:00Z",
					"alternateLink": "https://classroom.google.com/s1",
				})
				return
			case r.Method == http.MethodGet:
				if got := r.URL.Query().Get("late"); got != "NOT_LATE_ONLY" {
					t.Fatalf("expected late=NOT_LATE_ONLY, got %q", got)
				}
				writeJSON(map[string]any{
					"studentSubmissions": []map[string]any{{
						"id":         "s1",
						"userId":     "u1",
						"state":      "TURNED_IN",
						"late":       false,
						"draftGrade": 0,
					}},
					"nextPageToken": "sp",
				})
				return
			}
		case strings.Contains(path, "/courseWorkMaterials"):
			switch {
			case strings.Contains(path, "/courseWorkMaterials/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{"id": "m1", "title": "Material 1", "state": "PUBLISHED", "topicId": "t1"})
				return
			case strings.Contains(path, "/courseWorkMaterials/") && r.Method == http.MethodPatch:
				if got := r.URL.Query().Get("updateMask"); got != "title,topicId" {
					t.Fatalf("expected updateMask title,topicId, got %q", got)
				}
				writeJSON(map[string]any{"id": "m1", "title": "Updated Material", "state": "PUBLISHED"})
				return
			case strings.Contains(path, "/courseWorkMaterials/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
				return
			case strings.Contains(path, "/courseWorkMaterials") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "m3", "title": "New Material", "state": "DRAFT"})
				return
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"courseWorkMaterial": []map[string]any{{
						"id":      "m1",
						"title":   "Material 1",
						"state":   "PUBLISHED",
						"topicId": "t1",
					}, {
						"id":      "m2",
						"title":   "Material 2",
						"state":   "DRAFT",
						"topicId": "t2",
					}},
					"nextPageToken": "mp",
				})
				return
			}
		case strings.Contains(path, "/courseWork"):
			switch {
			case strings.Contains(path, ":modifyAssignees") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "cw1", "assigneeMode": "ALL_STUDENTS"})
				return
			case strings.Contains(path, "/courseWork/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"id":            "cw1",
					"title":         "Work 1",
					"state":         "PUBLISHED",
					"workType":      "ASSIGNMENT",
					"topicId":       "t1",
					"maxPoints":     10,
					"alternateLink": "https://classroom.google.com/cw1",
				})
				return
			case strings.Contains(path, "/courseWork/") && r.Method == http.MethodPatch:
				if got := r.URL.Query().Get("updateMask"); got != "title,state,maxPoints" {
					t.Fatalf("expected updateMask title,state,maxPoints, got %q", got)
				}
				writeJSON(map[string]any{"id": "cw1", "title": "Updated Work", "state": "PUBLISHED"})
				return
			case strings.Contains(path, "/courseWork/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
				return
			case strings.Contains(path, "/courseWork") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "cw3", "title": "New Work", "state": "DRAFT"})
				return
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"courseWork": []map[string]any{{
						"id":       "cw1",
						"title":    "Work 1",
						"state":    "PUBLISHED",
						"workType": "ASSIGNMENT",
						"topicId":  "t1",
					}, {
						"id":       "cw2",
						"title":    "Work 2",
						"state":    "DRAFT",
						"workType": "ASSIGNMENT",
						"topicId":  "t2",
					}},
					"nextPageToken": "cwp",
				})
				return
			}
		case strings.Contains(path, "/announcements"):
			switch {
			case strings.Contains(path, ":modifyAssignees") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "a1", "assigneeMode": "INDIVIDUAL_STUDENTS"})
				return
			case strings.Contains(path, "/announcements/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{"id": "a1", "text": "Hello", "state": "PUBLISHED", "alternateLink": "https://classroom.google.com/a1"})
				return
			case strings.Contains(path, "/announcements/") && r.Method == http.MethodPatch:
				if got := r.URL.Query().Get("updateMask"); got != "text,state" {
					t.Fatalf("expected updateMask text,state, got %q", got)
				}
				writeJSON(map[string]any{"id": "a1", "state": "PUBLISHED"})
				return
			case strings.Contains(path, "/announcements/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
				return
			case strings.Contains(path, "/announcements") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"id": "a3", "state": "DRAFT"})
				return
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"announcements": []map[string]any{{
						"id":    "a1",
						"text":  "Hello",
						"state": "PUBLISHED",
					}, {
						"id":    "a2",
						"text":  "Draft",
						"state": "DRAFT",
					}},
					"nextPageToken": "ap",
				})
				return
			}
		case strings.Contains(path, "/topics"):
			switch {
			case strings.Contains(path, "/topics/") && r.Method == http.MethodGet:
				writeJSON(map[string]any{"topicId": "t1", "name": "Topic 1"})
				return
			case strings.Contains(path, "/topics/") && r.Method == http.MethodPatch:
				if got := r.URL.Query().Get("updateMask"); got != "name" {
					t.Fatalf("expected updateMask name, got %q", got)
				}
				writeJSON(map[string]any{"topicId": "t1", "name": "Updated Topic"})
				return
			case strings.Contains(path, "/topics/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
				return
			case strings.Contains(path, "/topics") && r.Method == http.MethodPost:
				writeJSON(map[string]any{"topicId": "t3", "name": "New Topic"})
				return
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"topic":         []map[string]any{{"topicId": "t1", "name": "Topic 1"}, {"topicId": "t2", "name": "Topic 2"}},
					"nextPageToken": "tp",
				})
				return
			}
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
				return
			case strings.Contains(path, "/students/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
				return
			case strings.Contains(path, "/students") && r.Method == http.MethodPost:
				if got := r.URL.Query().Get("enrollmentCode"); got != "code" {
					t.Fatalf("expected enrollmentCode=code, got %q", got)
				}
				writeJSON(map[string]any{
					"userId": "s1",
					"profile": map[string]any{
						"emailAddress": "student@example.com",
						"name":         map[string]any{"fullName": "Student One"},
					},
				})
				return
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"students": []map[string]any{{
						"userId": "s1",
						"profile": map[string]any{
							"emailAddress": "student@example.com",
							"name":         map[string]any{"fullName": "Student One"},
						},
					}},
					"nextPageToken": "sp",
				})
				return
			}
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
				return
			case strings.Contains(path, "/teachers/") && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
				return
			case strings.Contains(path, "/teachers") && r.Method == http.MethodPost:
				writeJSON(map[string]any{
					"userId": "t1",
					"profile": map[string]any{
						"emailAddress": "teacher@example.com",
						"name":         map[string]any{"fullName": "Teacher One"},
					},
				})
				return
			case r.Method == http.MethodGet:
				writeJSON(map[string]any{
					"teachers": []map[string]any{{
						"userId": "t1",
						"profile": map[string]any{
							"emailAddress": "teacher@example.com",
							"name":         map[string]any{"fullName": "Teacher One"},
						},
					}},
					"nextPageToken": "tp",
				})
				return
			}
		case strings.HasSuffix(path, "/courses") && r.Method == http.MethodGet:
			writeJSON(map[string]any{
				"courses":       []map[string]any{{"id": "c1", "name": "Biology", "courseState": "ACTIVE", "ownerId": "me"}},
				"nextPageToken": "cp",
			})
			return
		case strings.HasSuffix(path, "/courses") && r.Method == http.MethodPost:
			writeJSON(map[string]any{"id": "c2", "name": "New Course", "courseState": "ACTIVE", "ownerId": "me"})
			return
		case strings.Contains(path, "/courses/"):
			switch r.Method {
			case http.MethodGet:
				state := "ACTIVE"
				if strings.HasSuffix(path, "/c-delete") {
					state = "ARCHIVED"
				}
				writeJSON(map[string]any{"id": "c1", "name": "Biology", "courseState": state, "ownerId": "me", "alternateLink": "https://classroom.google.com/c/c1"})
				return
			case http.MethodPatch:
				mask := r.URL.Query().Get("updateMask")
				if mask != "name,courseState" && mask != "courseState" {
					t.Fatalf("unexpected updateMask %q", mask)
				}
				writeJSON(map[string]any{"id": "c1", "name": "Updated Course", "courseState": "ARCHIVED"})
				return
			case http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		http.NotFound(w, r)
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

	runJSON := func(args ...string) executeTestResult {
		t.Helper()
		full := append([]string{"--json", "--account", "a@b.com"}, args...)
		result := executeWithClassroomTestService(t, full, svc)
		if result.err != nil {
			t.Fatalf("execute %v: %v", args, result.err)
		}
		return result
	}

	runJSONForce := func(args ...string) {
		t.Helper()
		full := append([]string{"--json", "--force", "--account", "a@b.com"}, args...)
		result := executeWithClassroomTestService(t, full, svc)
		if result.err != nil {
			t.Fatalf("execute %v: %v", args, result.err)
		}
	}

	runJSON("classroom", "courses", "--state", "active,archived", "--teacher", "t1", "--student", "s1", "--max", "2", "--page", "p1")
	runJSON("classroom", "courses", "get", "c1")
	runJSON("classroom", "courses", "create", "--name", "Biology", "--owner", "me", "--section", "sec", "--state", "active")
	runJSON("classroom", "courses", "update", "c1", "--name", "Bio 2", "--state", "archived")
	runJSON("classroom", "courses", "archive", "c1")
	runJSON("classroom", "courses", "unarchive", "c1")
	runJSON("classroom", "courses", "join", "c1", "--role", "student", "--user", "s1", "--enrollment-code", "code")
	runJSON("classroom", "courses", "join", "c1", "--role", "teacher", "--user", "t1")
	runJSONForce("classroom", "courses", "leave", "c1", "--role", "student", "--user", "s1")
	runJSONForce("classroom", "courses", "leave", "c1", "--role", "teacher", "--user", "t1")
	runJSON("classroom", "courses", "url", "c1", "c2")
	runJSONForce("classroom", "courses", "delete", "c-delete")

	runJSON("classroom", "students", "c1", "--max", "1", "--page", "p1")
	runJSON("classroom", "students", "get", "c1", "s1")
	runJSON("classroom", "students", "add", "c1", "s1", "--enrollment-code", "code")
	runJSONForce("classroom", "students", "remove", "c1", "s1")

	runJSON("classroom", "teachers", "c1", "--max", "1", "--page", "p1")
	runJSON("classroom", "teachers", "get", "c1", "t1")
	runJSON("classroom", "teachers", "add", "c1", "t1")
	runJSONForce("classroom", "teachers", "remove", "c1", "t1")

	runJSON("classroom", "roster", "c1", "--max", "1", "--page", "p1")

	courseworkOut := runJSON("classroom", "coursework", "c1", "--topic", "t1", "--state", "draft,published", "--max", "2", "--page", "p1").stdout
	if got := decodeJSONArrayLen(t, courseworkOut, "coursework"); got != 1 {
		t.Fatalf("expected 1 coursework item after topic filter, got %d", got)
	}
	runJSON("classroom", "coursework", "get", "c1", "cw1")
	runJSON("classroom", "coursework", "create", "c1", "--title", "Homework", "--type", "assignment", "--state", "draft", "--max-points", "10", "--due", "2024-03-15 14:30", "--scheduled", "2024-03-10T12:00:00Z", "--topic", "t1")
	runJSON("classroom", "coursework", "update", "c1", "cw1", "--title", "Homework 2", "--state", "published", "--max-points", "20")
	runJSONForce("classroom", "coursework", "delete", "c1", "cw1")
	runJSON("classroom", "coursework", "assignees", "c1", "cw1", "--mode", "ALL_STUDENTS")

	materialsOut := runJSON("classroom", "materials", "c1", "--topic", "t1", "--state", "draft,published", "--max", "2", "--page", "p1").stdout
	if got := decodeJSONArrayLen(t, materialsOut, "materials"); got != 1 {
		t.Fatalf("expected 1 material item after topic filter, got %d", got)
	}
	runJSON("classroom", "materials", "get", "c1", "m1")
	runJSON("classroom", "materials", "create", "c1", "--title", "Material", "--state", "draft", "--scheduled", "2024-03-10T12:00:00Z", "--topic", "t1")
	runJSON("classroom", "materials", "update", "c1", "m1", "--title", "Material 2", "--topic", "t2")
	runJSONForce("classroom", "materials", "delete", "c1", "m1")

	runJSON("classroom", "announcements", "c1", "--state", "draft,published", "--order-by", "updateTime desc", "--max", "2", "--page", "p1")
	runJSON("classroom", "announcements", "get", "c1", "a1")
	runJSON("classroom", "announcements", "create", "c1", "--text", "Hello", "--state", "draft", "--scheduled", "2024-03-10T12:00:00Z")
	runJSON("classroom", "announcements", "update", "c1", "a1", "--text", "Updated", "--state", "published")
	runJSONForce("classroom", "announcements", "delete", "c1", "a1")
	runJSON("classroom", "announcements", "assignees", "c1", "a1", "--mode", "INDIVIDUAL_STUDENTS", "--add-student", "s1")

	runJSON("classroom", "topics", "c1", "--max", "1", "--page", "p1")
	runJSON("classroom", "topics", "get", "c1", "t1")
	runJSON("classroom", "topics", "create", "c1", "--name", "Topic 3")
	runJSON("classroom", "topics", "update", "c1", "t1", "--name", "Topic 1 Updated")
	runJSONForce("classroom", "topics", "delete", "c1", "t1")

	runJSON("classroom", "submissions", "c1", "cw1", "--state", "turned_in", "--late", "not-late", "--user", "u1", "--max", "2", "--page", "p1")
	runJSON("classroom", "submissions", "get", "c1", "cw1", "s1")
	runJSON("classroom", "submissions", "turn-in", "c1", "cw1", "s1")
	runJSON("classroom", "submissions", "reclaim", "c1", "cw1", "s1")
	runJSON("classroom", "submissions", "return", "c1", "cw1", "s1")
	runJSON("classroom", "submissions", "grade", "c1", "cw1", "s1", "--draft", "5", "--assigned", "10")

	runJSON("classroom", "invitations", "--course", "c1", "--user", "u1", "--max", "1", "--page", "p1")
	runJSON("classroom", "invitations", "get", "i1")
	runJSON("classroom", "invitations", "create", "c1", "u2", "--role", "teacher")
	runJSON("classroom", "invitations", "accept", "i1")
	runJSONForce("classroom", "invitations", "delete", "i1")

	runJSON("classroom", "guardians", "s1", "--email", "guardian@example.com", "--max", "1", "--page", "p1")
	runJSON("classroom", "guardians", "get", "s1", "g1")
	runJSONForce("classroom", "guardians", "delete", "s1", "g1")

	runJSON("classroom", "guardian-invitations", "s1", "--email", "guardian@example.com", "--state", "pending", "--max", "1", "--page", "p1")
	runJSON("classroom", "guardian-invitations", "get", "s1", "gi1")
	runJSON("classroom", "guardian-invitations", "create", "s1", "--email", "guardian@example.com")

	runJSON("classroom", "profile")
}

func TestExecute_ClassroomValidationErrors(t *testing.T) {
	svc, svcErr := classroom.NewService(context.Background(), option.WithoutAuthentication())
	if svcErr != nil {
		t.Fatalf("NewService: %v", svcErr)
	}

	if err := executeWithClassroomTestService(t, []string{"--account", "a@b.com", "classroom", "courses", "join", "c1", "--role", "nope"}, svc).err; err == nil {
		t.Fatalf("expected error for invalid join role")
	}
	if err := executeWithClassroomTestService(t, []string{"--account", "a@b.com", "classroom", "coursework", "create", "c1", "--title", "Work", "--due-time", "10:00"}, svc).err; err == nil {
		t.Fatalf("expected error for due time without date")
	}
	if err := executeWithClassroomTestService(t, []string{"--account", "a@b.com", "classroom", "coursework", "assignees", "c1", "cw1"}, svc).err; err == nil {
		t.Fatalf("expected error for missing coursework assignee changes")
	}
	if err := executeWithClassroomTestService(t, []string{"--account", "a@b.com", "classroom", "announcements", "assignees", "c1", "a1"}, svc).err; err == nil {
		t.Fatalf("expected error for missing announcement assignee changes")
	}
	if err := executeWithClassroomTestService(t, []string{"--account", "a@b.com", "classroom", "materials", "update", "c1", "m1"}, svc).err; err == nil {
		t.Fatalf("expected error for empty materials update")
	}
	if err := executeWithClassroomTestService(t, []string{"--account", "a@b.com", "classroom", "submissions", "grade", "c1", "cw1", "s1"}, svc).err; err == nil {
		t.Fatalf("expected error for missing grades")
	}
	if err := executeWithClassroomTestService(t, []string{"--account", "a@b.com", "classroom", "submissions", "grade", "c1", "cw1", "s1", "--assigned", "bad"}, svc).err; err == nil {
		t.Fatalf("expected error for invalid grade value")
	}
	validationErr := executeWithClassroomTestService(t, []string{"--account", "a@b.com", "classroom", "invitations", "list"}, svc).err
	if validationErr == nil || !strings.Contains(validationErr.Error(), "at least one of --course or --user is required") {
		t.Fatalf("expected invitation filter error, got %v", validationErr)
	}
}

func TestExecute_ClassroomListInvalidMaxFailsBeforeService(t *testing.T) {
	factory := func(context.Context, string) (*classroom.Service, error) {
		t.Fatalf("expected max validation to fail before creating classroom service")
		return nil, errors.New("unexpected classroom service call")
	}

	cases := [][]string{
		{"--account", "a@b.com", "classroom", "courses", "--max", "0"},
		{"--account", "a@b.com", "classroom", "courses", "--max=-1"},
		{"--account", "a@b.com", "classroom", "students", "c1", "--max", "0"},
		{"--account", "a@b.com", "classroom", "students", "c1", "--max=-1"},
		{"--account", "a@b.com", "classroom", "teachers", "c1", "--max", "0"},
		{"--account", "a@b.com", "classroom", "teachers", "c1", "--max=-1"},
		{"--account", "a@b.com", "classroom", "roster", "c1", "--max", "0"},
		{"--account", "a@b.com", "classroom", "roster", "c1", "--max=-1"},
		{"--account", "a@b.com", "classroom", "coursework", "c1", "--max", "0"},
		{"--account", "a@b.com", "classroom", "coursework", "c1", "--max=-1"},
		{"--account", "a@b.com", "classroom", "materials", "c1", "--max", "0"},
		{"--account", "a@b.com", "classroom", "materials", "c1", "--max=-1"},
		{"--account", "a@b.com", "classroom", "announcements", "c1", "--max", "0"},
		{"--account", "a@b.com", "classroom", "announcements", "c1", "--max=-1"},
		{"--account", "a@b.com", "classroom", "topics", "c1", "--max", "0"},
		{"--account", "a@b.com", "classroom", "topics", "c1", "--max=-1"},
		{"--account", "a@b.com", "classroom", "submissions", "c1", "cw1", "--max", "0"},
		{"--account", "a@b.com", "classroom", "submissions", "c1", "cw1", "--max=-1"},
		{"--account", "a@b.com", "classroom", "invitations", "--course", "c1", "--max", "0"},
		{"--account", "a@b.com", "classroom", "invitations", "--course", "c1", "--max=-1"},
		{"--account", "a@b.com", "classroom", "guardians", "s1", "--max", "0"},
		{"--account", "a@b.com", "classroom", "guardians", "s1", "--max=-1"},
		{"--account", "a@b.com", "classroom", "guardian-invitations", "s1", "--max", "0"},
		{"--account", "a@b.com", "classroom", "guardian-invitations", "s1", "--max=-1"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			err := executeWithClassroomTestServiceFactory(t, args, factory).err
			if ExitCode(err) != 2 || !strings.Contains(err.Error(), "max must be > 0") {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func decodeJSONArrayLen(t *testing.T, output, key string) int {
	t.Helper()

	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("parse json output: %v", err)
	}

	raw, ok := payload[key]
	if !ok {
		t.Fatalf("missing %q key in output", key)
	}

	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("parse %s array: %v", key, err)
	}

	return len(items)
}
