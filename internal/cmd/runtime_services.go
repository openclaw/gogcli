package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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
	"github.com/steipete/gogcli/internal/googleapi"
)

var errRuntimeServiceRequired = errors.New("runtime service is required")

func composeRuntimeGoogleServices(runtime *app.Runtime, factory googleapi.Factory) {
	if runtime == nil || !runtime.ServicesManaged {
		return
	}

	services := &runtime.Services
	if services.AdminDirectory == nil {
		services.AdminDirectory = factory.AdminDirectory
	}
	if services.AdminOrgUnit == nil {
		services.AdminOrgUnit = factory.AdminOrgUnit
	}
	if services.AppScript == nil {
		services.AppScript = factory.AppScript
	}
	if services.AnalyticsAdmin == nil {
		services.AnalyticsAdmin = factory.AnalyticsAdmin
	}
	if services.AnalyticsData == nil {
		services.AnalyticsData = factory.AnalyticsData
	}
	if services.Calendar == nil {
		services.Calendar = factory.Calendar
	}
	if services.Chat == nil {
		services.Chat = factory.Chat
	}
	if services.Classroom == nil {
		services.Classroom = factory.Classroom
	}
	if services.CloudIdentity == nil {
		services.CloudIdentity = factory.CloudIdentity
	}
	if services.Docs == nil {
		services.Docs = factory.Docs
	}
	if services.DocsHTTP == nil {
		services.DocsHTTP = factory.DocsHTTP
	}
	if services.Drive == nil {
		services.Drive = factory.Drive
	}
	if services.DriveActivity == nil {
		services.DriveActivity = factory.DriveActivity
	}
	if services.DriveLabels == nil {
		services.DriveLabels = factory.DriveLabels
	}
	if services.Forms == nil {
		services.Forms = factory.Forms
	}
	if services.Gmail == nil {
		services.Gmail = factory.Gmail
	}
	if services.GmailDelete == nil {
		services.GmailDelete = factory.GmailDelete
	}
	if services.Keep == nil {
		services.Keep = factory.Keep
	}
	if services.Meet == nil {
		services.Meet = factory.Meet
	}
	if services.PeopleContacts == nil {
		services.PeopleContacts = factory.PeopleContacts
	}
	if services.PeopleDirectory == nil {
		services.PeopleDirectory = factory.PeopleDirectory
	}
	if services.PeopleOther == nil {
		services.PeopleOther = factory.PeopleOther
	}
	if services.Photos == nil {
		services.Photos = factory.Photos
	}
	if services.PhotosPicker == nil {
		services.PhotosPicker = factory.PhotosPicker
	}
	if services.SearchConsole == nil {
		services.SearchConsole = factory.SearchConsole
	}
	if services.Sheets == nil {
		services.Sheets = factory.Sheets
	}
	if services.SitesDrive == nil {
		services.SitesDrive = factory.SitesDrive
	}
	if services.Slides == nil {
		services.Slides = factory.Slides
	}
	if services.Tasks == nil {
		services.Tasks = factory.Tasks
	}
	if services.YouTubeAPIKey == nil {
		services.YouTubeAPIKey = factory.YouTubeAPIKey
	}
	if services.YouTubeAccount == nil {
		services.YouTubeAccount = factory.YouTubeAccount
	}
	if services.YouTubeComments == nil {
		services.YouTubeComments = factory.YouTubeComments
	}
	if services.YouTubeWrite == nil {
		services.YouTubeWrite = factory.YouTubeWrite
	}
}

func runtimeWithService(ctx context.Context, name string) (*app.Runtime, error) {
	runtime, ok := app.FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errRuntimeServiceRequired, name)
	}
	return runtime, nil
}

func adminDirectoryService(ctx context.Context, account string) (*admin.Service, error) {
	runtime, err := runtimeWithService(ctx, "admin directory")
	if err != nil || runtime.Services.AdminDirectory == nil {
		return nil, serviceError(err, "admin directory")
	}
	return runtime.Services.AdminDirectory(ctx, account)
}

func adminOrgUnitDirectoryService(ctx context.Context, account string) (*admin.Service, error) {
	runtime, err := runtimeWithService(ctx, "admin org unit")
	if err != nil || runtime.Services.AdminOrgUnit == nil {
		return nil, serviceError(err, "admin org unit")
	}
	return runtime.Services.AdminOrgUnit(ctx, account)
}

func appScriptService(ctx context.Context, account string) (*scriptapi.Service, error) {
	runtime, err := runtimeWithService(ctx, "app script")
	if err != nil || runtime.Services.AppScript == nil {
		return nil, serviceError(err, "app script")
	}
	return runtime.Services.AppScript(ctx, account)
}

func analyticsAdminService(ctx context.Context, account string) (*analyticsadmin.Service, error) {
	runtime, err := runtimeWithService(ctx, "analytics admin")
	if err != nil || runtime.Services.AnalyticsAdmin == nil {
		return nil, serviceError(err, "analytics admin")
	}
	return runtime.Services.AnalyticsAdmin(ctx, account)
}

