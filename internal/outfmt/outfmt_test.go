package outfmt

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestFromFlags(t *testing.T) {
	if _, err := FromFlags(true, true); err == nil {
		t.Fatalf("expected error when combining --json and --plain")
	}

	got, err := FromFlags(true, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !got.JSON || got.Plain {
		t.Fatalf("unexpected mode: %#v", got)
	}
}

func TestContextMode(t *testing.T) {
	ctx := context.Background()

	if IsJSON(ctx) || IsPlain(ctx) {
		t.Fatalf("expected default text")
	}
	ctx = WithMode(ctx, Mode{JSON: true})

	if !IsJSON(ctx) || IsPlain(ctx) {
		t.Fatalf("expected json-only")
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(context.Background(), &buf, map[string]any{"ok": true}); err != nil {
		t.Fatalf("err: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatalf("expected output")
	}
}

func TestWriteJSON_ResultsOnlyAndSelect(t *testing.T) {
	ctx := WithJSONTransform(context.Background(), JSONTransform{
		ResultsOnly: true,
		Select:      []string{"id"},
	})

	var buf bytes.Buffer
	if err := WriteJSON(ctx, &buf, map[string]any{
		"files": []map[string]any{
			{"id": "1", "name": "one"},
			{"id": "2", "name": "two"},
		},
		"nextPageToken": "tok",
	}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, buf.String())
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}

	if got[0]["id"] != "1" || got[1]["id"] != "2" {
		t.Fatalf("unexpected ids: %#v", got)
	}

	if _, ok := got[0]["name"]; ok {
		t.Fatalf("expected name to be stripped, got %#v", got[0])
	}
}

func TestWriteJSON_ResultsOnlyPrefersKnownResultKey(t *testing.T) {
	ctx := WithJSONTransform(context.Background(), JSONTransform{ResultsOnly: true})

	for range 50 {
		var buf bytes.Buffer
		if err := WriteJSON(ctx, &buf, map[string]any{
			"files":         []map[string]any{{"id": "primary"}},
			"warnings":      []string{"secondary"},
			"nextPageToken": "tok",
		}); err != nil {
			t.Fatalf("WriteJSON: %v", err)
		}

		var got []map[string]any
		if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v (out=%q)", err, buf.String())
		}

		if len(got) != 1 || got[0]["id"] != "primary" {
			t.Fatalf("selected wrong result array: %s", strings.TrimSpace(buf.String()))
		}
	}
}

func TestWriteJSON_ResultsOnlyDoesNotPreferScalarKnownKeyOverArray(t *testing.T) {
	ctx := WithJSONTransform(context.Background(), JSONTransform{ResultsOnly: true})

	var buf bytes.Buffer
	if err := WriteJSON(ctx, &buf, map[string]any{
		"notes":        "speaker notes",
		"textElements": []map[string]any{{"text": "primary"}},
	}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, buf.String())
	}

	if len(got) != 1 || got[0]["text"] != "primary" {
		t.Fatalf("selected scalar known key instead of result array: %s", strings.TrimSpace(buf.String()))
	}
}

func TestWriteJSON_ResultsOnlyPreservesExplicitPrimaryResult(t *testing.T) {
	ctx := WithJSONTransform(context.Background(), JSONTransform{ResultsOnly: true})
	input := map[string]any{
		"draftId": "d1",
		"message": map[string]any{
			"id":       "m1",
			"threadId": "t1",
		},
		"threadId": "t1",
		"attachments": []map[string]any{{
			"filename": "fixture.txt",
			"size":     7,
		}},
	}

	var baseline bytes.Buffer
	if err := WriteJSON(context.Background(), &baseline, input); err != nil {
		t.Fatalf("WriteJSON baseline: %v", err)
	}

	var got bytes.Buffer
	if err := WriteJSON(ctx, &got, PrimaryResult(input)); err != nil {
		t.Fatalf("WriteJSON primary result: %v", err)
	}

	if got.String() != baseline.String() {
		t.Fatalf("complete primary result changed:\ngot:\n%s\nwant:\n%s", got.String(), baseline.String())
	}
}

