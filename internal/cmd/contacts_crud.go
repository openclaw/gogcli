package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/timeparse"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	contactsReadMask       = "names,emailAddresses,phoneNumbers,organizations,urls"
	contactsGetReadMask    = contactsReadMask + ",birthdays,biographies,addresses,userDefined,metadata"
	contactsUpdateReadMask = contactsReadMask + ",birthdays,biographies,userDefined,metadata"
)

type ContactsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page string `name:"page" help:"Page token"`
}

func (c *ContactsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.People.Connections.List(peopleMeResource).
		PersonFields(contactsReadMask).
		PageSize(c.Max).
		PageToken(c.Page).
		Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource string `json:"resource"`
			Name     string `json:"name,omitempty"`
			Email    string `json:"email,omitempty"`
			Phone    string `json:"phone,omitempty"`
		}
		items := make([]item, 0, len(resp.Connections))
		for _, p := range resp.Connections {
			if p == nil {
				continue
			}
			items = append(items, item{
				Resource: p.ResourceName,
				Name:     primaryName(p),
				Email:    primaryEmail(p),
				Phone:    primaryPhone(p),
			})
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"contacts":      items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Connections) == 0 {
		u.Err().Println("No contacts")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL\tPHONE")
	for _, p := range resp.Connections {
		if p == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
			sanitizeTab(primaryPhone(p)),
		)
	}

	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ContactsGetCmd struct {
	Identifier string `arg:"" name:"resourceName" help:"Resource name (people/...) or email"`
}

func (c *ContactsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	identifier := strings.TrimSpace(c.Identifier)
	if identifier == "" {
		return usage("empty identifier")
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	var p *people.Person
	if strings.HasPrefix(identifier, "people/") {
		p, err = svc.People.Get(identifier).PersonFields(contactsGetReadMask).Do()
		if err != nil {
			return err
		}
	} else {
		resp, err := svc.People.SearchContacts().
			Query(identifier).
			PageSize(10).
			ReadMask(contactsGetReadMask).
			Do()
		if err != nil {
			return err
		}
		for _, r := range resp.Results {
			if r.Person == nil {
				continue
			}
			if strings.EqualFold(primaryEmail(r.Person), identifier) {
				p = r.Person
				break
			}
			if p == nil {
				p = r.Person
			}
		}
		if p == nil {
			if outfmt.IsJSON(ctx) {
				return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"found": false})
			}
			u.Err().Println("Not found")
			return nil
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"contact": p})
	}

	u.Out().Printf("resource\t%s", p.ResourceName)
	u.Out().Printf("name\t%s", primaryName(p))
	if e := primaryEmail(p); e != "" {
		u.Out().Printf("email\t%s", e)
	}
	if ph := primaryPhone(p); ph != "" {
		u.Out().Printf("phone\t%s", ph)
	}
	if bd := primaryBirthday(p); bd != "" {
		u.Out().Printf("birthday\t%s", bd)
	}
	if org, title := primaryOrganization(p); org != "" || title != "" {
		if org != "" && title != "" {
			u.Out().Printf("organization\t%s (%s)", org, title)
		} else if org != "" {
			u.Out().Printf("organization\t%s", org)
		} else {
			u.Out().Printf("title\t%s", title)
		}
	}
	for _, url := range allURLs(p) {
		u.Out().Printf("url\t%s", url)
	}
	if bio := primaryBio(p); bio != "" {
		u.Out().Printf("note\t%s", bio)
	}
	customFields := userDefinedFields(p)
	if len(customFields) > 0 {
		keys := make([]string, 0, len(customFields))
		for k := range customFields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			u.Out().Printf("custom:%s\t%s", k, customFields[k])
		}
	}
	return nil
}

type ContactsCreateCmd struct {
	Given        string   `name:"given" help:"Given name (required)"`
	Family       string   `name:"family" help:"Family name"`
	Email        string   `name:"email" help:"Email address"`
	Phone        string   `name:"phone" help:"Phone number"`
	Organization string   `name:"org" help:"Organization/company name"`
	Title        string   `name:"title" help:"Job title"`
	URL          []string `name:"url" help:"URL (can be repeated for multiple URLs)"`
	Note         string   `name:"note" help:"Note/biography"`
	Custom       []string `name:"custom" help:"Custom field as key=value (can be repeated)"`
}

func parseCustomUserDefined(values []string, allowEmptyClear bool) ([]*people.UserDefined, bool, error) {
	if len(values) == 0 {
		return nil, false, nil
	}
	if len(values) == 1 && strings.TrimSpace(values[0]) == "" {
		if !allowEmptyClear {
			return nil, false, fmt.Errorf("--custom entry cannot be empty")
		}
		return nil, true, nil
	}

	userDefined := make([]*people.UserDefined, 0, len(values))
	for _, kv := range values {
		parts := strings.SplitN(strings.TrimSpace(kv), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, false, fmt.Errorf("expected key=value for --custom, got %q", kv)
		}
		if strings.TrimSpace(parts[1]) == "" {
			return nil, false, fmt.Errorf("expected key=value for --custom, got %q", kv)
		}
		userDefined = append(userDefined, &people.UserDefined{
			Key:   strings.TrimSpace(parts[0]),
			Value: strings.TrimSpace(parts[1]),
		})
	}
	return userDefined, false, nil
}

