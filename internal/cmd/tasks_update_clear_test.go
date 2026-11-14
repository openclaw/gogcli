package cmd

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// runTasksUpdatePatch runs tasks update against a fake Tasks server and
// returns the PATCH body it sent ("" when no PATCH was sent).
func runTasksUpdatePatch(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var patched string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/tasks/v1/users/@me/lists":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":"l1","title":"One"}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/tasks/v1/lists/l1/tasks/t1":
			body, _ := io.ReadAll(r.Body)
			patched = strings.TrimSpace(string(body))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"t1","title":"Task","status":"needsAction"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx := withTasksTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), newTasksServiceFromServer(t, srv))
	err := runKong(t, &TasksUpdateCmd{}, append([]string{"l1", "t1"}, args...), ctx, &RootFlags{Account: "a@b.com"})
	return patched, err
}

func TestTasksUpdateCmd_EmptyNotesAndTitleAreSent(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: []string{"--notes", ""}, want: `{"notes":""}`},
		{args: []string{"--title", ""}, want: `{"title":""}`},
		{args: []string{"--due", ""}, want: `{"due":null}`},
		{args: []string{"--notes", "keep"}, want: `{"notes":"keep"}`},
	} {
		got, err := runTasksUpdatePatch(t, tc.args...)
		if err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if got != tc.want {
			t.Fatalf("%v sent PATCH %s, want %s", tc.args, got, tc.want)
		}
	}
}

func TestTasksUpdateCmd_EmptyStatusIsUsageError(t *testing.T) {
	got, err := runTasksUpdatePatch(t, "--status", "")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("err = %v, want usage error", err)
	}
	if got != "" {
		t.Fatalf("sent PATCH %s before rejecting", got)
	}
}