func TestWriteJSON_ResultsOnlyGenericArraySelectionIsDeterministic(t *testing.T) {
	ctx := WithJSONTransform(context.Background(), JSONTransform{ResultsOnly: true})
	input := map[string]any{
		"zItems": []map[string]any{{"id": "z"}},
		"aItems": []map[string]any{{"id": "a"}},
	}

	for range 50 {
		var buf bytes.Buffer
		if err := WriteJSON(ctx, &buf, input); err != nil {
			t.Fatalf("WriteJSON: %v", err)
		}

		var got []map[string]any
		if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v (out=%q)", err, buf.String())
		}

		if len(got) != 1 || got[0]["id"] != "a" {
			t.Fatalf("generic selection was not deterministic: %s", strings.TrimSpace(buf.String()))
		}
	}
}

func TestWriteJSON_ResultsOnlyAttachmentCommandStillReturnsAttachments(t *testing.T) {
	ctx := WithJSONTransform(context.Background(), JSONTransform{ResultsOnly: true})
	input := map[string]any{
		"threadId": "t1",
		"attachments": []map[string]any{{
			"messageId": "m1",
			"filename":  "fixture.txt",
		}},
	}

	var buf bytes.Buffer
	if err := WriteJSON(ctx, &buf, input); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, buf.String())
	}

	if len(got) != 1 || got[0]["messageId"] != "m1" || got[0]["filename"] != "fixture.txt" {
		t.Fatalf("attachment-specific result changed: %s", strings.TrimSpace(buf.String()))
	}
}

func TestWriteJSON_ResultsOnlyListCommandStillReturnsItems(t *testing.T) {
	ctx := WithJSONTransform(context.Background(), JSONTransform{ResultsOnly: true})
	input := map[string]any{
		"messages": []map[string]any{
			{"id": "m1", "threadId": "t1"},
			{"id": "m2", "threadId": "t2"},
		},
		"nextPageToken": "next-page",
	}

	var buf bytes.Buffer
	if err := WriteJSON(ctx, &buf, input); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, buf.String())
	}

	if len(got) != 2 || got[0]["id"] != "m1" || got[1]["id"] != "m2" {
		t.Fatalf("list result changed: %s", strings.TrimSpace(buf.String()))
	}
}

func TestWriteJSONTransformPreservesLargeNumbers(t *testing.T) {
	ctx := WithJSONTransform(context.Background(), JSONTransform{Select: []string{"id"}})

	var buf bytes.Buffer
	if err := WriteJSON(ctx, &buf, map[string]any{
		"id": int64(9007199254740993),
	}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	if !strings.Contains(buf.String(), "9007199254740993") {
		t.Fatalf("large integer was not preserved: %s", strings.TrimSpace(buf.String()))
	}
}

func TestFromEnvAndParseError(t *testing.T) {
	t.Setenv("GOG_JSON", "yes")
	t.Setenv("GOG_PLAIN", "0")
	mode := FromEnv()

	if !mode.JSON || mode.Plain {
		t.Fatalf("unexpected env mode: %#v", mode)
	}

	if err := (&ParseError{msg: "boom"}).Error(); err != "boom" {
		t.Fatalf("unexpected parse error: %q", err)
	}
}

func TestFromEnvJSONWinsOverPlain(t *testing.T) {
	t.Setenv("GOG_JSON", "1")
	t.Setenv("GOG_PLAIN", "1")

	mode := FromEnv()
	if !mode.JSON || mode.Plain {
		t.Fatalf("unexpected env mode: %#v", mode)
	}
}

func TestFromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey{}, "nope")
	if got := FromContext(ctx); got != (Mode{}) {
		t.Fatalf("expected zero mode, got %#v", got)
	}
}
