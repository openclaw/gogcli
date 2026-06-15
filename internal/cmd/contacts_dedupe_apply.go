package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const contactsDedupeContactSource = "READ_SOURCE_TYPE_CONTACT"

var contactsDedupeMutableFields = []string{
	"addresses",
	"biographies",
	"birthdays",
	"calendarUrls",
	"clientData",
	"emailAddresses",
	"events",
	"externalIds",
	"genders",
	"imClients",
	"interests",
	"locales",
	"locations",
	"memberships",
	"miscKeywords",
	"names",
	"nicknames",
	"occupations",
	"organizations",
	"phoneNumbers",
	"relations",
	"sipAddresses",
	"urls",
	"userDefined",
}

type contactsDedupeApplyPlan struct {
	Group        contactsDedupeGroup
	Merged       *people.Person
	UpdateFields []string
	Delete       []*people.Person
}

type contactsDedupeApplyResult struct {
	Scanned         int
	Plans           []contactsDedupeApplyPlan
	GroupsMerged    int
	ContactsDeleted int
}

func contactsDedupeApplyReadMask() string {
	return strings.Join(append(append([]string{}, contactsDedupeMutableFields...), "coverPhotos", "metadata", "photos", "skills"), ",")
}

func prepareContactsDedupeApply(
	ctx context.Context,
	svc *people.Service,
	groups []contactsDedupeGroup,
	match contactsDedupeMatch,
) ([]contactsDedupeApplyPlan, error) {
	plans := make([]contactsDedupeApplyPlan, 0, len(groups))
	for i, group := range groups {
		fresh, err := refreshContactsDedupeGroup(ctx, svc, group, match)
		if err != nil {
			return nil, fmt.Errorf("prepare duplicate group %d: %w", i+1, err)
		}
		plan, err := buildContactsDedupeApplyPlan(fresh)
		if err != nil {
			return nil, fmt.Errorf("prepare duplicate group %d: %w", i+1, err)
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func refreshContactsDedupeGroup(
	ctx context.Context,
	svc *people.Service,
	group contactsDedupeGroup,
	match contactsDedupeMatch,
) (contactsDedupeGroup, error) {
	members := make([]*people.Person, 0, len(group.Members))
	for _, member := range group.Members {
		resource := contactsDedupeResource(member)
		if resource == "" {
			return contactsDedupeGroup{}, fmt.Errorf("member is missing a resource name")
		}
		fresh, err := svc.People.Get(resource).
			PersonFields(contactsDedupeApplyReadMask()).
			Sources(contactsDedupeContactSource).
			Context(ctx).
			Do()
		if err != nil {
			return contactsDedupeGroup{}, wrapPeopleAPIError(err)
		}
		members = append(members, fresh)
	}

	freshGroups := buildContactsDedupeGroups(members, match)
	if len(freshGroups) != 1 || len(freshGroups[0].Members) != len(members) {
		return contactsDedupeGroup{}, fmt.Errorf("contacts changed and no longer form the same duplicate group; rerun without --apply to review")
	}
	return freshGroups[0], nil
}

func buildContactsDedupeApplyPlan(group contactsDedupeGroup) (contactsDedupeApplyPlan, error) {
	if group.Primary == nil {
		return contactsDedupeApplyPlan{}, fmt.Errorf("duplicate group has no primary contact")
	}
	if contactSourceETag(group.Primary) == "" {
		return contactsDedupeApplyPlan{}, fmt.Errorf("primary contact %s is missing a contact-source etag", contactsDedupeResource(group.Primary))
	}

	ordered := orderContactsDedupeMembers(group.Primary, group.Members)
	for _, member := range ordered[1:] {
		if hasContactPhoto(member) {
			return contactsDedupeApplyPlan{}, fmt.Errorf(
				"contact %s has a photo that the People API cannot merge; remove or move the photo before applying",
				contactsDedupeResource(member),
			)
		}
		if len(member.CoverPhotos) > 0 || len(member.Skills) > 0 {
			return contactsDedupeApplyPlan{}, fmt.Errorf(
				"contact %s contains data that the People API cannot update during a merge; merge this group in Google Contacts",
				contactsDedupeResource(member),
			)
		}
		if contactSourceETag(member) == "" {
			return contactsDedupeApplyPlan{}, fmt.Errorf("contact %s is missing a contact-source etag", contactsDedupeResource(member))
		}
	}

	merged, err := cloneContactPerson(group.Primary)
	if err != nil {
		return contactsDedupeApplyPlan{}, err
	}
	clearNonMutableContactFields(merged)
	if err := mergeContactsDedupeFields(merged, ordered); err != nil {
		return contactsDedupeApplyPlan{}, err
	}

	updateFields := populatedContactsDedupeFields(merged)
	if len(updateFields) == 0 {
		return contactsDedupeApplyPlan{}, fmt.Errorf("duplicate group has no mergeable contact fields")
	}

	return contactsDedupeApplyPlan{
		Group:        group,
		Merged:       merged,
		UpdateFields: updateFields,
		Delete:       append([]*people.Person(nil), ordered[1:]...),
	}, nil
}

func mergeContactsDedupeFields(dst *people.Person, members []*people.Person) error {
	var err error
	if dst.Addresses, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.Address { return p.Addresses })...); err != nil {
		return err
	}
	if dst.Biographies, err = mergeContactsDedupeSingleton("biographies", contactFieldSlices(members, func(p *people.Person) []*people.Biography { return p.Biographies })...); err != nil {
		return err
	}
	if dst.Birthdays, err = mergeContactsDedupeSingleton("birthdays", contactFieldSlices(members, func(p *people.Person) []*people.Birthday { return p.Birthdays })...); err != nil {
		return err
	}
	if dst.CalendarUrls, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.CalendarUrl { return p.CalendarUrls })...); err != nil {
		return err
	}
	if dst.ClientData, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.ClientData { return p.ClientData })...); err != nil {
		return err
	}
	if dst.EmailAddresses, err = mergeContactsDedupeKeyedItems(
		func(item *people.EmailAddress) string { return normalizeContactEmail(item.Value) },
		contactFieldSlices(members, func(p *people.Person) []*people.EmailAddress { return p.EmailAddresses })...,
	); err != nil {
		return err
	}
	if dst.Events, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.Event { return p.Events })...); err != nil {
		return err
	}
	if dst.ExternalIds, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.ExternalId { return p.ExternalIds })...); err != nil {
		return err
	}
	if dst.Genders, err = mergeContactsDedupeSingleton("genders", contactFieldSlices(members, func(p *people.Person) []*people.Gender { return p.Genders })...); err != nil {
		return err
	}
	if dst.ImClients, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.ImClient { return p.ImClients })...); err != nil {
		return err
	}
	if dst.Interests, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.Interest { return p.Interests })...); err != nil {
		return err
	}
	if dst.Locales, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.Locale { return p.Locales })...); err != nil {
		return err
	}
	if dst.Locations, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.Location { return p.Locations })...); err != nil {
		return err
	}
	if dst.Memberships, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.Membership { return p.Memberships })...); err != nil {
		return err
	}
	if dst.MiscKeywords, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.MiscKeyword { return p.MiscKeywords })...); err != nil {
		return err
	}
	if dst.Names, err = mergeContactsDedupeSingleton("names", contactFieldSlices(members, func(p *people.Person) []*people.Name { return p.Names })...); err != nil {
		return err
	}
	if dst.Nicknames, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.Nickname { return p.Nicknames })...); err != nil {
		return err
	}
	if dst.Occupations, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.Occupation { return p.Occupations })...); err != nil {
		return err
	}
	if dst.Organizations, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.Organization { return p.Organizations })...); err != nil {
		return err
	}
	if dst.PhoneNumbers, err = mergeContactsDedupeKeyedItems(
		func(item *people.PhoneNumber) string { return normalizeContactPhone(item.Value) },
		contactFieldSlices(members, func(p *people.Person) []*people.PhoneNumber { return p.PhoneNumbers })...,
	); err != nil {
		return err
	}
	if dst.Relations, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.Relation { return p.Relations })...); err != nil {
		return err
	}
	if dst.SipAddresses, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.SipAddress { return p.SipAddresses })...); err != nil {
		return err
	}
	if dst.Urls, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.Url { return p.Urls })...); err != nil {
		return err
	}
	if dst.UserDefined, err = mergeContactsDedupeItems(contactFieldSlices(members, func(p *people.Person) []*people.UserDefined { return p.UserDefined })...); err != nil {
		return err
	}
	return nil
}

