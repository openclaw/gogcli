package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/errfmt"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/termutil"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	colorAuto  = "auto"
	colorNever = "never"
	boolTrue   = "true"
	boolFalse  = "false"
)

type RootFlags struct {
	Color               string `help:"Color output: auto|always|never" default:"${color}"`
	Home                string `name:"home" help:"Override gogcli config/data/state/cache root (equivalent to GOG_HOME)"`
	Account             string `help:"Account email, alias, or auto for authenticated Google API commands" aliases:"acct" short:"a"`
	Client              string `help:"OAuth client name (selects stored credentials + token bucket)" default:"${client}"`
	AccessToken         string `help:"Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h)" env:"GOG_ACCESS_TOKEN"`
	EnableCommands      string `help:"Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI)" default:"${enabled_commands}"`
	EnableCommandsExact string `name:"enable-commands-exact" help:"Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children" default:"${enabled_commands_exact}"`
	DisableCommands     string `help:"Comma-separated list of disabled commands; dot paths allowed" default:"${disabled_commands}"`
	GmailNoSend         bool   `help:"Block Gmail send operations (agent safety)" default:"${gmail_no_send}"`
	ReadOnly            bool   `name:"readonly" help:"Block mutating API requests at runtime; auth add also requests read-only OAuth scopes" default:"${readonly}"`
	JSON                bool   `help:"Output JSON to stdout (best for scripting)" default:"${json}" aliases:"machine" short:"j"`
	Plain               bool   `help:"Output stable, parseable text to stdout (TSV; no colors)" default:"${plain}" aliases:"tsv" short:"p"`
	WrapUntrusted       bool   `name:"wrap-untrusted" help:"In JSON/raw output, wrap fetched text fields in external untrusted-content markers" default:"${wrap_untrusted}"`
	ResultsOnly         bool   `name:"results-only" help:"In JSON mode, emit only the primary result (drops envelope fields like nextPageToken)"`
	Select              string `name:"select" aliases:"pick,project" help:"In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands."`
	DryRun              bool   `help:"Do not make changes; print intended actions and exit successfully" aliases:"noop,preview,dryrun" short:"n"`
	Force               bool   `help:"Skip confirmations for destructive commands" aliases:"yes,assume-yes" short:"y"`
	NoInput             bool   `help:"Never prompt; fail instead (useful for CI)" aliases:"non-interactive,noninteractive"`
	Verbose             bool   `help:"Enable verbose logging" short:"v"`
	diagnostics         io.Writer
	authOperations      app.AuthOperations
	configStoreResolver func() (*config.ConfigStore, error)
	authMode            googleapi.AuthMode
}

