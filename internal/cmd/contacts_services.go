package cmd

import (
	"context"

	"google.golang.org/api/people/v1"

	"github.com/degree-analytics/ratatosk/internal/googleapi"
)

var (
	newPeopleContactsService      func(ctx context.Context, email string) (*people.Service, error) = googleapi.NewPeopleContacts
	newPeopleOtherContactsService func(ctx context.Context, email string) (*people.Service, error) = googleapi.NewPeopleOtherContacts
	newPeopleDirectoryService     func(ctx context.Context, email string) (*people.Service, error) = googleapi.NewPeopleDirectory
)