func contactFieldSlices[T any](members []*people.Person, field func(*people.Person) []*T) [][]*T {
	out := make([][]*T, 0, len(members))
	for _, member := range members {
		out = append(out, field(member))
	}
	return out
}

func mergeContactsDedupeSingleton[T any](field string, lists ...[]*T) ([]*T, error) {
	merged, err := mergeContactsDedupeItems(lists...)
	if err != nil {
		return nil, err
	}
	if len(merged) > 1 {
		return nil, fmt.Errorf("conflicting %s cannot be merged safely; resolve this group in Google Contacts", field)
	}
	return merged, nil
}

func mergeContactsDedupeItems[T any](lists ...[]*T) ([]*T, error) {
	return mergeContactsDedupeKeyedItems[T](nil, lists...)
}

func mergeContactsDedupeKeyedItems[T any](keyFn func(*T) string, lists ...[]*T) ([]*T, error) {
	var out []*T
	seen := map[string]bool{}
	for _, list := range lists {
		for _, item := range list {
			if item == nil {
				continue
			}
			clean, key, err := cleanContactsDedupeItem(item)
			if err != nil {
				return nil, err
			}
			if keyFn != nil {
				key = keyFn(clean)
			}
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, clean)
		}
	}
	return out, nil
}

func cleanContactsDedupeItem[T any](item *T) (*T, string, error) {
	data, err := json.Marshal(item)
	if err != nil {
		return nil, "", fmt.Errorf("encode contact field: %w", err)
	}
	var object map[string]any
	if decodeErr := json.Unmarshal(data, &object); decodeErr != nil {
		return nil, "", fmt.Errorf("decode contact field: %w", decodeErr)
	}
	delete(object, "metadata")
	delete(object, "formattedType")
	cleanData, err := json.Marshal(object)
	if err != nil {
		return nil, "", fmt.Errorf("encode cleaned contact field: %w", err)
	}
	var clean T
	if err := json.Unmarshal(cleanData, &clean); err != nil {
		return nil, "", fmt.Errorf("decode cleaned contact field: %w", err)
	}
	return &clean, string(cleanData), nil
}

