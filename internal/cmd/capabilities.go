package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
)

const (
	capabilitiesSchemaVersion = 1
	authMethodADC             = "adc"
)

type CapabilitiesCmd struct {
	IncludeAuth    bool `name:"include-auth" help:"Inspect the selected credential and include granted services, scopes, and token expiry"`
	IncludeAccount bool `name:"include-account" help:"Include the selected account identity"`
}

type capabilitiesSnapshot struct {
	SchemaVersion int                  `json:"schema_version"`
	Build         string               `json:"build"`
	Disclosure    capabilityDisclosure `json:"disclosure"`
	Automation    capabilityAutomation `json:"automation"`
	Auth          capabilityAuth       `json:"auth"`
	Safety        capabilitySafety     `json:"safety"`
	Discovery     capabilityDiscovery  `json:"discovery"`
	MCP           *capabilityMCP       `json:"mcp,omitempty"`
}

type capabilityDisclosure struct {
	AuthInspected   bool `json:"auth_inspected"`
	AccountIncluded bool `json:"account_included"`
}

type capabilityAutomation struct {
	OutputFormats        []string `json:"output_formats"`
	NoInputFlag          string   `json:"no_input_flag"`
	DryRunFlag           string   `json:"dry_run_flag"`
	UntrustedContentFlag string   `json:"untrusted_content_flag"`
}

type capabilityAuth struct {
	SupportedMethods     []string `json:"supported_methods"`
	AccountSelected      bool     `json:"account_selected"`
	Method               string   `json:"method,omitempty"`
	Account              string   `json:"account,omitempty"`
	Client               string   `json:"client,omitempty"`
	Services             []string `json:"services,omitempty"`
	Scopes               []string `json:"scopes,omitempty"`
	AccessTokenExpiresAt string   `json:"access_token_expires_at,omitempty"`
}

type capabilitySafety struct {
	DryRun        bool                   `json:"dry_run"`
	NoInput       bool                   `json:"no_input"`
	WrapUntrusted bool                   `json:"wrap_untrusted"`
	GmailNoSend   bool                   `json:"gmail_no_send"`
	BakedProfile  capabilityBakedProfile `json:"baked_profile"`
	CommandRules  capabilityCommandRules `json:"command_rules"`
}

type capabilityBakedProfile struct {
	Enabled bool   `json:"enabled"`
	Name    string `json:"name,omitempty"`
}

type capabilityCommandRules struct {
	EnabledPrefixes []string `json:"enabled_prefixes"`
	EnabledExact    []string `json:"enabled_exact"`
	Disabled        []string `json:"disabled"`
}

type capabilityDiscovery struct {
	SchemaCommand    string `json:"schema_command"`
	ExitCodesCommand string `json:"exit_codes_command"`
	MCPToolsCommand  string `json:"mcp_tools_command"`
}

type capabilityMCP struct {
	Tools             []capabilityMCPTool `json:"tools"`
	WriteToolsExposed bool                `json:"write_tools_exposed"`
}

type capabilityMCPTool struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	Risk    string `json:"risk"`
}

type capabilitiesOptions struct {
	IncludeAuth    bool
	IncludeAccount bool
	MCPTools       []mcpToolSpec
}

func (c *CapabilitiesCmd) Run(ctx context.Context, flags *RootFlags) error {
	snapshot, err := buildCapabilities(flags, capabilitiesOptions{
		IncludeAuth:    c.IncludeAuth,
		IncludeAccount: c.IncludeAccount,
	})
	if err != nil {
		return err
	}
	return writeCapabilities(ctx, snapshot)
}