type CLI struct {
	RootFlags `embed:""`

	Version kong.VersionFlag `help:"Print version and exit"`

	// Action-first desire paths.
	Send     GmailSendCmd     `cmd:"" name:"send" help:"Send an email (alias for 'gmail send')"`
	Ls       DriveLsCmd       `cmd:"" name:"ls" aliases:"list" help:"List Drive files (alias for 'drive ls')"`
	Search   DriveSearchCmd   `cmd:"" name:"search" aliases:"find" help:"Search Drive files (alias for 'drive search')"`
	Open     OpenCmd          `cmd:"" name:"open" aliases:"browse" help:"Print a best-effort web URL for a Google URL/ID (offline)"`
	Download DriveDownloadCmd `cmd:"" name:"download" aliases:"dl" help:"Download a Drive file (alias for 'drive download')"`
	Upload   DriveUploadCmd   `cmd:"" name:"upload" aliases:"up,put" help:"Upload a file to Drive (alias for 'drive upload')"`
	Login    AuthAddCmd       `cmd:"" name:"login" help:"Authorize and store a refresh token (alias for 'auth add')"`
	Logout   AuthRemoveCmd    `cmd:"" name:"logout" help:"Remove a stored refresh token (alias for 'auth remove')"`
	Status   AuthStatusCmd    `cmd:"" name:"status" aliases:"st" help:"Show auth/config status (alias for 'auth status')"`
	Me       PeopleMeCmd      `cmd:"" name:"me" help:"Show your profile (alias for 'people me')"`
	Whoami   PeopleMeCmd      `cmd:"" name:"whoami" aliases:"who-am-i" help:"Show your profile (alias for 'people me')"`

	Auth          AuthCmd               `cmd:"" help:"Auth and credentials"`
	Backup        BackupCmd             `cmd:"" help:"Encrypted Google account backups"`
	Batch         BatchCmd              `cmd:"" help:"Build and submit persisted Google Docs request batches"`
	Groups        GroupsCmd             `cmd:"" aliases:"group" help:"Cloud Identity Groups (Workspace only)"`
	Admin         AdminCmd              `cmd:"" help:"Google Workspace Admin (Directory API) - requires domain-wide delegation"`
	Drive         DriveCmd              `cmd:"" aliases:"drv" help:"Google Drive"`
	Docs          DocsCmd               `cmd:"" aliases:"doc" help:"Google Docs (export via Drive)"`
	Slides        SlidesCmd             `cmd:"" aliases:"slide" help:"Google Slides"`
	Calendar      CalendarCmd           `cmd:"" aliases:"cal" help:"Google Calendar"`
	Maps          MapsCmd               `cmd:"" aliases:"map" help:"Google Maps"`
	Classroom     ClassroomCmd          `cmd:"" aliases:"class" help:"Google Classroom"`
	Time          TimeCmd               `cmd:"" help:"Local time utilities"`
	Update        UpdateCmd             `cmd:"" help:"Check gogcli release status"`
	Gmail         GmailCmd              `cmd:"" aliases:"mail,email" help:"Gmail"`
	Chat          ChatCmd               `cmd:"" help:"Google Chat"`
	Contacts      ContactsCmd           `cmd:"" aliases:"contact" help:"Google Contacts"`
	Tasks         TasksCmd              `cmd:"" aliases:"task" help:"Google Tasks"`
	People        PeopleCmd             `cmd:"" aliases:"person" help:"Google People"`
	Keep          KeepCmd               `cmd:"" help:"Google Keep (Workspace only)"`
	Sheets        SheetsCmd             `cmd:"" aliases:"sheet" help:"Google Sheets"`
	Forms         FormsCmd              `cmd:"" aliases:"form" help:"Google Forms"`
	Sites         SitesCmd              `cmd:"" aliases:"site" help:"Google Sites (Drive-backed)"`
	Meet          MeetCmd               `cmd:"" aliases:"meeting" help:"Google Meet"`
	Zoom          ZoomCmd               `cmd:"" help:"Zoom"`
	AppScript     AppScriptCmd          `cmd:"" name:"appscript" aliases:"script,apps-script" help:"Google Apps Script"`
	Analytics     AnalyticsCmd          `cmd:"" aliases:"ga" help:"Google Analytics"`
	SearchConsole SearchConsoleCmd      `cmd:"" name:"searchconsole" aliases:"gsc,search-console,webmasters" help:"Google Search Console"`
	YouTube       YouTubeCmd            `cmd:"" name:"youtube" aliases:"yt" help:"YouTube Data API (search, activities, videos, playlists, comments, channels)"`
	Photos        PhotosCmd             `cmd:"" name:"photos" aliases:"photo" help:"Google Photos Library and Picker APIs"`
	API           APICmd                `cmd:"" name:"api" help:"Google Discovery APIs and generic method calls"`
	Config        ConfigCmd             `cmd:"" help:"Manage configuration"`
	Schema        SchemaCmd             `cmd:"" help:"Machine-readable command/flag schema" aliases:"help-json,helpjson"`
	Mcp           McpCmd                `cmd:"" name:"mcp" help:"Run a typed, allowlisted MCP server over stdio"`
	VersionCmd    VersionCmd            `cmd:"" name:"version" help:"Print version"`
	Completion    CompletionCmd         `cmd:"" help:"Generate shell completion scripts"`
	Complete      CompletionInternalCmd `cmd:"" name:"__complete" hidden:"" help:"Internal completion helper"`
}

type exitPanic struct{ code int }

func Execute(args []string) (err error) {
	return executeWithRuntime(args, newDefaultRuntime())
}

