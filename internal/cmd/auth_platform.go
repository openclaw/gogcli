package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
)

const (
	authPlatformScope             = "https://www.googleapis.com/auth/cloud-platform"
	defaultAuthPlatformBaseURL    = "https://cloudconsole-pa.clients6.google.com/v3/entityServices/OauthEntityService/schemas/OAUTH_GRAPHQL:batchGraphql"
	defaultCloudResourceManagerV1 = "https://cloudresourcemanager.googleapis.com/v1"

	authPlatformReadOp       = "GetTrustedUserList"
	authPlatformReadSig      = "2/MOTEiszs0jB3+r4gNdOqOHc6zxU1rHoLGwOZgzGJWNo="
	authPlatformReadQuery    = `query GetTrustedUserList($projectNumber: Int64Value!) @Signature(bytes: "2/MOTEiszs0jB3+r4gNdOqOHc6zxU1rHoLGwOZgzGJWNo=") { getTrustedUserList(projectNumber: $projectNumber) { userAccount } }`
	authPlatformWriteOp      = "SetTrustedUserList"
	authPlatformWriteSig     = "2/7gA8JWHyqFx3hPWBgvLvbsZAwIBEI2HTpajRUpYPVZM="
	authPlatformWriteQuery   = `mutation SetTrustedUserList($projectNumber: Int64Value!, $trustedUserList: [String]!) @Signature(bytes: "2/7gA8JWHyqFx3hPWBgvLvbsZAwIBEI2HTpajRUpYPVZM=") { setTrustedUserList(projectNumber: $projectNumber, trustedUserList: $trustedUserList) { trustedUserList { userAccount } } }`
	authPlatformClientVer    = "gogcli-auth-platform"
	authPlatformPagePath     = "/auth/audience"
	authPlatformDefaultLocal = "en_US"
)

type AuthPlatformCmd struct {
	Testers AuthPlatformTestersCmd `cmd:"" name:"testers" help:"Manage OAuth beta/test users"`
}

type AuthPlatformTestersCmd struct {
	List   AuthPlatformTestersListCmd   `cmd:"" name:"list" help:"List OAuth beta/test users"`
	Add    AuthPlatformTestersAddCmd    `cmd:"" name:"add" help:"Add an OAuth beta/test user idempotently"`
	Remove AuthPlatformTestersRemoveCmd `cmd:"" name:"remove" help:"Remove an OAuth beta/test user idempotently"`
}

type authPlatformProjectFlags struct {
	// --cloud-project avoids colliding with the existing global --project JSON projection alias.
	Project       string `name:"cloud-project" required:"" help:"Google Cloud project ID or number that owns the OAuth consent screen"`
	ProjectNumber string `name:"project-number" help:"Google Cloud project number (skips Cloud Resource Manager lookup)"`
}

type AuthPlatformTestersListCmd struct {
	authPlatformProjectFlags
}

type AuthPlatformTestersAddCmd struct {
	authPlatformProjectFlags
	Email string `name:"email" required:"" help:"Google account email to allow as a beta/test user"`
}

type AuthPlatformTestersRemoveCmd struct {
	authPlatformProjectFlags
	Email string `name:"email" required:"" help:"Google account email to remove from beta/test users"`
}

type authPlatformClient struct {
	httpClient *http.Client
	baseURL    string
	crmBaseURL string
	apiKey     string
}

type authPlatformTesterResult struct {
	Project       string   `json:"project"`
	ProjectNumber string   `json:"project_number"`
	Testers       []string `json:"testers"`
	Changed       bool     `json:"changed,omitempty"`
	Email         string   `json:"email,omitempty"`
	Action        string   `json:"-"`
}

func (c *AuthPlatformTestersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	client, account, err := newAuthPlatformCommandClient(ctx, flags)
	if err != nil {
		return err
	}
	projectNumber, err := client.resolveProjectNumber(ctx, c.Project, c.ProjectNumber)
	if err != nil {
		return err
	}
	testers, err := client.listTesters(ctx, c.Project, projectNumber)
	if err != nil {
		return wrapAuthPlatformError(err, account)
	}
	return writeAuthPlatformTesterResult(ctx, authPlatformTesterResult{
		Project:       c.Project,
		ProjectNumber: projectNumber,
		Testers:       sortedCopy(testers),
	})
}

