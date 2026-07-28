package outfmt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type Mode struct {
	JSON  bool
	Plain bool
}

type ParseError struct{ msg string }

func (e *ParseError) Error() string { return e.msg }

func FromFlags(jsonOut bool, plainOut bool) (Mode, error) {
	if jsonOut && plainOut {
		return Mode{}, &ParseError{msg: "invalid output mode (cannot combine --json and --plain)"}
	}

	return Mode{JSON: jsonOut, Plain: plainOut}, nil
}

func FromEnv() Mode {
	jsonOut := envBool("GOG_JSON")

	plainOut := envBool("GOG_PLAIN")
	if jsonOut {
		plainOut = false
	}

	return Mode{
		JSON:  jsonOut,
		Plain: plainOut,
	}
}

type ctxKey struct{}

func WithMode(ctx context.Context, mode Mode) context.Context {
	return context.WithValue(ctx, ctxKey{}, mode)
}

func FromContext(ctx context.Context) Mode {
	if v := ctx.Value(ctxKey{}); v != nil {
		if m, ok := v.(Mode); ok {
			return m
		}
	}

	return Mode{}
}

func IsJSON(ctx context.Context) bool  { return FromContext(ctx).JSON }
func IsPlain(ctx context.Context) bool { return FromContext(ctx).Plain }

type JSONTransform struct {
	// ResultsOnly unwraps the top-level envelope and emits only the primary results
	// (best-effort; drops metadata like nextPageToken).
	ResultsOnly bool
	// Select projects objects to only the requested fields (comma-separated; supports dot paths).
	// When applied to a list, it projects each element.
	Select []string
}

type jsonTransformKey struct{}

type primaryResult struct {
	value any
}

// PrimaryResult marks a complete JSON value as already being the command's
// primary result. WriteJSON still applies field selection, but --results-only
// preserves the complete value instead of inferring a nested collection.
func PrimaryResult(value any) any {
	return primaryResult{value: value}
}

func WithJSONTransform(ctx context.Context, t JSONTransform) context.Context {
	return context.WithValue(ctx, jsonTransformKey{}, t)
}

func JSONTransformFromContext(ctx context.Context) (JSONTransform, bool) {
	v := ctx.Value(jsonTransformKey{})
	if v == nil {
		return JSONTransform{}, false
	}

	t, ok := v.(JSONTransform)

	return t, ok
}

func WriteJSON(ctx context.Context, w io.Writer, v any) error {
	alreadyPrimary := false

	if result, ok := v.(primaryResult); ok {
		v = result.value
		alreadyPrimary = true
	}

	if t, ok := JSONTransformFromContext(ctx); ok && (t.ResultsOnly || len(t.Select) > 0) {
		if alreadyPrimary {
			t.ResultsOnly = false
		}

		transformed, err := applyJSONTransform(v, t)
		if err != nil {
			return fmt.Errorf("transform json: %w", err)
		}
		v = transformed
	}

	if opts, ok := UntrustedWrapperFromContext(ctx); ok {
		wrapped, err := wrapUntrustedJSONValue(v, opts)
		if err != nil {
			return fmt.Errorf("wrap untrusted json: %w", err)
		}
		v = wrapped
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}

	return nil
}

func applyJSONTransform(v any, t JSONTransform) (any, error) {
	// Convert typed structs into a generic representation so we can manipulate them.
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	var anyV any
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()

	if err := dec.Decode(&anyV); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if t.ResultsOnly {
		anyV = unwrapPrimary(anyV)
	}

	if len(t.Select) > 0 {
		anyV = selectFields(anyV, t.Select)
	}

	return anyV, nil
}

func unwrapPrimary(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}

	// Explicit common convention.
	if r, ok := m["results"]; ok {
		return r
	}

	// Exclude known envelope/meta keys.
	meta := map[string]struct{}{
		"nextPageToken": {},
		"next_cursor":   {},
		"has_more":      {},
		"count":         {},
		"query":         {},
		"dry_run":       {},
		"dryRun":        {},
		"op":            {},
		"action":        {},
		"note":          {},
		"notes":         {},
	}

	candidates := make([]string, 0, len(m))
	for k := range m {
		if _, ok := meta[k]; ok {
			continue
		}
		candidates = append(candidates, k)
	}

	sort.Strings(candidates)

	if len(candidates) == 1 {
		return m[candidates[0]]
	}

	// Prefer known result arrays before any generic array candidate.
	known := []string{
		"files",
		"threads",
		"messages",
		"labels",
		"events",
		"calendars",
		"courses",
		"topics",
		"announcements",
		"materials",
		"coursework",
		"submissions",
		"invitations",
		"guardians",
		"notes",
		"contacts",
		"people",
		"tasks",
		"lists",
		"groups",
		"members",
		"drives",
		"rules",
		"colors",
		"spaces",
		"request",
	}
	for _, k := range known {
		if val, ok := m[k]; ok {
			if _, isArray := val.([]any); isArray {
				return val
			}
		}
	}

	// If we have any array/slice-like candidates, prefer those.
	for _, k := range candidates {
		if _, ok := m[k].([]any); ok {
			return m[k]
		}
	}

	for _, k := range known {
		if val, ok := m[k]; ok {
			return val
		}
	}

	return v
}

func selectFields(v any, fields []string) any {
	switch vv := v.(type) {
	case []any:
		out := make([]any, 0, len(vv))
		for _, it := range vv {
			out = append(out, selectFieldsFromItem(it, fields))
		}

		return out
	default:
		return selectFieldsFromItem(v, fields)
	}
}

func selectFieldsFromItem(v any, fields []string) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}

	out := make(map[string]any, len(fields))
	for _, f := range fields {
		if val, ok := getAtPath(m, f); ok {
			out[f] = val
		}
	}

	return out
}

func getAtPath(v any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	segs := strings.Split(path, ".")
	cur := v

	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return nil, false
		}

		switch c := cur.(type) {
		case map[string]any:
			next, ok := c[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(c) {
				return nil, false
			}
			cur = c[i]
		default:
			return nil, false
		}
	}

	return cur, true
}

func KeyValuePayload(key string, value any) map[string]any {
	return map[string]any{
		"key":   key,
		"value": value,
	}
}

func KeysPayload(keys []string) map[string]any {
	return map[string]any{
		"keys": keys,
	}
}

func PathPayload(path string) map[string]any {
	return map[string]any{
		"path": path,
	}
}

func envBool(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