func buildCapabilities(flags *RootFlags, opts capabilitiesOptions) (capabilitiesSnapshot, error) {
	profile, err := loadBakedSafetyProfile()
	if err != nil {
		return capabilitiesSnapshot{}, usagef("invalid baked safety profile: %v", err)
	}

	snapshot := capabilitiesSnapshot{
		SchemaVersion: capabilitiesSchemaVersion,
		Build:         VersionString(),
		Disclosure: capabilityDisclosure{
			AuthInspected: opts.IncludeAuth,
		},
		Automation: capabilityAutomation{
			OutputFormats:        []string{"json", "plain"},
			NoInputFlag:          "--no-input",
			DryRunFlag:           "--dry-run",
			UntrustedContentFlag: "--wrap-untrusted",
		},
		Auth: capabilityAuth{
			SupportedMethods: []string{authTypeOAuth, authTypeServiceAccount, authMethodADC, "access_token"},
		},
		Safety: capabilitySafety{
			BakedProfile: capabilityBakedProfile{
				Enabled: profile.enabled,
				Name:    profile.name,
			},
			CommandRules: capabilityCommandRules{
				EnabledPrefixes: capabilityRuleValues(flags, func(f *RootFlags) string { return f.EnableCommands }),
				EnabledExact:    capabilityRuleValues(flags, func(f *RootFlags) string { return f.EnableCommandsExact }),
				Disabled:        capabilityRuleValues(flags, func(f *RootFlags) string { return f.DisableCommands }),
			},
		},
		Discovery: capabilityDiscovery{
			SchemaCommand:    "gog schema --json",
			ExitCodesCommand: "gog exit-codes --json",
			MCPToolsCommand:  "gog mcp --list-tools",
		},
	}
	if flags != nil {
		snapshot.Safety.DryRun = flags.DryRun
		snapshot.Safety.NoInput = flags.NoInput
		snapshot.Safety.WrapUntrusted = flags.WrapUntrusted
		snapshot.Safety.GmailNoSend = flags.GmailNoSend
	}

	account, accountKnown, err := configuredCapabilityAccount(flags)
	if err != nil {
		return capabilitiesSnapshot{}, err
	}
	if opts.IncludeAuth || opts.IncludeAccount {
		account, err = requireAccount(flags)
		if err != nil {
			return capabilitiesSnapshot{}, err
		}
		accountKnown = strings.TrimSpace(account) != ""
	}
	snapshot.Auth.AccountSelected = accountKnown

	if opts.IncludeAuth {
		if err := inspectCapabilityAuth(flags, account, &snapshot.Auth); err != nil {
			return capabilitiesSnapshot{}, err
		}
	}
	if accountKnown {
		noSend, noSendErr := config.IsNoSendAccount(account)
		if noSendErr != nil {
			return capabilitiesSnapshot{}, noSendErr
		}
		snapshot.Safety.GmailNoSend = snapshot.Safety.GmailNoSend || noSend
	}
	if opts.IncludeAccount && accountKnown && capabilityAccountIsIdentity(account) {
		snapshot.Auth.Account = account
		snapshot.Disclosure.AccountIncluded = true
	}

	if opts.MCPTools != nil {
		snapshot.MCP = buildCapabilityMCP(opts.MCPTools)
	}
	return snapshot, nil
}

func capabilityAccountIsIdentity(account string) bool {
	account = strings.TrimSpace(account)
	if account == "" || account == accessTokenPlaceholderAccount {
		return false
	}
	return !googleapi.IsADCMode() || account != authMethodADC
}

func configuredCapabilityAccount(flags *RootFlags) (string, bool, error) {
	account, ok, err := configuredAccount(flags)
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(account), ok, nil
}

func inspectCapabilityAuth(flags *RootFlags, account string, auth *capabilityAuth) error {
	if auth == nil {
		return nil
	}
	if googleapi.IsADCMode() {
		auth.Method = authMethodADC
		return nil
	}
	if hasDirectAccessToken(flags) {
		auth.Method = "access_token"
		return nil
	}
	if _, _, ok := bestServiceAccountPathAndMtime(normalizeEmail(account)); ok {
		auth.Method = authTypeServiceAccount
		return nil
	}

	client, err := resolveClientForEmail(account, flags)
	if err != nil {
		return err
	}
	store, err := openSecretsStore()
	if err != nil {
		return fmt.Errorf("open secrets store: %w", err)
	}
	token, err := store.GetToken(client, account)
	if err != nil {
		return fmt.Errorf("read token metadata: %w", err)
	}
	auth.Method = authTypeOAuth
	auth.Client = client
	auth.Services = sortedUniqueStrings(token.Services)
	auth.Scopes = sortedUniqueStrings(token.Scopes)
	if !token.AccessTokenExpiresAt.IsZero() {
		auth.AccessTokenExpiresAt = token.AccessTokenExpiresAt.UTC().Format(time.RFC3339)
	}
	return nil
}

