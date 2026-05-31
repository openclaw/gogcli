package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type McpCmd struct {
	AllowTool      []string `name:"allow-tool" aliases:"tool" sep:"," help:"Tool or service allowlist (default: all read-only tools). Examples: gmail.*,docs_get,sheets"`
	AllowWrite     bool     `name:"allow-write" help:"Expose write tools. Write tools must also match --allow-tool when that flag is set."`
	ListTools      bool     `name:"list-tools" help:"Print enabled MCP tools as JSON and exit"`
	TimeoutSeconds int      `name:"timeout-seconds" help:"Per-tool subprocess timeout" default:"60"`
	MaxOutputBytes int      `name:"max-output-bytes" help:"Max stdout/stderr bytes captured per tool call" default:"102400"`
}

type mcpToolRisk string

const (
	mcpRiskRead  mcpToolRisk = "read"
	mcpRiskWrite mcpToolRisk = "write"
)

type mcpToolSpec struct {
	Name        string
	Service     string
	Risk        mcpToolRisk
	Description string
	Options     []mcp.ToolOption
	BuildArgs   func(mcp.CallToolRequest) ([]string, error)
}

type mcpCommandResult struct {
	Tool     string `json:"tool"`
	Service  string `json:"service"`
	Risk     string `json:"risk"`
	ExitCode int    `json:"exit_code"`
	Stdout   any    `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

func (c *McpCmd) Run(_ context.Context, flags *RootFlags) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	if c.TimeoutSeconds <= 0 {
		return usage("--timeout-seconds must be greater than zero")
	}
	if c.MaxOutputBytes <= 0 {
		return usage("--max-output-bytes must be greater than zero")
	}

	tools := mcpEnabledTools(*c)
	if len(tools) == 0 {
		return usage("no MCP tools enabled")
	}
	if c.ListTools {
		return mcpPrintTools(tools)
	}

	baseArgs := mcpParentRootArgs(flags)
	safetySuffix := mcpParentSafetyArgs(flags)
	timeout := time.Duration(c.TimeoutSeconds) * time.Second
	maxOutputBytes := c.MaxOutputBytes

	s := server.NewMCPServer("gog", VersionString(), server.WithToolCapabilities(false))
	for _, spec := range tools {
		tool := spec
		opts := append([]mcp.ToolOption{
			mcp.WithDescription(tool.Description),
			mcp.WithReadOnlyHintAnnotation(tool.Risk == mcpRiskRead),
			mcp.WithDestructiveHintAnnotation(tool.Risk == mcpRiskWrite),
			mcp.WithIdempotentHintAnnotation(tool.Risk == mcpRiskRead),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithSchemaAdditionalProperties(false),
		}, tool.Options...)
		s.AddTool(mcp.NewTool(tool.Name, opts...), func(reqCtx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			childCommandArgs, buildErr := tool.BuildArgs(req)
			if buildErr != nil {
				result := mcp.NewToolResultError(buildErr.Error())
				result.IsError = true
				return result, nil
			}
			return mcpRunGogTool(reqCtx, mcpRunOptions{
				self:           self,
				tool:           tool,
				baseArgs:       baseArgs,
				commandArgs:    childCommandArgs,
				safetySuffix:   safetySuffix,
				timeout:        timeout,
				maxOutputBytes: maxOutputBytes,
				accessToken:    directAccessToken(flags),
			}), nil
		})
	}
	return server.ServeStdio(s)
}

type mcpRunOptions struct {
	self           string
	tool           mcpToolSpec
	baseArgs       []string
	commandArgs    []string
	safetySuffix   []string
	timeout        time.Duration
	maxOutputBytes int
	accessToken    string
}

func mcpRunGogTool(reqCtx context.Context, opts mcpRunOptions) *mcp.CallToolResult {
	ctx, cancel := context.WithTimeout(reqCtx, opts.timeout)
	defer cancel()

	args := make([]string, 0, len(opts.baseArgs)+len(opts.commandArgs)+len(opts.safetySuffix))
	args = append(args, opts.baseArgs...)
	args = append(args, opts.safetySuffix...)
	args = append(args, opts.commandArgs...)

	//nolint:gosec // argv comes from typed tool schemas, not model-supplied shell text.
	cmd := exec.CommandContext(ctx, opts.self, args...)
	if strings.TrimSpace(opts.accessToken) != "" {
		cmd.Env = append(os.Environ(), "GOG_ACCESS_TOKEN="+opts.accessToken)
	}
	stdoutBuf := newMCPLimitedBuffer(opts.maxOutputBytes)
	stderrBuf := newMCPLimitedBuffer(opts.maxOutputBytes)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	if ctx.Err() == context.DeadlineExceeded {
		exitCode = 124
	}

	result := mcpCommandResult{
		Tool:     opts.tool.Name,
		Service:  opts.tool.Service,
		Risk:     string(opts.tool.Risk),
		ExitCode: exitCode,
		Stdout:   parseMCPStdout(stdoutBuf.String()),
		Stderr:   stderrBuf.String(),
	}
	callResult := mcp.NewToolResultStructuredOnly(result)
	if exitCode != 0 {
		callResult.IsError = true
	}
	return callResult
}

func parseMCPStdout(s string) any {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	var v any
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&v); err == nil {
		return v
	}
	return s
}

func mcpParentRootArgs(flags *RootFlags) []string {
	args := []string{"--json", "--wrap-untrusted", "--no-input", "--color=never"}
	if flags == nil {
		return args
	}
	if s := strings.TrimSpace(flags.Home); s != "" {
		args = append(args, "--home", s)
	}
	if s := strings.TrimSpace(flags.Account); s != "" {
		args = append(args, "--account", s)
	}
	if s := strings.TrimSpace(flags.Client); s != "" {
		args = append(args, "--client", s)
	}
	if flags.ResultsOnly {
		args = append(args, "--results-only")
	}
	if s := strings.TrimSpace(flags.Select); s != "" {
		args = append(args, "--select", s)
	}
	if flags.DryRun {
		args = append(args, "--dry-run")
	}
	return args
}

func mcpParentSafetyArgs(flags *RootFlags) []string {
	if flags == nil {
		return nil
	}
	var out []string
	if flags.GmailNoSend {
		out = append(out, "--gmail-no-send")
	}
	if s := strings.TrimSpace(flags.EnableCommands); s != "" {
		out = append(out, "--enable-commands="+s)
	}
	if s := strings.TrimSpace(flags.EnableCommandsExact); s != "" {
		out = append(out, "--enable-commands-exact="+s)
	}
	if s := strings.TrimSpace(flags.DisableCommands); s != "" {
		out = append(out, "--disable-commands="+s)
	}
	return out
}

func mcpEnabledTools(cmd McpCmd) []mcpToolSpec {
	all := mcpAllTools()
	allow := splitCommaValues(cmd.AllowTool)
	out := make([]mcpToolSpec, 0, len(all))
	for _, tool := range all {
		if tool.Risk == mcpRiskWrite && !cmd.AllowWrite {
			continue
		}
		if len(allow) > 0 && !mcpToolAllowed(tool, allow) {
			continue
		}
		out = append(out, tool)
	}
	return out
}

func splitCommaValues(values []string) []string {
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func mcpToolAllowed(tool mcpToolSpec, allow []string) bool {
	for _, pattern := range allow {
		switch pattern {
		case "*", literalAll, string(tool.Risk), tool.Name, tool.Service:
			return true
		}
		if strings.HasSuffix(pattern, ".*") && strings.TrimSuffix(pattern, ".*") == tool.Service {
			return true
		}
	}
	return false
}

func mcpPrintTools(tools []mcpToolSpec) error {
	items := make([]map[string]string, 0, len(tools))
	for _, tool := range tools {
		items = append(items, map[string]string{
			"name":        tool.Name,
			"service":     tool.Service,
			"risk":        string(tool.Risk),
			"description": tool.Description,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"tools": items})
}

func requireMCPString(req mcp.CallToolRequest, key string) (string, error) {
	value, err := req.RequireString(key)
	if err != nil {
		return "", err
	}
	if value = strings.TrimSpace(value); value == "" {
		return "", fmt.Errorf("empty %s", key)
	}
	return value, nil
}

func clampMCPInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

type mcpLimitedBuffer struct {
	buf       bytes.Buffer
	maxBytes  int
	truncated bool
}

func newMCPLimitedBuffer(maxBytes int) mcpLimitedBuffer {
	return mcpLimitedBuffer{maxBytes: maxBytes}
}

func (b *mcpLimitedBuffer) Write(p []byte) (int, error) {
	if b.maxBytes <= 0 {
		b.truncated = true
		return len(p), nil
	}
	remaining := b.maxBytes - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *mcpLimitedBuffer) String() string {
	raw := b.buf.Bytes()
	for len(raw) > 0 && !utf8.Valid(raw) {
		raw = raw[:len(raw)-1]
	}
	out := string(raw)
	if !b.truncated {
		return out
	}
	return out + "\n... [output truncated]"
}
