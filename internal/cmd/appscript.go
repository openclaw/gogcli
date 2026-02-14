package cmd

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	scriptapi "google.golang.org/api/script/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newAppScriptService = googleapi.NewAppScript

type AppScriptCmd struct {
	Get     AppScriptGetCmd     `cmd:"" name:"get" aliases:"info,show" help:"Get Apps Script project metadata"`
	Content AppScriptContentCmd `cmd:"" name:"content" aliases:"cat" help:"Get Apps Script project content"`
	Run     AppScriptRunCmd     `cmd:"" name:"run" help:"Run a deployed Apps Script function"`
	Create  AppScriptCreateCmd  `cmd:"" name:"create" aliases:"new" help:"Create an Apps Script project"`
}

type AppScriptGetCmd struct {
	ScriptID string `arg:"" name:"scriptId" help:"Script ID"`
}

func (c *AppScriptGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	scriptID := strings.TrimSpace(normalizeGoogleID(c.ScriptID))
	if scriptID == "" {
		return usage("empty scriptId")
	}

	svc, err := newAppScriptService(ctx, account)
	if err != nil {
		return err
	}
	project, err := svc.Projects.Get(scriptID).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"project":    project,
			"editor_url": appScriptEditURL(scriptID),
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("script_id\t%s", project.ScriptId)
	if project.Title != "" {
		u.Out().Printf("title\t%s", project.Title)
	}
	if project.ParentId != "" {
		u.Out().Printf("parent_id\t%s", project.ParentId)
	}
	if project.CreateTime != "" {
		u.Out().Printf("created\t%s", project.CreateTime)
	}
	if project.UpdateTime != "" {
		u.Out().Printf("updated\t%s", project.UpdateTime)
	}
	u.Out().Printf("editor_url\t%s", appScriptEditURL(scriptID))
	return nil
}

type AppScriptContentCmd struct {
	ScriptID string `arg:"" name:"scriptId" help:"Script ID"`
}

func (c *AppScriptContentCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	scriptID := strings.TrimSpace(normalizeGoogleID(c.ScriptID))
	if scriptID == "" {
		return usage("empty scriptId")
	}

	svc, err := newAppScriptService(ctx, account)
	if err != nil {
		return err
	}
	content, err := svc.Projects.GetContent(scriptID).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"content": content,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("script_id\t%s", content.ScriptId)
	u.Out().Printf("files\t%d", len(content.Files))
	for _, file := range content.Files {
		if file == nil {
			continue
		}
		u.Out().Printf("file\t%s\t%s", file.Name, file.Type)
	}
	return nil
}

type AppScriptRunCmd struct {
	ScriptID string `arg:"" name:"scriptId" help:"Script ID"`
	Function string `arg:"" name:"function" help:"Function name to run"`
	Params   string `name:"params" help:"JSON array of function parameters" default:"[]"`
	DevMode  bool   `name:"dev-mode" help:"Run latest saved code if you own the script"`
}

func (c *AppScriptRunCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	scriptID := strings.TrimSpace(normalizeGoogleID(c.ScriptID))
	if scriptID == "" {
		return usage("empty scriptId")
	}
	function := strings.TrimSpace(c.Function)
	if function == "" {
		return usage("empty function")
	}

	params, err := parseJSONArray(c.Params)
	if err != nil {
		return err
	}

	svc, err := newAppScriptService(ctx, account)
	if err != nil {
		return err
	}
	op, err := svc.Scripts.Run(scriptID, &scriptapi.ExecutionRequest{
		Function:   function,
		Parameters: params,
		DevMode:    c.DevMode,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"operation": op,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("done\t%t", op.Done)

	if op.Error != nil {
		if op.Error.Code != 0 {
			u.Out().Printf("error_code\t%d", op.Error.Code)
		}
		if op.Error.Message != "" {
			u.Out().Printf("error\t%s", op.Error.Message)
		}
		if detail := parseExecutionError(op.Error); detail != nil {
			if detail.ErrorType != "" {
				u.Out().Printf("error_type\t%s", detail.ErrorType)
			}
			if detail.ErrorMessage != "" {
				u.Out().Printf("error_message\t%s", detail.ErrorMessage)
			}
		}
		return nil
	}

	if len(op.Response) > 0 {
		var execResp scriptapi.ExecutionResponse
		if err := json.Unmarshal(op.Response, &execResp); err == nil && execResp.Result != nil {
			if b, marshalErr := json.Marshal(execResp.Result); marshalErr == nil {
				u.Out().Printf("result\t%s", string(b))
			}
		}
	}
	return nil
}

type AppScriptCreateCmd struct {
	Title    string `name:"title" help:"Project title" required:""`
	ParentID string `name:"parent-id" help:"Optional Drive file ID to bind to"`
}

func (c *AppScriptCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty --title")
	}
	parentID := strings.TrimSpace(normalizeGoogleID(c.ParentID))

	if dryRunErr := dryRunExit(ctx, flags, "appscript.create", map[string]any{
		"title":     title,
		"parent_id": parentID,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := newAppScriptService(ctx, account)
	if err != nil {
		return err
	}
	project, err := svc.Projects.Create(&scriptapi.CreateProjectRequest{
		Title:    title,
		ParentId: parentID,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"created":    true,
			"project":    project,
			"editor_url": appScriptEditURL(project.ScriptId),
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("created\ttrue")
	u.Out().Printf("script_id\t%s", project.ScriptId)
	if project.Title != "" {
		u.Out().Printf("title\t%s", project.Title)
	}
	if project.ParentId != "" {
		u.Out().Printf("parent_id\t%s", project.ParentId)
	}
	u.Out().Printf("editor_url\t%s", appScriptEditURL(project.ScriptId))
	return nil
}

func parseJSONArray(raw string) ([]interface{}, error) {
	val := strings.TrimSpace(raw)
	if val == "" {
		return nil, nil
	}
	var out []interface{}
	if err := json.Unmarshal([]byte(val), &out); err != nil {
		return nil, usagef("invalid --params JSON array: %v", err)
	}
	return out, nil
}

func parseExecutionError(status *scriptapi.Status) *scriptapi.ExecutionError {
	if status == nil || len(status.Details) == 0 {
		return nil
	}
	var detail scriptapi.ExecutionError
	if err := json.Unmarshal(status.Details[0], &detail); err != nil {
		return nil
	}
	return &detail
}

func appScriptEditURL(scriptID string) string {
	scriptID = strings.TrimSpace(scriptID)
	if scriptID == "" {
		return ""
	}
	return "https://script.google.com/d/" + scriptID + "/edit"
}
