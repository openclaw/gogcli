package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	admin "google.golang.org/api/admin/directory/v1"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/chat/v1"
	"google.golang.org/api/classroom/v1"
	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	driveactivityapi "google.golang.org/api/driveactivity/v2"
	drivelabelsapi "google.golang.org/api/drivelabels/v2"
	formsapi "google.golang.org/api/forms/v1"
	"google.golang.org/api/gmail/v1"
	keepapi "google.golang.org/api/keep/v1"
	meetapi "google.golang.org/api/meet/v2"
	"google.golang.org/api/people/v1"
	scriptapi "google.golang.org/api/script/v1"
	searchconsoleapi "google.golang.org/api/searchconsole/v1"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"
	"google.golang.org/api/tasks/v1"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/termutil"
)

var (
	errIncompleteRuntimeLayout = errors.New("injected config store has incomplete runtime layout")
	errRuntimeLayoutMismatch   = errors.New("runtime layout does not match injected config store")
)

func newDefaultRuntime() *app.Runtime {
	return &app.Runtime{
		IO: app.IO{
			In:  os.Stdin,
			Out: os.Stdout,
			Err: os.Stderr,
		},
		Services: app.Services{
			AdminDirectory: googleapi.NewAdminDirectory,
			AdminOrgUnit:   googleapi.NewAdminDirectoryOrgUnit,
			AppScript:      googleapi.NewAppScript,
			AnalyticsAdmin: googleapi.NewAnalyticsAdmin,
			AnalyticsData:  googleapi.NewAnalyticsData,
			Calendar:       googleapi.NewCalendar,
			Chat:           googleapi.NewChat,
			Classroom:      googleapi.NewClassroom,
			CloudIdentity:  googleapi.NewCloudIdentityGroups,
			Docs:           googleapi.NewDocs,
			DocsHTTP: func(ctx context.Context, account string) (*http.Client, error) {
				return googleapi.NewHTTPClient(ctx, googleauth.ServiceDocs, account)
			},
			Drive:           googleapi.NewDrive,
			DriveActivity:   googleapi.NewDriveActivity,
			DriveLabels:     googleapi.NewDriveLabels,
			Forms:           googleapi.NewForms,
			Gmail:           googleapi.NewGmail,
			GmailDelete:     googleapi.NewGmailBatchDelete,
			Keep:            googleapi.NewKeepWithServiceAccount,
			Meet:            googleapi.NewMeet,
			PeopleContacts:  googleapi.NewPeopleContacts,
			PeopleDirectory: googleapi.NewPeopleDirectory,
			PeopleOther:     googleapi.NewPeopleOtherContacts,
			Photos:          newPhotosClient,
			PhotosPicker:    newPhotosPickerClient,
			SearchConsole:   googleapi.NewSearchConsole,
			Sheets:          googleapi.NewSheets,
			SitesDrive:      googleapi.NewSitesDrive,
			Slides:          googleapi.NewSlides,
			Tasks:           googleapi.NewTasks,
			YouTubeAPIKey:   googleapi.NewYouTubeWithAPIKey,
			YouTubeAccount:  googleapi.NewYouTubeForAccount,
			YouTubeComments: googleapi.NewYouTubeCommentsForAccount,
			Zoom:            newZoomMeetingClient,
			DriveDownload:   driveDownload,
			DriveExport:     driveExportDownload,
			OpenURL:         openPhotosPickerBrowser,
		},
		Auth: app.AuthOperations{
			AuthorizeGoogle:         googleauth.Authorize,
			StartManageServer:       googleauth.StartManageServer,
			CheckRefreshToken:       googleauth.CheckRefreshToken,
			EnsureKeychainAccess:    secrets.EnsureKeychainAccessContext,
			FetchAuthorizedIdentity: googleauth.IdentityForRefreshToken,
			ManualAuthURL:           googleauth.ManualAuthURL,
		},
	}
}

