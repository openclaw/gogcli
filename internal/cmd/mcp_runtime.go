package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/config"
	api "github.com/openclaw/gogcli/internal/googleapi"
)

type mcpToolRuntime struct {
	ctx            context.Context //nolint:containedctx // server lifetime context retains its frozen auth/runtime dependencies
	flags          RootFlags
	policy         config.MCPPolicy
	self           string
	env            []string
	timeout        time.Duration
	budget         int
	partition      string
	identityErr    error
	store          *mcpSnapshotStore
	storeErr       error
	calls          chan struct{}
	fetchExport    func(context.Context, gmailExportKey) ([]byte, gmailExportMetadata, error)
	fetchThreadIDs func(context.Context, string) ([]byte, gmailExportMetadata, error)
	runChild       func(context.Context, mcpToolSpec, []string) (mcpCommandResult, error)
}

func newMCPToolRuntime(ctx context.Context, self string, cmd *McpCmd, flags *RootFlags, policy config.MCPPolicy) (*mcpToolRuntime, error) {
	store, err := newMCPSnapshotStore("", mcpSnapshotLimits{Bytes: 256 << 20, Entries: 128, Fetches: 2})
	rt := &mcpToolRuntime{storeErr: err, ctx: ctx, self: self, env: os.Environ(), timeout: time.Duration(cmd.TimeoutSeconds) * time.Second, budget: min(cmd.MaxOutputBytes, mcpMaximumOutputBytes), store: store, calls: make(chan struct{}, 8)}
	if flags != nil {
		rt.flags = *flags
	}
	rt.flags.Account, rt.identityErr = requireAccount(&rt.flags)
	if rt.identityErr == nil {
		rt.flags.Client, rt.identityErr = authclient.ResolveClient(ctx, rt.flags.Account)
	}
	var partition [16]byte
	if _, err = rand.Read(partition[:]); err != nil {
		if store != nil {
			_ = store.Close()
		}
		return nil, err
	}
	rt.partition = hex.EncodeToString(partition[:])
	if token := directAccessToken(&rt.flags); token != "" {
		rt.env = append(rt.env, "GOG_ACCESS_TOKEN="+token)
	}
	rt.fetchExport = rt.fetchExportChild
	rt.fetchThreadIDs = rt.fetchThreadIDsDirect
	rt.runChild = rt.runBoundedChild
	rt.policy = policy
	return rt, nil
}

//nolint:contextcheck // derive auth values from the server context and bridge request cancellation explicitly
func (rt *mcpToolRuntime) fetchThreadIDsDirect(reqCtx context.Context, id string) ([]byte, gmailExportMetadata, error) {
	ctx, cancel := context.WithCancel(rt.ctx)
	defer cancel()
	stop := context.AfterFunc(reqCtx, cancel)
	defer stop()
	ctx = authclient.WithClient(ctx, rt.flags.Client)
	ctx = api.WithResponseByteLimit(ctx, 12<<20)
	svc, err := gmailService(ctx, rt.flags.Account)
	if err != nil {
		return nil, gmailExportMetadata{}, err
	}
	snapshot, err := fetchGmailThreadIDs(ctx, svc, id)
	if err != nil {
		return nil, gmailExportMetadata{}, err
	}
	data, err := json.Marshal(snapshot)
	return data, gmailExportMetadata{ThreadID: id}, err
}

func validateMCPCatalog(tools []mcpToolSpec) error {
	seen := map[string]bool{}
	for _, tool := range tools {
		if seen[tool.Name] {
			return fmt.Errorf("duplicate MCP tool %q", tool.Name)
		}
		seen[tool.Name] = true
		if (tool.BuildArgs == nil) == (tool.Handle == nil) {
			return fmt.Errorf("MCP tool %q must have exactly one execution mode", tool.Name)
		}
		if tool.Handle != nil && len(tool.CommandPath) == 0 {
			return fmt.Errorf("MCP tool %q needs a canonical command path", tool.Name)
		}
	}
	return nil
}