func executeWithRuntime(args []string, runtime *app.Runtime) (err error) {
	runtime = normalizedRuntime(runtime)
	runtimeIO := runtime.IO

	if len(args) == 0 {
		args = []string{"--help"}
	}
	args = rewriteHelpArgs(args)

	home, homeProvided := preScanHomeArg(args)
	if bindErr := bindRuntimeLayoutResolver(runtime, home); bindErr != nil {
		return reportEarlyError(runtimeIO.Err, newUsageError(bindErr))
	}
	if homeProvided {
		if validateErr := runtime.LayoutResolver.ValidateHomeOverride(); validateErr != nil {
			return reportEarlyError(runtimeIO.Err, newUsageError(validateErr))
		}
	}

	parser, cli, err := newParserWithWriters(helpDescription(runtime), runtimeIO.Out, runtimeIO.Err)
	if err != nil {
		return reportEarlyError(runtimeIO.Err, err)
	}
	args = rewriteDocsCellUpdateContentArgs(parser.Model, args)
	args = rewriteDesirePathArgs(parser.Model, args)

	defer func() {
		if r := recover(); r != nil {
			if ep, ok := r.(exitPanic); ok {
				if ep.code == 0 {
					err = nil
					return
				}
				err = &ExitError{Code: ep.code, Err: errors.New("exited")}
				return
			}
			panic(r)
		}
	}()

	kctx, err := parser.Parse(args)
	if err != nil {
		return reportEarlyError(runtimeIO.Err, wrapParseError(err))
	}
	cli.diagnostics = runtimeIO.Err
	cli.authOperations = runtime.Auth
	cli.authMode = googleapi.ParseAuthMode(os.Getenv("GOG_AUTH_MODE"))
	applyExplicitOutputModePrecedence(kctx, &cli.RootFlags)

	// Make config-backed account and alias resolution available to the
	// pre-Run enforcement hooks below (enforceGmailNoSend resolves the
	// target account). The context-backed resolver installed later
	// replaces this with an equivalent one; both reduce to
	// configureRuntimeConfig + runtime.Config.
	cli.configStoreResolver = func() (*config.ConfigStore, error) {
		if cfgErr := configureRuntimeConfig(runtime); cfgErr != nil {
			return nil, cfgErr
		}
		return runtime.Config, nil
	}

	if err = enforceBakedSafetyProfile(kctx); err != nil {
		return reportEarlyError(runtimeIO.Err, err)
	}
	if err = enforceEnabledCommands(kctx, cli.EnableCommands, cli.EnableCommandsExact); err != nil {
		return reportEarlyError(runtimeIO.Err, err)
	}
	if err = enforceDisabledCommands(kctx, cli.DisableCommands); err != nil {
		return reportEarlyError(runtimeIO.Err, err)
	}
	if err = enforceGmailNoSend(kctx, &cli.RootFlags, runtime); err != nil {
		return reportEarlyError(runtimeIO.Err, err)
	}

	logLevel := slog.LevelWarn
	if cli.Verbose {
		logLevel = slog.LevelDebug
	}
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(runtimeIO.Err, &slog.HandlerOptions{
		Level: logLevel,
	})))
	defer slog.SetDefault(previousLogger)

	// Optional automatic JSON output when stdout is piped/non-TTY.
	// We intentionally do this after parsing so `--plain` can override it.
	if envBool("GOG_AUTO_JSON") && !cli.JSON && !cli.Plain && !isTerminalWriter(runtimeIO.Out) {
		cli.JSON = true
	}

	mode, err := outfmt.FromFlags(cli.JSON, cli.Plain)
	if err != nil {
		return reportEarlyError(runtimeIO.Err, newUsageError(err))
	}
	err = validateJSONTransformFlags(mode, &cli.RootFlags)
	if err != nil {
		return reportEarlyError(runtimeIO.Err, err)
	}

	ctx := context.Background()
	ctx = app.WithRuntime(ctx, runtime)
	ctx = googleapi.WithReadOnly(ctx, cli.ReadOnly)
	runtimeContext := ctx
	serviceAccounts := func() (*config.ServiceAccountStore, error) {
		return commandServiceAccountStore(runtimeContext)
	}
	cli.configStoreResolver = func() (*config.ConfigStore, error) {
		return commandConfigStore(runtimeContext)
	}
	readCredentials := func(client string) (config.ClientCredentials, error) {
		store, resolveErr := commandOAuthCredentialsStore(runtimeContext)
		if resolveErr != nil {
			return config.ClientCredentials{}, resolveErr
		}
		return store.Read(client)
	}
	openTokens := func() (secrets.Store, error) {
		return runtime.Auth.OpenSecretsStore()
	}
	updateEmailReferences := func(oldEmail, newEmail string) error {
		store, resolveErr := cli.configStoreResolver()
		if resolveErr != nil {
			return resolveErr
		}
		return store.MigrateAccountEmailReferences(oldEmail, newEmail)
	}
	resolveClient := func(email string, override string) (string, error) {
		return resolveRuntimeClient(runtime, email, override)
	}
	authDependencies := googleapi.AuthDependencies{
		ResolveClient:             resolveClient,
		ReadCredentials:           readCredentials,
		OpenTokens:                openTokens,
		ServiceAccounts:           serviceAccounts,
		UpdateEmailReferences:     updateEmailReferences,
		Mode:                      cli.authMode,
		ADCTokenSource:            googleapi.DefaultADCTokenSource,
		ServiceAccountTokenSource: googleapi.DefaultServiceAccountTokenSource,
	}
	ctx = googleapi.WithAuthDependencies(ctx, authDependencies)
	composeRuntimeGoogleServices(runtime, googleapi.NewFactory(authDependencies, googleapi.FactoryOptions{
		PhotosBaseURL:       os.Getenv("GOG_PHOTOS_BASE_URL"),
		PhotosPickerBaseURL: os.Getenv("GOG_PHOTOS_PICKER_BASE_URL"),
	}))
	ctx = authclient.WithCredentialsReader(ctx, readCredentials)
	ctx = authclient.WithSecretsStoreOpener(ctx, openTokens)
	ctx = authclient.WithEmailReferenceUpdater(ctx, updateEmailReferences)
	ctx = authclient.WithClientResolver(ctx, resolveClient)
	ctx = outfmt.WithMode(ctx, mode)
	ctx = outfmt.WithJSONTransform(ctx, outfmt.JSONTransform{
		ResultsOnly: cli.ResultsOnly,
		Select:      splitCommaList(cli.Select),
	})
	if cli.WrapUntrusted {
		ctx = outfmt.WithUntrustedWrapper(ctx, outfmt.UntrustedWrapOptions{
			Enabled: true,
			Source:  "google_api",
		})
	}
	ctx = authclient.WithClient(ctx, cli.Client)
	ctx = authclient.WithAccessToken(ctx, directAccessToken(&cli.RootFlags))

	uiColor := cli.Color
	if outfmt.IsJSON(ctx) || outfmt.IsPlain(ctx) {
		uiColor = colorNever
	}

	u, err := ui.New(ui.Options{
		Stdout: runtimeIO.Out,
		Stderr: runtimeIO.Err,
		Color:  uiColor,
	})
	if err != nil {
		return reportEarlyError(runtimeIO.Err, newUsageError(err))
	}
	ctx = ui.WithUI(ctx, u)

	kctx.BindTo(ctx, (*context.Context)(nil))
	kctx.Bind(&cli.RootFlags)

	err = kctx.Run()
	if err == nil {
		return nil
	}
	// Some commands intentionally exit early with success.
	if ExitCode(err) == 0 {
		return nil
	}
	err = stableExitCode(err)

	if u := ui.FromContext(ctx); u != nil {
		msg := strings.TrimSpace(errfmt.Format(err))
		if msg != "" {
			u.Err().Error(msg)
		}
		return err
	}
	msg := strings.TrimSpace(errfmt.Format(err))
	if msg != "" {
		_, _ = fmt.Fprintln(runtimeIO.Err, msg)
	}
	return err
}

