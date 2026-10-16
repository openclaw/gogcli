package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	mcpMinimumOutputBytes = 4096
	mcpMaximumOutputBytes = 1 << 20
)

func boundedMCPResult(result mcpCommandResult, budget int) (*mcp.CallToolResult, error) {
	output := mcp.NewToolResultStructuredOnly(result)
	output.IsError = result.ExitCode != 0
	// The SDK decorates modern protocol results after our handler returns.
	// Measure that exact shape too; legacy results can only be smaller.
	measured := *output
	measured.SetResultType(mcp.ResultTypeComplete)
	measured.EnsureResultMeta().SetServerInfo(mcp.Implementation{Name: "gog", Version: VersionString()})
	encoded, err := json.Marshal(&measured)
	if err != nil {
		return nil, err
	}
	if len(encoded) > min(budget, mcpMaximumOutputBytes) {
		return nil, fmt.Errorf("%w", errMCPOutputBudget)
	}
	return output, nil
}

func mcpBoundedError(tool mcpToolSpec, code string, budget int) *mcp.CallToolResult {
	result := mcpCommandResult{Tool: tool.Name, Service: tool.Service, Risk: string(tool.Risk), Capability: string(tool.Capability), ExitCode: 1, Stdout: map[string]any{"error": map[string]string{"code": code}}}
	output, err := boundedMCPResult(result, budget)
	if err != nil {
		return mcp.NewToolResultError("output_budget_too_small")
	}
	return output
}

func mcpBoundedInt(req mcp.CallToolRequest, key string, fallback, lower, upper int64) (int64, error) {
	value, present := req.GetArguments()[key]
	if !present {
		return fallback, nil
	}
	var number int64
	switch v := value.(type) {
	case json.Number:
		var err error
		number, err = strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v < math.MinInt64 || v >= 9223372036854775808.0 || math.Trunc(v) != v {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		number = int64(v)
	case int:
		number = int64(v)
	case int64:
		number = v
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	if number < lower || number > upper {
		return 0, fmt.Errorf("%s outside permitted range", key)
	}
	return number, nil
}
