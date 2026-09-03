package app

import (
	"context"
	"io"
	"net/http"
	"time"

	admin "google.golang.org/api/admin/directory/v1"
	adsense "google.golang.org/api/adsense/v2"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/chat/v1"
	"google.golang.org/api/classroom/v1"
	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/docs/v1"
	drivev2 "google.golang.org/api/drive/v2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/drivelabels/v2"
	"google.golang.org/api/forms/v1"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/keep/v1"
	"google.golang.org/api/meet/v2"
	"google.golang.org/api/people/v1"
	"google.golang.org/api/script/v1"
	"google.golang.org/api/searchconsole/v1"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"
	"google.golang.org/api/tasks/v1"
	"google.golang.org/api/youtube/v3"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/googleauth"
	"github.com/openclaw/gogcli/internal/secrets"
	"github.com/openclaw/gogcli/internal/zoom"
)

type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

type (
	AdminDirectoryServiceFactory func(context.Context, string) (*admin.Service, error)
	AdSenseServiceFactory        func(context.Context, string) (*adsense.Service, error)
	AppScriptServiceFactory      func(context.Context, string) (*script.Service, error)
	AnalyticsAdminServiceFactory func(context.Context, string) (*analyticsadmin.Service, error)
	AnalyticsDataServiceFactory  func(context.Context, string) (*analyticsdata.Service, error)
	CalendarServiceFactory       func(context.Context, string) (*calendar.Service, error)
	ChatServiceFactory           func(context.Context, string) (*chat.Service, error)
	ChatSearchServiceFactory     func(context.Context, string) (*googleapi.ChatSearchClient, error)
	ClassroomServiceFactory      func(context.Context, string) (*classroom.Service, error)
	CloudIdentityServiceFactory  func(context.Context, string) (*cloudidentity.Service, error)
	DocsServiceFactory           func(context.Context, string) (*docs.Service, error)
	DocsHTTPClientFactory        func(context.Context, string) (*http.Client, error)
	DriveServiceFactory          func(context.Context, string) (*drive.Service, error)
	DriveV2ServiceFactory        func(context.Context, string) (*drivev2.Service, error)
	DriveActivityServiceFactory  func(context.Context, string) (*driveactivity.Service, error)
	DriveLabelsServiceFactory    func(context.Context, string) (*drivelabels.Service, error)
	FormsServiceFactory          func(context.Context, string) (*forms.Service, error)
	GmailServiceFactory          func(context.Context, string) (*gmail.Service, error)
	KeepServiceAccountFactory    func(context.Context, string, string) (*keep.Service, error)
	MeetServiceFactory           func(context.Context, string) (*meet.Service, error)
	PeopleServiceFactory         func(context.Context, string) (*people.Service, error)
	PhotosServiceFactory         func(context.Context, string) (*googleapi.PhotosClient, error)
	PhotosPickerServiceFactory   func(context.Context, string) (*googleapi.PhotosPickerClient, error)
	SearchConsoleServiceFactory  func(context.Context, string) (*searchconsole.Service, error)
	SheetsServiceFactory         func(context.Context, string) (*sheets.Service, error)
	SlidesServiceFactory         func(context.Context, string) (*slides.Service, error)
	TasksServiceFactory          func(context.Context, string) (*tasks.Service, error)
	YouTubeServiceFactory        func(context.Context, string) (*youtube.Service, error)
	ZoomMeetingClientFactory     func(context.Context, string) (ZoomMeetingClient, error)
	DriveDownloadFunc            func(context.Context, *drive.Service, string) (*http.Response, error)
	DriveExportFunc              func(context.Context, *drive.Service, string, string) (*http.Response, error)
	OpenURLFunc                  func(context.Context, string) error
	OpenSecretsStoreFunc         func() (secrets.Store, error)
	OpenSecretStoreFunc          func() (secrets.SecretStore, error)
	AuthorizeGoogleFunc          func(context.Context, googleauth.AuthorizeOptions) (string, error)
	StartManageServerFunc        func(context.Context, googleauth.ManageServerOptions) error
	CheckRefreshTokenFunc        func(context.Context, string, string, []string, time.Duration) error
	EnsureKeychainAccessFunc     func(context.Context) error
	FetchAuthorizedIdentityFunc  func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error)
	ManualAuthURLFunc            func(context.Context, googleauth.AuthorizeOptions) (googleauth.ManualAuthURLResult, error)
)