func (c *AuthPlatformTestersAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	email, err := normalizeTesterEmail(c.Email)
	if err != nil {
		return err
	}
	if googleapi.ReadOnly(ctx) {
		return fmt.Errorf("%w: auth-platform testers add", googleapi.ErrReadOnly)
	}
	if err = dryRunExit(ctx, flags, "auth-platform.testers.add", map[string]any{
		"project": c.Project,
		"email":   email,
	}); err != nil {
		return err
	}

	client, account, err := newAuthPlatformCommandClient(ctx, flags)
	if err != nil {
		return err
	}
	projectNumber, err := client.resolveProjectNumber(ctx, c.Project, c.ProjectNumber)
	if err != nil {
		return err
	}
	before, err := client.listTesters(ctx, c.Project, projectNumber)
	if err != nil {
		return wrapAuthPlatformError(err, account)
	}
	if containsEmailFold(before, email) {
		return writeAuthPlatformTesterResult(ctx, authPlatformTesterResult{
			Project:       c.Project,
			ProjectNumber: projectNumber,
			Testers:       sortedCopy(before),
			Changed:       false,
			Email:         email,
			Action:        "add",
		})
	}
	after := append(append([]string(nil), before...), email)
	written, err := client.setTesters(ctx, c.Project, projectNumber, after)
	if err != nil {
		return wrapAuthPlatformError(err, account)
	}
	if !containsEmailFold(written, email) {
		return fmt.Errorf("auth-platform tester add verification failed for %s", email)
	}
	return writeAuthPlatformTesterResult(ctx, authPlatformTesterResult{
		Project:       c.Project,
		ProjectNumber: projectNumber,
		Testers:       sortedCopy(written),
		Changed:       true,
		Email:         email,
		Action:        "add",
	})
}

func (c *AuthPlatformTestersRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	email, err := normalizeTesterEmail(c.Email)
	if err != nil {
		return err
	}
	if googleapi.ReadOnly(ctx) {
		return fmt.Errorf("%w: auth-platform testers remove", googleapi.ErrReadOnly)
	}
	if err = dryRunExit(ctx, flags, "auth-platform.testers.remove", map[string]any{
		"project": c.Project,
		"email":   email,
	}); err != nil {
		return err
	}
	if err = confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), "remove OAuth beta/test user "+email); err != nil {
		return err
	}

	client, account, err := newAuthPlatformCommandClient(ctx, flags)
	if err != nil {
		return err
	}
	projectNumber, err := client.resolveProjectNumber(ctx, c.Project, c.ProjectNumber)
	if err != nil {
		return err
	}
	before, err := client.listTesters(ctx, c.Project, projectNumber)
	if err != nil {
		return wrapAuthPlatformError(err, account)
	}
	after := removeEmailFold(before, email)
	if len(after) == len(before) {
		return writeAuthPlatformTesterResult(ctx, authPlatformTesterResult{
			Project:       c.Project,
			ProjectNumber: projectNumber,
			Testers:       sortedCopy(before),
			Changed:       false,
			Email:         email,
			Action:        "remove",
		})
	}
	written, err := client.setTesters(ctx, c.Project, projectNumber, after)
	if err != nil {
		return wrapAuthPlatformError(err, account)
	}
	if containsEmailFold(written, email) {
		return fmt.Errorf("auth-platform tester remove verification failed for %s", email)
	}
	return writeAuthPlatformTesterResult(ctx, authPlatformTesterResult{
		Project:       c.Project,
		ProjectNumber: projectNumber,
		Testers:       sortedCopy(written),
		Changed:       true,
		Email:         email,
		Action:        "remove",
	})
}

