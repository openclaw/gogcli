package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type AuthSetupCmd struct {
	Email         string `arg:"" optional:"" name:"email" help:"Google account to authorize after setup"`
	Project       string `name:"gcloud-project" aliases:"project-id" help:"Google Cloud project ID (default: active gcloud project)"`
	ProjectName   string `name:"project-name" help:"Display name when creating a project" default:"gog CLI"`
	ServicesCSV   string `name:"services" help:"Services to configure: comma-separated ${auth_services}" default:"gmail,calendar,drive,docs,sheets,contacts"`
	Credentials   string `name:"credentials" type:"path" help:"Downloaded Desktop OAuth client JSON to store"`
	CreateProject bool   `name:"create-project" help:"Create --gcloud-project with gcloud (requires confirmation)"`
	EnableAPIs    bool   `name:"enable-apis" help:"Enable selected Google APIs with gcloud"`
	Login         bool   `name:"login" help:"Run browser OAuth after project/client setup"`
	ForceConsent  bool   `name:"force-consent" help:"Force OAuth consent when --login runs"`
	OpenConsole   bool   `name:"open-console" help:"Open the OAuth client page for the selected project"`
}

type authSetupResult struct {
	Status           string   `json:"status"`
	Project          string   `json:"project,omitempty"`
	GcloudAccount    string   `json:"gcloud_account,omitempty"`
	Services         []string `json:"services"`
	APIs             []string `json:"apis"`
	ProjectCreated   bool     `json:"project_created"`
	APIsEnabled      bool     `json:"apis_enabled"`
	CredentialsSaved bool     `json:"credentials_saved"`
	NextSteps        []string `json:"next_steps"`
}

func (c *AuthSetupCmd) Run(ctx context.Context, flags *RootFlags) error {
	readonly := readOnlyEnabled(flags)
	services, err := parseAuthServices(c.ServicesCSV)
	if err != nil {
		return err
	}
	serviceNames := make([]string, 0, len(services))
	for _, service := range services {
		serviceNames = append(serviceNames, string(service))
	}
	apiIDs, err := googleauth.APIServiceIDsForServices(services)
	if err != nil {
		return usage(err.Error())
	}

	project := strings.TrimSpace(c.Project)
	account := ""
	gcloudAvailable := authSetupGcloudAvailable()
	if gcloudAvailable {
		account, _ = authSetupGcloudValue(ctx, "account")
		if project == "" {
			project, _ = authSetupGcloudValue(ctx, "project")
		}
	}
	if project != "" && !validGoogleCloudProjectID(project) {
		return usagef("invalid --gcloud-project %q (use 6-30 lowercase letters, digits, or hyphens; start with a letter)", project)
	}
	if c.CreateProject && project == "" {
		return usage("--create-project requires --gcloud-project")
	}
	if c.EnableAPIs && project == "" {
		return usage("--enable-apis requires --gcloud-project or an active gcloud project")
	}
	if c.OpenConsole && project == "" {
		return usage("--open-console requires --gcloud-project or an active gcloud project")
	}
	if readonly && (c.CreateProject || c.EnableAPIs) && (flags == nil || !flags.DryRun) {
		return fmt.Errorf("%w: auth setup project/API mutations are disabled", googleapi.ErrReadOnly)
	}
	if (c.CreateProject || c.EnableAPIs) && !gcloudAvailable && (flags == nil || !flags.DryRun) {
		return usage("gcloud is required for --create-project or --enable-apis; install it or omit those flags for manual guidance")
	}
	if c.Login && strings.TrimSpace(c.Email) == "" {
		return usage("--login requires an email argument")
	}
	if c.Login && flags != nil && flags.NoInput && !flags.DryRun {
		return usage("--login requires interactive browser input; omit --no-input or run `gog auth import`")
	}
	if c.OpenConsole && flags != nil && flags.NoInput && !flags.DryRun {
		return usage("--open-console is not available with --no-input")
	}

	plan := map[string]any{
		"project":          project,
		"project_name":     strings.TrimSpace(c.ProjectName),
		"services":         serviceNames,
		"apis":             apiIDs,
		"create_project":   c.CreateProject,
		"enable_apis":      c.EnableAPIs,
		"credentials_file": strings.TrimSpace(c.Credentials),
		"login_email":      strings.TrimSpace(c.Email),
		"readonly":         readonly,
		"force_consent":    c.ForceConsent,
		"open_console":     c.OpenConsole,
	}
	if dryRunErr := dryRunExit(ctx, flags, "auth.setup", plan); dryRunErr != nil {
		return dryRunErr
	}

	result := authSetupResult{
		Status:        "guided",
		Project:       project,
		GcloudAccount: account,
		Services:      serviceNames,
		APIs:          apiIDs,
		NextSteps:     []string{},
	}

	if c.CreateProject {
		if err := confirmDestructiveChecked(ctx, flags, "create Google Cloud project "+project); err != nil {
			return err
		}
		args := []string{"projects", "create", project}
		if name := strings.TrimSpace(c.ProjectName); name != "" {
			args = append(args, "--name", name)
		}
		if _, err := authSetupRunGcloud(ctx, args...); err != nil {
			return fmt.Errorf("create Google Cloud project: %w", err)
		}
		result.ProjectCreated = true
	}

	if c.EnableAPIs {
		args := append([]string{"services", "enable"}, apiIDs...)
		args = append(args, "--project", project)
		if _, err := authSetupRunGcloud(ctx, args...); err != nil {
			return fmt.Errorf("enable Google APIs: %w", err)
		}
		result.APIsEnabled = true
	}

	if credentials := strings.TrimSpace(c.Credentials); credentials != "" {
		if err := runAuthSetupCredentials(ctx, flags, credentials, strings.TrimSpace(c.Email)); err != nil {
			return err
		}
		result.CredentialsSaved = true
	}

	if c.OpenConsole {
		if err := openURL(ctx, googleCloudOAuthClientsURL(project)); err != nil {
			return fmt.Errorf("open OAuth client page: %w", err)
		}
	}

	result.NextSteps = authSetupNextSteps(c, result, gcloudAvailable, authclient.ClientOverrideFromContext(ctx), readonly)
	if c.Login {
		return (&AuthAddCmd{
			Email:        strings.TrimSpace(c.Email),
			ServicesCSV:  c.ServicesCSV,
			ForceConsent: c.ForceConsent,
		}).Run(ctx, flags)
	}

	return writeAuthSetupResult(ctx, result)
}