func normalizedRuntime(runtime *app.Runtime) *app.Runtime {
	defaults := newDefaultRuntime()
	if runtime == nil {
		return defaults
	}
	normalized := *runtime
	if normalized.IO.In == nil {
		normalized.IO.In = defaults.IO.In
	}
	if normalized.IO.Out == nil {
		normalized.IO.Out = defaults.IO.Out
	}
	if normalized.IO.Err == nil {
		normalized.IO.Err = defaults.IO.Err
	}
	if normalized.Services.AdminDirectory == nil {
		normalized.Services.AdminDirectory = defaults.Services.AdminDirectory
	}
	if normalized.Services.AdminOrgUnit == nil {
		normalized.Services.AdminOrgUnit = defaults.Services.AdminOrgUnit
	}
	if normalized.Services.AppScript == nil {
		normalized.Services.AppScript = defaults.Services.AppScript
	}
	if normalized.Services.AnalyticsAdmin == nil {
		normalized.Services.AnalyticsAdmin = defaults.Services.AnalyticsAdmin
	}
	if normalized.Services.AnalyticsData == nil {
		normalized.Services.AnalyticsData = defaults.Services.AnalyticsData
	}
	if normalized.Services.Calendar == nil {
		normalized.Services.Calendar = defaults.Services.Calendar
	}
	if normalized.Services.Chat == nil {
		normalized.Services.Chat = defaults.Services.Chat
	}
	if normalized.Services.Classroom == nil {
		normalized.Services.Classroom = defaults.Services.Classroom
	}
	if normalized.Services.CloudIdentity == nil {
		normalized.Services.CloudIdentity = defaults.Services.CloudIdentity
	}
	if normalized.Services.Drive == nil {
		normalized.Services.Drive = defaults.Services.Drive
	}
	if normalized.Services.DriveActivity == nil {
		normalized.Services.DriveActivity = defaults.Services.DriveActivity
	}
	if normalized.Services.DriveLabels == nil {
		normalized.Services.DriveLabels = defaults.Services.DriveLabels
	}
	if normalized.Services.Docs == nil {
		normalized.Services.Docs = defaults.Services.Docs
	}
	if normalized.Services.DocsHTTP == nil {
		normalized.Services.DocsHTTP = defaults.Services.DocsHTTP
	}
	if normalized.Services.Forms == nil {
		normalized.Services.Forms = defaults.Services.Forms
	}
	if normalized.Services.GmailDelete == nil {
		if normalized.Services.Gmail != nil {
			normalized.Services.GmailDelete = normalized.Services.Gmail
		} else {
			normalized.Services.GmailDelete = defaults.Services.GmailDelete
		}
	}
	if normalized.Services.Gmail == nil {
		normalized.Services.Gmail = defaults.Services.Gmail
	}
	if normalized.Services.Keep == nil {
		normalized.Services.Keep = defaults.Services.Keep
	}
	if normalized.Services.Meet == nil {
		normalized.Services.Meet = defaults.Services.Meet
	}
	if normalized.Services.PeopleContacts == nil {
		normalized.Services.PeopleContacts = defaults.Services.PeopleContacts
	}
	if normalized.Services.PeopleDirectory == nil {
		normalized.Services.PeopleDirectory = defaults.Services.PeopleDirectory
	}
	if normalized.Services.PeopleOther == nil {
		normalized.Services.PeopleOther = defaults.Services.PeopleOther
	}
	if normalized.Services.Photos == nil {
		normalized.Services.Photos = defaults.Services.Photos
	}
	if normalized.Services.PhotosPicker == nil {
		normalized.Services.PhotosPicker = defaults.Services.PhotosPicker
	}
	if normalized.Services.SearchConsole == nil {
		normalized.Services.SearchConsole = defaults.Services.SearchConsole
	}
	if normalized.Services.Sheets == nil {
		normalized.Services.Sheets = defaults.Services.Sheets
	}
	if normalized.Services.SitesDrive == nil {
		normalized.Services.SitesDrive = defaults.Services.SitesDrive
	}
	if normalized.Services.Slides == nil {
		normalized.Services.Slides = defaults.Services.Slides
	}
	if normalized.Services.Tasks == nil {
		normalized.Services.Tasks = defaults.Services.Tasks
	}
	if normalized.Services.YouTubeAPIKey == nil {
		normalized.Services.YouTubeAPIKey = defaults.Services.YouTubeAPIKey
	}
	if normalized.Services.YouTubeAccount == nil {
		normalized.Services.YouTubeAccount = defaults.Services.YouTubeAccount
	}
	if normalized.Services.YouTubeComments == nil {
		normalized.Services.YouTubeComments = defaults.Services.YouTubeComments
	}
	if normalized.Services.Zoom == nil {
		normalized.Services.Zoom = defaults.Services.Zoom
	}
	if normalized.Services.DriveDownload == nil {
		normalized.Services.DriveDownload = defaults.Services.DriveDownload
	}
	if normalized.Services.DriveExport == nil {
		normalized.Services.DriveExport = defaults.Services.DriveExport
	}
	if normalized.Services.OpenURL == nil {
		normalized.Services.OpenURL = defaults.Services.OpenURL
	}
	normalizeRuntimeAuth(&normalized, defaults)
	return &normalized
}

