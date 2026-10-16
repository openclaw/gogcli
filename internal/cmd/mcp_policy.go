package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/config"
)

func mcpEnabledToolsForRun(ctx context.Context, cmd McpCmd, flags *RootFlags) ([]mcpToolSpec, string, config.MCPPolicy, error) {
	if cmd.AllowTool != nil && len(splitCommaValues(cmd.AllowTool)) == 0 {
		return nil, "", config.MCPPolicy{}, usage("--allow-tool must contain at least one selector")
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return nil, "", config.MCPPolicy{}, err
	}
	cfg, err := store.Read()
	if err != nil {
		return nil, "", config.MCPPolicy{}, err
	}
	if cfg.MCP == nil {
		if (cmd.AllowCalendarNotify || cmd.AllowCalendarDelete || cmd.AllowGmailSettingsDelete) && !cmd.AllowWrite {
			return nil, "", config.MCPPolicy{}, usage("Calendar and settings capabilities require --allow-write")
		}
		if (cmd.AllowGmailSend || cmd.AllowGmailDelete) && !cmd.AllowWrite {
			return nil, "", config.MCPPolicy{}, usage("--allow-gmail-send and --allow-gmail-delete require --allow-write")
		}
		policy := config.MCPPolicy{AllowWrite: cmd.AllowWrite, AllowGmailSend: cmd.AllowGmailSend, AllowGmailDelete: cmd.AllowGmailDelete, AllowCalendarNotify: cmd.AllowCalendarNotify, AllowCalendarDelete: cmd.AllowCalendarDelete, AllowGmailSettingsDelete: cmd.AllowGmailSettingsDelete}
		return mcpEnabledTools(cmd, flags), "", policy, nil
	}

	account := ""
	if len(cfg.MCP.Accounts) > 0 {
		account, err = resolveMCPPolicyAccount(flags)
		if err != nil {
			return nil, "", config.MCPPolicy{}, fmt.Errorf("resolve account for MCP policy: %w", err)
		}
	}
	policy, err := selectMCPPolicy(*cfg.MCP, account)
	if err != nil {
		return nil, "", config.MCPPolicy{}, err
	}
	tools, err := mcpEnabledToolsWithPolicy(cmd, flags, policy)
	return mcpToolsWithinBudget(tools, cmd.MaxOutputBytes), account, policy, err
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
	if (policy.AllowCalendarDelete || policy.AllowGmailSettingsDelete) && !policy.AllowWrite {
		return config.MCPPolicy{}, usage("MCP deletion capabilities require allow_write")
	}
	if policy.AllowCalendarNotify && !policy.AllowWrite {
		return config.MCPPolicy{}, usage("MCP policy allow_calendar_notify requires allow_write")
	}
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
	if cmd.AllowCalendarDelete && !policy.AllowCalendarDelete {
		return nil, usage("--allow-calendar-delete cannot widen the configured MCP policy")
	}
	if cmd.AllowGmailSettingsDelete && !policy.AllowGmailSettingsDelete {
		return nil, usage("--allow-gmail-settings-delete cannot widen the configured MCP policy")
	}
	if cmd.AllowCalendarNotify && !policy.AllowCalendarNotify {
		return nil, usage("--allow-calendar-notify cannot widen the configured MCP policy")
	}
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
	return mcpToolsWithinBudget(mcpFilterTools(config.MCPPolicy{
		AllowWrite:               cmd.AllowWrite,
		AllowGmailSend:           cmd.AllowGmailSend,
		AllowGmailDelete:         cmd.AllowGmailDelete,
		AllowCalendarNotify:      cmd.AllowCalendarNotify,
		AllowCalendarDelete:      cmd.AllowCalendarDelete,
		AllowGmailSettingsDelete: cmd.AllowGmailSettingsDelete,
	}, splitCommaValues(cmd.AllowTool), flags), cmd.MaxOutputBytes)
}

func mcpToolsWithinBudget(tools []mcpToolSpec, budget int) []mcpToolSpec {
	if budget == 0 || budget >= mcpMinimumOutputBytes {
		return tools
	}
	out := make([]mcpToolSpec, 0, len(tools))
	for _, tool := range tools {
		if !tool.Bounded {
			out = append(out, tool)
		}
	}
	return out
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
		case mcpCapabilityCalendarNotify:
			if !policy.AllowWrite || !policy.AllowCalendarNotify {
				continue
			}
		case mcpCapabilityCalendarDelete, mcpCapabilityGmailSettingsDelete:
			if !mcpCapabilityAllowed(policy, tool.Capability) {
				continue
			}
		default:
			continue
		}
		allowed := true
		for _, capability := range tool.RequiredCapabilities {
			if !mcpCapabilityAllowed(policy, capability) {
				allowed = false
				break
			}
		}
		if !allowed {
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

func mcpCapabilityAllowed(policy config.MCPPolicy, capability mcpToolCapability) bool {
	if !policy.AllowWrite {
		return false
	}
	switch capability {
	case mcpCapabilityGmailSend:
		return policy.AllowGmailSend
	case mcpCapabilityGmailDelete:
		return policy.AllowGmailDelete
	case mcpCapabilityCalendarNotify:
		return policy.AllowCalendarNotify
	case mcpCapabilityCalendarDelete:
		return policy.AllowCalendarDelete
	case mcpCapabilityGmailSettingsDelete:
		return policy.AllowGmailSettingsDelete
	default:
		return false
	}
}

func requireMCPCapabilities(tool mcpToolSpec, req mcp.CallToolRequest, policy config.MCPPolicy) error {
	if tool.Risk == mcpRiskWrite && !policy.AllowWrite {
		return usage("write capability required")
	}
	capabilities := append([]mcpToolCapability{}, tool.RequiredCapabilities...)
	if tool.Capability != "" {
		capabilities = append(capabilities, tool.Capability)
	}
	for _, capability := range capabilities {
		if !mcpCapabilityAllowed(policy, capability) {
			return usage("required capability is not authorized")
		}
	}
	if tool.Service == "calendar" && tool.Risk == mcpRiskWrite {
		if mode, present := req.GetArguments()["send_updates"]; present {
			value, ok := mode.(string)
			if !ok {
				return usage("send_updates must be string")
			}
			if value != "none" && !policy.AllowCalendarNotify {
				return usage("Calendar notifications require calendar_notify")
			}
		}
	}
	return nil
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
