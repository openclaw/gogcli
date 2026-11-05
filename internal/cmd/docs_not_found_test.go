package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/openclaw/gogcli/internal/errfmt"
)

func TestDocsNotFound_ExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  any
		args []string
	}{
		{name: "cat", cmd: &DocsCatCmd{}, args: []string{"missing"}},
		{name: "info", cmd: &DocsInfoCmd{}, args: []string{"missing"}},
		{name: "write append", cmd: &DocsWriteCmd{}, args: []string{"missing", "--text", "hello", "--append"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{"code": http.StatusNotFound, "message": "Requested entity was not found.", "status": "NOT_FOUND"},
				})
			}))
			defer cleanup()

			ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)
			err := runKong(t, tt.cmd, tt.args, ctx, &RootFlags{Account: "a@b.com"})
			if got := ExitCode(stableExitCode(err)); got != exitCodeNotFound {
				t.Fatalf("expected exit code %d, got %d: %v", exitCodeNotFound, got, err)
			}
			if got := errfmt.Format(err); got != "doc not found or not a Google Doc (id=missing)" {
				t.Fatalf("unexpected message: %q", got)
			}
		})
	}
}