func newAuthPlatformCommandClient(ctx context.Context, flags *RootFlags) (*authPlatformClient, string, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return nil, "", err
	}
	httpClient, err := googleapi.NewHTTPClientForScopes(ctx, "auth-platform", account, []string{authPlatformScope})
	if err != nil {
		return nil, "", err
	}
	return &authPlatformClient{
		httpClient: httpClient,
		baseURL:    envOr("GOG_AUTH_PLATFORM_BASE_URL", defaultAuthPlatformBaseURL),
		crmBaseURL: strings.TrimRight(envOr("GOG_CLOUD_RESOURCE_MANAGER_BASE_URL", defaultCloudResourceManagerV1), "/"),
		apiKey:     strings.TrimSpace(os.Getenv("GOG_AUTH_PLATFORM_API_KEY")),
	}, account, nil
}

func (c *authPlatformClient) resolveProjectNumber(ctx context.Context, project string, override string) (string, error) {
	project = strings.TrimSpace(project)
	override = strings.TrimSpace(override)
	if override != "" {
		return override, nil
	}
	if project == "" {
		return "", usage("missing --cloud-project")
	}
	if allDigits(project) {
		return project, nil
	}
	requestURL := c.crmBaseURL + "/projects/" + url.PathEscape(project)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("build project lookup request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("lookup project number: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read project lookup response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &googleapi.HTTPStatusError{Code: resp.StatusCode, Status: resp.Status, Err: fmt.Errorf("project lookup failed: %s", strings.TrimSpace(string(raw)))}
	}
	var parsed struct {
		ProjectNumber string `json:"projectNumber"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode project lookup response: %w", err)
	}
	if parsed.ProjectNumber == "" {
		return "", fmt.Errorf("project lookup response did not include projectNumber")
	}
	return parsed.ProjectNumber, nil
}

func (c *authPlatformClient) listTesters(ctx context.Context, project string, projectNumber string) ([]string, error) {
	var parsed struct {
		GetTrustedUserList struct {
			UserAccount []string `json:"userAccount"`
		} `json:"getTrustedUserList"`
	}
	if err := c.call(ctx, project, authPlatformReadOp, authPlatformReadSig, authPlatformReadQuery, map[string]any{
		"projectNumber": projectNumber,
	}, &parsed); err != nil {
		return nil, err
	}
	return normalizeTesterList(parsed.GetTrustedUserList.UserAccount), nil
}

func (c *authPlatformClient) setTesters(ctx context.Context, project string, projectNumber string, testers []string) ([]string, error) {
	var parsed struct {
		SetTrustedUserList struct {
			TrustedUserList struct {
				UserAccount []string `json:"userAccount"`
			} `json:"trustedUserList"`
		} `json:"setTrustedUserList"`
	}
	if err := c.call(ctx, project, authPlatformWriteOp, authPlatformWriteSig, authPlatformWriteQuery, map[string]any{
		"projectNumber":   projectNumber,
		"trustedUserList": normalizeTesterList(testers),
	}, &parsed); err != nil {
		return nil, err
	}
	return normalizeTesterList(parsed.SetTrustedUserList.TrustedUserList.UserAccount), nil
}

func (c *authPlatformClient) call(ctx context.Context, project string, operation string, signature string, query string, variables map[string]any, out any) error {
	endpoint, err := authPlatformEndpoint(c.baseURL, c.apiKey)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"requestContext": map[string]any{
			"platformMetadata": map[string]any{"platformType": "RIF"},
			"p2Metadata": map[string]any{
				"feature":     "features/1691453455344",
				"environment": "environments/production",
				"extension":   "extensions/oauth",
			},
			"clientVersion":   authPlatformClientVer,
			"pagePath":        authPlatformPagePath,
			"projectId":       project,
			"selectedPurview": map[string]any{"projectId": project},
			"jurisdiction":    "global",
			"localizationData": map[string]any{
				"locale":   authPlatformDefaultLocal,
				"timezone": "UTC",
			},
		},
		"querySignature": signature,
		"operationName":  operation,
		"query":          query,
		"variables":      variables,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode auth-platform request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build auth-platform request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://console.cloud.google.com")
	if c.apiKey == "" {
		req.Header.Set("X-Goog-User-Project", project)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call auth-platform %s: %w", operation, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read auth-platform response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &googleapi.HTTPStatusError{Code: resp.StatusCode, Status: resp.Status, Err: fmt.Errorf("auth-platform %s failed: %s", operation, strings.TrimSpace(string(raw)))}
	}
	var envelope []struct {
		Results []struct {
			Data  json.RawMessage `json:"data"`
			Error json.RawMessage `json:"error"`
		} `json:"results"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode auth-platform response: %w", err)
	}
	if len(envelope) == 0 || len(envelope[0].Results) == 0 {
		return fmt.Errorf("auth-platform %s response did not include results", operation)
	}
	if len(envelope[0].Error) > 0 && string(envelope[0].Error) != jsonNullLiteral {
		return fmt.Errorf("auth-platform %s error: %s", operation, string(envelope[0].Error))
	}
	result := envelope[0].Results[0]
	if len(result.Error) > 0 && string(result.Error) != jsonNullLiteral {
		return fmt.Errorf("auth-platform %s error: %s", operation, string(result.Error))
	}
	if err := json.Unmarshal(result.Data, out); err != nil {
		return fmt.Errorf("decode auth-platform %s data: %w", operation, err)
	}
	return nil
}

