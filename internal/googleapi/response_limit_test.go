package googleapi

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/secrets"
)

func TestResponseLimitUnknownLengthAndDecompression(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		for _, size := range []int{16, 17} {
			t.Run(fmt.Sprintf("%d/gzip=%t", size, compressed), func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if compressed {
						w.Header().Set("Content-Encoding", "gzip")
					}

					w.(http.Flusher).Flush()

					var out io.Writer = w
					if compressed {
						z := gzip.NewWriter(w)
						defer z.Close()
						out = z
					}
					_, _ = io.WriteString(out, strings.Repeat("x", size))
				}))
				defer srv.Close()
				ctx := WithResponseByteLimit(context.Background(), 16)
				client := &http.Client{Transport: responseLimitedTransportFromContext(ctx, http.DefaultTransport)}

				req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
				if err != nil {
					t.Fatal(err)
				}
				// Explicit encoding also has to be bounded after decompression.
				if compressed {
					req.Header.Set("Accept-Encoding", "gzip")
				}

				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()

				data, err := io.ReadAll(resp.Body)
				if size == 16 {
					if err != nil || string(data) != strings.Repeat("x", 16) {
						t.Fatalf("boundary response = %q, %v", data, err)
					}
				} else if !errors.Is(err, ErrResponseTooLarge) || len(data) > 16 {
					t.Fatalf("oversized response = %d bytes, %v", len(data), err)
				}
			})
		}
	}
}

type responseLimitTestBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *responseLimitTestBody) Close() error {
	b.closed.Store(true)
	return nil
}

func TestResponseLimitClosesOnError(t *testing.T) {
	ctx := WithResponseByteLimit(context.Background(), 4)
	body := &responseLimitTestBody{Reader: strings.NewReader("12345")}
	tr := responseLimitedTransportFromContext(ctx, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, ContentLength: -1}, nil
	}))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = resp.Body.Close() }()

	if _, err := io.ReadAll(resp.Body); !errors.Is(err, ErrResponseTooLarge) || !body.closed.Load() {
		t.Fatalf("limit error=%v, closed=%v", err, body.closed.Load())
	}
}

func TestResponseLimitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(WithResponseByteLimit(context.Background(), 4))
	body := &responseLimitTestBody{Reader: strings.NewReader("1234")}
	tr := responseLimitedTransportFromContext(ctx, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, ContentLength: -1}, nil
	}))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	defer func() { _ = resp.Body.Close() }()

	if _, err := io.ReadAll(resp.Body); !errors.Is(err, context.Canceled) || !body.closed.Load() {
		t.Fatalf("canceled read=%v closed=%v", err, body.closed.Load())
	}
}

func TestResponseLimitKeepsSafetyAndQuota(t *testing.T) {
	for _, mode := range []string{"direct", "adc", "service-account-options", "stored", "service-account"} {
		t.Run(mode, func(t *testing.T) {
			var gets, writes atomic.Int32

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Goog-User-Project") != "test-quota" || r.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Error("auth/quota headers lost")
				}

				if r.Method == http.MethodGet {
					gets.Add(1)
				} else {
					writes.Add(1)
				}
				_, _ = io.WriteString(w, `{"items":[],"padding":"012345678901234567890123456789"}`)
			}))
			defer srv.Close()
			ctx := WithResponseByteLimit(context.Background(), 16)
			ctx = authclient.WithQuotaProject(ctx, "test-quota")
			ctx = WithReadOnly(ctx, true)

			switch mode {
			case "direct":
				ctx = authclient.WithAccessToken(ctx, "fixture-token")
			case "adc", "service-account-options":
				ctx = WithAuthDependencies(ctx, AuthDependencies{
					Mode: AuthModeADC,
					ADCTokenSource: func(context.Context, ...string) (oauth2.TokenSource, error) {
						return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fixture-token"}), nil
					},
				})
			default:
				resolvedCtx, serviceAccounts := testServiceAccountContext(t, ctx)

				deps, _ := authDependenciesFromContext(resolvedCtx)
				if mode == "stored" {
					deps.OpenTokens = func() (secrets.Store, error) {
						return &stubStore{tok: secrets.Token{Email: "fixture@example.invalid", RefreshToken: "fixture-refresh", AccessToken: "fixture-token", AccessTokenExpiresAt: time.Now().Add(time.Hour)}}, nil
					}
				} else {
					if _, err := serviceAccounts.Write("fixture@example.invalid", []byte(`{"type":"service_account"}`)); err != nil {
						t.Fatal(err)
					}
					deps.ServiceAccountTokenSource = func(context.Context, []byte, string, []string) (oauth2.TokenSource, error) {
						return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fixture-token"}), nil
					}
				}
				ctx = WithAuthDependencies(resolvedCtx, deps)
			}
			var opts []option.ClientOption

			var err error
			if mode == "service-account-options" {
				opts, err = optionsForServiceAccountScopes(ctx, "tasks", "fixture@example.invalid", []string{"scope"})
			} else {
				opts, err = optionsForAccountScopes(ctx, "tasks", "fixture@example.invalid", []string{"scope"})
			}

			if err != nil {
				t.Fatal(err)
			}

			svc, err := tasks.NewService(ctx, append(opts, option.WithEndpoint(srv.URL+"/"))...)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := svc.Tasklists.List().Context(ctx).Do(); !errors.Is(err, ErrResponseTooLarge) {
				t.Fatalf("generated client bypassed response cap: %v", err)
			}

			if _, err := svc.Tasklists.Insert(&tasks.TaskList{Title: "blocked"}).Context(ctx).Do(); err == nil {
				t.Fatal("readonly write accepted")
			}

			if gets.Load() != 1 || writes.Load() != 0 {
				t.Fatalf("provider gets/writes=%d/%d", gets.Load(), writes.Load())
			}
		})
	}
}
