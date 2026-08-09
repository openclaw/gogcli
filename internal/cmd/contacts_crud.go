package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/people/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/timeparse"
	"github.com/openclaw/gogcli/internal/ui"
)

const (
	contactsReadMask       = "names,emailAddresses,phoneNumbers,birthdays,organizations,urls"
	contactsGetReadMask    = contactsReadMask + ",biographies,addresses,genders,userDefined,relations,metadata"
	contactsUpdateReadMask = contactsReadMask + ",biographies,addresses,genders,userDefined,relations,metadata"
)

type ContactsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page string `name:"page" help:"Page token"`
}

func (c *ContactsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := peopleContactsService(ctx, account)
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
			Birthday string `json:"birthday,omitempty"`
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
				Birthday: primaryBirthday(p),
			})
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"contacts":      items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Connections) == 0 {
		u.Err().Println("No contacts")
		return nil
	}

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactPeopleRows(resp.Connections),
		contactColumns(),
	); err != nil {
		return err
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

	svc, err := peopleContactsService(ctx, account)
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
		warmSearchContactsCache(ctx, svc)
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
				return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"found": false})
			}
			u.Err().Println("Not found")
			return nil
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"contact": p})
	}

	u.Out().Linef("resource\t%s", p.ResourceName)
	u.Out().Linef("name\t%s", primaryName(p))
	if e := primaryEmail(p); e != "" {
		u.Out().Linef("email\t%s", e)
	}
	if ph := primaryPhone(p); ph != "" {
		u.Out().Linef("phone\t%s", ph)
	}
	if bd := primaryBirthday(p); bd != "" {
		u.Out().Linef("birthday\t%s", bd)
	}
	if gender := primaryGender(p); gender != "" {
		u.Out().Linef("gender\t%s", gender)
	}
	if org, title := primaryOrganization(p); org != "" || title != "" {
		switch {
		case org != "" && title != "":
			u.Out().Linef("organization\t%s (%s)", org, title)
		case org != "":
			u.Out().Linef("organization\t%s", org)
		default:
			u.Out().Linef("title\t%s", title)
		}
	}
	for _, url := range allURLs(p) {
		u.Out().Linef("url\t%s", url)
	}
	for _, addr := range allAddresses(p) {
		u.Out().Linef("address\t%s", sanitizeTab(addr))
	}
	if bio := primaryBio(p); bio != "" {
		u.Out().Linef("note\t%s", bio)
	}
	for _, rel := range p.Relations {
		if rel == nil {
			continue
		}
		u.Out().Linef("relation:%s\t%s", rel.Type, rel.Person)
	}
	customFields := userDefinedFields(p)
	if len(customFields) > 0 {
		keys := make([]string, 0, len(customFields))
		for k := range customFields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			u.Out().Linef("custom:%s\t%s", k, customFields[k])
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
	Address      []string `name:"address" sep:";" help:"Postal address (can be repeated for multiple addresses)"`
	Gender       string   `name:"gender" help:"Gender value"`
	Custom       []string `name:"custom" help:"Custom field as key=value (can be repeated)"`
	Relation     []string `name:"relation" help:"Relation as type=person (can be repeated)"`
}

func parseKeyValuePairs(values []string, allowEmptyClear bool, flag, format string) ([][2]string, bool, error) {
	if len(values) == 0 {
		return nil, false, nil
	}
	if len(values) == 1 && strings.TrimSpace(values[0]) == "" {
		if !allowEmptyClear {
			return nil, false, fmt.Errorf("--%s entry cannot be empty", flag)
		}
		return nil, true, nil
	}

	pairs := make([][2]string, 0, len(values))
	for _, kv := range values {
		parts := strings.SplitN(strings.TrimSpace(kv), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, false, fmt.Errorf("expected %s for --%s, got %q", format, flag, kv)
		}
		pairs = append(pairs, [2]string{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])})
	}
	return pairs, false, nil
}