func contactsURLs(values []string) []*people.Url {
	if len(values) == 0 {
		return nil
	}
	out := make([]*people.Url, 0, len(values))
	for _, u := range values {
		if trimmed := strings.TrimSpace(u); trimmed != "" {
			out = append(out, &people.Url{Value: trimmed})
		}
	}
	return out
}

func contactsApplyPersonName(person *people.Person, givenSet bool, given string, familySet bool, family string) {
	curGiven := ""
	curFamily := ""
	if len(person.Names) > 0 && person.Names[0] != nil {
		curGiven = person.Names[0].GivenName
		curFamily = person.Names[0].FamilyName
	}
	if givenSet {
		curGiven = strings.TrimSpace(given)
	}
	if familySet {
		curFamily = strings.TrimSpace(family)
	}
	person.Names = []*people.Name{{GivenName: curGiven, FamilyName: curFamily}}
}

func contactsApplyPersonOrganization(person *people.Person, orgSet bool, org string, titleSet bool, title string) {
	curOrg := ""
	curTitle := ""
	if len(person.Organizations) > 0 && person.Organizations[0] != nil {
		curOrg = person.Organizations[0].Name
		curTitle = person.Organizations[0].Title
	}
	if orgSet {
		curOrg = strings.TrimSpace(org)
	}
	if titleSet {
		curTitle = strings.TrimSpace(title)
	}
	if curOrg == "" && curTitle == "" {
		person.Organizations = nil
		return
	}
	person.Organizations = []*people.Organization{{Name: curOrg, Title: curTitle}}
}

func (c *ContactsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.Given) == "" {
		return usage("required: --given")
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	p := &people.Person{
		Names: []*people.Name{{
			GivenName:  strings.TrimSpace(c.Given),
			FamilyName: strings.TrimSpace(c.Family),
		}},
	}
	if strings.TrimSpace(c.Email) != "" {
		p.EmailAddresses = []*people.EmailAddress{{Value: strings.TrimSpace(c.Email)}}
	}
	if strings.TrimSpace(c.Phone) != "" {
		p.PhoneNumbers = []*people.PhoneNumber{{Value: strings.TrimSpace(c.Phone)}}
	}
	if strings.TrimSpace(c.Organization) != "" || strings.TrimSpace(c.Title) != "" {
		p.Organizations = []*people.Organization{{
			Name:  strings.TrimSpace(c.Organization),
			Title: strings.TrimSpace(c.Title),
		}}
	}
	if len(c.URL) > 0 {
		if urls := contactsURLs(c.URL); len(urls) > 0 {
			p.Urls = urls
		}
	}
	if strings.TrimSpace(c.Note) != "" {
		p.Biographies = []*people.Biography{{Value: strings.TrimSpace(c.Note)}}
	}
	if len(c.Custom) > 0 {
		userDefined, _, parseErr := parseCustomUserDefined(c.Custom, false)
		if parseErr != nil {
			return usage(parseErr.Error())
		}
		if len(userDefined) > 0 {
			p.UserDefined = userDefined
		}
	}

	created, err := svc.People.CreateContact(p).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"contact": created})
	}
	u.Out().Printf("resource\t%s", created.ResourceName)
	return nil
}

type ContactsUpdateCmd struct {
	ResourceName string   `arg:"" name:"resourceName" help:"Resource name (people/...)"`
	Given        string   `name:"given" help:"Given name"`
	Family       string   `name:"family" help:"Family name"`
	Email        string   `name:"email" help:"Email address (empty clears)"`
	Phone        string   `name:"phone" help:"Phone number (empty clears)"`
	Organization string   `name:"org" help:"Organization/company name (empty clears)"`
	Title        string   `name:"title" help:"Job title (empty clears)"`
	URL          []string `name:"url" help:"URL (can be repeated; empty clears all)"`
	Note         string   `name:"note" help:"Note/biography (empty clears)"`
	Custom       []string `name:"custom" help:"Custom field as key=value (can be repeated; empty clears all)"`
	FromFile     string   `name:"from-file" help:"Update from contact JSON file (use - for stdin)"`
	IgnoreETag   bool     `name:"ignore-etag" help:"Allow updating even if the JSON etag is stale (may overwrite concurrent changes)"`

	// Extra People API fields (not previously exposed by gog)
	Birthday string `name:"birthday" help:"Birthday in YYYY-MM-DD (empty clears)"`
	Notes    string `name:"notes" help:"Notes (stored as People API biography; empty clears)"`
}