func cloneContactPerson(person *people.Person) (*people.Person, error) {
	data, err := json.Marshal(person)
	if err != nil {
		return nil, fmt.Errorf("encode contact %s: %w", contactsDedupeResource(person), err)
	}
	var clone people.Person
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("decode contact %s: %w", contactsDedupeResource(person), err)
	}
	return &clone, nil
}

func clearNonMutableContactFields(person *people.Person) {
	if person == nil {
		return
	}
	person.AgeRange = ""
	person.AgeRanges = nil
	person.BraggingRights = nil
	person.CoverPhotos = nil
	person.FileAses = nil
	person.Photos = nil
	person.RelationshipInterests = nil
	person.RelationshipStatuses = nil
	person.Residences = nil
	person.Skills = nil
	person.Taglines = nil
}

func populatedContactsDedupeFields(person *people.Person) []string {
	value := reflect.ValueOf(person)
	if !value.IsValid() || value.IsNil() {
		return nil
	}
	value = value.Elem()
	var fields []string
	for _, field := range contactsDedupeMutableFields {
		goField := contactsPersonFieldToGoField(field)
		current := value.FieldByName(goField)
		if current.IsValid() && current.Kind() == reflect.Slice && current.Len() > 0 {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	return fields
}

func hasContactPhoto(person *people.Person) bool {
	if person == nil {
		return false
	}
	for _, photo := range person.Photos {
		if photo != nil && !photo.Default {
			return true
		}
	}
	return false
}

func applyContactsDedupePlans(
	ctx context.Context,
	svc *people.Service,
	scanned int,
	plans []contactsDedupeApplyPlan,
) (contactsDedupeApplyResult, error) {
	result := contactsDedupeApplyResult{Scanned: scanned, Plans: plans}
	for index, plan := range plans {
		resource := contactsDedupeResource(plan.Merged)
		if _, err := svc.People.UpdateContact(resource, plan.Merged).
			UpdatePersonFields(strings.Join(plan.UpdateFields, ",")).
			PersonFields("metadata").
			Sources(contactsDedupeContactSource).
			Context(ctx).
			Do(); err != nil {
			return result, fmt.Errorf(
				"contacts dedupe apply stopped after %d/%d groups and %d deletions: update primary %s: %w",
				result.GroupsMerged, len(plans), result.ContactsDeleted, resource, wrapPeopleAPIError(err),
			)
		}

		for _, redundant := range plan.Delete {
			redundantResource := contactsDedupeResource(redundant)
			latest, err := svc.People.Get(redundantResource).
				PersonFields("metadata").
				Sources(contactsDedupeContactSource).
				Context(ctx).
				Do()
			if err != nil {
				return result, fmt.Errorf(
					"contacts dedupe apply stopped in group %d/%d after %d deletions: recheck %s: %w",
					index+1, len(plans), result.ContactsDeleted, redundantResource, wrapPeopleAPIError(err),
				)
			}
			if contactSourceETag(latest) != contactSourceETag(redundant) {
				return result, fmt.Errorf(
					"contacts dedupe apply stopped in group %d/%d after %d deletions: contact %s changed after preview and was not deleted; rerun the command",
					index+1, len(plans), result.ContactsDeleted, redundantResource,
				)
			}
			if _, err := svc.People.DeleteContact(redundantResource).Context(ctx).Do(); err != nil {
				return result, fmt.Errorf(
					"contacts dedupe apply stopped in group %d/%d after %d deletions: delete %s: %w",
					index+1, len(plans), result.ContactsDeleted, redundantResource, wrapPeopleAPIError(err),
				)
			}
			result.ContactsDeleted++
		}
		result.GroupsMerged++
	}
	return result, nil
}

func contactsDedupeDeleteCount(plans []contactsDedupeApplyPlan) int {
	total := 0
	for _, plan := range plans {
		total += len(plan.Delete)
	}
	return total
}

func contactsDedupeApplyAction(groups, contacts int) string {
	return fmt.Sprintf("merge %d duplicate contact group(s) and delete %d redundant contact(s)", groups, contacts)
}

func contactsDedupeApplyPayload(scanned int, plans []contactsDedupeApplyPlan) map[string]any {
	groups := make([]map[string]any, 0, len(plans))
	for _, plan := range plans {
		deleted := make([]contactsDedupeSummary, 0, len(plan.Delete))
		for _, member := range plan.Delete {
			deleted = append(deleted, summarizeContactsDedupeContact(member))
		}
		groups = append(groups, map[string]any{
			"primary":       summarizeContactsDedupeContact(plan.Group.Primary),
			"merged":        summarizeContactsDedupeContact(plan.Merged),
			"matched_on":    plan.Group.MatchedOn,
			"update_fields": plan.UpdateFields,
			"delete":        deleted,
		})
	}
	return map[string]any{
		"scanned":          scanned,
		"groups":           groups,
		"groups_merged":    len(plans),
		"contacts_deleted": contactsDedupeDeleteCount(plans),
	}
}

func writeContactsDedupeApplyResult(ctx context.Context, u *ui.UI, result contactsDedupeApplyResult) error {
	payload := contactsDedupeApplyPayload(result.Scanned, result.Plans)
	payload["applied"] = true
	payload["groups_merged"] = result.GroupsMerged
	payload["contacts_deleted"] = result.ContactsDeleted
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}
	if outfmt.IsPlain(ctx) {
		out := stdoutWriter(ctx)
		fmt.Fprintf(out, "applied\ttrue\n")
		fmt.Fprintf(out, "groups_merged\t%d\n", result.GroupsMerged)
		fmt.Fprintf(out, "contacts_deleted\t%d\n", result.ContactsDeleted)
		return nil
	}
	if u != nil {
		u.Out().Successf("Merged %d duplicate contact group(s); deleted %d redundant contact(s)", result.GroupsMerged, result.ContactsDeleted)
	}
	return nil
}
