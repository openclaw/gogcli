package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	formsapi "google.golang.org/api/forms/v1"
)

func TestBuildQuestion(t *testing.T) {
	t.Run("choice question requires options", func(t *testing.T) {
		_, err := buildQuestion("radio", &FormsAddQuestionCmd{})
		if err == nil || !strings.Contains(err.Error(), "--option is required") {
			t.Fatalf("expected option validation error, got %v", err)
		}
	})

	t.Run("scale question", func(t *testing.T) {
		q, err := buildQuestion("scale", &FormsAddQuestionCmd{Required: true, ScaleLow: 1, ScaleHigh: 7, ScaleLowLabel: "low", ScaleHighLabel: "high"})
		if err != nil {
			t.Fatalf("buildQuestion: %v", err)
		}
		if q.ScaleQuestion == nil || q.ScaleQuestion.Low != 1 || q.ScaleQuestion.High != 7 {
			t.Fatalf("unexpected scale question: %#v", q)
		}
		if !q.Required {
			t.Fatalf("expected required question")
		}
	})
}

func TestFormsAddQuestionAppend(t *testing.T) {
	origNew := newFormsService
	t.Cleanup(func() { newFormsService = origNew })

	var gotBatch formsapi.BatchUpdateFormRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/forms/form1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"formId": "form1",
				"items": []map[string]any{
					{"title": "Q1"},
					{"title": "Q2"},
				},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/forms/form1:batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&gotBatch); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"form": map[string]any{
					"formId": "form1",
					"items":  []map[string]any{{}, {}, {}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	newFormsService = func(ctx context.Context, account string) (*formsapi.Service, error) {
		return newFormsTestService(t, ctx, srv), nil
	}

	err := runKong(t, &FormsAddQuestionCmd{}, []string{"form1", "--title", "Favorite color", "--type", "radio", "--option", "Red", "--option", "Blue"}, newQuietUIContext(t), &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if len(gotBatch.Requests) != 1 || gotBatch.Requests[0].CreateItem == nil {
		t.Fatalf("expected createItem request, got %#v", gotBatch.Requests)
	}
	req := gotBatch.Requests[0].CreateItem
	if req.Location == nil || req.Location.Index != 2 {
		t.Fatalf("expected append index 2, got %#v", req.Location)
	}
	if req.Item == nil || req.Item.Title != "Favorite color" {
		t.Fatalf("unexpected item: %#v", req.Item)
	}
	if req.Item.QuestionItem == nil || req.Item.QuestionItem.Question == nil || req.Item.QuestionItem.Question.ChoiceQuestion == nil {
		t.Fatalf("missing choice question: %#v", req.Item)
	}
	if req.Item.QuestionItem.Question.ChoiceQuestion.Type != "RADIO" {
		t.Fatalf("unexpected choice type: %#v", req.Item.QuestionItem.Question.ChoiceQuestion)
	}
}

func TestFormsDeleteQuestionValidationAndDryRun(t *testing.T) {
	origNew := newFormsService
	t.Cleanup(func() { newFormsService = origNew })

	getCalls := 0
	batchCalls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/forms/form1"):
			getCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"formId": "form1",
				"items": []map[string]any{
					{"title": "Q1"},
				},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/forms/form1:batchUpdate"):
			batchCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	newFormsService = func(ctx context.Context, account string) (*formsapi.Service, error) {
		return newFormsTestService(t, ctx, srv), nil
	}

	ctx := newQuietUIContext(t)

	t.Run("out of range before confirmation", func(t *testing.T) {
		err := runKong(t, &FormsDeleteQuestionCmd{}, []string{"form1", "5"}, ctx, &RootFlags{Account: "a@b.com", NoInput: true})
		if err == nil || !strings.Contains(err.Error(), "out of range") {
			t.Fatalf("expected out of range error, got %v", err)
		}
	})

	t.Run("dry run skips mutation", func(t *testing.T) {
		before := batchCalls
		err := runKong(t, &FormsDeleteQuestionCmd{}, []string{"form1", "0"}, ctx, &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true})
		if ExitCode(err) != 0 {
			t.Fatalf("expected dry-run exit 0, got %v", err)
		}
		if batchCalls != before {
			t.Fatalf("expected no batch update during dry-run, got %d -> %d", before, batchCalls)
		}
	})

	t.Run("force delete performs mutation", func(t *testing.T) {
		before := batchCalls
		err := runKong(t, &FormsDeleteQuestionCmd{}, []string{"form1", "0"}, ctx, &RootFlags{Account: "a@b.com", Force: true})
		if err != nil {
			t.Fatalf("runKong: %v", err)
		}
		if batchCalls != before+1 {
			t.Fatalf("expected one batch update, got %d -> %d", before, batchCalls)
		}
	})

	if getCalls < 3 {
		t.Fatalf("expected form fetches for validation, got %d", getCalls)
	}
}
