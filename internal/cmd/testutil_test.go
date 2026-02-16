package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/googleauth"
)

// withPrimaryCalendar wraps an http.Handler to also respond to primary calendar requests
// with a default timezone. This is needed because time-aware commands now fetch the
// user's timezone from their primary calendar.
func withPrimaryCalendar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle primary calendar list request for timezone
		if strings.Contains(r.URL.Path, "/calendarList/primary") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "primary",
				"summary":  "Test Calendar",
				"timeZone": "UTC",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func pickTimezoneExcluding(t *testing.T, exclude ...string) string {
	t.Helper()

	excluded := make(map[string]struct{}, len(exclude))
	for _, v := range exclude {
		excluded[strings.ToLower(v)] = struct{}{}
	}

	candidates := []string{
		"UTC",
		"America/New_York",
		"America/Los_Angeles",
		"Europe/London",
		"Asia/Tokyo",
	}

	for _, tz := range candidates {
		if _, ok := excluded[strings.ToLower(tz)]; ok {
			continue
		}
		if _, err := time.LoadLocation(tz); err != nil {
			continue
		}
		return tz
	}

	t.Skipf("no suitable timezone available (exclude=%v)", exclude)
	return ""
}

func pickNonLocalTimezone(t *testing.T) string {
	t.Helper()
	return pickTimezoneExcluding(t, time.Local.String(), "local")
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = orig
	b, _ := io.ReadAll(r)
	_ = r.Close()
	return string(b)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	fn()

	_ = w.Close()
	os.Stderr = orig
	b, _ := io.ReadAll(r)
	_ = r.Close()
	return string(b)
}

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()

	orig := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r

	_, _ = io.WriteString(w, input)
	_ = w.Close()

	fn()

	_ = r.Close()
	os.Stdin = orig
}

func runKong(t *testing.T, cmd any, args []string, ctx context.Context, flags *RootFlags) (err error) {
	t.Helper()

	parser, err := kong.New(
		cmd,
		kong.Vars(kong.Vars{
			"auth_services": googleauth.UserServiceCSV(),
		}),
		kong.Writers(io.Discard, io.Discard),
		kong.Exit(func(code int) { panic(exitPanic{code: code}) }),
	)
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			if ep, ok := r.(exitPanic); ok {
				if ep.code == 0 {
					err = nil
					return
				}
				err = &ExitError{Code: ep.code, Err: errors.New("exited")}
				return
			}
			panic(r)
		}
	}()

	kctx, err := parser.Parse(args)
	if err != nil {
		return err
	}

	if ctx != nil {
		kctx.BindTo(ctx, (*context.Context)(nil))
	}
	if flags == nil {
		flags = &RootFlags{}
	}
	kctx.Bind(flags)

	return kctx.Run()
}
