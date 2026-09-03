package cmd

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

func TestCollectAllPages(t *testing.T) {
	t.Parallel()

	t.Run("empty next token ends", func(t *testing.T) {
		t.Parallel()
		var calls int
		got, err := collectAllPages("", func(pageToken string) ([]string, string, error) {
			calls++
			if pageToken != "" {
				t.Fatalf("pageToken = %q, want empty", pageToken)
			}
			return []string{"a"}, "", nil
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if calls != 1 {
			t.Fatalf("calls = %d, want 1", calls)
		}
		if len(got) != 1 || got[0] != "a" {
			t.Fatalf("got = %#v", got)
		}
	})

	t.Run("two distinct pages succeed", func(t *testing.T) {
		t.Parallel()
		var calls int
		got, err := collectAllPages("", func(pageToken string) ([]string, string, error) {
			calls++
			switch pageToken {
			case "":
				return []string{"a"}, "page-2", nil
			case "page-2":
				return []string{"b"}, "", nil
			default:
				t.Fatalf("unexpected pageToken %q", pageToken)
				return nil, "", nil
			}
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if calls != 2 {
			t.Fatalf("calls = %d, want 2", calls)
		}
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Fatalf("got = %#v", got)
		}
	})

	t.Run("repeated next page token returns an error", func(t *testing.T) {
		t.Parallel()
		var calls int
		got, err := collectAllPages("", func(pageToken string) ([]string, string, error) {
			calls++
			if calls > 4 {
				return nil, "", errors.New("fetch called after repeated token")
			}
			return []string{"a"}, "stuck", nil
		})
		if err == nil || !strings.Contains(err.Error(), "repeated page token") {
			t.Fatalf("err = %v", err)
		}
		if got != nil {
			t.Fatalf("got = %#v, want nil", got)
		}
		if calls != 2 {
			t.Fatalf("calls = %d, want 2", calls)
		}
	})

	t.Run("fetch error discards partial results", func(t *testing.T) {
		t.Parallel()
		fetchErr := errors.New("page two failed")
		var calls int
		got, err := collectAllPages("", func(pageToken string) ([]string, string, error) {
			calls++
			if calls == 1 {
				return []string{"a"}, "page-2", nil
			}
			return nil, "", fetchErr
		})
		if !errors.Is(err, fetchErr) || got != nil || calls != 2 {
			t.Fatalf("got = %#v, err = %v, calls = %d", got, err, calls)
		}
	})

	t.Run("multi-token cycle stops before revisiting a page", func(t *testing.T) {
		t.Parallel()
		nextTokens := []string{"a", "b", "a"}
		var calls int
		got, err := collectAllPages("", func(pageToken string) ([]string, string, error) {
			calls++
			if calls > len(nextTokens) {
				return nil, "", errors.New("fetch called after cycle")
			}
			return []string{"item"}, nextTokens[calls-1], nil
		})
		if got != nil || err == nil || !strings.Contains(err.Error(), "repeated page token") || calls != 3 {
			t.Fatalf("got = %#v, err = %v, calls = %d", got, err, calls)
		}
	})

	t.Run("unique tokens respect the page limit", func(t *testing.T) {
		t.Parallel()
		var calls int
		got, err := collectAllPages("", func(pageToken string) ([]int, string, error) {
			calls++
			if calls > 10_000 {
				return nil, "", errors.New("fetch called beyond page limit")
			}
			return []int{calls}, strconv.Itoa(calls), nil
		})
		if got != nil || err == nil || !strings.Contains(err.Error(), "pagination exceeded") || calls != 10_000 {
			t.Fatalf("rows = %d, err = %v, calls = %d", len(got), err, calls)
		}
	})
}
