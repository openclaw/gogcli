package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/config"
)

func mcpEnabledToolsForRun(ctx context.Context, cmd McpCmd, flags *RootFlags) ([]mcpToolSpec, string, error) {
	store, err := commandConfigStore(ctx)
	if err != nil {
		return nil, "", err
	}
	cfg, err := store.Read()
	if err != nil {
		return nil, "", err
	}
	if cfg.MCP == nil {
		return mcpEnabledTools(cmd), "", nil
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

	allowWrite := policy.AllowWrite
	if flags != nil && flags.ReadOnly {
		allowWrite = false
	}
	runtimeAllow := splitCommaValues(cmd.AllowTool)
	tools := make([]mcpToolSpec, 0, len(mcpAllTools()))
	for _, tool := range mcpAllTools() {
		if tool.Risk == mcpRiskWrite && !allowWrite {
			continue
		}
		if !mcpToolAllowed(tool, policy.AllowTools) {
			continue
		}
		if len(runtimeAllow) > 0 && !mcpToolAllowed(tool, runtimeAllow) {
			continue
		}
		tools = append(tools, tool)
	}
	return tools, nil
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
