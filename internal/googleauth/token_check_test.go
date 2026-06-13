package googleauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
)

func TestCheckRefreshTokenSuccess(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}

		if r.Form.Get("refresh_token") != "good" {
			http.Error(w, "bad token", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	oauthEndpoint = oauth2.Endpoint{AuthURL: srv.URL, TokenURL: srv.URL}

	if err := CheckRefreshToken(context.Background(), "default", "good", []string{"scope"}, time.Second); err != nil {
		t.Fatalf("CheckRefreshToken: %v", err)
	}
}

func TestCheckRefreshTokenFailure(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token", http.StatusBadRequest)
	}))
	defer srv.Close()

	oauthEndpoint = oauth2.Endpoint{AuthURL: srv.URL, TokenURL: srv.URL}

	err := CheckRefreshToken(context.Background(), "default", "bad", []string{"scope"}, time.Second)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestCheckRefreshTokenReadCredentialsError(t *testing.T) {
	origRead := readClientCredentials

	t.Cleanup(func() { readClientCredentials = origRead })

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{}, errBoom
	}

	err := CheckRefreshToken(context.Background(), "default", "good", []string{"scope"}, time.Second)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestCheckRefreshTokenUsesContextCredentials(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})
	readClientCredentials = nil

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	oauthEndpoint = oauth2.Endpoint{AuthURL: srv.URL, TokenURL: srv.URL}
	ctx := authclient.WithCredentialsReader(context.Background(), func(client string) (config.ClientCredentials, error) {
		if client != "runtime" {
			t.Fatalf("client = %q", client)
		}

		return config.ClientCredentials{ClientID: "runtime-id", ClientSecret: "runtime-secret"}, nil
	})

	if err := CheckRefreshToken(ctx, "runtime", "good", []string{"scope"}, time.Second); err != nil {
		t.Fatalf("CheckRefreshToken: %v", err)
	}
}
