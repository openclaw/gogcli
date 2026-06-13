package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	formsapi "google.golang.org/api/forms/v1"
)

func TestBuildQuestion(t *testing.T) {
	t.Run("choice question requires options", func(t *testing.T) {
		_, err := buildQuestion("radio", &formsAddQuestionInput{})
		if err == nil || !strings.Contains(err.Error(), "--option is required") {
			t.Fatalf("expected option validation error, got %v", err)
		}
	})

	t.Run("scale question", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			low  int
			high int
		}{
			{name: "minimum bounds", low: 0, high: 2},
			{name: "maximum bounds", low: 1, high: 10},
		} {
			t.Run(tc.name, func(t *testing.T) {
				q, err := buildQuestion("scale", &formsAddQuestionInput{
					Required:       true,
					ScaleLow:       tc.low,
					ScaleHigh:      tc.high,
					ScaleLowLabel:  "low",
					ScaleHighLabel: "high",
				})
				if err != nil {
					t.Fatalf("buildQuestion: %v", err)
				}
				if q.ScaleQuestion == nil || q.ScaleQuestion.Low != int64(tc.low) || q.ScaleQuestion.High != int64(tc.high) {
					t.Fatalf("unexpected scale question: %#v", q)
				}
				if !q.Required {
					t.Fatalf("expected required question")
				}
			})
		}
	})

	t.Run("scale question validation", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			low  int
			high int
			want string
		}{
			{name: "low below range", low: -1, high: 5, want: "--scale-low must be 0 or 1"},
			{name: "low above range", low: 2, high: 5, want: "--scale-low must be 0 or 1"},
			{name: "high below range", low: 1, high: 1, want: "--scale-high must be between 2 and 10"},
			{name: "high above range", low: 1, high: 11, want: "--scale-high must be between 2 and 10"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := buildQuestion("scale", &formsAddQuestionInput{ScaleLow: tc.low, ScaleHigh: tc.high})
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("expected %q, got %v", tc.want, err)
				}
			})
		}
	})

	t.Run("quiz grading", func(t *testing.T) {
		q, err := buildQuestion("radio", &formsAddQuestionInput{
			Options: []string{"1", "2", "4"},
			Correct: []string{
				"4",
			},
			Points: 2,
		})
		if err != nil {
			t.Fatalf("buildQuestion: %v", err)
		}
		if q.Grading == nil || q.Grading.PointValue != 2 {
			t.Fatalf("missing grading: %#v", q.Grading)
		}
		if got := q.Grading.CorrectAnswers.Answers[0].Value; got != "4" {
			t.Fatalf("correct answer = %q", got)
		}
	})

	t.Run("quiz grading validation", func(t *testing.T) {
		_, err := buildQuestion("radio", &formsAddQuestionInput{
			Options: []string{"1", "2"},
			Correct: []string{
				"2",
			},
		})
		if err == nil || !strings.Contains(err.Error(), "--correct requires --points") {
			t.Fatalf("expected points validation, got %v", err)
		}

		_, err = buildQuestion("scale", &formsAddQuestionInput{ScaleLow: 1, ScaleHigh: 5, Correct: []string{"5"}, Points: 1})
		if err == nil || !strings.Contains(err.Error(), "supported only") {
			t.Fatalf("expected type validation, got %v", err)
		}
	})
}

