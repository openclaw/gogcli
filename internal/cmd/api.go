package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/steipete/gogcli/internal/discoveryapi"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
)

const maxDiscoveryResponseBytes = 64 << 20

var errDiscoveryResponseTooLarge = errors.New("discovery API response exceeds output limit")

type APICmd struct {
	List     APIListCmd     `cmd:"" help:"List Google Discovery APIs"`
	Describe APIDescribeCmd `cmd:"" help:"Describe a Discovery API or method"`
	Call     APICallCmd     `cmd:"" help:"Call a Discovery-described API method"`
}

type APIListCmd struct {
	All bool `name:"all" help:"Include non-preferred API versions"`
}

type APIDescribeCmd struct {
	API     string `arg:"" name:"api" help:"Discovery API name (for example gmail)"`
	Version string `arg:"" name:"version" help:"Discovery API version (for example v1)"`
	Method  string `arg:"" optional:"" name:"method" help:"Optional Discovery method ID"`
}

type APICallCmd struct {
	API        string `arg:"" name:"api" help:"Discovery API name"`
	Version    string `arg:"" name:"version" help:"Discovery API version"`
	Method     string `arg:"" name:"method" help:"Discovery method ID"`
	ParamsJSON string `name:"params" help:"JSON object of path and query parameters" default:"{}"`
	BodyJSON   string `name:"body" help:"JSON request body or @file"`
	Scope      string `name:"scope" help:"OAuth scope override (default: narrowest Discovery-listed scope)"`
	AllowWrite bool   `name:"allow-write" help:"Allow non-read HTTP methods (also requires confirmation or --force)"`
}

func discoveryClient() discoveryapi.Client {
	return discoveryapi.Client{BaseURL: os.Getenv("GOG_DISCOVERY_BASE_URL")}
}

func (c *APIListCmd) Run(ctx context.Context) error {
	raw, err := discoveryClient().List(ctx, !c.All)
	if err != nil {
		return err
	}

	return writeDiscoveryRaw(ctx, raw)
}

func (c *APIDescribeCmd) Run(ctx context.Context) error {
	description, err := discoveryClient().Description(ctx, c.API, c.Version)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.Method) == "" {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"name": description.Name, "version": description.Version, "title": description.Title,
			"documentation_link": description.DocumentationLink, "methods": discoveryapi.Methods(description),
		})
	}

	method, err := discoveryapi.FindMethod(description, c.Method)
	if err != nil {
		return usage(err.Error())
	}

	return outfmt.WriteJSON(ctx, stdoutWriter(ctx), method)
}

