package cmd

import (
	"context"
	"fmt"
	"os"
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"person": person})
	}

	name := primaryName(person)
	email := primaryEmail(person)
	photo := ""
	if len(person.Photos) > 0 && person.Photos[0] != nil {
		photo = person.Photos[0].Url
	}

	u.Out().Printf("resource\t%s", person.ResourceName)
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

type PeopleSearchCmd struct {
	Query     []string `arg:"" name:"query" help:"Search query"`
	Max       int64    `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page      string   `name:"page" aliases:"cursor" help:"Page token"`
	All       bool     `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *PeopleSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(c.Query, " "))
	if query == "" {
		return usage("required: query")
	}

	svc, err := newPeopleDirectoryService(ctx, account)
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
		resp, err := call.Do()
		if err != nil {
			return nil, "", wrapPeopleAPIError(err)
		}
		return resp.People, resp.NextPageToken, nil
	}

	var peopleList []*people.Person
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		peopleList = all
	} else {
		var err error
		peopleList, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
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
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
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

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL")
	for _, p := range peopleList {
		if p == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
		)
	}
	printNextPageHint(u, nextPageToken)
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
	relations := person.Relations
	if relationType != "" {
		filtered := relations[:0]
		for _, rel := range relations {
			if rel == nil {
				continue
			}
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
		return outfmt.WriteJSON(ctx, os.Stdout, resp)
	}

	if len(relations) == 0 {
		u.Err().Println("No relations")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TYPE\tPERSON")
	for _, rel := range relations {
		if rel == nil {
			continue
		}
		typ := rel.Type
		if typ == "" {
			typ = rel.FormattedType
		}
		fmt.Fprintf(w, "%s\t%s\n",
			sanitizeTab(typ),
			sanitizeTab(rel.Person),
		)
	}
	return nil
}

func peopleServiceForResource(ctx context.Context, account string, resource string) (*people.Service, error) {
	if resource == peopleMeResource {
		return newPeopleContactsService(ctx, account)
	}
	return newPeopleDirectoryService(ctx, account)
}
