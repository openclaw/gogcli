package cmd

import (
	"context"
	"strconv"
	"strings"

	scriptapi "google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type AppScriptDeploymentsCmd struct {
	ScriptID  string `arg:"" name:"scriptId" help:"Script ID"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no deployments"`
}

func (c *AppScriptDeploymentsCmd) Run(ctx context.Context, flags *RootFlags) error {
	scriptID, svc, err := appScriptListSetup(ctx, flags, c.ScriptID, c.Max)
	if err != nil {
		return err
	}

	return runAppScriptList(ctx, appScriptListRequest[*scriptapi.Deployment]{
		ScriptID:     scriptID,
		ItemsKey:     "deployments",
		EmptyMessage: "No deployments",
		Page:         c.Page,
		All:          c.All,
		FailEmpty:    c.FailEmpty,
		Columns:      appScriptDeploymentColumns(),
		Fetch:        appScriptDeploymentPager(ctx, svc, scriptID, c.Max),
	})
}

func appScriptDeploymentPager(ctx context.Context, svc *scriptapi.Service, scriptID string, maxResults int64) pageFetchFunc[*scriptapi.Deployment] {
	return func(pageToken string) ([]*scriptapi.Deployment, string, error) {
		call := svc.Projects.Deployments.List(scriptID).PageSize(maxResults).Context(ctx)
		if page := strings.TrimSpace(pageToken); page != "" {
			call = call.PageToken(page)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}

		return resp.Deployments, resp.NextPageToken, nil
	}
}

func appScriptVersionPager(ctx context.Context, svc *scriptapi.Service, scriptID string, maxResults int64) pageFetchFunc[*scriptapi.Version] {
	return func(pageToken string) ([]*scriptapi.Version, string, error) {
		call := svc.Projects.Versions.List(scriptID).PageSize(maxResults).Context(ctx)
		if page := strings.TrimSpace(pageToken); page != "" {
			call = call.PageToken(page)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}

		return resp.Versions, resp.NextPageToken, nil
	}
}

// appScriptListSetup applies the validation and service construction the
// appscript list commands share.
func appScriptListSetup(ctx context.Context, flags *RootFlags, rawScriptID string, maxResults int64) (string, *scriptapi.Service, error) {
	scriptID := strings.TrimSpace(normalizeGoogleID(rawScriptID))
	if scriptID == "" {
		return "", nil, usage("empty scriptId")
	}

	if maxResults <= 0 {
		return "", nil, usage("max must be > 0")
	}

	svc, err := requireAppScriptService(ctx, flags)
	if err != nil {
		return "", nil, err
	}

	return scriptID, svc, nil
}

// appScriptListRequest carries everything the two appscript list commands vary:
// the API call, how their items are named, and how they render.
type appScriptListRequest[T any] struct {
	ScriptID     string
	ItemsKey     string
	EmptyMessage string
	Page         string
	All          bool
	FailEmpty    bool
	Columns      []outfmt.Column[T]
	Fetch        pageFetchFunc[T]
}

func runAppScriptList[T any](ctx context.Context, req appScriptListRequest[T]) error {
	u := ui.FromContext(ctx)

	items, nextPageToken, err := loadPagedItems(req.Page, req.All, req.Fetch)
	if err != nil {
		return err
	}

	if items == nil {
		items = []T{}
	}

	if outfmt.IsJSON(ctx) {
		return writePagedJSONResult(ctx, map[string]any{
			"scriptId":      req.ScriptID,
			req.ItemsKey:    items,
			"nextPageToken": nextPageToken,
		}, len(items), req.FailEmpty)
	}

	if len(items) == 0 {
		u.Err().Println(req.EmptyMessage)
		return failEmptyExit(req.FailEmpty)
	}

	if err := outfmt.WriteTable(ctx, stdoutWriter(ctx), items, req.Columns); err != nil {
		return err
	}

	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")

	return nil
}

func appScriptDeploymentColumns() []outfmt.Column[*scriptapi.Deployment] {
	return []outfmt.Column[*scriptapi.Deployment]{
		{Header: "DEPLOYMENT_ID", Value: appScriptDeploymentID},
		{Header: "VERSION", Value: appScriptDeploymentVersion},
		{Header: "DESCRIPTION", Value: appScriptDeploymentDescription},
		{Header: "WEB_APP_URL", Value: appScriptWebAppURL},
	}
}

func appScriptDeploymentID(deployment *scriptapi.Deployment) string {
	if deployment == nil {
		return ""
	}

	return deployment.DeploymentId
}

// appScriptDeploymentVersion reports HEAD for a deployment pinned to the live
// editor content rather than to a cut version.
func appScriptDeploymentVersion(deployment *scriptapi.Deployment) string {
	if deployment == nil || deployment.DeploymentConfig == nil || deployment.DeploymentConfig.VersionNumber == 0 {
		return "HEAD"
	}

	return "v" + strconv.FormatInt(deployment.DeploymentConfig.VersionNumber, 10)
}

func appScriptDeploymentDescription(deployment *scriptapi.Deployment) string {
	if deployment == nil || deployment.DeploymentConfig == nil {
		return ""
	}

	return sanitizeTab(deployment.DeploymentConfig.Description)
}

func appScriptWebAppURL(deployment *scriptapi.Deployment) string {
	if deployment == nil {
		return ""
	}

	for _, entry := range deployment.EntryPoints {
		if entry == nil || entry.WebApp == nil {
			continue
		}

		if entry.WebApp.Url != "" {
			return entry.WebApp.Url
		}
	}

	return ""
}

type AppScriptVersionsCmd struct {
	ScriptID  string `arg:"" name:"scriptId" help:"Script ID"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no versions"`
}

func (c *AppScriptVersionsCmd) Run(ctx context.Context, flags *RootFlags) error {
	scriptID, svc, err := appScriptListSetup(ctx, flags, c.ScriptID, c.Max)
	if err != nil {
		return err
	}

	return runAppScriptList(ctx, appScriptListRequest[*scriptapi.Version]{
		ScriptID:     scriptID,
		ItemsKey:     "versions",
		EmptyMessage: "No versions",
		Page:         c.Page,
		All:          c.All,
		FailEmpty:    c.FailEmpty,
		Columns:      appScriptVersionColumns(),
		Fetch:        appScriptVersionPager(ctx, svc, scriptID, c.Max),
	})
}

func appScriptVersionColumns() []outfmt.Column[*scriptapi.Version] {
	return []outfmt.Column[*scriptapi.Version]{
		{Header: "VERSION", Value: func(version *scriptapi.Version) string {
			if version == nil {
				return ""
			}

			return strconv.FormatInt(version.VersionNumber, 10)
		}},
		{Header: "CREATED", Value: func(version *scriptapi.Version) string {
			if version == nil {
				return ""
			}

			return formatDateTime(version.CreateTime)
		}},
		{Header: "DESCRIPTION", Value: func(version *scriptapi.Version) string {
			if version == nil {
				return ""
			}

			return sanitizeTab(version.Description)
		}},
	}
}