type ZoomMeetingClient interface {
	CreateMeeting(context.Context, string, zoom.CreateMeetingRequest) (*zoom.Meeting, error)
	DeleteMeeting(context.Context, string) error
}

type Services struct {
	AdminDirectory  AdminDirectoryServiceFactory
	AdminOrgUnit    AdminDirectoryServiceFactory
	AdSense         AdSenseServiceFactory
	AppScript       AppScriptServiceFactory
	AnalyticsAdmin  AnalyticsAdminServiceFactory
	AnalyticsData   AnalyticsDataServiceFactory
	Calendar        CalendarServiceFactory
	Chat            ChatServiceFactory
	ChatSearch      ChatSearchServiceFactory
	Classroom       ClassroomServiceFactory
	CloudIdentity   CloudIdentityServiceFactory
	Docs            DocsServiceFactory
	DocsHTTP        DocsHTTPClientFactory
	Drive           DriveServiceFactory
	DriveV2         DriveV2ServiceFactory
	DriveActivity   DriveActivityServiceFactory
	DriveLabels     DriveLabelsServiceFactory
	Forms           FormsServiceFactory
	Gmail           GmailServiceFactory
	GmailDelete     GmailServiceFactory
	Keep            KeepServiceAccountFactory
	Meet            MeetServiceFactory
	PeopleContacts  PeopleServiceFactory
	PeopleDirectory PeopleServiceFactory
	PeopleOther     PeopleServiceFactory
	Photos          PhotosServiceFactory
	PhotosPicker    PhotosPickerServiceFactory
	SearchConsole   SearchConsoleServiceFactory
	Sheets          SheetsServiceFactory
	ConnectedSheets SheetsServiceFactory
	SitesDrive      DriveServiceFactory
	Slides          SlidesServiceFactory
	Tasks           TasksServiceFactory
	YouTubeAPIKey   YouTubeServiceFactory
	YouTubeAccount  YouTubeServiceFactory
	YouTubeComments YouTubeServiceFactory
	YouTubeWrite    YouTubeServiceFactory
	Zoom            ZoomMeetingClientFactory
	DriveDownload   DriveDownloadFunc
	DriveExport     DriveExportFunc
	OpenURL         OpenURLFunc

	ConnectedSheetsWriter SheetsServiceFactory
}

type AuthOperations struct {
	OpenSecretsStore        OpenSecretsStoreFunc
	OpenSecretStore         OpenSecretStoreFunc
	AuthorizeGoogle         AuthorizeGoogleFunc
	StartManageServer       StartManageServerFunc
	CheckRefreshToken       CheckRefreshTokenFunc
	EnsureKeychainAccess    EnsureKeychainAccessFunc
	FetchAuthorizedIdentity FetchAuthorizedIdentityFunc
	ManualAuthURL           ManualAuthURLFunc
}

type Runtime struct {
	IO              IO
	Services        Services
	Auth            AuthOperations
	LayoutResolver  *config.Resolver
	Layout          config.Layout
	Config          *config.ConfigStore
	ServiceAccounts *config.ServiceAccountStore
	KeyringOptions  *secrets.OpenOptions
	ConfigManaged   bool
	ServicesManaged bool
}

type runtimeContextKey struct{}

func WithRuntime(ctx context.Context, runtime *Runtime) context.Context {
	return context.WithValue(ctx, runtimeContextKey{}, runtime)
}

func FromContext(ctx context.Context) (*Runtime, bool) {
	runtime, ok := ctx.Value(runtimeContextKey{}).(*Runtime)
	return runtime, ok && runtime != nil
}

func IOFromContext(ctx context.Context) (IO, bool) {
	runtime, ok := FromContext(ctx)
	if !ok {
		return IO{}, false
	}

	return runtime.IO, true
}
