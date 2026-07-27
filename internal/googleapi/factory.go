package googleapi

import (
	"context"
	"net/http"

	admin "google.golang.org/api/admin/directory/v1"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/chat/v1"
	"google.golang.org/api/classroom/v1"
	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/docs/v1"
	drivev2 "google.golang.org/api/drive/v2"
	"google.golang.org/api/drive/v3"
	driveactivity "google.golang.org/api/driveactivity/v2"
	drivelabels "google.golang.org/api/drivelabels/v2"
	"google.golang.org/api/forms/v1"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/keep/v1"
	"google.golang.org/api/meet/v2"
	"google.golang.org/api/people/v1"
	"google.golang.org/api/script/v1"
	searchconsole "google.golang.org/api/searchconsole/v1"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"
	"google.golang.org/api/tasks/v1"
	"google.golang.org/api/youtube/v3"

	"github.com/steipete/gogcli/internal/googleauth"
)

type FactoryOptions struct {
	PhotosBaseURL       string
	PhotosPickerBaseURL string
}

type Factory struct {
	auth                AuthDependencies
	photosBaseURL       string
	photosPickerBaseURL string
}

func NewFactory(auth AuthDependencies, options FactoryOptions) Factory {
	return Factory{
		auth:                auth,
		photosBaseURL:       options.PhotosBaseURL,
		photosPickerBaseURL: options.PhotosPickerBaseURL,
	}
}

func (f Factory) withAuth(ctx context.Context) context.Context {
	return WithAuthDependencies(ctx, f.auth)
}

func (f Factory) AdminDirectory(ctx context.Context, account string) (*admin.Service, error) {
	return NewAdminDirectory(f.withAuth(ctx), account)
}

func (f Factory) AdminOrgUnit(ctx context.Context, account string) (*admin.Service, error) {
	return NewAdminDirectoryOrgUnit(f.withAuth(ctx), account)
}

func (f Factory) AppScript(ctx context.Context, account string) (*script.Service, error) {
	return NewAppScript(f.withAuth(ctx), account)
}

func (f Factory) AnalyticsAdmin(ctx context.Context, account string) (*analyticsadmin.Service, error) {
	return NewAnalyticsAdmin(f.withAuth(ctx), account)
}

func (f Factory) AnalyticsData(ctx context.Context, account string) (*analyticsdata.Service, error) {
	return NewAnalyticsData(f.withAuth(ctx), account)
}

func (f Factory) Calendar(ctx context.Context, account string) (*calendar.Service, error) {
	return NewCalendar(f.withAuth(ctx), account)
}

func (f Factory) Chat(ctx context.Context, account string) (*chat.Service, error) {
	return NewChat(f.withAuth(ctx), account)
}

func (f Factory) Classroom(ctx context.Context, account string) (*classroom.Service, error) {
	return NewClassroom(f.withAuth(ctx), account)
}

func (f Factory) CloudIdentity(ctx context.Context, account string) (*cloudidentity.Service, error) {
	return NewCloudIdentityGroups(f.withAuth(ctx), account)
}

func (f Factory) Docs(ctx context.Context, account string) (*docs.Service, error) {
	return NewDocs(f.withAuth(ctx), account)
}

func (f Factory) DocsHTTP(ctx context.Context, account string) (*http.Client, error) {
	return NewHTTPClient(f.withAuth(ctx), googleauth.ServiceDocs, account)
}

func (f Factory) Drive(ctx context.Context, account string) (*drive.Service, error) {
	return NewDrive(f.withAuth(ctx), account)
}

func (f Factory) DriveV2(ctx context.Context, account string) (*drivev2.Service, error) {
	return NewDriveV2(f.withAuth(ctx), account)
}

func (f Factory) DriveActivity(ctx context.Context, account string) (*driveactivity.Service, error) {
	return NewDriveActivity(f.withAuth(ctx), account)
}

func (f Factory) DriveLabels(ctx context.Context, account string) (*drivelabels.Service, error) {
	return NewDriveLabels(f.withAuth(ctx), account)
}

func (f Factory) Forms(ctx context.Context, account string) (*forms.Service, error) {
	return NewForms(f.withAuth(ctx), account)
}

func (f Factory) Gmail(ctx context.Context, account string) (*gmail.Service, error) {
	return NewGmail(f.withAuth(ctx), account)
}

func (f Factory) GmailDelete(ctx context.Context, account string) (*gmail.Service, error) {
	return NewGmailBatchDelete(f.withAuth(ctx), account)
}

func (f Factory) Keep(ctx context.Context, path, impersonate string) (*keep.Service, error) {
	return newKeepWithServiceAccount(f.withAuth(ctx), path, impersonate, f.auth.ServiceAccountTokenSource)
}

func (f Factory) Meet(ctx context.Context, account string) (*meet.Service, error) {
	return NewMeet(f.withAuth(ctx), account)
}

func (f Factory) PeopleContacts(ctx context.Context, account string) (*people.Service, error) {
	return NewPeopleContacts(f.withAuth(ctx), account)
}

func (f Factory) PeopleDirectory(ctx context.Context, account string) (*people.Service, error) {
	return NewPeopleDirectory(f.withAuth(ctx), account)
}

func (f Factory) PeopleOther(ctx context.Context, account string) (*people.Service, error) {
	return NewPeopleOtherContacts(f.withAuth(ctx), account)
}

func (f Factory) Photos(ctx context.Context, account string) (*PhotosClient, error) {
	return NewPhotosClientForAccount(f.withAuth(ctx), account, WithPhotosBaseURL(f.photosBaseURL))
}

func (f Factory) PhotosPicker(ctx context.Context, account string) (*PhotosPickerClient, error) {
	return NewPhotosPickerClientForAccount(f.withAuth(ctx), account, WithPhotosPickerBaseURL(f.photosPickerBaseURL))
}

func (f Factory) SearchConsole(ctx context.Context, account string) (*searchconsole.Service, error) {
	return NewSearchConsole(f.withAuth(ctx), account)
}

func (f Factory) Sheets(ctx context.Context, account string) (*sheets.Service, error) {
	return NewSheets(f.withAuth(ctx), account)
}

func (f Factory) SitesDrive(ctx context.Context, account string) (*drive.Service, error) {
	return NewSitesDrive(f.withAuth(ctx), account)
}

func (f Factory) Slides(ctx context.Context, account string) (*slides.Service, error) {
	return NewSlides(f.withAuth(ctx), account)
}

func (f Factory) Tasks(ctx context.Context, account string) (*tasks.Service, error) {
	return NewTasks(f.withAuth(ctx), account)
}

func (f Factory) YouTubeAPIKey(ctx context.Context, apiKey string) (*youtube.Service, error) {
	return NewYouTubeWithAPIKey(ctx, apiKey)
}

func (f Factory) YouTubeAccount(ctx context.Context, account string) (*youtube.Service, error) {
	return NewYouTubeForAccount(f.withAuth(ctx), account)
}

func (f Factory) YouTubeComments(ctx context.Context, account string) (*youtube.Service, error) {
	return NewYouTubeCommentsForAccount(f.withAuth(ctx), account)
}

func (f Factory) YouTubeWrite(ctx context.Context, account string) (*youtube.Service, error) {
	return NewYouTubeWriteForAccount(f.withAuth(ctx), account)
}
