package cmd

import (
	"context"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type PeopleCmd struct {
	Me        PeopleMeCmd        `cmd:"" name:"me" help:"Show your profile (people/me)"`
	Get       PeopleGetCmd       `cmd:"" name:"get" help:"Get a user profile by ID"`
	Search    PeopleSearchCmd    `cmd:"" name:"search" help:"Search the Workspace directory"`
	Relations PeopleRelationsCmd `cmd:"" name:"relations" help:"Get user relations"`
}

type PeopleMeCmd struct{}

func (c *PeopleMeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	person, err := svc.People.Get("people/me").
		PersonFields("names,emailAddresses,photos").
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"person": person})
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
		u.Out().Printf("name\t%s", name)
	}
	if email != "" {
		u.Out().Printf("email\t%s", email)
	}
	if photo != "" {
		u.Out().Printf("photo\t%s", photo)
	}
	return nil
}
