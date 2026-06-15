package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	peopleProfileReadMask   = "names,emailAddresses,photos"
	peopleRelationsReadMask = "relations"
)

type PeopleGetCmd struct {
	UserID string `arg:"" name:"userId" help:"User ID (people/...)"`
}

func (c *PeopleGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	resource := normalizePeopleResource(c.UserID)
	if resource == "" {
		return usage("required: userId")
	}

	svc, err := peopleServiceForResource(ctx, account, resource)
	if err != nil {
		return wrapPeopleAPIError(err)
	}

	person, err := svc.People.Get(resource).PersonFields(peopleProfileReadMask).Do()
	if err != nil {
		return wrapPeopleAPIError(err)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"person": person})
	}

	name := primaryName(person)
	email := primaryEmail(person)
	photo := ""
	if len(person.Photos) > 0 && person.Photos[0] != nil {
		photo = person.Photos[0].Url
	}

	u.Out().Linef("resource\t%s", person.ResourceName)
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

type PeopleSearchCmd struct {
	Query     []string `arg:"" name:"query" help:"Search query"`
	Max       int64    `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page      string   `name:"page" aliases:"cursor" help:"Page token"`
	All       bool     `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *PeopleSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	query := strings.TrimSpace(strings.Join(c.Query, " "))
	if query == "" {
		return usage("required: query")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := peopleDirectoryService(ctx, account)
	if err != nil {
		return wrapPeopleAPIError(err)
	}

	fetch := func(pageToken string) ([]*people.Person, string, error) {
		ctxTimeout, cancel := context.WithTimeout(ctx, directoryRequestTimeout)
		defer cancel()

		call := svc.People.SearchDirectoryPeople().
			Query(query).
			Sources("DIRECTORY_SOURCE_TYPE_DOMAIN_CONTACT", "DIRECTORY_SOURCE_TYPE_DOMAIN_PROFILE").
			ReadMask(directoryReadMask).
			PageSize(c.Max).
			Context(ctxTimeout)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", wrapPeopleAPIError(callErr)
		}
		return resp.People, resp.NextPageToken, nil
	}

	peopleList, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource string `json:"resource"`
			Name     string `json:"name,omitempty"`
			Email    string `json:"email,omitempty"`
		}
		items := make([]item, 0, len(peopleList))
		for _, p := range peopleList {
			if p == nil {
				continue
			}
			items = append(items, item{
				Resource: p.ResourceName,
				Name:     primaryName(p),
				Email:    primaryEmail(p),
			})
		}
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"people":        items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(peopleList) == 0 {
		u.Err().Println("No results")
		return failEmptyExit(c.FailEmpty)
	}

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactPeopleRows(peopleList),
		directoryPersonColumns(),
	); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type PeopleRelationsCmd struct {
	UserID string `arg:"" optional:"" name:"userId" help:"User ID (people/...)"`
	Type   string `name:"type" help:"Filter relation type"`
}

func (c *PeopleRelationsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	resource := normalizePeopleResource(c.UserID)
	if resource == "" {
		resource = peopleMeResource
	}

	svc, err := peopleServiceForResource(ctx, account, resource)
	if err != nil {
		return wrapPeopleAPIError(err)
	}

	person, err := svc.People.Get(resource).PersonFields(peopleRelationsReadMask).Do()
	if err != nil {
		return wrapPeopleAPIError(err)
	}

	relationType := strings.TrimSpace(c.Type)
	relations := make([]*people.Relation, 0, len(person.Relations))
	for _, rel := range person.Relations {
		if rel != nil {
			relations = append(relations, rel)
		}
	}
	if relationType != "" {
		filtered := relations[:0]
		for _, rel := range relations {
			if strings.EqualFold(rel.Type, relationType) {
				filtered = append(filtered, rel)
			}
		}
		relations = filtered
	}

	resourceName := person.ResourceName
	if resourceName == "" {
		resourceName = resource
	}

	if outfmt.IsJSON(ctx) {
		resp := map[string]any{
			"resource":  resourceName,
			"relations": relations,
		}
		if relationType != "" {
			resp["relationType"] = relationType
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), resp)
	}

	if len(relations) == 0 {
		u.Err().Println("No relations")
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), relations, peopleRelationColumns())
}

func peopleServiceForResource(ctx context.Context, account string, resource string) (*people.Service, error) {
	if resource == peopleMeResource {
		return peopleContactsService(ctx, account)
	}
	return peopleDirectoryService(ctx, account)
}