func (rt *mcpToolRuntime) runSpecial(ctx context.Context, tool mcpToolSpec, req mcp.CallToolRequest) *mcp.CallToolResult {
	if err := ctx.Err(); err != nil {
		return mcpBoundedError(tool, mcpSnapshotErrorCode(err), rt.budget)
	}
	if !rt.flags.Force && (tool.Capability == mcpCapabilityCalendarDelete || tool.Capability == mcpCapabilityGmailSettingsDelete) {
		return mcpBoundedError(tool, "confirmation_required", rt.budget)
	}
	if (rt.flags.ReadOnly && tool.Risk == mcpRiskWrite) || requireMCPCapabilities(tool, req, rt.policy) != nil {
		return mcpBoundedError(tool, "capability_denied", rt.budget)
	}
	if rt.flags.ResultsOnly || strings.TrimSpace(rt.flags.Select) != "" {
		return mcpBoundedError(tool, "unsupported_output_transform", rt.budget)
	}
	if err := enforceCommandPathPolicy(tool.CommandPath, rt.flags.EnableCommands, rt.flags.EnableCommandsExact, rt.flags.DisableCommands); err != nil {
		return mcpBoundedError(tool, "command_denied", rt.budget)
	}
	schema := newMCPTool(tool).InputSchema
	for key := range req.GetArguments() {
		if _, ok := schema.Properties[key]; !ok {
			return mcpBoundedError(tool, "invalid_input", rt.budget)
		}
	}
	encoded, err := json.Marshal(req.GetArguments())
	if err != nil || len(encoded) > 1<<20 {
		return mcpBoundedError(tool, "input_too_large", rt.budget)
	}
	if tool.NeedsSnapshot && (rt.storeErr != nil || rt.store == nil) {
		return mcpBoundedError(tool, "snapshot_unavailable", rt.budget)
	}
	if rt.identityErr != nil {
		return mcpBoundedError(tool, "auth_required", rt.budget)
	}
	select {
	case rt.calls <- struct{}{}:
		defer func() { <-rt.calls }()
	default:
		return mcpBoundedError(tool, "resource_exhausted", rt.budget)
	}
	if rt.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rt.timeout)
		defer cancel()
	}
	if tool.Handle != nil {
		return tool.Handle(ctx, req, rt)
	}
	return rt.runTyped(ctx, tool, req)
}

func (rt *mcpToolRuntime) fetchExportChild(ctx context.Context, key gmailExportKey) ([]byte, gmailExportMetadata, error) {
	// Only generated private paths enter the child. A client never supplies a path.
	dir, err := os.MkdirTemp(rt.store.dir, "fetch-")
	if err != nil {
		return nil, gmailExportMetadata{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err = makeMCPStoragePrivate(dir, true); err != nil {
		return nil, gmailExportMetadata{}, err
	}
	dest := filepath.Join(dir, "data")
	args := append(mcpParentRootArgs(&rt.flags), mcpParentSafetyArgs(&rt.flags)...)
	args = append(args, "gmail", "export", key.Kind, "--out="+dest, "--max-bytes=52428800", "--", key.MessageID)
	if key.Kind == "attachment" {
		args = append(args, key.AttachmentID)
	}
	child := exec.CommandContext(ctx, rt.self, args...) //nolint:gosec // fixed command and typed IDs
	child.Env = rt.env
	stdout := newMCPLimitedBuffer(16 << 10)
	child.Stdout = &stdout
	child.Stderr = io.Discard
	if err = child.Run(); err != nil || stdout.truncated {
		return nil, gmailExportMetadata{}, fmt.Errorf("export fetch failed")
	}
	var info gmailExportMetadata
	if err = json.Unmarshal([]byte(stdout.String()), &info); err != nil {
		return nil, info, fmt.Errorf("invalid export metadata")
	}
	if info.MessageID != key.MessageID || info.AttachmentID != key.AttachmentID || info.Size < 0 || info.Size > gmailExportMaxBytes {
		return nil, info, fmt.Errorf("export metadata mismatch")
	}
	if err = makeMCPStoragePrivate(dest, false); err != nil {
		return nil, info, err
	}
	file, err := os.Open(dest) //nolint:gosec // generated private export path
	if err != nil {
		return nil, info, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, gmailExportMaxBytes+1))
	if err != nil || int64(len(data)) != info.Size {
		return nil, info, fmt.Errorf("export size mismatch")
	}
	return data, info, nil
}
