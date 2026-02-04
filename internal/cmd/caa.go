package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/accesscontextmanager/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newAccessContextManagerService = googleapi.NewAccessContextManager

type CAACmd struct {
	Levels CAALevelsCmd `cmd:"" name:"levels" help:"Manage access levels"`
}

type CAALevelsCmd struct {
	List   CAALevelsListCmd   `cmd:"" name:"list" help:"List access levels"`
	Get    CAALevelsGetCmd    `cmd:"" name:"get" help:"Get access level"`
	Create CAALevelsCreateCmd `cmd:"" name:"create" help:"Create access level"`
	Update CAALevelsUpdateCmd `cmd:"" name:"update" help:"Update access level"`
	Delete CAALevelsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete access level"`
}

type CAALevelsListCmd struct {
	Policy string `name:"policy" help:"Access policy ID or resource name"`
	Max    int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page   string `name:"page" help:"Page token"`
}

func (c *CAALevelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	policy := strings.TrimSpace(c.Policy)
	if policy == "" {
		return usage("--policy is required")
	}

	svc, err := newAccessContextManagerService(ctx, account)
	if err != nil {
		return err
	}

	parent := normalizeAccessPolicy(policy)
	call := svc.AccessPolicies.AccessLevels.List(parent)
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list access levels: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.AccessLevels) == 0 {
		u.Err().Println("No access levels found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tTITLE\tTYPE\tDESCRIPTION")
	for _, level := range resp.AccessLevels {
		if level == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(level.Name),
			sanitizeTab(level.Title),
			sanitizeTab(accessLevelType(level)),
			sanitizeTab(level.Description),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type CAALevelsGetCmd struct {
	Name   string `arg:"" name:"name" help:"Access level name"`
	Policy string `name:"policy" help:"Access policy ID or resource name"`
}

func (c *CAALevelsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("access level name is required")
	}

	svc, err := newAccessContextManagerService(ctx, account)
	if err != nil {
		return err
	}

	fullName, err := normalizeAccessLevelName(c.Policy, name)
	if err != nil {
		return err
	}

	level, err := svc.AccessPolicies.AccessLevels.Get(fullName).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get access level %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, level)
	}

	u.Out().Printf("Name:        %s\n", level.Name)
	if level.Title != "" {
		u.Out().Printf("Title:       %s\n", level.Title)
	}
	if level.Description != "" {
		u.Out().Printf("Description: %s\n", level.Description)
	}
	u.Out().Printf("Type:        %s\n", accessLevelType(level))
	if level.Custom != nil && level.Custom.Expr != nil && level.Custom.Expr.Expression != "" {
		u.Out().Printf("Expression:  %s\n", level.Custom.Expr.Expression)
	}
	if level.Basic != nil {
		u.Out().Printf("Conditions:  %d\n", len(level.Basic.Conditions))
	}
	return nil
}

type CAALevelsCreateCmd struct {
	Name        string   `arg:"" name:"name" help:"Access level name"`
	Description string   `name:"description" help:"Access level description"`
	Policy      string   `name:"policy" help:"Access policy ID or resource name"`
	Basic       bool     `name:"basic" help:"Create a basic access level"`
	Custom      bool     `name:"custom" help:"Create a custom access level"`
	Conditions  []string `name:"condition" help:"Condition JSON (repeatable)"`
	Expr        string   `name:"expr" help:"CEL expression for custom access levels"`
}

func (c *CAALevelsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("access level name is required")
	}
	policy := strings.TrimSpace(c.Policy)
	if policy == "" {
		return usage("--policy is required")
	}
	if c.Basic == c.Custom {
		return usage("exactly one of --basic or --custom is required")
	}

	svc, err := newAccessContextManagerService(ctx, account)
	if err != nil {
		return err
	}

	fullName, err := normalizeAccessLevelName(policy, name)
	if err != nil {
		return err
	}
	title := accessLevelTitle(fullName)
	level := &accesscontextmanager.AccessLevel{
		Name:        fullName,
		Title:       title,
		Description: c.Description,
	}

	if c.Basic {
		var conditions []*accesscontextmanager.Condition
		conditions, err = parseCAAConditions(c.Conditions)
		if err != nil {
			return err
		}
		if len(conditions) == 0 {
			return usage("--condition is required for basic access levels")
		}
		level.Basic = &accesscontextmanager.BasicLevel{Conditions: conditions}
	}

	if c.Custom {
		expr := strings.TrimSpace(c.Expr)
		if expr == "" {
			return usage("--expr is required for custom access levels")
		}
		level.Custom = &accesscontextmanager.CustomLevel{Expr: &accesscontextmanager.Expr{Expression: expr}}
	}

	op, err := svc.AccessPolicies.AccessLevels.Create(normalizeAccessPolicy(policy), level).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create access level: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Created access level: %s\n", fullName)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

