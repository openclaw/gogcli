package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/openclaw/gogcli/internal/config"
)

func mcpEnabledToolsForRun(ctx context.Context, cmd McpCmd, flags *RootFlags) ([]mcpToolSpec, string, error) {
	if cmd.AllowTool != nil && len(splitCommaValues(cmd.AllowTool)) == 0 {
		return nil, "", usage("--allow-tool must contain at least one selector")
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return nil, "", err
	}
	cfg, err := store.Read()
	if err != nil {
		return nil, "", err
	}
	if cfg.MCP == nil {
		if (cmd.AllowGmailSend || cmd.AllowGmailDelete) && !cmd.AllowWrite {
			return nil, "", usage("--allow-gmail-send and --allow-gmail-delete require --allow-write")
		}
		return mcpEnabledTools(cmd, flags), "", nil
	}

	account := ""
	if len(cfg.MCP.Accounts) > 0 {
		account, err = resolveMCPPolicyAccount(flags)
		if err != nil {
			return nil, "", fmt.Errorf("resolve account for MCP policy: %w", err)
		}
	}
	policy, err := selectMCPPolicy(*cfg.MCP, account)
	if err != nil {
		return nil, "", err
	}
	tools, err := mcpEnabledToolsWithPolicy(cmd, flags, policy)
	return tools, account, err
}

func resolveMCPPolicyAccount(flags *RootFlags) (string, error) {
	if hasDirectAccessToken(flags) || isADCAuthMode(flags) {
		// Account values are only labels in these modes, so they must never select
		// a per-account authorization policy. The global policy still applies.
		return "", nil
	}
	return requireAccount(flags)
}

func selectMCPPolicy(cfg config.MCPConfig, account string) (config.MCPPolicy, error) {
	policy, err := normalizeMCPPolicy(cfg.MCPPolicy)
	if err != nil {
		return config.MCPPolicy{}, fmt.Errorf("global MCP policy: %w", err)
	}
	normalizedAccount := normalizeMCPAccount(account)
	seen := make(map[string]string, len(cfg.Accounts))
	for configuredAccount, accountPolicy := range cfg.Accounts {
		normalized := normalizeMCPAccount(configuredAccount)
		if normalized == "" {
			return config.MCPPolicy{}, usage("MCP policy account must not be empty")
		}
		if previous, ok := seen[normalized]; ok {
			return config.MCPPolicy{}, usagef("duplicate MCP policy accounts %q and %q", previous, configuredAccount)
		}
		seen[normalized] = configuredAccount
		normalizedPolicy, err := normalizeMCPPolicy(accountPolicy)
		if err != nil {
			return config.MCPPolicy{}, fmt.Errorf("MCP policy account %q: %w", configuredAccount, err)
		}
		if normalized == normalizedAccount {
			// Account entries are complete policies, not inherited patches.
			policy = normalizedPolicy
		}
	}
	return policy, nil
}

func normalizeMCPPolicy(policy config.MCPPolicy) (config.MCPPolicy, error) {
	explicitSelectors := splitCommaValues(policy.AllowTools)
	selectorsProvided := policy.AllowTools != nil
	if selectorsProvided && len(explicitSelectors) == 0 {
		return config.MCPPolicy{}, usage("MCP policy allow_tools must contain at least one selector")
	}
	if policy.AllowWrite && !selectorsProvided {
		return config.MCPPolicy{}, usage("MCP policy allow_write requires an explicit allow_tools list")
	}
	if (policy.AllowGmailSend || policy.AllowGmailDelete) && !policy.AllowWrite {
		return config.MCPPolicy{}, usage("MCP policy allow_gmail_send and allow_gmail_delete require allow_write")
	}
	if !selectorsProvided {
		explicitSelectors = []string{string(mcpRiskRead)}
	}
	for _, selector := range explicitSelectors {
		if !mcpSelectorMatchesAnyTool(selector) {
			return config.MCPPolicy{}, usagef("MCP policy allow_tools selector %q matches no tool", selector)
		}
	}
	policy.AllowTools = explicitSelectors
	return policy, nil
}

func mcpEnabledToolsWithPolicy(cmd McpCmd, flags *RootFlags, policy config.MCPPolicy) ([]mcpToolSpec, error) {
	if cmd.AllowWrite && !policy.AllowWrite {
		return nil, usage("--allow-write cannot widen the configured MCP policy")
	}
	if cmd.AllowGmailSend && !policy.AllowGmailSend {
		return nil, usage("--allow-gmail-send cannot widen the configured MCP policy")
	}
	if cmd.AllowGmailDelete && !policy.AllowGmailDelete {
		return nil, usage("--allow-gmail-delete cannot widen the configured MCP policy")
	}
	return mcpFilterTools(policy, splitCommaValues(cmd.AllowTool), flags), nil
}

func mcpEnabledTools(cmd McpCmd, flags *RootFlags) []mcpToolSpec {
	return mcpFilterTools(config.MCPPolicy{
		AllowWrite:       cmd.AllowWrite,
		AllowGmailSend:   cmd.AllowGmailSend,
		AllowGmailDelete: cmd.AllowGmailDelete,
	}, splitCommaValues(cmd.AllowTool), flags)
}

func mcpFilterTools(policy config.MCPPolicy, runtimeAllow []string, flags *RootFlags) []mcpToolSpec {
	if flags != nil && flags.ReadOnly {
		policy.AllowWrite = false
	}
	tools := make([]mcpToolSpec, 0, len(mcpAllTools()))
	for _, tool := range mcpAllTools() {
		if tool.Risk != mcpRiskRead && !policy.AllowWrite {
			continue
		}
		switch tool.Capability {
		case "":
		case mcpCapabilityGmailSend:
			if !policy.AllowWrite || !policy.AllowGmailSend {
				continue
			}
		case mcpCapabilityGmailDelete:
			if !policy.AllowWrite || !policy.AllowGmailDelete {
				continue
			}
		default:
			continue
		}
		if len(policy.AllowTools) > 0 && !mcpToolAllowed(tool, policy.AllowTools) {
			continue
		}
		if len(runtimeAllow) > 0 && !mcpToolAllowed(tool, runtimeAllow) {
			continue
		}
		tools = append(tools, tool)
	}
	return tools
}

func mcpSelectorMatchesAnyTool(selector string) bool {
	for _, tool := range mcpAllTools() {
		if mcpToolAllowed(tool, []string{selector}) {
			return true
		}
	}
	return false
}

func normalizeMCPAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}
