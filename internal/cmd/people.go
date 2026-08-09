package cmd

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/99designs/keyring"
	gapi "google.golang.org/api/googleapi"
	"google.golang.org/api/people/v1"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleauth"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type PeopleCmd struct {
	Me        PeopleMeCmd        `cmd:"" name:"me" help:"Show your profile (people/me)"`
	Get       PeopleGetCmd       `cmd:"" name:"get" aliases:"info,show" help:"Get a user profile by ID"`
	Search    PeopleSearchCmd    `cmd:"" name:"search" aliases:"find,query" help:"Search the Workspace directory"`
	Relations PeopleRelationsCmd `cmd:"" name:"relations" help:"Get user relations"`
	Raw       PeopleRawCmd       `cmd:"" name:"raw" help:"Dump raw People API response as JSON (People.Get; lossless; for scripting and LLM consumption)"`
}

type PeopleMeCmd struct{}

var fallbackPeopleMeProfile = fetchPeopleMeProfileFromToken

func (c *PeopleMeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := peopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	person, err := svc.People.Get(peopleMeResource).
		PersonFields("names,emailAddresses,photos").
		Do()
	if err != nil {
		if !isPeopleAccessNotConfigured(err) {
			return err
		}
		person, err = fallbackPeopleMeProfile(ctx, account)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"person": person})
	}

	name := ""
	email := ""
	photo := ""
	if len(person.Names) > 0 && person.Names[0] != nil {
		name = person.Names[0].DisplayName
	}
	if len(person.EmailAddresses) > 0 && person.EmailAddresses[0] != nil {
		email = person.EmailAddresses[0].Value
	}
	if len(person.Photos) > 0 && person.Photos[0] != nil {
		photo = person.Photos[0].Url
	}

	if name != "" {
		u.Out().Linef("name\t%s", name)
	}
	if email != "" {
		u.Out().Linef("email\t%s", email)
	}
	if photo != "" {
		u.Out().Linef("photo\t%s", photo)
	}
	return nil
}

func isPeopleAccessNotConfigured(err error) bool {
	var apiErr *gapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == 403 {
		for _, item := range apiErr.Errors {
			if item.Reason == "accessNotConfigured" {
				return true
			}
		}
	}
	text := err.Error()
	return strings.Contains(text, "accessNotConfigured") ||
		strings.Contains(text, "People API has not been used")
}

func fetchPeopleMeProfileFromToken(ctx context.Context, account string) (*people.Person, error) {
	client, err := authclient.ResolveClient(ctx, account)
	if err != nil {
		return nil, err
	}
	store, err := openAuthSecretsStore(ctx)
	if err != nil {
		return nil, err
	}
	tok, err := store.GetToken(client, account)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return nil, err
		}
		return nil, err
	}
	identity, err := googleauth.IdentityForRefreshToken(ctx, client, tok.RefreshToken, googleauth.IdentityScopes(), 15*time.Second)
	if err != nil {
		return nil, err
	}
	person := &people.Person{
		ResourceName: peopleMeResource,
	}
	if strings.TrimSpace(identity.Email) != "" {
		person.EmailAddresses = []*people.EmailAddress{{Value: strings.TrimSpace(identity.Email)}}
	}
	return person, nil
}