func TestFormsAddQuestionAppend(t *testing.T) {
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

	svc := newFormsTestService(t, t.Context(), srv)
	ctx := withFormsTestService(newQuietUIContext(t), svc)
	err := runKong(t, &FormsAddQuestionCmd{}, []string{"form1", "--title", "Favorite color", "--type", "radio", "--option", "Red", "--option", "Blue"}, ctx, &RootFlags{Account: "a@b.com"})
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

func TestFormsAddQuestionRejectsInvalidAppendIndexBeforeDryRun(t *testing.T) {
	ctx := withFormsTestServiceFactory(newQuietUIContext(t), unexpectedFormsTestService(t, "forms service should not be created"))
	err := runKong(t, &FormsAddQuestionCmd{}, []string{
		"form1",
		"--title", "Favorite color",
		"--type", "text",
		"--index=-2",
	}, ctx, &RootFlags{Account: "a@b.com", DryRun: true})
	if err == nil {
		t.Fatal("expected index validation error")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
	}
}

func TestFormsAddQuestionRejectsInvalidScaleBeforeDryRun(t *testing.T) {
	ctx := withFormsTestServiceFactory(newQuietUIContext(t), unexpectedFormsTestService(t, "forms service should not be created"))
	for _, args := range [][]string{
		{"form1", "--title", "Rating", "--type", "scale", "--scale-low", "2"},
		{"form1", "--title", "Rating", "--type", "scale", "--scale-high", "11"},
	} {
		err := runKong(t, &FormsAddQuestionCmd{}, args, ctx, &RootFlags{Account: "a@b.com", DryRun: true})
		if err == nil {
			t.Fatalf("expected scale validation error for %v", args)
		}
		if got := ExitCode(err); got != 2 {
			t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
		}
	}
}

func TestFormsAddQuestionAppendWithGrading(t *testing.T) {
	var gotBatch formsapi.BatchUpdateFormRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/forms/form1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"formId": "form1"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/forms/form1:batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&gotBatch); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"form": map[string]any{
					"formId": "form1",
					"items":  []map[string]any{{}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := newFormsTestService(t, t.Context(), srv)
	ctx := withFormsTestService(newQuietUIContext(t), svc)
	err := runKong(t, &FormsAddQuestionCmd{}, []string{
		"form1",
		"--title", "What is 2+2?",
		"--type", "radio",
		"--option", "1",
		"--option", "4",
		"--correct", "4",
		"--points", "1",
	}, ctx, &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}

	req := gotBatch.Requests[0].CreateItem.Item.QuestionItem.Question
	if req.Grading == nil {
		t.Fatalf("missing grading in request: %#v", req)
	}
	if req.Grading.PointValue != 1 {
		t.Fatalf("point value = %d", req.Grading.PointValue)
	}
	if got := req.Grading.CorrectAnswers.Answers[0].Value; got != "4" {
		t.Fatalf("correct answer = %q", got)
	}
}

func TestFormsDeleteQuestionValidationAndDryRun(t *testing.T) {
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

	svc := newFormsTestService(t, t.Context(), srv)
	ctx := withFormsTestService(newQuietUIContext(t), svc)

	t.Run("out of range before confirmation", func(t *testing.T) {
		err := runKong(t, &FormsDeleteQuestionCmd{}, []string{"form1", "5"}, ctx, &RootFlags{Account: "a@b.com", NoInput: true})
		if err == nil || !strings.Contains(err.Error(), "out of range") {
			t.Fatalf("expected out of range error, got %v", err)
		}
	})

	t.Run("dry run skips mutation", func(t *testing.T) {
		before := batchCalls
		beforeGets := getCalls
		err := runKong(t, &FormsDeleteQuestionCmd{}, []string{"form1", "0"}, ctx, &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true})
		if ExitCode(err) != 0 {
			t.Fatalf("expected dry-run exit 0, got %v", err)
		}
		if batchCalls != before {
			t.Fatalf("expected no batch update during dry-run, got %d -> %d", before, batchCalls)
		}
		if getCalls != beforeGets {
			t.Fatalf("expected no form fetch during dry-run, got %d -> %d", beforeGets, getCalls)
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

	if getCalls < 2 {
		t.Fatalf("expected form fetches for validation, got %d", getCalls)
	}
}

func TestFormsMoveQuestionSendsZeroIndex(t *testing.T) {
	var gotBatch formsapi.BatchUpdateFormRequest
	var rawBatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/forms/form1:batchUpdate"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read batchUpdate: %v", err)
			}
			rawBatch = string(body)
			if err := json.Unmarshal(body, &gotBatch); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := newFormsTestService(t, t.Context(), srv)
	ctx := withFormsTestService(newQuietUIContext(t), svc)
	err := runKong(t, &FormsMoveQuestionCmd{}, []string{"form1", "1", "0"}, ctx, &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if len(gotBatch.Requests) != 1 || gotBatch.Requests[0].MoveItem == nil {
		t.Fatalf("expected moveItem request, got %#v", gotBatch.Requests)
	}
	move := gotBatch.Requests[0].MoveItem
	if move.OriginalLocation == nil || move.OriginalLocation.Index != 1 {
		t.Fatalf("unexpected original location: %#v", move.OriginalLocation)
	}
	if move.NewLocation == nil || move.NewLocation.Index != 0 {
		t.Fatalf("expected new index 0, got %#v", move.NewLocation)
	}
	if !strings.Contains(rawBatch, `"newLocation":{"index":0}`) {
		t.Fatalf("newLocation index 0 omitted from request: %s", rawBatch)
	}
}
