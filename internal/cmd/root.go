package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/errfmt"
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
	Account             string `help:"Account email for API commands (gmail/calendar/chat/classroom/drive/drivelabels/docs/slides/contacts/tasks/people/sheets/forms/sites/appscript/analytics/searchconsole/ads/photos)" aliases:"acct" short:"a"`
	Client              string `help:"OAuth client name (selects stored credentials + token bucket)" default:"${client}"`
	AccessToken         string `help:"Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h)" env:"GOG_ACCESS_TOKEN"`
	EnableCommands      string `help:"Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI)" default:"${enabled_commands}"`
	EnableCommandsExact string `name:"enable-commands-exact" help:"Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children"`
	DisableCommands     string `help:"Comma-separated list of disabled commands; dot paths allowed" default:"${disabled_commands}"`
	GmailNoSend         bool   `help:"Block Gmail send operations (agent safety)" default:"${gmail_no_send}"`
	JSON                bool   `help:"Output JSON to stdout (best for scripting)" default:"${json}" aliases:"machine" short:"j"`
	Plain               bool   `help:"Output stable, parseable text to stdout (TSV; no colors)" default:"${plain}" aliases:"tsv" short:"p"`
	WrapUntrusted       bool   `name:"wrap-untrusted" help:"In JSON/raw output, wrap fetched text fields in external untrusted-content markers" default:"${wrap_untrusted}"`
	ResultsOnly         bool   `name:"results-only" help:"In JSON mode, emit only the primary result (drops envelope fields like nextPageToken)"`
	Select              string `name:"select" aliases:"pick,project" help:"In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands."`
	DryRun              bool   `help:"Do not make changes; print intended actions and exit successfully" aliases:"noop,preview,dryrun" short:"n"`
	Force               bool   `help:"Skip confirmations for destructive commands" aliases:"yes,assume-yes" short:"y"`
	NoInput             bool   `help:"Never prompt; fail instead (useful for CI)" aliases:"non-interactive,noninteractive"`
	Verbose             bool   `help:"Enable verbose logging" short:"v"`
}

type CLI struct {
	RootFlags `embed:""`

	Version kong.VersionFlag `help:"Print version and exit"`

	// Action-first desire paths (agent-friendly shortcuts).
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
	Groups        GroupsCmd             `cmd:"" aliases:"group" help:"Google Groups"`
	Admin         AdminCmd              `cmd:"" help:"Google Workspace Admin (Directory API) - requires domain-wide delegation"`
	Drive         DriveCmd              `cmd:"" aliases:"drv" help:"Google Drive"`
	Docs          DocsCmd               `cmd:"" aliases:"doc" help:"Google Docs (export via Drive)"`
	Slides        SlidesCmd             `cmd:"" aliases:"slide" help:"Google Slides"`
	Calendar      CalendarCmd           `cmd:"" aliases:"cal" help:"Google Calendar"`
	Maps          MapsCmd               `cmd:"" aliases:"map" help:"Google Maps"`
	Classroom     ClassroomCmd          `cmd:"" aliases:"class" help:"Google Classroom"`
	Time          TimeCmd               `cmd:"" help:"Local time utilities"`
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
	YouTube       YouTubeCmd            `cmd:"" name:"youtube" aliases:"yt" help:"YouTube Data API (activities, videos, playlists, comments, channels)"`
	Photos        PhotosCmd             `cmd:"" name:"photos" aliases:"photo" help:"Google Photos Library API (app-created media)"`
	Config        ConfigCmd             `cmd:"" help:"Manage configuration"`
	ExitCodes     AgentExitCodesCmd     `cmd:"" name:"exit-codes" aliases:"exitcodes" help:"Print stable exit codes (alias for 'agent exit-codes')"`
	Agent         AgentCmd              `cmd:"" help:"Agent-friendly helpers"`
	Schema        SchemaCmd             `cmd:"" help:"Machine-readable command/flag schema" aliases:"help-json,helpjson"`
	VersionCmd    VersionCmd            `cmd:"" name:"version" help:"Print version"`
	Completion    CompletionCmd         `cmd:"" help:"Generate shell completion scripts"`
	Complete      CompletionInternalCmd `cmd:"" name:"__complete" hidden:"" help:"Internal completion helper"`
}

type exitPanic struct{ code int }

func Execute(args []string) (err error) {
	if len(args) == 0 {
		args = []string{"--help"}
	}
	args = rewriteDesirePathArgs(args)

	preHomeApplied := false
	if home, ok := preScanHomeArg(args); ok {
		restoreHome, homeErr := config.SetHomeOverride(home)
		if homeErr != nil {
			return newUsageError(homeErr)
		}
		preHomeApplied = true
		defer restoreHome()
	}

	parser, cli, err := newParser(helpDescription())
	if err != nil {
		return err
	}

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
		parsedErr := wrapParseError(err)
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(parsedErr))
		return parsedErr
	}
	if !preHomeApplied && strings.TrimSpace(cli.Home) != "" {
		restoreHome, homeErr := config.SetHomeOverride(cli.Home)
		if homeErr != nil {
			return newUsageError(homeErr)
		}
		defer restoreHome()
	}

	if err = enforceBakedSafetyProfile(kctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
		return err
	}
	if err = enforceEnabledCommands(kctx, cli.EnableCommands, cli.EnableCommandsExact); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
		return err
	}
	if err = enforceDisabledCommands(kctx, cli.DisableCommands); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
		return err
	}
	if err = enforceGmailNoSend(kctx, &cli.RootFlags); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
		return err
	}

	logLevel := slog.LevelWarn
	if cli.Verbose {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	// Opt-in "agent mode": default to JSON when stdout is piped/non-TTY.
	// We intentionally do this after parsing so `--plain` can override it.
	if envBool("GOG_AUTO_JSON") && !cli.JSON && !cli.Plain && !termutil.IsTerminal(os.Stdout) {
		cli.JSON = true
	}

	mode, err := outfmt.FromFlags(cli.JSON, cli.Plain)
	if err != nil {
		return newUsageError(err)
	}

	ctx := context.Background()
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
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Color:  uiColor,
	})
	if err != nil {
		return err
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
	return err
}

func Main() {
	args := os.Args[1:]
	if err := Execute(args); err != nil {
		os.Exit(ExitCode(err))
	}
}

func newParser(desc string) (*kong.Kong, *CLI, error) {
	cli := &CLI{}
	parser, err := kong.New(cli,
		kong.Name("gog"),
		kong.Description(desc),
		kong.UsageOnError(),
		kong.Vars{
			"color":             envOrDefault("GOG_COLOR", colorAuto),
			"client":            envOrDefault("GOG_CLIENT", googleauth.DefaultClientName),
			"json":              envOrDefault("GOG_JSON", boolFalse),
			"plain":             envOrDefault("GOG_PLAIN", boolFalse),
			"wrap_untrusted":    envOrDefault("GOG_WRAP_UNTRUSTED", boolFalse),
			"gmail_no_send":     envOrDefault("GOG_GMAIL_NO_SEND", boolFalse),
			"enabled_commands":  envOrDefault("GOG_ENABLE_COMMANDS", ""),
			"disabled_commands": envOrDefault("GOG_DISABLE_COMMANDS", ""),
		},
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}),
	)
	if err != nil {
		return nil, nil, err
	}
	return parser, cli, nil
}

func helpDescription() string {
	return "Google services CLI for terminal automation"
}

func directAccessToken(flags *RootFlags) string {
	if flags == nil {
		return ""
	}
	return strings.TrimSpace(flags.AccessToken)
}

func envOrDefault(name string, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