func rewriteHelpArgs(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--help" || arg == "-h" {
			return append([]string(nil), args[:i+1]...)
		}
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return args
		}
		if strings.HasPrefix(arg, "-") {
			if globalFlagTakesValue(arg) && i+1 < len(args) {
				i++
			}
			continue
		}
		if arg != "help" {
			return args
		}

		out := make([]string, 0, len(args))
		out = append(out, args[:i]...)
		out = append(out, args[i+1:]...)
		out = append(out, "--help")
		return out
	}
	return args
}

func validateJSONTransformFlags(mode outfmt.Mode, flags *RootFlags) error {
	if flags == nil || mode.JSON {
		return nil
	}

	hasResultsOnly := flags.ResultsOnly
	hasSelect := strings.TrimSpace(flags.Select) != ""
	switch {
	case hasResultsOnly && hasSelect:
		return usage("--results-only and --select require --json")
	case hasResultsOnly:
		return usage("--results-only requires --json")
	case hasSelect:
		return usage("--select requires --json")
	default:
		return nil
	}
}

func applyExplicitOutputModePrecedence(kctx *kong.Context, flags *RootFlags) {
	if flags == nil {
		return
	}

	jsonSet := flagProvided(kctx, "json")
	plainSet := flagProvided(kctx, "plain")
	switch {
	case jsonSet && !plainSet:
		flags.Plain = false
	case plainSet && !jsonSet:
		flags.JSON = false
	}
}