func parseCustomUserDefined(values []string, allowEmptyClear bool) ([]*people.UserDefined, bool, error) {
	pairs, clearAll, err := parseKeyValuePairs(values, allowEmptyClear, "custom", "key=value")
	if err != nil || clearAll {
		return nil, clearAll, err
	}
	out := make([]*people.UserDefined, len(pairs))
	for i, p := range pairs {
		out[i] = &people.UserDefined{Key: p[0], Value: p[1]}
	}
	return out, false, nil
}

func parseRelations(values []string, allowEmptyClear bool) ([]*people.Relation, bool, error) {
	pairs, clearAll, err := parseKeyValuePairs(values, allowEmptyClear, "relation", "type=person")
	if err != nil || clearAll {
		return nil, clearAll, err
	}
	out := make([]*people.Relation, len(pairs))
	for i, p := range pairs {
		out[i] = &people.Relation{Type: p[0], Person: p[1]}
	}
	return out, false, nil
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

func contactsAddresses(values []string) []*people.Address {
	if len(values) == 0 {
		return nil
	}
	out := make([]*people.Address, 0, len(values))
	for _, a := range values {
		if trimmed := strings.TrimSpace(a); trimmed != "" {
			out = append(out, &people.Address{StreetAddress: trimmed})
		}
	}
	return out
}

func contactsGenders(value string) []*people.Gender {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []*people.Gender{{
		Value:    value,
		Metadata: &people.FieldMetadata{Primary: true},
	}}
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

func anyFlagProvided(kctx *kong.Context, names ...string) bool {
	for _, name := range names {
		if flagProvided(kctx, name) {
			return true
		}
	}
	return false
}

func flagValue[T any](set bool, value T) any {
	if !set {
		return nil
	}
	return value
}

func (c *ContactsUpdateCmd) validateFlagUpdateInputs(wantBirthday, wantCustom, wantRelation bool) error {
	if strings.TrimSpace(c.Email) != "" {
		if err := validatePlainEmail("--email", strings.TrimSpace(c.Email)); err != nil {
			return err
		}
	}
	if wantCustom {
		if _, _, err := parseCustomUserDefined(c.Custom, true); err != nil {
			return usage(err.Error())
		}
	}
	if wantRelation {
		if _, _, err := parseRelations(c.Relation, true); err != nil {
			return usage(err.Error())
		}
	}
	if wantBirthday && strings.TrimSpace(c.Birthday) != "" {
		if _, err := parseYYYYMMDD(strings.TrimSpace(c.Birthday)); err != nil {
			return usage("invalid --birthday (expected YYYY-MM-DD)")
		}
	}
	return nil
}

func (c *ContactsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if strings.TrimSpace(c.Given) == "" {
		return usage("required: --given")
	}

	p := &people.Person{
		Names: []*people.Name{{
			GivenName:  strings.TrimSpace(c.Given),
			FamilyName: strings.TrimSpace(c.Family),
		}},
	}
	if strings.TrimSpace(c.Email) != "" {
		email := strings.TrimSpace(c.Email)
		if err := validatePlainEmail("--email", email); err != nil {
			return err
		}
		p.EmailAddresses = []*people.EmailAddress{{Value: email}}
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
	if len(c.Address) > 0 {
		if addrs := contactsAddresses(c.Address); len(addrs) > 0 {
			p.Addresses = addrs
		}
	}
	if genders := contactsGenders(c.Gender); len(genders) > 0 {
		p.Genders = genders
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
	if len(c.Relation) > 0 {
		relations, _, parseErr := parseRelations(c.Relation, false)
		if parseErr != nil {
			return usage(parseErr.Error())
		}
		if len(relations) > 0 {
			p.Relations = relations
		}
	}

	if err := dryRunExit(ctx, flags, "contacts.create", map[string]any{"contact": p}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := peopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	created, err := svc.People.CreateContact(p).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"contact": created})
	}
	u.Out().Linef("resource\t%s", created.ResourceName)
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
	Address      []string `name:"address" sep:";" help:"Postal address (can be repeated; empty clears all)"`
	Gender       string   `name:"gender" help:"Gender value (empty clears)"`
	Custom       []string `name:"custom" help:"Custom field as key=value (can be repeated; empty clears all)"`
	Relation     []string `name:"relation" help:"Relation as type=person (can be repeated; empty clears all)"`
	FromFile     string   `name:"from-file" help:"Update from contact JSON file (use - for stdin)"`
	IgnoreETag   bool     `name:"ignore-etag" help:"Allow updating even if the JSON etag is stale (may overwrite concurrent changes)"`

	// Extra People API fields (not previously exposed by gog)
	Birthday string `name:"birthday" help:"Birthday in YYYY-MM-DD (empty clears)"`
	Notes    string `name:"notes" help:"Notes (stored as People API biography; empty clears)"`
}

type contactsUpdateFieldFlags struct {
	given    bool
	family   bool
	email    bool
	phone    bool
	org      bool
	title    bool
	url      bool
	note     bool
	address  bool
	gender   bool
	birthday bool
	notes    bool
	custom   bool
	relation bool
}

func contactsUpdateFieldFlagsFromKong(kctx *kong.Context) contactsUpdateFieldFlags {
	return contactsUpdateFieldFlags{
		given:    flagProvided(kctx, "given"),
		family:   flagProvided(kctx, "family"),
		email:    flagProvided(kctx, "email"),
		phone:    flagProvided(kctx, "phone"),
		org:      flagProvided(kctx, "org"),
		title:    flagProvided(kctx, "title"),
		url:      flagProvided(kctx, "url"),
		note:     flagProvided(kctx, "note"),
		address:  flagProvided(kctx, "address"),
		gender:   flagProvided(kctx, "gender"),
		birthday: flagProvided(kctx, "birthday"),
		notes:    flagProvided(kctx, "notes"),
		custom:   flagProvided(kctx, "custom"),
		relation: flagProvided(kctx, "relation"),
	}
}

func (w contactsUpdateFieldFlags) any() bool {
	return w.given || w.family || w.email || w.phone || w.org || w.title ||
		w.url || w.note || w.address || w.gender || w.birthday || w.notes ||
		w.custom || w.relation
}

func (c *ContactsUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	resourceName := strings.TrimSpace(c.ResourceName)
	if !strings.HasPrefix(resourceName, "people/") {
		return usage("resourceName must start with people/")
	}

	if strings.TrimSpace(c.FromFile) != "" {
		if anyFlagProvided(kctx,
			"given", "family", "email", "phone",
			"org", "title", "url", "note",
			"address", "gender", "custom", "birthday",
			"notes", "relation",
		) {
			return usage("can't combine --from-file with other update flags")
		}
		if flags != nil && flags.DryRun {
			inputPerson, updateFields, err := c.readUpdateJSONInput(ctx, resourceName)
			if err != nil {
				return err
			}
			if err := dryRunExit(ctx, flags, "contacts.update", map[string]any{
				"resourceName": resourceName,
				"from_file":    strings.TrimSpace(c.FromFile),
				"updateFields": updateFields,
				"contact":      inputPerson,
			}); err != nil {
				return err
			}
		}
		account, err := requireAccount(flags)
		if err != nil {
			return err
		}
		svc, err := peopleContactsService(ctx, account)
		if err != nil {
			return err
		}
		return c.updateFromJSON(ctx, svc, resourceName, u)
	}

	want := contactsUpdateFieldFlagsFromKong(kctx)
	if !want.any() {
		return usage("no updates provided")
	}
	if err := c.validateFlagUpdateInputs(want.birthday, want.custom, want.relation); err != nil {
		return err
	}

	if err := dryRunExit(ctx, flags, "contacts.update", map[string]any{
		"resourceName": resourceName,
		"fields": map[string]any{
			"given":        flagValue(want.given, c.Given),
			"family":       flagValue(want.family, c.Family),
			"email":        flagValue(want.email, c.Email),
			"phone":        flagValue(want.phone, c.Phone),
			"organization": flagValue(want.org, c.Organization),
			"title":        flagValue(want.title, c.Title),
			"url":          flagValue(want.url, c.URL),
			"note":         flagValue(want.note, c.Note),
			"address":      flagValue(want.address, c.Address),
			"gender":       flagValue(want.gender, c.Gender),
			"birthday":     flagValue(want.birthday, c.Birthday),
			"notes":        flagValue(want.notes, c.Notes),
			"custom":       flagValue(want.custom, c.Custom),
			"relation":     flagValue(want.relation, c.Relation),
		},
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := peopleContactsService(ctx, account)
	if err != nil {
		return err
	}
	existing, err := svc.People.Get(resourceName).PersonFields(contactsUpdateReadMask).Do()
	if err != nil {
		return err
	}

	updateFields := make([]string, 0, 8)

	if want.given || want.family {
		contactsApplyPersonName(existing, want.given, c.Given, want.family, c.Family)
		updateFields = append(updateFields, "names")
	}
	if want.email {
		if strings.TrimSpace(c.Email) == "" {
			existing.EmailAddresses = nil // will be forced to [] for patch
		} else {
			existing.EmailAddresses = []*people.EmailAddress{{Value: strings.TrimSpace(c.Email)}}
		}
		updateFields = append(updateFields, "emailAddresses")
	}
	if want.phone {
		if strings.TrimSpace(c.Phone) == "" {
			existing.PhoneNumbers = nil // will be forced to [] for patch
		} else {
			existing.PhoneNumbers = []*people.PhoneNumber{{Value: strings.TrimSpace(c.Phone)}}
		}
		updateFields = append(updateFields, "phoneNumbers")
	}
	if want.org || want.title {
		contactsApplyPersonOrganization(existing, want.org, c.Organization, want.title, c.Title)
		updateFields = append(updateFields, "organizations")
	}
	if want.url {
		urls := contactsURLs(c.URL)
		if len(urls) == 0 {
			existing.Urls = nil
		} else {
			existing.Urls = urls
		}
		updateFields = append(updateFields, "urls")
	}
	if want.note {
		if strings.TrimSpace(c.Note) == "" {
			existing.Biographies = nil
		} else {
			existing.Biographies = []*people.Biography{{Value: strings.TrimSpace(c.Note)}}
		}
		updateFields = append(updateFields, "biographies")
	}
	if want.address {
		addrs := contactsAddresses(c.Address)
		if len(addrs) == 0 {
			existing.Addresses = nil // will be forced to [] for patch
		} else {
			existing.Addresses = addrs
		}
		updateFields = append(updateFields, "addresses")
	}
	if want.gender {
		genders := contactsGenders(c.Gender)
		if len(genders) == 0 {
			existing.Genders = nil // will be forced to [] for patch
		} else {
			existing.Genders = genders
		}
		updateFields = append(updateFields, "genders")
	}
	if want.custom {
		userDefined, clearAll, parseErr := parseCustomUserDefined(c.Custom, true)
		if parseErr != nil {
			return usage(parseErr.Error())
		}
		if clearAll {
			existing.UserDefined = nil
		} else {
			existing.UserDefined = userDefined
		}
		updateFields = append(updateFields, "userDefined")
	}
	if want.relation {
		relations, clearAll, parseErr := parseRelations(c.Relation, true)
		if parseErr != nil {
			return usage(parseErr.Error())
		}
		if clearAll {
			existing.Relations = nil
		} else {
			existing.Relations = relations
		}
		updateFields = append(updateFields, "relations")
	}

	if want.birthday {
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

	if want.notes {
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"contact": updated})
	}
	u.Out().Linef("resource\t%s", updated.ResourceName)
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
	resourceName := strings.TrimSpace(c.ResourceName)
	if !strings.HasPrefix(resourceName, "people/") {
		return usage("resourceName must start with people/")
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "contacts.delete", map[string]any{
		"resourceName": resourceName,
	}, fmt.Sprintf("delete contact %s", resourceName)); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := peopleContactsService(ctx, account)
	if err != nil {
		return err
	}
	if _, err := svc.People.DeleteContact(resourceName).Do(); err != nil {
		return err
	}
	return writeDeleteResult(ctx, u, resourceName)
}