func authPlatformEndpoint(baseURL string, apiKey string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultAuthPlatformBaseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse auth-platform endpoint: %w", err)
	}
	q := u.Query()
	if strings.TrimSpace(apiKey) != "" {
		q.Set("key", strings.TrimSpace(apiKey))
	}
	q.Set("prettyPrint", "false")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func normalizeTesterEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		return "", usage("missing --email")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || addr.Name != "" || strings.ContainsAny(email, " <>") {
		return "", usagef("invalid --email %q: expected a bare Google account email address", raw)
	}
	return strings.ToLower(email), nil
}

func normalizeTesterList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		email, err := normalizeTesterEmail(value)
		if err != nil {
			continue
		}
		if !containsEmailFold(out, email) {
			out = append(out, email)
		}
	}
	return out
}

func containsEmailFold(values []string, email string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), email) {
			return true
		}
	}
	return false
}

func removeEmailFold(values []string, email string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), email) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func writeAuthPlatformTesterResult(ctx context.Context, result authPlatformTesterResult) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}
	if outfmt.IsPlain(ctx) {
		for _, tester := range result.Testers {
			fmt.Fprintln(stdoutWriter(ctx), tester)
		}
		return nil
	}
	if result.Email != "" {
		if result.Changed {
			state := "present"
			if result.Action == "remove" {
				state = "absent"
			}
			fmt.Fprintf(stdoutWriter(ctx), "Updated OAuth beta/test users for %s; %s is %s\n", result.Project, result.Email, state)
		} else {
			fmt.Fprintf(stdoutWriter(ctx), "OAuth beta/test users for %s already matched requested state for %s\n", result.Project, result.Email)
		}
		return nil
	}
	for _, tester := range result.Testers {
		fmt.Fprintln(stdoutWriter(ctx), tester)
	}
	return nil
}

func wrapAuthPlatformError(err error, account string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "cloudclient-pa.googleapis.com") || strings.Contains(msg, "Cloud Client Private API") {
		return fmt.Errorf("%w\nAuth Platform tester management currently uses Google Cloud Console's private backend. If Google rejects the call, use the Console UI fallback or enable a supported Auth Platform API when Google publishes one", err)
	}
	if strings.Contains(msg, "insufficient") || strings.Contains(msg, "PERMISSION_DENIED") {
		switch account {
		case adcPlaceholderAccount:
			return fmt.Errorf("%w\nADC principal needs oauthconfig.testusers.get, oauthconfig.testusers.update, and resourcemanager.projects.get on the project", err)
		case accessTokenPlaceholderAccount:
			return fmt.Errorf("%w\nDirect access token needs cloud-platform scope and OAuthConfig test-user permissions on the project", err)
		default:
			return fmt.Errorf("%w\nAccount %s needs oauthconfig.testusers.get, oauthconfig.testusers.update, and resourcemanager.projects.get on the project", err, account)
		}
	}
	if errors.Is(err, googleapi.ErrReadOnly) {
		return err
	}
	return err
}