func normalizeRuntimeAuth(runtime *app.Runtime, defaults *app.Runtime) {
	if runtime.Auth.OpenSecretsStore == nil {
		runtime.Auth.OpenSecretsStore = func() (secrets.Store, error) {
			if err := configureRuntimeSecrets(runtime, ""); err != nil {
				return nil, err
			}
			return secrets.OpenWithConfig(runtime.Layout, runtime.Config)
		}
	}
	if runtime.Auth.OpenSecretStore == nil {
		runtime.Auth.OpenSecretStore = func() (secrets.SecretStore, error) {
			if err := configureRuntimeSecrets(runtime, ""); err != nil {
				return nil, err
			}
			return secrets.OpenWithConfig(runtime.Layout, runtime.Config)
		}
	}
	if runtime.Auth.AuthorizeGoogle == nil {
		runtime.Auth.AuthorizeGoogle = defaults.Auth.AuthorizeGoogle
	}
	if runtime.Auth.StartManageServer == nil {
		runtime.Auth.StartManageServer = defaults.Auth.StartManageServer
	}
	if runtime.Auth.CheckRefreshToken == nil {
		runtime.Auth.CheckRefreshToken = defaults.Auth.CheckRefreshToken
	}
	if runtime.Auth.EnsureKeychainAccess == nil {
		runtime.Auth.EnsureKeychainAccess = defaults.Auth.EnsureKeychainAccess
	}
	if runtime.Auth.FetchAuthorizedIdentity == nil {
		runtime.Auth.FetchAuthorizedIdentity = defaults.Auth.FetchAuthorizedIdentity
	}
	if runtime.Auth.ManualAuthURL == nil {
		runtime.Auth.ManualAuthURL = defaults.Auth.ManualAuthURL
	}
}

func configureRuntimeConfig(runtime *app.Runtime, homeOverride string) error {
	if runtime.Config != nil {
		return hydrateRuntimeLayoutFromConfig(runtime)
	}

	if err := configureRuntimeLayout(runtime, homeOverride, config.PathKindConfig); err != nil {
		return err
	}

	runtime.Config = config.NewConfigStore(runtime.Layout)
	runtime.ConfigManaged = true
	return nil
}

func configureRuntimeSecrets(runtime *app.Runtime, homeOverride string) error {
	if err := configureRuntimeLayout(runtime, homeOverride, config.PathKindConfig, config.PathKindData); err != nil {
		return err
	}
	if runtime.Config == nil {
		runtime.Config = config.NewConfigStore(runtime.Layout)
		runtime.ConfigManaged = true
	}
	return nil
}