func capabilityRuleValues(flags *RootFlags, value func(*RootFlags) string) []string {
	if flags == nil {
		return []string{}
	}
	return sortedUniqueStrings(splitCommaValues([]string{value(flags)}))
}

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func buildCapabilityMCP(tools []mcpToolSpec) *capabilityMCP {
	out := &capabilityMCP{Tools: make([]capabilityMCPTool, 0, len(tools))}
	for _, tool := range tools {
		out.Tools = append(out.Tools, capabilityMCPTool{
			Name:    tool.Name,
			Service: tool.Service,
			Risk:    string(tool.Risk),
		})
		if tool.Risk == mcpRiskWrite {
			out.WriteToolsExposed = true
		}
	}
	return out
}

func writeCapabilities(ctx context.Context, snapshot capabilitiesSnapshot) error {
	// Local process metadata is trusted and should not inherit result projections.
	ctx = outfmt.WithJSONTransform(ctx, outfmt.JSONTransform{})
	ctx = outfmt.WithUntrustedWrapper(ctx, outfmt.UntrustedWrapOptions{})
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, snapshot)
	}

	lines := [][2]string{
		{"schema_version", fmt.Sprintf("%d", snapshot.SchemaVersion)},
		{"build", snapshot.Build},
		{"auth_inspected", fmt.Sprintf("%t", snapshot.Disclosure.AuthInspected)},
		{"account_included", fmt.Sprintf("%t", snapshot.Disclosure.AccountIncluded)},
		{"output_formats", strings.Join(snapshot.Automation.OutputFormats, ",")},
		{"no_input_flag", snapshot.Automation.NoInputFlag},
		{"dry_run_flag", snapshot.Automation.DryRunFlag},
		{"untrusted_content_flag", snapshot.Automation.UntrustedContentFlag},
		{"auth_supported_methods", strings.Join(snapshot.Auth.SupportedMethods, ",")},
		{"account_selected", fmt.Sprintf("%t", snapshot.Auth.AccountSelected)},
		{"auth_method", snapshot.Auth.Method},
		{"account", snapshot.Auth.Account},
		{"client", snapshot.Auth.Client},
		{"services", strings.Join(snapshot.Auth.Services, ",")},
		{"scopes", strings.Join(snapshot.Auth.Scopes, ",")},
		{"access_token_expires_at", snapshot.Auth.AccessTokenExpiresAt},
		{"dry_run", fmt.Sprintf("%t", snapshot.Safety.DryRun)},
		{"no_input", fmt.Sprintf("%t", snapshot.Safety.NoInput)},
		{"wrap_untrusted", fmt.Sprintf("%t", snapshot.Safety.WrapUntrusted)},
		{"gmail_no_send", fmt.Sprintf("%t", snapshot.Safety.GmailNoSend)},
		{"baked_profile_enabled", fmt.Sprintf("%t", snapshot.Safety.BakedProfile.Enabled)},
		{"baked_profile_name", snapshot.Safety.BakedProfile.Name},
		{"enabled_command_prefixes", strings.Join(snapshot.Safety.CommandRules.EnabledPrefixes, ",")},
		{"enabled_commands_exact", strings.Join(snapshot.Safety.CommandRules.EnabledExact, ",")},
		{"disabled_commands", strings.Join(snapshot.Safety.CommandRules.Disabled, ",")},
		{"schema_command", snapshot.Discovery.SchemaCommand},
		{"exit_codes_command", snapshot.Discovery.ExitCodesCommand},
		{"mcp_tools_command", snapshot.Discovery.MCPToolsCommand},
	}
	if snapshot.MCP != nil {
		lines = append(lines,
			[2]string{"mcp_tools", strings.Join(capabilityMCPToolNames(snapshot.MCP.Tools), ",")},
			[2]string{"mcp_write_tools_exposed", fmt.Sprintf("%t", snapshot.MCP.WriteToolsExposed)},
		)
	}
	for _, line := range lines {
		if line[1] == "" {
			continue
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s\t%s\n", line[0], line[1])
	}
	return nil
}

func capabilityMCPToolNames(tools []capabilityMCPTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}
