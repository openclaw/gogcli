package googleapi

import (
	"context"

	"google.golang.org/api/people/v1"
)

const (
	scopeContactsWrite   = "https://www.googleapis.com/auth/contacts"
	scopeContactsOtherRO = "https://www.googleapis.com/auth/contacts.other.readonly"
	scopeDirectoryRO     = "https://www.googleapis.com/auth/directory.readonly"
)

func NewPeopleContacts(ctx context.Context, email string) (*people.Service, error) {
	return newGoogleServiceForScopes(ctx, email, "contacts", "contacts", []string{scopeContactsWrite}, people.NewService)
}

func NewPeopleOtherContacts(ctx context.Context, email string) (*people.Service, error) {
	return newGoogleServiceForScopes(ctx, email, "contacts", "contacts", []string{scopeContactsOtherRO}, people.NewService)
}

func NewPeopleDirectory(ctx context.Context, email string) (*people.Service, error) {
	return newGoogleServiceForScopes(ctx, email, "contacts", "contacts", []string{scopeDirectoryRO}, people.NewService)
}
