package outfmt

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteRaw_CompactByDefault(t *testing.T) {
	var buf bytes.Buffer

	in := map[string]any{"id": "abc", "nested": map[string]any{"k": "v"}}

	if err := WriteRaw(context.Background(), &buf, in, RawOptions{}); err != nil {
		t.Fatalf("err: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "  ") || strings.Contains(out, "\n ") {
		t.Fatalf("expected compact output, got: %q", out)
	}
	// Must still end with newline for pipe friendliness.
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("expected trailing newline, got: %q", out)
	}

	var round map[string]any

	if err := json.Unmarshal([]byte(out), &round); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if round["id"] != "abc" {
		t.Fatalf("unexpected id: %v", round["id"])
	}
}

func TestWriteRaw_Pretty(t *testing.T) {
	var buf bytes.Buffer

	in := map[string]any{"id": "abc", "nested": map[string]any{"k": "v"}}

	if err := WriteRaw(context.Background(), &buf, in, RawOptions{Pretty: true}); err != nil {
		t.Fatalf("err: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "\n  ") {
		t.Fatalf("expected indented output, got: %q", out)
	}

	var round map[string]any
	if err := json.Unmarshal([]byte(out), &round); err != nil {
		t.Fatalf("pretty output is not valid JSON: %v", err)
	}
}

func TestWriteRaw_NoHTMLEscape(t *testing.T) {
	var buf bytes.Buffer

	in := map[string]any{"url": "https://example.com/?a=1&b=2"}

	if err := WriteRaw(context.Background(), &buf, in, RawOptions{}); err != nil {
		t.Fatalf("err: %v", err)
	}

	if strings.Contains(buf.String(), "\\u0026") {
		t.Fatalf("expected raw & in output, got: %q", buf.String())
	}
}

func TestWriteRaw_BareStruct_NoWrapper(t *testing.T) {
	// raw must emit the value as-is, with no envelope/wrapper added.
	type apiResp struct {
		DocumentId string `json:"documentId"`
		Title      string `json:"title"`
	}
	var buf bytes.Buffer

	if err := WriteRaw(context.Background(), &buf, apiResp{DocumentId: "d1", Title: "t"}, RawOptions{}); err != nil {
		t.Fatalf("err: %v", err)
	}

	var round map[string]any

	if err := json.Unmarshal(buf.Bytes(), &round); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if _, hasWrapper := round["document"]; hasWrapper {
		t.Fatalf("raw output must not be wrapped under a key, got: %v", round)
	}

	if round["documentId"] != "d1" {
		t.Fatalf("expected documentId=d1, got: %v", round["documentId"])
	}
}

func TestWriteRaw_MarshalError(t *testing.T) {
	var buf bytes.Buffer

	// channels cannot be marshaled to JSON.
	err := WriteRaw(context.Background(), &buf, make(chan int), RawOptions{})
	if err == nil {
		t.Fatalf("expected error marshaling channel")
	}
}
