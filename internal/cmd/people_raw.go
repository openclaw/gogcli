package cmd

import (
	"context"
	"fmt"
	"strings"
)

// defaultPeopleRawMask is the field mask used when the user does not
// supply --person-fields. Covers the commonly useful Person fields.
const defaultPeopleRawMask = "names,emailAddresses,phoneNumbers,organizations,urls,addresses,biographies,birthdays,photos,metadata,relations,userDefined,memberships,events,imClients,interests,locales,nicknames,occupations,skills"

// PeopleRawCmd dumps the full People.Get response as JSON. Requires the
// People API field mask (set via --person-fields). Defaults to a broad
// set covering commonly useful Person resource fields.
//
// REST reference: https://developers.google.com/people/api/rest/v1/people/get
// Go type: https://pkg.go.dev/google.golang.org/api/people/v1#Person
type PeopleRawCmd struct {
	UserID       string `arg:"" name:"userId" help:"Person resource name (people/...) or email"`
	PersonFields string `name:"person-fields" help:"People API personFields mask (default: broad set; pass a narrower list to reduce output)"`
	Pretty       bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *PeopleRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runPeopleRaw(ctx, flags, c.UserID, c.PersonFields, c.Pretty)
}

// ContactsRawCmd mirrors PeopleRawCmd but lives under the `contacts` group
// for users who think of these operations in contact terms. Wraps the
// same underlying People.Get call.
//
// REST reference: https://developers.google.com/people/api/rest/v1/people/get
type ContactsRawCmd struct {
	Identifier   string `arg:"" name:"identifier" help:"Contact resource name (people/...) or email"`
	PersonFields string `name:"person-fields" help:"People API personFields mask (default: broad set)"`
	Pretty       bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *ContactsRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runPeopleRaw(ctx, flags, c.Identifier, c.PersonFields, c.Pretty)
}

func runPeopleRaw(ctx context.Context, flags *RootFlags, id, fields string, pretty bool) error {
	identifier := strings.TrimSpace(id)
	if identifier == "" {
		return usage("required: resource name or email")
	}

	mask := defaultPeopleRawMask
	if trimmed := strings.TrimSpace(fields); trimmed != "" {
		mask = trimmed
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := peopleContactsService(ctx, account)
	if err != nil {
		return wrapPeopleAPIError(err)
	}

	resource := normalizePeopleResource(identifier)
	if strings.Contains(identifier, "@") && !strings.HasPrefix(identifier, "people/") {
		seen := make(map[string]bool)
		fetch := func(pageToken string) ([]string, string, error) {
			call := svc.People.Connections.List(peopleMeResource).
				PersonFields("names,emailAddresses,metadata").
				PageSize(1000).
				Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			connections, listErr := call.Do()
			if listErr != nil {
				return nil, "", wrapPeopleAPIError(listErr)
			}
			var matches []string
			for _, person := range connections.Connections {
				if person == nil || !personHasEmail(person, identifier) || person.ResourceName == "" || seen[person.ResourceName] {
					continue
				}
				seen[person.ResourceName] = true
				matches = append(matches, person.ResourceName)
			}
			return matches, connections.NextPageToken, nil
		}
		matches, listErr := collectAllPages("", fetch)
		if listErr != nil {
			return listErr
		}
		switch len(matches) {
		case 0:
			return fmt.Errorf("contact not found for email %q", identifier)
		case 1:
			resource = matches[0]
		default:
			return fmt.Errorf("email %q matched multiple contacts; use a people/... resource name", identifier)
		}
	}

	person, err := svc.People.Get(resource).PersonFields(mask).Context(ctx).Do()
	if err != nil {
		return wrapPeopleAPIError(err)
	}
	person, err = requireRawResponse(person, "person not found")
	if err != nil {
		return err
	}

	return writeRawJSON(ctx, person, pretty)
}
