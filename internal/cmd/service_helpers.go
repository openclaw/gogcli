package cmd

import (
	"context"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/classroom/v1"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/sheets/v4"
)

func requireDocsService(ctx context.Context, flags *RootFlags) (*docs.Service, error) {
	_, svc, err := requireGoogleService(ctx, flags, newDocsService)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

func requireDriveService(ctx context.Context, flags *RootFlags) (string, *drive.Service, error) {
	return requireGoogleService(ctx, flags, newDriveService)
}

func requireCalendarService(ctx context.Context, flags *RootFlags) (string, *calendar.Service, error) {
	return requireGoogleService(ctx, flags, newCalendarService)
}

func requireGmailService(ctx context.Context, flags *RootFlags) (string, *gmail.Service, error) {
	return requireGoogleService(ctx, flags, newGmailService)
}

func requireClassroomService(ctx context.Context, flags *RootFlags) (string, *classroom.Service, error) {
	return requireGoogleService(ctx, flags, newClassroomService)
}

func requireSheetsService(ctx context.Context, flags *RootFlags) (string, *sheets.Service, error) {
	return requireGoogleService(ctx, flags, newSheetsService)
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