func configureRuntimeLayout(runtime *app.Runtime, homeOverride string, kinds ...config.PathKind) error {
	if err := hydrateRuntimeLayoutFromConfig(runtime); err != nil {
		return err
	}

	missing := make([]config.PathKind, 0, len(kinds))
	for _, kind := range kinds {
		dir, err := runtime.Layout.Dir(kind)
		if err != nil {
			return err
		}
		if dir == "" {
			missing = append(missing, kind)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if runtime.Config != nil && !runtime.ConfigManaged {
		return fmt.Errorf("%w: missing %v", errIncompleteRuntimeLayout, missing)
	}

	layout, err := config.ResolveSystemLayoutFor(homeOverride, missing...)
	if err != nil {
		return err
	}
	for _, kind := range missing {
		dir, dirErr := layout.Dir(kind)
		if dirErr != nil {
			return dirErr
		}
		switch kind {
		case config.PathKindConfig:
			runtime.Layout.ConfigDir = dir
			runtime.Layout.ExplicitConfig = layout.ExplicitConfig
		case config.PathKindData:
			runtime.Layout.DataDir = dir
			runtime.Layout.ExplicitData = layout.ExplicitData
		case config.PathKindState:
			runtime.Layout.StateDir = dir
			runtime.Layout.ExplicitState = layout.ExplicitState
		case config.PathKindCache:
			runtime.Layout.CacheDir = dir
			runtime.Layout.ExplicitCache = layout.ExplicitCache
		}
	}
	runtime.Layout.UsesXDG = runtime.Layout.UsesXDG || layout.UsesXDG
	runtime.Layout.UsesXDGState = runtime.Layout.UsesXDGState || layout.UsesXDGState
	return nil
}

func hydrateRuntimeLayoutFromConfig(runtime *app.Runtime) error {
	if runtime.Config == nil {
		return nil
	}

	storeLayout := runtime.Config.Layout()
	if runtime.Layout.ConfigDir != "" &&
		storeLayout.ConfigDir != "" &&
		runtime.Layout.ConfigDir != storeLayout.ConfigDir {
		return fmt.Errorf("%w: runtime=%s config_store=%s",
			errRuntimeLayoutMismatch, runtime.Layout.ConfigDir, storeLayout.ConfigDir)
	}

	mergeLayoutKind(&runtime.Layout, storeLayout, config.PathKindConfig)
	mergeLayoutKind(&runtime.Layout, storeLayout, config.PathKindData)
	mergeLayoutKind(&runtime.Layout, storeLayout, config.PathKindState)
	mergeLayoutKind(&runtime.Layout, storeLayout, config.PathKindCache)
	runtime.Layout.UsesXDG = runtime.Layout.UsesXDG || storeLayout.UsesXDG
	runtime.Layout.UsesXDGState = runtime.Layout.UsesXDGState || storeLayout.UsesXDGState
	return nil
}

func mergeLayoutKind(target *config.Layout, source config.Layout, kind config.PathKind) {
	targetDir, _ := target.Dir(kind)
	if targetDir != "" {
		return
	}
	sourceDir, _ := source.Dir(kind)
	if sourceDir == "" {
		return
	}

	switch kind {
	case config.PathKindConfig:
		target.ConfigDir = sourceDir
		target.ExplicitConfig = source.ExplicitConfig
	case config.PathKindData:
		target.DataDir = sourceDir
		target.ExplicitData = source.ExplicitData
	case config.PathKindState:
		target.StateDir = sourceDir
		target.ExplicitState = source.ExplicitState
	case config.PathKindCache:
		target.CacheDir = sourceDir
		target.ExplicitCache = source.ExplicitCache
	}
}

func commandLayout(ctx context.Context, kinds ...config.PathKind) (config.Layout, error) {
	if runtime, ok := app.FromContext(ctx); ok {
		if err := configureRuntimeLayout(runtime, "", kinds...); err != nil {
			return config.Layout{}, err
		}
		return runtime.Layout, nil
	}
	return config.ResolveSystemLayoutFor("", kinds...)
}

func resolveRuntimeClient(runtime *app.Runtime, homeOverride string, email string, override string) (string, error) {
	if err := configureRuntimeConfig(runtime, homeOverride); err != nil {
		return "", err
	}
	cfg, err := runtime.Config.Read()
	if err != nil {
		return "", err
	}

	return config.ResolveClientForAccountWithCredentials(cfg, email, override, func(client string) (bool, error) {
		if err := configureRuntimeLayout(runtime, homeOverride, config.PathKindConfig, config.PathKindData); err != nil {
			return false, err
		}
		files := config.NewClientCredentialsStore(runtime.Layout)
		_, exists, err := files.ExistingPath(client)
		return exists, err
	})
}

func commandIO(ctx context.Context) app.IO {
	commandIO := newDefaultRuntime().IO
	if runtimeIO, ok := app.IOFromContext(ctx); ok {
		if runtimeIO.In != nil {
			commandIO.In = runtimeIO.In
		}
		if runtimeIO.Out != nil {
			commandIO.Out = runtimeIO.Out
		}
		if runtimeIO.Err != nil {
			commandIO.Err = runtimeIO.Err
		}
	}
	return commandIO
}

func stdoutWriter(ctx context.Context) io.Writer {
	return commandIO(ctx).Out
}

func stderrWriter(ctx context.Context) io.Writer {
	return commandIO(ctx).Err
}

func stdinReader(ctx context.Context) io.Reader {
	return commandIO(ctx).In
}

func stdinIsTerminal(ctx context.Context) bool {
	file, ok := stdinReader(ctx).(*os.File)
	return ok && termutil.IsTerminal(file)
}

func startAuthManageServer(ctx context.Context, options googleauth.ManageServerOptions) error {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Auth.StartManageServer != nil {
		return runtime.Auth.StartManageServer(ctx, options)
	}
	return googleauth.StartManageServer(ctx, options)
}

func checkAuthRefreshToken(ctx context.Context, client, refreshToken string, scopes []string, timeout time.Duration) error {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Auth.CheckRefreshToken != nil {
		return runtime.Auth.CheckRefreshToken(ctx, client, refreshToken, scopes, timeout)
	}
	return googleauth.CheckRefreshToken(ctx, client, refreshToken, scopes, timeout)
}

func buildManualAuthURL(ctx context.Context, options googleauth.AuthorizeOptions) (googleauth.ManualAuthURLResult, error) {
	if err := bindManualAuthStateStore(ctx, &options); err != nil {
		return googleauth.ManualAuthURLResult{}, err
	}
	if runtime, ok := app.FromContext(ctx); ok && runtime.Auth.ManualAuthURL != nil {
		return runtime.Auth.ManualAuthURL(ctx, options)
	}
	return googleauth.ManualAuthURL(ctx, options)
}

func adminDirectoryService(ctx context.Context, account string) (*admin.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.AdminDirectory != nil {
		return runtime.Services.AdminDirectory(ctx, account)
	}
	return googleapi.NewAdminDirectory(ctx, account)
}

func adminOrgUnitDirectoryService(ctx context.Context, account string) (*admin.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.AdminOrgUnit != nil {
		return runtime.Services.AdminOrgUnit(ctx, account)
	}
	return googleapi.NewAdminDirectoryOrgUnit(ctx, account)
}

func appScriptService(ctx context.Context, account string) (*scriptapi.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.AppScript != nil {
		return runtime.Services.AppScript(ctx, account)
	}
	return googleapi.NewAppScript(ctx, account)
}

func analyticsAdminService(ctx context.Context, account string) (*analyticsadmin.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.AnalyticsAdmin != nil {
		return runtime.Services.AnalyticsAdmin(ctx, account)
	}
	return googleapi.NewAnalyticsAdmin(ctx, account)
}

func analyticsDataService(ctx context.Context, account string) (*analyticsdata.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.AnalyticsData != nil {
		return runtime.Services.AnalyticsData(ctx, account)
	}
	return googleapi.NewAnalyticsData(ctx, account)
}

func calendarService(ctx context.Context, account string) (*calendar.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Calendar != nil {
		return runtime.Services.Calendar(ctx, account)
	}
	return googleapi.NewCalendar(ctx, account)
}

func chatService(ctx context.Context, account string) (*chat.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Chat != nil {
		return runtime.Services.Chat(ctx, account)
	}
	return googleapi.NewChat(ctx, account)
}

func classroomService(ctx context.Context, account string) (*classroom.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Classroom != nil {
		return runtime.Services.Classroom(ctx, account)
	}
	return googleapi.NewClassroom(ctx, account)
}

func cloudIdentityService(ctx context.Context, account string) (*cloudidentity.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.CloudIdentity != nil {
		return runtime.Services.CloudIdentity(ctx, account)
	}
	return googleapi.NewCloudIdentityGroups(ctx, account)
}

func keepServiceWithServiceAccount(ctx context.Context, path, impersonate string) (*keepapi.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Keep != nil {
		return runtime.Services.Keep(ctx, path, impersonate)
	}
	return googleapi.NewKeepWithServiceAccount(ctx, path, impersonate)
}

func meetService(ctx context.Context, account string) (*meetapi.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Meet != nil {
		return runtime.Services.Meet(ctx, account)
	}
	return googleapi.NewMeet(ctx, account)
}

func photosService(ctx context.Context, account string) (*googleapi.PhotosClient, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Photos != nil {
		return runtime.Services.Photos(ctx, account)
	}
	return newPhotosClient(ctx, account)
}

func photosPickerService(ctx context.Context, account string) (*googleapi.PhotosPickerClient, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.PhotosPicker != nil {
		return runtime.Services.PhotosPicker(ctx, account)
	}
	return newPhotosPickerClient(ctx, account)
}

func openURL(ctx context.Context, uri string) error {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.OpenURL != nil {
		return runtime.Services.OpenURL(ctx, uri)
	}
	return openPhotosPickerBrowser(ctx, uri)
}

func driveService(ctx context.Context, account string) (*drive.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Drive != nil {
		return runtime.Services.Drive(ctx, account)
	}
	return googleapi.NewDrive(ctx, account)
}

func driveActivityService(ctx context.Context, account string) (*driveactivityapi.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.DriveActivity != nil {
		return runtime.Services.DriveActivity(ctx, account)
	}
	return googleapi.NewDriveActivity(ctx, account)
}

func driveLabelsService(ctx context.Context, account string) (*drivelabelsapi.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.DriveLabels != nil {
		return runtime.Services.DriveLabels(ctx, account)
	}
	return googleapi.NewDriveLabels(ctx, account)
}

func docsService(ctx context.Context, account string) (*docs.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Docs != nil {
		return runtime.Services.Docs(ctx, account)
	}
	return googleapi.NewDocs(ctx, account)
}

func docsHTTPClient(ctx context.Context, account string) (*http.Client, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.DocsHTTP != nil {
		return runtime.Services.DocsHTTP(ctx, account)
	}
	return googleapi.NewHTTPClient(ctx, googleauth.ServiceDocs, account)
}

func formsService(ctx context.Context, account string) (*formsapi.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Forms != nil {
		return runtime.Services.Forms(ctx, account)
	}
	return googleapi.NewForms(ctx, account)
}

func searchConsoleService(ctx context.Context, account string) (*searchconsoleapi.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.SearchConsole != nil {
		return runtime.Services.SearchConsole(ctx, account)
	}
	return googleapi.NewSearchConsole(ctx, account)
}

func gmailService(ctx context.Context, account string) (*gmail.Service, error) {
	return gmailServiceFactory(ctx)(ctx, account)
}

func gmailServiceFactory(ctx context.Context) app.GmailServiceFactory {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Gmail != nil {
		return runtime.Services.Gmail
	}
	return googleapi.NewGmail
}

func gmailBatchDeleteService(ctx context.Context, account string) (*gmail.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.GmailDelete != nil {
		return runtime.Services.GmailDelete(ctx, account)
	}
	return googleapi.NewGmailBatchDelete(ctx, account)
}

func peopleContactsService(ctx context.Context, account string) (*people.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.PeopleContacts != nil {
		return runtime.Services.PeopleContacts(ctx, account)
	}
	return googleapi.NewPeopleContacts(ctx, account)
}

func peopleDirectoryService(ctx context.Context, account string) (*people.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.PeopleDirectory != nil {
		return runtime.Services.PeopleDirectory(ctx, account)
	}
	return googleapi.NewPeopleDirectory(ctx, account)
}

func peopleOtherContactsService(ctx context.Context, account string) (*people.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.PeopleOther != nil {
		return runtime.Services.PeopleOther(ctx, account)
	}
	return googleapi.NewPeopleOtherContacts(ctx, account)
}

func sheetsService(ctx context.Context, account string) (*sheets.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Sheets != nil {
		return runtime.Services.Sheets(ctx, account)
	}
	return googleapi.NewSheets(ctx, account)
}

func sitesDriveService(ctx context.Context, account string) (*drive.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.SitesDrive != nil {
		return runtime.Services.SitesDrive(ctx, account)
	}
	return googleapi.NewSitesDrive(ctx, account)
}

func tasksService(ctx context.Context, account string) (*tasks.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Tasks != nil {
		return runtime.Services.Tasks(ctx, account)
	}
	return googleapi.NewTasks(ctx, account)
}

func slidesService(ctx context.Context, account string) (*slides.Service, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Slides != nil {
		return runtime.Services.Slides(ctx, account)
	}
	return googleapi.NewSlides(ctx, account)
}

func zoomMeetingClient(ctx context.Context, alias string) (app.ZoomMeetingClient, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.Zoom != nil {
		return runtime.Services.Zoom(ctx, alias)
	}
	return newZoomMeetingClient(ctx, alias)
}

func driveDownloadRequest(ctx context.Context, svc *drive.Service, fileID string) (*http.Response, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.DriveDownload != nil {
		return runtime.Services.DriveDownload(ctx, svc, fileID)
	}
	return driveDownload(ctx, svc, fileID)
}

func driveExportRequest(ctx context.Context, svc *drive.Service, fileID, mimeType string) (*http.Response, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Services.DriveExport != nil {
		return runtime.Services.DriveExport(ctx, svc, fileID, mimeType)
	}
	return driveExportDownload(ctx, svc, fileID, mimeType)
}