func (c *ContactsUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	resourceName := strings.TrimSpace(c.ResourceName)
	if !strings.HasPrefix(resourceName, "people/") {
		return usage("resourceName must start with people/")
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.FromFile) != "" {
		if flagProvided(kctx, "given") || flagProvided(kctx, "family") || flagProvided(kctx, "email") || flagProvided(kctx, "phone") || flagProvided(kctx, "birthday") || flagProvided(kctx, "notes") {
			return usage("can't combine --from-file with other update flags")
		}
		return c.updateFromJSON(ctx, svc, resourceName, u)
	}

	existing, err := svc.People.Get(resourceName).PersonFields(contactsUpdateReadMask).Do()
	if err != nil {
		return err
	}

	updateFields := make([]string, 0, 8)

	wantGiven := flagProvided(kctx, "given")
	wantFamily := flagProvided(kctx, "family")
	wantEmail := flagProvided(kctx, "email")
	wantPhone := flagProvided(kctx, "phone")
	wantOrg := flagProvided(kctx, "org")
	wantTitle := flagProvided(kctx, "title")
	wantURL := flagProvided(kctx, "url")
	wantNote := flagProvided(kctx, "note")
	wantBirthday := flagProvided(kctx, "birthday")
	wantNotes := flagProvided(kctx, "notes")
	wantCustom := flagProvided(kctx, "custom")

	if wantGiven || wantFamily {
		contactsApplyPersonName(existing, wantGiven, c.Given, wantFamily, c.Family)
		updateFields = append(updateFields, "names")
	}
	if wantEmail {
		if strings.TrimSpace(c.Email) == "" {
			existing.EmailAddresses = nil // will be forced to [] for patch
		} else {
			existing.EmailAddresses = []*people.EmailAddress{{Value: strings.TrimSpace(c.Email)}}
		}
		updateFields = append(updateFields, "emailAddresses")
	}
	if wantPhone {
		if strings.TrimSpace(c.Phone) == "" {
			existing.PhoneNumbers = nil // will be forced to [] for patch
		} else {
			existing.PhoneNumbers = []*people.PhoneNumber{{Value: strings.TrimSpace(c.Phone)}}
		}
		updateFields = append(updateFields, "phoneNumbers")
	}
	if wantOrg || wantTitle {
		contactsApplyPersonOrganization(existing, wantOrg, c.Organization, wantTitle, c.Title)
		updateFields = append(updateFields, "organizations")
	}
	if wantURL {
		urls := contactsURLs(c.URL)
		if len(urls) == 0 {
			existing.Urls = nil
		} else {
			existing.Urls = urls
		}
		updateFields = append(updateFields, "urls")
	}
	if wantNote {
		if strings.TrimSpace(c.Note) == "" {
			existing.Biographies = nil
		} else {
			existing.Biographies = []*people.Biography{{Value: strings.TrimSpace(c.Note)}}
		}
		updateFields = append(updateFields, "biographies")
	}
	if wantCustom {
		userDefined, clear, parseErr := parseCustomUserDefined(c.Custom, true)
		if parseErr != nil {
			return usage(parseErr.Error())
		}
		if clear {
			existing.UserDefined = nil
		} else {
			existing.UserDefined = userDefined
		}
		updateFields = append(updateFields, "userDefined")
	}

	if wantBirthday {
		if strings.TrimSpace(c.Birthday) == "" {
			existing.Birthdays = nil // will be forced to [] for patch
		} else {
			d, parseErr := parseYYYYMMDD(strings.TrimSpace(c.Birthday))
			if parseErr != nil {
				return usage("invalid --birthday (expected YYYY-MM-DD)")
			}
			existing.Birthdays = []*people.Birthday{{
				Date:     d,
				Metadata: &people.FieldMetadata{Primary: true},
			}}
		}
		updateFields = append(updateFields, "birthdays")
	}

	if wantNotes {
		if strings.TrimSpace(c.Notes) == "" {
			existing.Biographies = nil // will be forced to [] for patch
		} else {
			existing.Biographies = []*people.Biography{{
				Value:       c.Notes,
				ContentType: "TEXT_PLAIN",
				Metadata:    &people.FieldMetadata{Primary: true},
			}}
		}
		updateFields = append(updateFields, "biographies")
	}

	if len(updateFields) == 0 {
		return usage("no updates provided")
	}

	for _, f := range updateFields {
		// Clearing list fields requires forcing them into the patch payload (Google API client omits empty values by default).
		forceSendEmptyPersonListField(existing, f)
	}

	updated, err := svc.People.UpdateContact(resourceName, existing).
		UpdatePersonFields(strings.Join(updateFields, ",")).
		Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"contact": updated})
	}
	u.Out().Printf("resource\t%s", updated.ResourceName)
	return nil
}

type ContactsDeleteCmd struct {
	ResourceName string `arg:"" name:"resourceName" help:"Resource name (people/...)"`
}

func parseYYYYMMDD(s string) (*people.Date, error) {
	t, err := timeparse.ParseDate(s)
	if err != nil {
		return nil, err
	}
	return &people.Date{Year: int64(t.Year()), Month: int64(t.Month()), Day: int64(t.Day())}, nil
}

func (c *ContactsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	resourceName := strings.TrimSpace(c.ResourceName)
	if !strings.HasPrefix(resourceName, "people/") {
		return usage("resourceName must start with people/")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete contact %s", resourceName)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}
	if _, err := svc.People.DeleteContact(resourceName).Do(); err != nil {
		return err
	}
	return writeDeleteResult(ctx, u, resourceName)
}
