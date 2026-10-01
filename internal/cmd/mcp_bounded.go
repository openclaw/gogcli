package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

var errMCPOutputBudget = errors.New("MCP output budget exceeded")

func (rt *mcpToolRuntime) runBoundedChild(ctx context.Context, tool mcpToolSpec, command []string) (mcpCommandResult, error) {
	args := append(mcpParentRootArgs(&rt.flags), mcpParentSafetyArgs(&rt.flags)...)
	if tool.Risk == mcpRiskWrite {
		args = append(args, "--mcp-receipt")
	} else {
		args = append(args, "--mcp-bounded-read")
	}
	if rt.flags.Force && (tool.Capability == mcpCapabilityCalendarDelete || tool.Capability == mcpCapabilityGmailSettingsDelete) {
		args = append(args, "--force")
	}
	args = append(args, command...)
	child := exec.CommandContext(ctx, rt.self, args...) //nolint:gosec // fixed typed command arguments
	child.Env = rt.env
	stdout := newMCPLimitedBuffer(rt.budget)
	child.Stdout = &stdout
	child.Stderr = io.Discard
	runErr := child.Run()
	result := mcpCommandResult{Tool: tool.Name, Service: tool.Service, Risk: string(tool.Risk), Capability: string(tool.Capability)}
	if runErr != nil {
		result.ExitCode = 1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
	}
	if stdout.truncated {
		if tool.Risk == mcpRiskWrite {
			return mcpUnknownMutationResult(result), nil
		}
		return result, errMCPOutputBudget
	}
	var payload any
	decoder := json.NewDecoder(strings.NewReader(stdout.String()))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		if tool.Risk == mcpRiskWrite {
			return mcpUnknownMutationResult(result), nil
		}
		return result, fmt.Errorf("provider read failed")
	}
	result.Stdout = payload
	if tool.Risk == mcpRiskWrite {
		receipt, valid := mcpDecodeMutationReceipt(payload)
		if !valid {
			return mcpUnknownMutationResult(result), nil
		}
		result.Stdout = receipt
		if result.ExitCode != 0 {
			result.Stdout = map[string]any{"error": map[string]string{"code": "mutation_failed"}, "receipt": receipt}
		}
	}
	if result.ExitCode != 0 && tool.Risk != mcpRiskWrite {
		return result, fmt.Errorf("provider read failed")
	}
	return result, nil
}

func (rt *mcpToolRuntime) runTyped(ctx context.Context, tool mcpToolSpec, req mcp.CallToolRequest) *mcp.CallToolResult {
	args, err := tool.BuildArgs(req)
	if err != nil {
		return mcpBoundedError(tool, "invalid_input", rt.budget)
	}
	result, err := rt.runChild(ctx, tool, args)
	if err != nil {
		if tool.Risk != mcpRiskWrite {
			return mcpBoundedError(tool, "provider_error", rt.budget)
		}
		result = mcpUnknownMutationResult(mcpCommandResult{Tool: tool.Name, Service: tool.Service, Risk: string(tool.Risk), Capability: string(tool.Capability)})
	}
	output, err := boundedMCPResult(result, rt.budget)
	if err == nil {
		return output
	}
	if tool.Risk == mcpRiskWrite {
		receipt, valid := mcpDecodeMutationReceipt(result.Stdout)
		if !valid {
			receipt = mcpMutationReceipt{Outcome: "outcome_unknown"}
		}
		receipt.IDs = nil
		receipt.MetadataOmitted = true
		result.Stdout = receipt
		if result.ExitCode != 0 {
			result.Stdout = map[string]any{"error": map[string]string{"code": "mutation_failed"}, "receipt": receipt}
		}
		output, err = boundedMCPResult(result, rt.budget)
		if err == nil {
			return output
		}
	}
	return mcpBoundedError(tool, "response_too_large", rt.budget)
}

func mcpUnknownMutationResult(result mcpCommandResult) mcpCommandResult {
	result.ExitCode = 1
	result.Stdout = map[string]any{"error": map[string]string{"code": "outcome_unknown"}, "receipt": mcpMutationReceipt{Outcome: "outcome_unknown", MetadataOmitted: true}}
	return result
}

func mcpDecodeMutationReceipt(payload any) (mcpMutationReceipt, bool) {
	if object, ok := payload.(map[string]any); ok {
		if nested, exists := object["receipt"]; exists {
			payload = nested
		}
	}
	raw, err := json.Marshal(payload)
	var receipt mcpMutationReceipt
	if err != nil || json.Unmarshal(raw, &receipt) != nil || receipt.RetrySafe || receipt.KnownSteps < 0 || receipt.AttemptedSteps < receipt.KnownSteps || len(receipt.IDs) > 16 {
		return receipt, false
	}
	switch receipt.Outcome {
	case "not_attempted", "failed", "committed", "partial", "outcome_unknown":
	default:
		return receipt, false
	}
	for _, id := range receipt.IDs {
		if len(id) > 256 {
			return receipt, false
		}
	}
	return receipt, true
}