func (c *APICallCmd) Run(ctx context.Context, flags *RootFlags) error {
	description, err := discoveryClient().Description(ctx, c.API, c.Version)
	if err != nil {
		return err
	}
	method, err := discoveryapi.FindMethod(description, c.Method)
	if err != nil {
		return usage(err.Error())
	}

	params := map[string]any{}
	paramsJSON := strings.TrimSpace(c.ParamsJSON)
	if paramsJSON == "" {
		paramsJSON = "{}"
	}
	if decodeErr := json.Unmarshal([]byte(paramsJSON), &params); decodeErr != nil {
		return usagef("invalid --params JSON: %v", decodeErr)
	}
	requestURL, err := discoveryapi.BuildURL(description, method, params)
	if err != nil {
		return usage(err.Error())
	}
	body, err := readDiscoveryBody(c.BodyJSON)
	if err != nil {
		return err
	}

	read := method.Spec.HttpMethod == http.MethodGet || method.Spec.HttpMethod == http.MethodHead
	if !read && !c.AllowWrite {
		return usagef("method %s uses %s; pass --allow-write to opt in", method.ID, method.Spec.HttpMethod)
	}
	plan := map[string]any{"api": c.API, "version": c.Version, "method": method.ID, "http_method": method.Spec.HttpMethod, "url": requestURL, "has_body": len(body) > 0}
	if dryRunErr := dryRunExit(ctx, flags, "api.call", plan); dryRunErr != nil {
		return dryRunErr
	}
	if !read {
		if confirmErr := confirmDestructiveChecked(ctx, flags, "invoke Discovery method "+method.ID); confirmErr != nil {
			return confirmErr
		}
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if guardErr := checkDiscoveryGmailNoSend(ctx, flags, account, method.ID); guardErr != nil {
		return guardErr
	}
	scopes, err := discoveryScopes(method.Spec.Scopes, c.Scope)
	if err != nil {
		return usage(err.Error())
	}
	httpClient, err := googleapi.NewHTTPClientForScopes(ctx, "discovery:"+c.API, account, scopes)
	if err != nil {
		return err
	}
	request, err := discoveryapi.NewRequest(ctx, method.Spec.HttpMethod, requestURL, body)
	if err != nil {
		return fmt.Errorf("build API request: %w", err)
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call %s: %w", method.ID, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxDiscoveryResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read API response: %w", err)
	}
	if len(raw) > maxDiscoveryResponseBytes {
		return fmt.Errorf("%w (%d bytes)", errDiscoveryResponseTooLarge, maxDiscoveryResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &googleapi.HTTPStatusError{Code: response.StatusCode, Status: response.Status, Err: fmt.Errorf("%s: %s", method.ID, strings.TrimSpace(string(raw)))}
	}

	return writeDiscoveryResponse(ctx, response.Header.Get("Content-Type"), raw)
}

func discoveryScopes(available []string, override string) ([]string, error) {
	override = strings.TrimSpace(override)
	if override != "" {
		for _, scope := range available {
			if scope == override {
				return []string{override}, nil
			}
		}

		return nil, fmt.Errorf("scope %q is not listed for this Discovery method", override)
	}
	if len(available) == 0 {
		return nil, nil
	}

	best := available[0]
	for _, scope := range available[1:] {
		if discoveryScopeScore(scope) < discoveryScopeScore(best) {
			best = scope
		}
	}

	return []string{best}, nil
}

func discoveryScopeScore(scope string) int {
	normalized := strings.ToLower(scope)
	score := len(scope)
	switch {
	case strings.Contains(normalized, "readonly"):
		return score
	case strings.HasSuffix(normalized, ".file"):
		return 1000 + score
	default:
		return 2000 + score
	}
}

func checkDiscoveryGmailNoSend(ctx context.Context, flags *RootFlags, account, methodID string) error {
	if methodID != "gmail.users.messages.send" && methodID != "gmail.users.drafts.send" {
		return nil
	}
	if flags != nil && flags.GmailNoSend {
		return usage("Gmail sending is blocked by --gmail-no-send")
	}

	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	cfg, err := store.Read()
	if err != nil {
		return err
	}
	if cfg.GmailNoSend {
		return usage("Gmail sending is blocked by config gmail_no_send")
	}

	return checkAccountNoSend(ctx, account)
}

func readDiscoveryBody(value string) (json.RawMessage, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if strings.HasPrefix(value, "@") {
		raw, err := os.ReadFile(strings.TrimPrefix(value, "@"))
		if err != nil {
			return nil, err
		}
		value = string(raw)
	}
	if !json.Valid([]byte(value)) {
		return nil, usage("--body must be valid JSON or @file")
	}

	return json.RawMessage(value), nil
}

func writeDiscoveryRaw(ctx context.Context, raw json.RawMessage) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("decode JSON response: %w", err)
	}

	return outfmt.WriteJSON(ctx, stdoutWriter(ctx), value)
}

func writeDiscoveryResponse(ctx context.Context, contentType string, raw []byte) error {
	if json.Valid(raw) {
		return writeDiscoveryRaw(ctx, raw)
	}
	if options, ok := outfmt.UntrustedWrapperFromContext(ctx); ok && discoveryTextResponse(contentType, raw) {
		raw = []byte(outfmt.WrapUntrustedContent(string(raw), options))
	}

	if _, err := stdoutWriter(ctx).Write(raw); err != nil {
		return fmt.Errorf("write API response: %w", err)
	}

	return nil
}

func discoveryTextResponse(contentType string, raw []byte) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	if strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "xml") || strings.Contains(contentType, "csv") || strings.Contains(contentType, "html") {
		return true
	}

	return contentType == "" && utf8.Valid(raw)
}
