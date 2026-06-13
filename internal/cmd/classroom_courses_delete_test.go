package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestClassroomCoursesDeleteRequiresArchivedCourse(t *testing.T) {
	tests := []struct {
		state string
		want  string
	}{
		{state: "ACTIVE", want: "archive it before deleting: gog classroom courses archive c1"},
		{state: "PROVISIONED", want: "accept its teaching invitation in Google Classroom"},
		{state: "DECLINED", want: "declined courses cannot be deleted or recovered"},
		{state: "UNKNOWN", want: "must be ARCHIVED before deletion (current state: UNKNOWN)"},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			deleteCalls := 0
			svc, closeService := newClassroomTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":          "c1",
						"courseState": tt.state,
					})
				case http.MethodDelete:
					deleteCalls++
					w.WriteHeader(http.StatusNoContent)
				default:
					http.NotFound(w, r)
				}
			}))
			defer closeService()

			result := executeWithClassroomTestService(t, []string{
				"--force",
				"--account", "a@b.com",
				"classroom", "courses", "delete", "c1",
			}, svc)
			if result.err == nil || !strings.Contains(result.err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", result.err, tt.want)
			}
			if deleteCalls != 0 {
				t.Fatalf("delete calls = %d, want 0", deleteCalls)
			}
		})
	}
}

func TestClassroomCoursesDeleteArchivedCourse(t *testing.T) {
	deleteCalls := 0
	svc, closeService := newClassroomTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "c1",
				"courseState": "ARCHIVED",
			})
		case http.MethodDelete:
			deleteCalls++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer closeService()

	result := executeWithClassroomTestService(t, []string{
		"--json",
		"--force",
		"--account", "a@b.com",
		"classroom", "courses", "delete", "c1",
	}, svc)
	if result.err != nil {
		t.Fatalf("delete archived course: %v", result.err)
	}
	if deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", deleteCalls)
	}
}