type CAALevelsUpdateCmd struct {
	Name        string   `arg:"" name:"name" help:"Access level name"`
	Description *string  `name:"description" help:"Access level description"`
	Policy      string   `name:"policy" help:"Access policy ID or resource name"`
	Conditions  []string `name:"condition" help:"Condition JSON (repeatable)"`
	Expr        string   `name:"expr" help:"CEL expression for custom access levels"`
}

func (c *CAALevelsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("access level name is required")
	}

	svc, err := newAccessContextManagerService(ctx, account)
	if err != nil {
		return err
	}

	fullName, err := normalizeAccessLevelName(c.Policy, name)
	if err != nil {
		return err
	}

	updateMask := make([]string, 0, 3)
	patch := &accesscontextmanager.AccessLevel{}

	if c.Description != nil {
		patch.Description = strings.TrimSpace(*c.Description)
		updateMask = append(updateMask, "description")
	}

	expr := strings.TrimSpace(c.Expr)
	if expr != "" && len(c.Conditions) > 0 {
		return usage("cannot combine --expr with --condition")
	}

	if len(c.Conditions) > 0 {
		var conditions []*accesscontextmanager.Condition
		conditions, err = parseCAAConditions(c.Conditions)
		if err != nil {
			return err
		}
		if len(conditions) == 0 {
			return usage("no conditions specified")
		}
		patch.Basic = &accesscontextmanager.BasicLevel{Conditions: conditions}
		updateMask = append(updateMask, "basic")
	}

	if expr != "" {
		patch.Custom = &accesscontextmanager.CustomLevel{Expr: &accesscontextmanager.Expr{Expression: expr}}
		updateMask = append(updateMask, "custom")
	}

	if len(updateMask) == 0 {
		return usage("no updates specified")
	}

	op, err := svc.AccessPolicies.AccessLevels.Patch(fullName, patch).
		UpdateMask(strings.Join(updateMask, ",")).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("update access level %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Updated access level: %s\n", fullName)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

type CAALevelsDeleteCmd struct {
	Name   string `arg:"" name:"name" help:"Access level name"`
	Policy string `name:"policy" help:"Access policy ID or resource name"`
}

func (c *CAALevelsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("access level name is required")
	}

	fullName, err := normalizeAccessLevelName(c.Policy, name)
	if err != nil {
		return err
	}

	if err = confirmDestructive(ctx, flags, fmt.Sprintf("delete access level %s", fullName)); err != nil {
		return err
	}

	svc, err := newAccessContextManagerService(ctx, account)
	if err != nil {
		return err
	}

	op, err := svc.AccessPolicies.AccessLevels.Delete(fullName).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("delete access level %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Deleted access level: %s\n", fullName)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

func normalizeAccessPolicy(policy string) string {
	policy = strings.TrimSpace(policy)
	if policy == "" {
		return ""
	}
	if strings.HasPrefix(policy, "accessPolicies/") {
		return policy
	}
	return "accessPolicies/" + policy
}

func normalizeAccessLevelName(policy, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", usage("access level name is required")
	}
	if strings.HasPrefix(name, "accessPolicies/") {
		return name, nil
	}
	policy = strings.TrimSpace(policy)
	if policy == "" {
		return "", usage("--policy is required when using short names")
	}
	return fmt.Sprintf("%s/accessLevels/%s", normalizeAccessPolicy(policy), name), nil
}

func accessLevelType(level *accesscontextmanager.AccessLevel) string {
	if level == nil {
		return ""
	}
	if level.Basic != nil {
		return "basic"
	}
	if level.Custom != nil {
		return "custom"
	}
	return trackingUnknown
}

func accessLevelTitle(name string) string {
	if idx := strings.LastIndex(name, "/"); idx >= 0 && idx < len(name)-1 {
		return name[idx+1:]
	}
	return name
}

func parseCAAConditions(inputs []string) ([]*accesscontextmanager.Condition, error) {
	conditions := make([]*accesscontextmanager.Condition, 0, len(inputs))
	for _, raw := range inputs {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		payload, err := readValueOrFile(trimmed)
		if err != nil {
			return nil, err
		}
		var cond accesscontextmanager.Condition
		if err := json.Unmarshal([]byte(payload), &cond); err != nil {
			return nil, fmt.Errorf("parse condition: %w", err)
		}
		conditions = append(conditions, &cond)
	}
	return conditions, nil
}
