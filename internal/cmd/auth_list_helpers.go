package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

type authTokenReadError struct {
	Client string
	Email  string
	Err    error
}

type authListEntry struct {
	Email    string
	Client   string
	Token    *secrets.Token
	SA       bool
	ReadErr  error
	ReadHint string
}

type authListJSONItem struct {
	Email     string   `json:"email"`
	Subject   string   `json:"subject,omitempty"`
	Client    string   `json:"client,omitempty"`
	Services  []string `json:"services,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	Auth      string   `json:"auth"`
	Valid     *bool    `json:"valid,omitempty"`
	Error     string   `json:"error,omitempty"`
	Hint      string   `json:"hint,omitempty"`
}

func listAuthTokensWithFallback(store secrets.Store) ([]secrets.Token, []authTokenReadError, error) {
	tokens, err := store.ListTokens()
	if err == nil {
		return tokens, nil, nil
	}

	return readableTokens(store)
}

func filterAuthListTokensByClient(tokens []secrets.Token, client string) []secrets.Token {
	filtered := make([]secrets.Token, 0, len(tokens))
	for _, tok := range tokens {
		if authListCanonicalClient(tok.Client) == client {
			filtered = append(filtered, tok)
		}
	}
	return filtered
}

func filterAuthListReadErrorsByClient(readErrors []authTokenReadError, client string) []authTokenReadError {
	filtered := make([]authTokenReadError, 0, len(readErrors))
	for _, readErr := range readErrors {
		if authListCanonicalClient(readErr.Client) == client {
			filtered = append(filtered, readErr)
		}
	}
	return filtered
}

func buildAuthListEntries(tokens []secrets.Token, tokenReadErrors []authTokenReadError, serviceAccountEmails []string) []authListEntry {
	sort.Slice(tokens, func(i, j int) bool {
		left := authListTokenKey(tokens[i].Client, tokens[i].Email)
		right := authListTokenKey(tokens[j].Client, tokens[j].Email)
		return left < right
	})

	entries := make([]authListEntry, 0, len(tokens)+len(serviceAccountEmails)+len(tokenReadErrors))
	seen := make(map[string]struct{})
	for _, email := range serviceAccountEmails {
		email = normalizeEmail(email)
		if email == "" {
			continue
		}
		key := authListServiceAccountKey(email)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, authListEntryForServiceAccount(email))
	}
	for _, t := range tokens {
		email := normalizeEmail(t.Email)
		if email == "" {
			continue
		}
		key := authListTokenKey(t.Client, email)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		tok := t
		entries = append(entries, authListEntry{Email: email, Token: &tok})
	}
	for _, readErr := range tokenReadErrors {
		email := normalizeEmail(readErr.Email)
		if email == "" {
			continue
		}
		key := authListTokenKey(readErr.Client, email)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, authListEntryForReadError(email, readErr))
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Email != entries[j].Email {
			return entries[i].Email < entries[j].Email
		}
		clientI, _, _, _ := entries[i].details()
		clientJ, _, _, _ := entries[j].details()
		if clientI != clientJ {
			return clientI < clientJ
		}
		return entries[i].authType() < entries[j].authType()
	})

	return entries
}

func authListTokenKey(client string, email string) string {
	return "oauth\t" + authListCanonicalClient(client) + "\t" + normalizeEmail(email)
}

func authListServiceAccountKey(email string) string {
	return "service-account\t" + normalizeEmail(email)
}

func authListCanonicalClient(client string) string {
	client = strings.TrimSpace(client)
	if client == "" {
		return literalDefault
	}
	return client
}

func authListEntryForServiceAccount(email string) authListEntry {
	return authListEntry{Email: email, SA: true}
}

func authListEntryForReadError(email string, readErr authTokenReadError) authListEntry {
	entry := authListEntry{Email: email, Client: readErr.Client, ReadErr: readErr.Err}
	_, entry.ReadHint = classifyAuthDoctorError(readErr.Err)

	return entry
}

func (e authListEntry) authType() string {
	if e.SA && (e.Token != nil || e.ReadErr != nil) {
		return authTypeOAuthServiceAccount
	}
	if e.SA {
		return authTypeServiceAccount
	}

	return authTypeOAuth
}

func (e authListEntry) details() (client string, created string, services []string, scopes []string) {
	client = e.Client
	if e.Token != nil {
		if !e.Token.CreatedAt.IsZero() {
			created = e.Token.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		return e.Token.Client, created, e.Token.Services, e.Token.Scopes
	}
	if e.SA {
		if _, mtime, ok := bestServiceAccountPathAndMtime(e.Email); ok {
			created = mtime.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		return client, created, []string{"service-account"}, nil
	}

	return client, created, nil, nil
}

func (c *AuthListCmd) writeAuthListJSON(ctx context.Context, entries []authListEntry) error {
	out := make([]authListJSONItem, 0, len(entries))
	for _, e := range entries {
		client, created, services, scopes := e.details()
		it := authListJSONItem{
			Email:     e.Email,
			Client:    client,
			Services:  services,
			Scopes:    scopes,
			CreatedAt: created,
			Auth:      e.authType(),
		}
		if e.Token != nil {
			it.Subject = e.Token.Subject
		}
		if e.ReadErr != nil {
			it.Error = e.ReadErr.Error()
			it.Hint = e.ReadHint
		}
		if c.Check {
			c.annotateAuthListCheck(ctx, e, &it)
		}
		out = append(out, it)
	}

	return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"accounts": out})
}

func (c *AuthListCmd) annotateAuthListCheck(ctx context.Context, e authListEntry, it *authListJSONItem) {
	switch {
	case e.ReadErr != nil:
		valid := false
		it.Valid = &valid
	case e.Token == nil:
		valid := true
		it.Valid = &valid
		it.Error = "service account (not checked)"
	default:
		err := checkRefreshToken(ctx, e.Token.Client, e.Token.RefreshToken, e.Token.Scopes, c.Timeout)
		valid := err == nil
		it.Valid = &valid
		if err != nil {
			it.Error = err.Error()
		}
	}
}

func (c *AuthListCmd) writeAuthListText(ctx context.Context, u *ui.UI, entries []authListEntry) error {
	for _, e := range entries {
		writeAuthListReadWarning(u, e)

		client, created, services, _ := e.details()
		servicesCSV := strings.Join(services, ",")
		if c.Check {
			c.writeAuthListCheckRow(ctx, u, e, client, servicesCSV, created)
			continue
		}

		u.Out().Printf("%s\t%s\t%s\t%s\t%s", e.Email, client, servicesCSV, created, e.authType())
	}

	return nil
}

func writeAuthListReadWarning(u *ui.UI, e authListEntry) {
	if e.ReadErr == nil {
		return
	}
	u.Err().Printf("WARN\t%s\t%s", e.Email, e.ReadErr.Error())
	if e.ReadHint != "" {
		u.Err().Printf("hint\t%s\t%s", e.Email, e.ReadHint)
	}
}

func (c *AuthListCmd) writeAuthListCheckRow(ctx context.Context, u *ui.UI, e authListEntry, client string, servicesCSV string, created string) {
	switch {
	case e.ReadErr != nil:
		u.Out().Printf("%s\t%s\t%s\t%s\t%t\t%s\t%s", e.Email, client, servicesCSV, created, false, e.ReadErr.Error(), e.authType())
	case e.Token == nil:
		u.Out().Printf("%s\t%s\t%s\t%s\t%t\t%s\t%s", e.Email, client, servicesCSV, created, true, "service account (not checked)", e.authType())
	default:
		err := checkRefreshToken(ctx, e.Token.Client, e.Token.RefreshToken, e.Token.Scopes, c.Timeout)
		valid := err == nil
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		u.Out().Printf("%s\t%s\t%s\t%s\t%t\t%s\t%s", e.Email, client, servicesCSV, created, valid, msg, e.authType())
	}
}

func readableTokens(store secrets.Store) ([]secrets.Token, []authTokenReadError, error) {
	keys, err := store.Keys()
	if err != nil {
		return nil, nil, fmt.Errorf("list tokens: %w", err)
	}

	out := make([]secrets.Token, 0)
	readErrors := make([]authTokenReadError, 0)
	seen := make(map[string]struct{})

	for _, key := range keys {
		client, email, ok := secrets.ParseTokenKey(key)
		if !ok {
			continue
		}
		keyID := client + "\n" + email
		if _, ok := seen[keyID]; ok {
			continue
		}
		seen[keyID] = struct{}{}

		tok, err := store.GetToken(client, email)
		if err != nil {
			readErrors = append(readErrors, authTokenReadError{
				Client: client,
				Email:  email,
				Err:    fmt.Errorf("read token for %s: %w", email, err),
			})
			continue
		}

		if tok.Subject != "" {
			subjectID := tok.Client + "\nsub:" + tok.Subject
			if _, ok := seen[subjectID]; ok {
				continue
			}
			seen[subjectID] = struct{}{}
		}
		out = append(out, tok)
	}

	return out, readErrors, nil
}