func analyticsDataService(ctx context.Context, account string) (*analyticsdata.Service, error) {
	runtime, err := runtimeWithService(ctx, "analytics data")
	if err != nil || runtime.Services.AnalyticsData == nil {
		return nil, serviceError(err, "analytics data")
	}
	return runtime.Services.AnalyticsData(ctx, account)
}

func calendarService(ctx context.Context, account string) (*calendar.Service, error) {
	runtime, err := runtimeWithService(ctx, "calendar")
	if err != nil || runtime.Services.Calendar == nil {
		return nil, serviceError(err, "calendar")
	}
	return runtime.Services.Calendar(ctx, account)
}

func chatService(ctx context.Context, account string) (*chat.Service, error) {
	runtime, err := runtimeWithService(ctx, "chat")
	if err != nil || runtime.Services.Chat == nil {
		return nil, serviceError(err, "chat")
	}
	return runtime.Services.Chat(ctx, account)
}

func classroomService(ctx context.Context, account string) (*classroom.Service, error) {
	runtime, err := runtimeWithService(ctx, "classroom")
	if err != nil || runtime.Services.Classroom == nil {
		return nil, serviceError(err, "classroom")
	}
	return runtime.Services.Classroom(ctx, account)
}

func cloudIdentityService(ctx context.Context, account string) (*cloudidentity.Service, error) {
	runtime, err := runtimeWithService(ctx, "cloud identity")
	if err != nil || runtime.Services.CloudIdentity == nil {
		return nil, serviceError(err, "cloud identity")
	}
	return runtime.Services.CloudIdentity(ctx, account)
}

func keepServiceWithServiceAccount(ctx context.Context, path, impersonate string) (*keepapi.Service, error) {
	runtime, err := runtimeWithService(ctx, "keep")
	if err != nil || runtime.Services.Keep == nil {
		return nil, serviceError(err, "keep")
	}
	return runtime.Services.Keep(ctx, path, impersonate)
}

func meetService(ctx context.Context, account string) (*meetapi.Service, error) {
	runtime, err := runtimeWithService(ctx, "meet")
	if err != nil || runtime.Services.Meet == nil {
		return nil, serviceError(err, "meet")
	}
	return runtime.Services.Meet(ctx, account)
}

func photosService(ctx context.Context, account string) (*googleapi.PhotosClient, error) {
	runtime, err := runtimeWithService(ctx, "photos")
	if err != nil || runtime.Services.Photos == nil {
		return nil, serviceError(err, "photos")
	}
	return runtime.Services.Photos(ctx, account)
}

func photosPickerService(ctx context.Context, account string) (*googleapi.PhotosPickerClient, error) {
	runtime, err := runtimeWithService(ctx, "photos picker")
	if err != nil || runtime.Services.PhotosPicker == nil {
		return nil, serviceError(err, "photos picker")
	}
	return runtime.Services.PhotosPicker(ctx, account)
}

func openURL(ctx context.Context, uri string) error {
	runtime, err := runtimeWithService(ctx, "open URL")
	if err != nil || runtime.Services.OpenURL == nil {
		return serviceError(err, "open URL")
	}
	return runtime.Services.OpenURL(ctx, uri)
}

func driveService(ctx context.Context, account string) (*drive.Service, error) {
	runtime, err := runtimeWithService(ctx, "drive")
	if err != nil || runtime.Services.Drive == nil {
		return nil, serviceError(err, "drive")
	}
	return runtime.Services.Drive(ctx, account)
}

func driveActivityService(ctx context.Context, account string) (*driveactivityapi.Service, error) {
	runtime, err := runtimeWithService(ctx, "drive activity")
	if err != nil || runtime.Services.DriveActivity == nil {
		return nil, serviceError(err, "drive activity")
	}
	return runtime.Services.DriveActivity(ctx, account)
}

func driveLabelsService(ctx context.Context, account string) (*drivelabelsapi.Service, error) {
	runtime, err := runtimeWithService(ctx, "drive labels")
	if err != nil || runtime.Services.DriveLabels == nil {
		return nil, serviceError(err, "drive labels")
	}
	return runtime.Services.DriveLabels(ctx, account)
}

func docsService(ctx context.Context, account string) (*docs.Service, error) {
	runtime, err := runtimeWithService(ctx, "docs")
	if err != nil || runtime.Services.Docs == nil {
		return nil, serviceError(err, "docs")
	}
	return runtime.Services.Docs(ctx, account)
}

func docsHTTPClient(ctx context.Context, account string) (*http.Client, error) {
	runtime, err := runtimeWithService(ctx, "docs HTTP")
	if err != nil || runtime.Services.DocsHTTP == nil {
		return nil, serviceError(err, "docs HTTP")
	}
	return runtime.Services.DocsHTTP(ctx, account)
}

