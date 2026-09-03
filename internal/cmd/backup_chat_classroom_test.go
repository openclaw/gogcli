package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/chat/v1"
	"google.golang.org/api/classroom/v1"
)

func TestFetchBackupChatSpacesRejectsRepeatedPageToken(t *testing.T) {
	var calls atomic.Int32
	svc := newChatTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces")) {
			http.NotFound(w, r)
			return
		}
		if calls.Add(1) > 2 {
			http.Error(w, "unexpected extra space page request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spaces":        []map[string]any{{"name": "spaces/aaa"}},
			"nextPageToken": "stuck",
		})
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := fetchBackupChatSpaces(ctx, svc)
	if err == nil || !strings.Contains(err.Error(), "repeated page token") {
		t.Fatalf("err = %v after %d list calls", err, calls.Load())
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("list calls = %d, want 2", got)
	}
}

func TestFetchBackupClassroomCoursesRejectsRepeatedPageToken(t *testing.T) {
	var calls atomic.Int32
	svc, closeService := newClassroomTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/courses")) {
			http.NotFound(w, r)
			return
		}
		if calls.Add(1) > 2 {
			http.Error(w, "unexpected extra course page request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"courses":       []map[string]any{{"id": "c1", "name": "Biology"}},
			"nextPageToken": "stuck",
		})
	}))
	defer closeService()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := fetchBackupClassroomCourses(ctx, svc)
	if err == nil || !strings.Contains(err.Error(), "repeated page token") {
		t.Fatalf("err = %v after %d list calls", err, calls.Load())
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("list calls = %d, want 2", got)
	}
}

func TestFetchBackupChatMessagesRejectsRepeatedPageToken(t *testing.T) {
	var calls atomic.Int32
	svc := newChatTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/messages") {
			http.NotFound(w, r)
			return
		}
		if calls.Add(1) > 2 {
			http.Error(w, "unexpected extra message page request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"messages":      []map[string]any{{"name": "spaces/aaa/messages/m1"}},
			"nextPageToken": "stuck",
		})
	}))

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	got, err := fetchBackupChatMessages(ctx, svc, []*chat.Space{{Name: "spaces/aaa"}})
	if err == nil || !strings.Contains(err.Error(), "repeated page token") {
		t.Fatalf("err = %v after %d list calls", err, calls.Load())
	}
	if got != nil || calls.Load() != 2 {
		t.Fatalf("messages = %#v, calls = %d; want no partial result and two calls", got, calls.Load())
	}
}

func TestFetchBackupClassroomChildrenRejectRepeatedPageTokens(t *testing.T) {
	tests := []struct {
		path        string
		responseKey string
	}{
		{path: "/topics", responseKey: "topic"},
		{path: "/announcements", responseKey: "announcements"},
		{path: "/courseWork", responseKey: "courseWork"},
		{path: "/courseWorkMaterials", responseKey: "courseWorkMaterial"},
		{path: "/studentSubmissions", responseKey: "studentSubmissions"},
	}
	for _, tc := range tests {
		t.Run(tc.responseKey, func(t *testing.T) {
			var calls atomic.Int32
			svc, closeService := newClassroomTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, tc.path) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte("{}"))
					return
				}
				if calls.Add(1) > 2 {
					http.Error(w, "unexpected extra child page request", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					tc.responseKey:  []map[string]any{{"id": "item-1"}},
					"nextPageToken": "stuck",
				})
			}))
			defer closeService()

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			_, _, _, _, _, err := fetchBackupClassroomChildren(ctx, svc, []*classroom.Course{{Id: "course-1"}})
			if err == nil || !strings.Contains(err.Error(), "repeated page token") {
				t.Fatalf("err = %v after %d child list calls", err, calls.Load())
			}
			if got := calls.Load(); got != 2 {
				t.Fatalf("child list calls = %d, want 2", got)
			}
		})
	}
}