func runAuthSetupCredentials(ctx context.Context, flags *RootFlags, path string, email string) error {
	client, err := authSetupClient(ctx, email)
	if err != nil {
		return err
	}
	expanded, err := config.ExpandPath(path)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(expanded) //nolint:gosec // explicit user-provided credentials path
	if err != nil {
		return err
	}
	credentials, err := config.ParseGoogleOAuthClientJSON(raw)
	if err != nil {
		return err
	}
	if flags != nil && flags.DryRun {
		return nil
	}
	store, err := commandOAuthCredentialsStore(ctx)
	if err != nil {
		return err
	}
	if err := store.Write(client, credentials, false); err != nil {
		return fmt.Errorf("store OAuth client credentials: %w", err)
	}
	return nil
}

func authSetupClient(ctx context.Context, email string) (string, error) {
	override := authclient.ClientOverrideFromContext(ctx)
	if email != "" {
		return authclient.ResolveClientWithOverride(ctx, email, override)
	}

	return normalizeClientForFlag(override)
}

func authSetupNextSteps(c *AuthSetupCmd, result authSetupResult, gcloudAvailable bool, clientOverride string, readonly bool) []string {
	steps := make([]string, 0, 6)
	gogCommand := "gog"
	if client := strings.TrimSpace(clientOverride); client != "" {
		gogCommand += " --client " + client
	}
	if !gcloudAvailable {
		steps = append(steps, "Install gcloud, or create/select a Google Cloud project manually")
	} else if result.GcloudAccount == "" {
		steps = append(steps, "Run `gcloud auth login`")
	}
	if result.Project == "" {
		steps = append(steps, "Choose a project with `--gcloud-project PROJECT_ID` or `gcloud config set project PROJECT_ID`")
	} else {
		steps = append(steps,
			"Configure OAuth consent: "+googleCloudConsentURL(result.Project),
			"Create a Desktop OAuth client and download its JSON: "+googleCloudOAuthClientsURL(result.Project),
		)
	}
	if !result.CredentialsSaved {
		steps = append(steps, "Store the download with `"+gogCommand+" auth credentials /path/to/client_secret.json`")
	}
	authCommand := gogCommand
	if readonly {
		authCommand += " --readonly"
	}
	if strings.TrimSpace(c.Email) != "" {
		steps = append(steps, fmt.Sprintf("Authorize with `%s auth add %s --services %s`", authCommand, strings.TrimSpace(c.Email), strings.Join(result.Services, ",")))
	} else {
		steps = append(steps, "Authorize with `"+authCommand+" auth add you@example.com --services "+strings.Join(result.Services, ",")+"`")
	}
	steps = append(steps, "Verify with `gog auth doctor --check`")
	return steps
}

func writeAuthSetupResult(ctx context.Context, result authSetupResult) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("status\t%s", result.Status)
	if result.GcloudAccount != "" {
		u.Out().Linef("gcloud_account\t%s", result.GcloudAccount)
	}
	if result.Project != "" {
		u.Out().Linef("project\t%s", result.Project)
	}
	u.Out().Linef("services\t%s", strings.Join(result.Services, ","))
	u.Out().Linef("apis\t%s", strings.Join(result.APIs, ","))
	for index, step := range result.NextSteps {
		u.Out().Linef("next_%d\t%s", index+1, step)
	}
	return nil
}

func authSetupGcloudBinary() string {
	if runtime.GOOS == "windows" {
		return "gcloud.cmd"
	}
	return "gcloud"
}

func authSetupGcloudAvailable() bool {
	_, err := exec.LookPath(authSetupGcloudBinary())
	return err == nil
}

func authSetupGcloudValue(ctx context.Context, key string) (string, error) {
	output, err := authSetupRunGcloud(ctx, "config", "get-value", key)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(output)
	if value == "(unset)" {
		return "", nil
	}
	return value, nil
}

//nolint:gosec // Fixed gcloud binary; arguments are validated or generated.
func authSetupRunGcloud(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, authSetupGcloudBinary(), args...)
	command.Env = append(os.Environ(), "CLOUDSDK_CORE_DISABLE_PROMPTS=1")
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return "", err
		}
		return "", fmt.Errorf("%s: %w", detail, err)
	}
	return string(output), nil
}

func validGoogleCloudProjectID(value string) bool {
	if len(value) < 6 || len(value) > 30 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			continue
		}
		return false
	}
	return !strings.HasSuffix(value, "-")
}

func googleCloudConsentURL(project string) string {
	return "https://console.cloud.google.com/auth/branding?project=" + url.QueryEscape(project)
}

func googleCloudOAuthClientsURL(project string) string {
	return "https://console.cloud.google.com/auth/clients?project=" + url.QueryEscape(project)
}
