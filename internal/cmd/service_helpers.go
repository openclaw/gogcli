package cmd

import (
	"context"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/classroom/v1"
	"google.golang.org/api/docs/v1"
	drivev2 "google.golang.org/api/drive/v2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/meet/v2"
	scriptapi "google.golang.org/api/script/v1"
	"google.golang.org/api/sheets/v4"
)

func requireDocsService(ctx context.Context, flags *RootFlags) (*docs.Service, error) {
	_, svc, err := requireGoogleService(ctx, flags, docsService)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

func requireAppScriptService(ctx context.Context, flags *RootFlags) (*scriptapi.Service, error) {
	_, svc, err := requireGoogleService(ctx, flags, appScriptService)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

func requireDriveService(ctx context.Context, flags *RootFlags) (string, *drive.Service, error) {
	return requireGoogleService(ctx, flags, driveService)
}

func requireDriveV2Service(ctx context.Context, flags *RootFlags) (string, *drivev2.Service, error) {
	return requireGoogleService(ctx, flags, driveV2Service)
}

func requireDriveActivityService(ctx context.Context, flags *RootFlags) (string, *driveactivity.Service, error) {
	return requireGoogleService(ctx, flags, driveActivityService)
}

func requireCalendarService(ctx context.Context, flags *RootFlags) (string, *calendar.Service, error) {
	return requireGoogleService(ctx, flags, calendarService)
}

func requireGmailService(ctx context.Context, flags *RootFlags) (string, *gmail.Service, error) {
	return requireGoogleService(ctx, flags, gmailService)
}

func requireClassroomService(ctx context.Context, flags *RootFlags) (string, *classroom.Service, error) {
	return requireGoogleService(ctx, flags, classroomService)
}

func requireMeetService(ctx context.Context, flags *RootFlags) (string, *meet.Service, error) {
	return requireGoogleService(ctx, flags, meetService)
}

func requireSheetsService(ctx context.Context, flags *RootFlags) (string, *sheets.Service, error) {
	return requireGoogleService(ctx, flags, sheetsService)
}

func requireConnectedSheetsService(ctx context.Context, flags *RootFlags) (string, *sheets.Service, error) {
	return requireGoogleService(ctx, flags, connectedSheetsService)
}

func requireConnectedSheetsWriterService(ctx context.Context, flags *RootFlags) (string, *sheets.Service, error) {
	return requireGoogleService(ctx, flags, connectedSheetsWriterService)
}

func requireGoogleService[T any](ctx context.Context, flags *RootFlags, newService func(context.Context, string) (*T, error)) (string, *T, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return "", nil, err
	}
	svc, err := newService(ctx, account)
	if err != nil {
		return "", nil, err
	}
	return account, svc, nil
}