func TestFetchBackupClassroomChildrenPreservesPartialPagesOnAPIError(t *testing.T) {
	for _, message := range []string{"permission denied", "upstream pagination loop"} {
		t.Run(message, func(t *testing.T) {
			responses := map[string]struct {
				key  string
				item map[string]any
			}{
				"topics":              {key: "topic", item: map[string]any{"topicId": "topic-1"}},
				"announcements":       {key: "announcements", item: map[string]any{"id": "announcement-1"}},
				"courseWork":          {key: "courseWork", item: map[string]any{"id": "work-1"}},
				"courseWorkMaterials": {key: "courseWorkMaterial", item: map[string]any{"id": "material-1"}},
				"studentSubmissions":  {key: "studentSubmissions", item: map[string]any{"id": "submission-1", "courseWorkId": "work-1"}},
			}
			calls := make(map[string]*atomic.Int32, len(responses))
			for resource := range responses {
				calls[resource] = &atomic.Int32{}
			}
			svc, closeService := newClassroomTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resource := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
				response, ok := responses[resource]
				if !ok || r.Method != http.MethodGet {
					http.NotFound(w, r)
					return
				}
				n := calls[resource].Add(1)
				if n == 1 {
					if token := r.URL.Query().Get("pageToken"); token != "" {
						t.Errorf("%s initial page token = %q", resource, token)
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						response.key:    []map[string]any{response.item},
						"nextPageToken": "page-2",
					})
					return
				}
				if n != 2 || r.URL.Query().Get("pageToken") != "page-2" {
					t.Errorf("%s page request %d has token %q", resource, n, r.URL.Query().Get("pageToken"))
				}
				http.Error(w, message, http.StatusForbidden)
			}))
			defer closeService()

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			topics, announcements, work, materials, submissions, err := fetchBackupClassroomChildren(ctx, svc, []*classroom.Course{{Id: "course-1"}})
			if err != nil {
				t.Fatalf("ordinary API error must remain best-effort: %v", err)
			}
			if len(topics) != 1 || topics[0].Topic.TopicId != "topic-1" || topics[0].CourseID != "course-1" {
				t.Errorf("topics = %#v, want the first page", topics)
			}
			if len(announcements) != 1 || announcements[0].Announcement.Id != "announcement-1" {
				t.Errorf("announcements = %#v, want the first page", announcements)
			}
			if len(work) != 1 || work[0].CourseWork.Id != "work-1" {
				t.Errorf("coursework = %#v, want the first page", work)
			}
			if len(materials) != 1 || materials[0].Material.Id != "material-1" {
				t.Errorf("materials = %#v, want the first page", materials)
			}
			if len(submissions) != 1 || submissions[0].Submission.Id != "submission-1" || submissions[0].CourseWorkID != "work-1" {
				t.Errorf("submissions = %#v, want the first page", submissions)
			}
			for resource, count := range calls {
				if got := count.Load(); got != 2 {
					t.Errorf("%s list calls = %d, want 2", resource, got)
				}
			}
		})
	}
}

func TestFetchBackupChatSpacesTwoDistinctPagesSucceed(t *testing.T) {
	var calls atomic.Int32
	svc := newChatTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces")) {
			http.NotFound(w, r)
			return
		}
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spaces":        []map[string]any{{"name": "spaces/aaa"}},
				"nextPageToken": "page-2",
			})
			return
		}
		if n == 2 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spaces": []map[string]any{{"name": "spaces/bbb"}},
			})
			return
		}
		http.Error(w, "unexpected extra space page request", http.StatusBadRequest)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got, err := fetchBackupChatSpaces(ctx, svc)
	if err != nil {
		t.Fatalf("err = %v after %d list calls", err, calls.Load())
	}
	if calls.Load() != 2 {
		t.Fatalf("list calls = %d, want 2", calls.Load())
	}
	if len(got) != 2 || got[0].Name != "spaces/aaa" || got[1].Name != "spaces/bbb" {
		t.Fatalf("spaces = %#v", got)
	}
}