func reportEarlyError(w io.Writer, err error) error {
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(errfmt.Format(err))
	if msg != "" {
		_, _ = fmt.Fprintln(w, msg)
	}
	return err
}

func isTerminalWriter(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && termutil.IsTerminal(file)
}

func preScanHomeArg(args []string) (string, bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return "", false
		}
		if arg == "--home" {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", false
		}
		if strings.HasPrefix(arg, "--home=") {
			return strings.TrimPrefix(arg, "--home="), true
		}
		if strings.HasPrefix(arg, "-") {
			if globalFlagTakesValue(arg) && i+1 < len(args) {
				i++
			}
			continue
		}
	}
	return "", false
}

func globalFlagTakesValue(flag string) bool {
	switch flag {
	case "--color", "--account", "--acct", "--client", "--access-token", "--enable-commands", "--enable-commands-exact", "--disable-commands", "--select", "--pick", "--project", "--home", "-a":
		return true
	default:
		return false
	}
}

func wrapParseError(err error) error {
	if err == nil {
		return nil
	}
	var parseErr *kong.ParseError
	if errors.As(err, &parseErr) {
		return &ExitError{Code: 2, Err: parseErr}
	}
	return err
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", boolTrue, "yes", "y", "on":
		return true
	default:
		return false
	}
}

func boolString(v bool) string {
	if v {
		return boolTrue
	}
	return boolFalse
}

func newParser(description string) (*kong.Kong, *CLI, error) {
	return newParserWithWriters(description, os.Stdout, os.Stderr)
}

func newParserWithWriters(description string, stdout, stderr io.Writer) (*kong.Kong, *CLI, error) {
	envMode := outfmt.FromEnv()
	vars := kong.Vars{
		"auth_services":          googleauth.UserServiceCSV(),
		"color":                  envOr("GOG_COLOR", "auto"),
		"calendar_weekday":       envOr("GOG_CALENDAR_WEEKDAY", "false"),
		"client":                 envOr("GOG_CLIENT", ""),
		"disabled_commands":      envOr("GOG_DISABLE_COMMANDS", ""),
		"enabled_commands":       envOr("GOG_ENABLE_COMMANDS", ""),
		"enabled_commands_exact": envOr("GOG_ENABLE_COMMANDS_EXACT", ""),
		"gmail_no_send":          boolString(envBool("GOG_GMAIL_NO_SEND")),
		"json":                   boolString(envMode.JSON),
		"plain":                  boolString(envMode.Plain),
		"readonly":               boolString(envBool("GOG_READONLY")),
		"wrap_untrusted":         boolString(envBool("GOG_WRAP_UNTRUSTED")),
		"version":                VersionString(),
	}

	cli := &CLI{}
	parser, err := kong.New(
		cli,
		kong.Name("gog"),
		kong.Description(description),
		kong.ConfigureHelp(helpOptions()),
		kong.Help(helpPrinter),
		kong.Vars(vars),
		kong.Writers(stdout, stderr),
		kong.Exit(func(code int) { panic(exitPanic{code: code}) }),
	)
	if err != nil {
		return nil, nil, err
	}
	return parser, cli, nil
}

func baseDescription() string {
	return "Google CLI for Gmail/Calendar/Chat/Classroom/Drive/Contacts/Tasks/Sheets/Docs/Slides/People/Forms/Meet/App Script/Analytics/Search Console/Groups/Admin/Keep/YouTube/Maps/Photos"
}

func helpDescription(runtime *app.Runtime) string {
	desc := baseDescription()

	configLine := "unknown"
	if err := configureRuntimeConfig(runtime); err != nil {
		configLine = fmt.Sprintf("error: %v", err)
	} else if runtime.Config.Path() != "" {
		configLine = runtime.Config.Path()
	}

	backendInfo, err := runtimeKeyringBackendInfo(runtime)
	var backendLine string
	if err != nil {
		backendLine = fmt.Sprintf("error: %v", err)
	} else if backendInfo.Value != "" {
		backendLine = fmt.Sprintf("%s (source: %s)", backendInfo.Value, backendInfo.Source)
	}

	return fmt.Sprintf("%s\n\nConfig:\n  file: %s\n  keyring backend: %s", desc, configLine, backendLine)
}

// newUsageError wraps errors in a way main() can map to exit code 2.
func newUsageError(err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: 2, Err: err}
}