func formsService(ctx context.Context, account string) (*formsapi.Service, error) {
	runtime, err := runtimeWithService(ctx, "forms")
	if err != nil || runtime.Services.Forms == nil {
		return nil, serviceError(err, "forms")
	}
	return runtime.Services.Forms(ctx, account)
}

func searchConsoleService(ctx context.Context, account string) (*searchconsoleapi.Service, error) {
	runtime, err := runtimeWithService(ctx, "search console")
	if err != nil || runtime.Services.SearchConsole == nil {
		return nil, serviceError(err, "search console")
	}
	return runtime.Services.SearchConsole(ctx, account)
}

func gmailService(ctx context.Context, account string) (*gmail.Service, error) {
	factory, err := gmailServiceFactory(ctx)
	if err != nil {
		return nil, err
	}
	return factory(ctx, account)
}

func gmailServiceFactory(ctx context.Context) (app.GmailServiceFactory, error) {
	runtime, err := runtimeWithService(ctx, "gmail")
	if err != nil || runtime.Services.Gmail == nil {
		return nil, serviceError(err, "gmail")
	}
	return runtime.Services.Gmail, nil
}

func gmailBatchDeleteService(ctx context.Context, account string) (*gmail.Service, error) {
	runtime, err := runtimeWithService(ctx, "gmail batch delete")
	if err != nil || runtime.Services.GmailDelete == nil {
		return nil, serviceError(err, "gmail batch delete")
	}
	return runtime.Services.GmailDelete(ctx, account)
}

func peopleContactsService(ctx context.Context, account string) (*people.Service, error) {
	runtime, err := runtimeWithService(ctx, "people contacts")
	if err != nil || runtime.Services.PeopleContacts == nil {
		return nil, serviceError(err, "people contacts")
	}
	return runtime.Services.PeopleContacts(ctx, account)
}

func peopleDirectoryService(ctx context.Context, account string) (*people.Service, error) {
	runtime, err := runtimeWithService(ctx, "people directory")
	if err != nil || runtime.Services.PeopleDirectory == nil {
		return nil, serviceError(err, "people directory")
	}
	return runtime.Services.PeopleDirectory(ctx, account)
}

func peopleOtherContactsService(ctx context.Context, account string) (*people.Service, error) {
	runtime, err := runtimeWithService(ctx, "people other contacts")
	if err != nil || runtime.Services.PeopleOther == nil {
		return nil, serviceError(err, "people other contacts")
	}
	return runtime.Services.PeopleOther(ctx, account)
}

func sheetsService(ctx context.Context, account string) (*sheets.Service, error) {
	runtime, err := runtimeWithService(ctx, "sheets")
	if err != nil || runtime.Services.Sheets == nil {
		return nil, serviceError(err, "sheets")
	}
	return runtime.Services.Sheets(ctx, account)
}

func sitesDriveService(ctx context.Context, account string) (*drive.Service, error) {
	runtime, err := runtimeWithService(ctx, "sites drive")
	if err != nil || runtime.Services.SitesDrive == nil {
		return nil, serviceError(err, "sites drive")
	}
	return runtime.Services.SitesDrive(ctx, account)
}

func tasksService(ctx context.Context, account string) (*tasks.Service, error) {
	runtime, err := runtimeWithService(ctx, "tasks")
	if err != nil || runtime.Services.Tasks == nil {
		return nil, serviceError(err, "tasks")
	}
	return runtime.Services.Tasks(ctx, account)
}

func slidesService(ctx context.Context, account string) (*slides.Service, error) {
	runtime, err := runtimeWithService(ctx, "slides")
	if err != nil || runtime.Services.Slides == nil {
		return nil, serviceError(err, "slides")
	}
	return runtime.Services.Slides(ctx, account)
}

func zoomMeetingClient(ctx context.Context, alias string) (app.ZoomMeetingClient, error) {
	if googleapi.ReadOnly(ctx) {
		return nil, fmt.Errorf("%w: Zoom meeting mutations are disabled", googleapi.ErrReadOnly)
	}

	runtime, err := runtimeWithService(ctx, "zoom")
	if err != nil || runtime.Services.Zoom == nil {
		return nil, serviceError(err, "zoom")
	}
	return runtime.Services.Zoom(ctx, alias)
}

func driveDownloadRequest(ctx context.Context, svc *drive.Service, fileID string) (*http.Response, error) {
	runtime, err := runtimeWithService(ctx, "drive download")
	if err != nil || runtime.Services.DriveDownload == nil {
		return nil, serviceError(err, "drive download")
	}
	return runtime.Services.DriveDownload(ctx, svc, fileID)
}

func driveExportRequest(ctx context.Context, svc *drive.Service, fileID, mimeType string) (*http.Response, error) {
	runtime, err := runtimeWithService(ctx, "drive export")
	if err != nil || runtime.Services.DriveExport == nil {
		return nil, serviceError(err, "drive export")
	}
	return runtime.Services.DriveExport(ctx, svc, fileID, mimeType)
}

func serviceError(err error, name string) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: %s", errRuntimeServiceRequired, name)
}
