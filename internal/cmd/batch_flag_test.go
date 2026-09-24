package cmd

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func TestExecuteRejectsEmptyBatchFlag(t *testing.T) {
	commands := [][]string{
		{"forms", "add-question", "form1", "--title", "Question"},
		{"forms", "delete-question", "form1", "0"},
		{"forms", "move-question", "form1", "0", "1"},
		{"forms", "update", "form1", "--quiz=false"},
		{"forms", "questions", "add", "form1", "--title", "Question"},
		{"forms", "questions", "delete", "form1", "0"},
		{"forms", "questions", "move", "form1", "0", "1"},
		{"docs", "write", "doc1", "--text", "text"},
		{"docs", "update", "doc1", "--text", "text"},
		{"docs", "insert", "doc1", "text", "--index", "1"},
		{"docs", "delete", "doc1", "--start", "1", "--end", "2"},
		{"docs", "format", "doc1", "--bold"},
		{"docs", "cell-style", "doc1", "--row", "1", "--col", "1", "--bold"},
		{"docs", "table-column-width", "doc1", "--col", "1", "--width", "50"},
		{"docs", "insert-person", "doc1", "--email", "person@example.com"},
		{"docs", "insert-file-chip", "doc1", "--file-id", "file1"},
		{"docs", "insert-date-chip", "doc1", "--date", "2026-09-24"},
		{"docs", "insert-page-break", "doc1", "--index", "1"},
		{"docs", "insert-section-break", "doc1", "--index", "1"},
		{"docs", "insert-horizontal-rule", "doc1", "--index", "1"},
		{"docs", "section-columns", "doc1", "--count", "2"},
		{"slides", "new-slide", "deck1"},
		{"slides", "element", "create-shape", "deck1", "slide1"},
		{"slides", "element", "delete", "deck1", "shape1"},
		{"slides", "insert-text", "deck1", "shape1", "text"},
		{"slides", "style-text", "deck1", "shape1", "--range", "0:4", "--bold"},
		{"slides", "paragraph-style", "deck1", "shape1", "--align", "CENTER"},
		{"slides", "table", "create", "deck1", "slide1", "--rows", "2", "--cols", "2"},
		{"slides", "table", "cell", "style", "deck1", "table1", "--row", "0", "--col", "0", "--bold"},
		{"slides", "table", "merge", "deck1", "table1", "--row", "0", "--col", "0", "--col-span", "2"},
	}
	for _, command := range commands {
		for _, batchFlag := range [][]string{{"--batch", ""}, {"--batch="}, {"--batch", " \t\n"}} {
			for _, dryRun := range []bool{false, true} {
				name := strings.Join(command[:2], "/") + "/" + strings.Join(batchFlag, " ")
				if dryRun {
					name += "/dry-run"
				}
				t.Run(name, func(t *testing.T) {
					args := []string{"--account", "user@example.com", "--no-input", "--json"}
					if dryRun {
						args = append(args, "--dry-run")
					}
					args = append(args, command...)
					args = append(args, batchFlag...)
					result := executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
						Docs: func(context.Context, string) (*docs.Service, error) {
							t.Error("empty --batch reached authentication")
							return nil, errors.New("unexpected authentication")
						},
					}})
					if ExitCode(result.err) != 2 || !strings.Contains(result.stderr, "--batch requires a non-empty batch ID") || !strings.Contains(result.stderr, "omit --batch") {
						t.Fatalf("error = %v; stderr = %q", result.err, result.stderr)
					}
					if result.stdout != "" {
						t.Fatalf("unexpected success output: %s", result.stdout)
					}
				})
			}
		}
	}
}

func TestExecuteRejectsEmptyBatchBeforeReadingInput(t *testing.T) {
	for _, command := range []string{"write", "update", "insert"} {
		t.Run(command, func(t *testing.T) {
			input := &batchInputReader{}
			err := executeWithRuntime([]string{
				"--account", "user@example.com", "--no-input", "docs", command, "doc1", "--file", "-", "--batch=",
			}, &app.Runtime{IO: app.IO{In: input, Out: io.Discard, Err: io.Discard}})
			if input.read {
				t.Fatal("empty --batch consumed input")
			}
			if ExitCode(err) != 2 || !strings.Contains(err.Error(), "--batch requires a non-empty batch ID") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestExecuteBatchFlagPresence(t *testing.T) {
	for _, args := range [][]string{
		{"docs", "insert", "doc1", "text", "--index", "1"},
		{"docs", "insert", "doc1", "text", "--index", "1", "--batch", "01900000-0000-7000-8000-000000000000"},
		{"docs", "insert", "doc1", "--index", "1", "--", "--batch="},
		{"docs", "insert", "doc1", "--index", "1", "--", "--batch"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			result := executeWithTestRuntime(t, append([]string{"--dry-run", "--json"}, args...), nil)
			if result.err != nil {
				t.Fatalf("error = %v; stderr = %q", result.err, result.stderr)
			}
			if !strings.Contains(result.stdout, `"dry_run": true`) {
				t.Fatalf("missing dry-run output: %s", result.stdout)
			}
		})
	}
}

type batchInputReader struct {
	read bool
}

func (r *batchInputReader) Read([]byte) (int, error) {
	r.read = true
	return 0, errors.New("unexpected input read")
}
