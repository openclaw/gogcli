package cmd

import (
	"context"
	"sort"
	"strings"

	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ContactsDedupeCmd struct {
	Match     string   `name:"match" help:"Match fields: email,phone,name" default:"email,phone"`
	Max       int64    `name:"max" aliases:"limit" help:"Max contacts to scan (0 = all)" default:"0"`
	Resources []string `name:"resource" help:"Limit dedupe to exact contact resource names (people/...); repeatable"`
	Apply     bool     `name:"apply" aliases:"merge" help:"Merge duplicate groups and delete redundant contacts (requires confirmation)"`
	FailEmpty bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no duplicates"`
}

func (c *ContactsDedupeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	match, err := parseContactsDedupeMatch(c.Match)
	if err != nil {
		return err
	}
	if c.Max < 0 {
		return usage("--max must be >= 0")
	}
	resources, err := normalizeContactsDedupeResources(c.Resources)
	if err != nil {
		return err
	}
	if len(resources) > 0 && c.Max != 0 {
		return usage("--max cannot be combined with --resource")
	}

	svc, err := peopleContactsService(ctx, account)
	if err != nil {
		return err
	}
	var contacts []*people.Person
	if len(resources) > 0 {
		contacts, err = contactsDedupeGetResources(ctx, svc, resources)
	} else {
		contacts, err = contactsDedupeList(ctx, svc, c.Max)
	}
	if err != nil {
		return wrapPeopleAPIError(err)
	}

	groups := buildContactsDedupeGroups(contacts, match)
	if len(groups) == 0 {
		if writeErr := writeContactsDedupe(ctx, u, groups, len(contacts)); writeErr != nil {
			return writeErr
		}
		return failEmptyExit(c.FailEmpty)
	}
	if !c.Apply {
		return writeContactsDedupe(ctx, u, groups, len(contacts))
	}

	plans, err := prepareContactsDedupeApply(ctx, svc, groups, match)
	if err != nil {
		return err
	}
	deleteCount := contactsDedupeDeleteCount(plans)
	if u != nil {
		u.Err().Linef("Prepared %d duplicate group(s); %d redundant contact(s) will be deleted", len(plans), deleteCount)
	}
	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "contacts.dedupe.apply", contactsDedupeApplyPayload(len(contacts), plans),
		contactsDedupeApplyAction(len(plans), deleteCount)); confirmErr != nil {
		return confirmErr
	}

	result, err := applyContactsDedupePlans(ctx, svc, len(contacts), plans)
	if err != nil {
		return err
	}
	return writeContactsDedupeApplyResult(ctx, u, result)
}

type contactsDedupeMatch struct {
	Email bool
	Phone bool
	Name  bool
}

func parseContactsDedupeMatch(value string) (contactsDedupeMatch, error) {
	out := contactsDedupeMatch{}
	for _, part := range strings.Split(value, ",") {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "email":
			out.Email = true
		case "phone":
			out.Phone = true
		case "name":
			out.Name = true
		case "":
			continue
		default:
			return contactsDedupeMatch{}, usagef("invalid --match %q (use email, phone, name)", part)
		}
	}
	if !out.Email && !out.Phone && !out.Name {
		return contactsDedupeMatch{}, usage("invalid --match (no fields enabled)")
	}
	return out, nil
}

func normalizeContactsDedupeResources(values []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		resource := strings.TrimSpace(value)
		if !strings.HasPrefix(resource, "people/") || len(resource) == len("people/") {
			return nil, usagef("invalid --resource %q (expected people/...)", value)
		}
		if seen[resource] {
			continue
		}
		seen[resource] = true
		out = append(out, resource)
	}
	return out, nil
}

func contactsDedupeGetResources(ctx context.Context, svc *people.Service, resources []string) ([]*people.Person, error) {
	contacts := make([]*people.Person, 0, len(resources))
	for _, resource := range resources {
		person, err := svc.People.Get(resource).
			PersonFields(contactsReadMask).
			Sources(contactsDedupeContactSource).
			Context(ctx).
			Do()
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, person)
	}
	return contacts, nil
}

func contactsDedupeList(ctx context.Context, svc *people.Service, maxResults int64) ([]*people.Person, error) {
	var out []*people.Person
	pageToken := ""
	for {
		pageSize := int64(500)
		if maxResults > 0 && maxResults-int64(len(out)) < pageSize {
			pageSize = maxResults - int64(len(out))
		}
		resp, err := svc.People.Connections.List(peopleMeResource).
			PersonFields(contactsReadMask).
			PageSize(pageSize).
			PageToken(pageToken).
			RequestSyncToken(false).
			Sources("READ_SOURCE_TYPE_CONTACT").
			Context(ctx).
			Do()
		if err != nil {
			return nil, err
		}
		for _, p := range resp.Connections {
			if p != nil {
				out = append(out, p)
			}
			if maxResults > 0 && int64(len(out)) >= maxResults {
				return out, nil
			}
		}
		if resp.NextPageToken == "" {
			return out, nil
		}
		pageToken = resp.NextPageToken
	}
}

type contactsDedupeGroup struct {
	Primary   *people.Person
	Members   []*people.Person
	MatchedOn []string
	Merged    contactsDedupeSummary
}

type contactsDedupeSummary struct {
	Resource string   `json:"resource,omitempty"`
	Name     string   `json:"name,omitempty"`
	Emails   []string `json:"emails,omitempty"`
	Phones   []string `json:"phones,omitempty"`
}

func buildContactsDedupeGroups(contacts []*people.Person, match contactsDedupeMatch) []contactsDedupeGroup {
	if len(contacts) == 0 {
		return nil
	}
	uf := newContactsDedupeUnionFind(len(contacts))
	keyOwners := map[string]int{}
	keyCounts := map[string]int{}
	groupKeys := map[int]map[string]bool{}
	for i, p := range contacts {
		for _, key := range contactsDedupeKeys(p, match) {
			keyCounts[key]++
			if prev, ok := keyOwners[key]; ok {
				uf.union(i, prev)
			} else {
				keyOwners[key] = i
			}
		}
	}
	for key, owner := range keyOwners {
		if keyCounts[key] < 2 {
			continue
		}
		root := uf.find(owner)
		if groupKeys[root] == nil {
			groupKeys[root] = map[string]bool{}
		}
		groupKeys[root][key] = true
	}

	byRoot := map[int][]*people.Person{}
	for i, p := range contacts {
		byRoot[uf.find(i)] = append(byRoot[uf.find(i)], p)
	}

	groups := make([]contactsDedupeGroup, 0)
	for root, members := range byRoot {
		if len(members) < 2 {
			continue
		}
		primary := chooseContactsDedupePrimary(members)
		matchedOn := sortedContactsDedupeKeys(groupKeys[root])
		groups = append(groups, contactsDedupeGroup{
			Primary:   primary,
			Members:   orderContactsDedupeMembers(primary, members),
			MatchedOn: matchedOn,
			Merged:    summarizeContactsDedupeMerge(primary, members),
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		return contactsDedupeResource(groups[i].Primary) < contactsDedupeResource(groups[j].Primary)
	})
	return groups
}

func contactsDedupeKeys(p *people.Person, match contactsDedupeMatch) []string {
	var keys []string
	if p == nil {
		return keys
	}
	if match.Email {
		for _, email := range p.EmailAddresses {
			if email != nil {
				if v := normalizeContactEmail(email.Value); v != "" {
					keys = append(keys, "email:"+v)
				}
			}
		}
	}
	if match.Phone {
		for _, phone := range p.PhoneNumbers {
			if phone != nil {
				if v := normalizeContactPhone(phone.Value); v != "" {
					keys = append(keys, "phone:"+v)
				}
			}
		}
	}
	if match.Name {
		if v := normalizeContactName(primaryName(p)); v != "" {
			keys = append(keys, "name:"+v)
		}
	}
	return keys
}

func chooseContactsDedupePrimary(members []*people.Person) *people.Person {
	var best *people.Person
	bestScore := -1
	for _, p := range members {
		score := contactsDedupeScore(p)
		if best == nil || score > bestScore || score == bestScore && contactsDedupeResource(p) < contactsDedupeResource(best) {
			best = p
			bestScore = score
		}
	}
	return best
}

func contactsDedupeScore(p *people.Person) int {
	if p == nil {
		return 0
	}
	score := 0
	if primaryName(p) != "" {
		score += 2
	}
	score += len(p.EmailAddresses) * 2
	score += len(p.PhoneNumbers) * 2
	if len(p.Organizations) > 0 {
		score++
	}
	if len(p.Urls) > 0 {
		score++
	}
	return score
}

func summarizeContactsDedupeMerge(primary *people.Person, members []*people.Person) contactsDedupeSummary {
	ordered := orderContactsDedupeMembers(primary, members)
	return contactsDedupeSummary{
		Resource: contactsDedupeResource(primary),
		Name:     firstContactsDedupeName(primary, ordered),
		Emails:   uniqueContactsDedupeEmails(ordered),
		Phones:   uniqueContactsDedupePhones(ordered),
	}
}

func writeContactsDedupe(ctx context.Context, u *ui.UI, groups []contactsDedupeGroup, scanned int) error {
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"scanned": scanned,
			"groups":  contactsDedupeGroupsJSON(groups),
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}
	if len(groups) == 0 {
		if u != nil {
			u.Err().Linef("No duplicate contacts found (scanned %d)", scanned)
		}
		return nil
	}
	return outfmt.WriteTable(ctx, stdoutWriter(ctx), contactsDedupeRows(groups), contactsDedupeColumns())
}

func contactsDedupeGroupsJSON(groups []contactsDedupeGroup) []map[string]any {
	out := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		members := make([]contactsDedupeSummary, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, summarizeContactsDedupeContact(member))
		}
		out = append(out, map[string]any{
			"primary":    summarizeContactsDedupeContact(group.Primary),
			"merged":     group.Merged,
			"matched_on": group.MatchedOn,
			"members":    members,
		})
	}
	return out
}

func summarizeContactsDedupeContact(p *people.Person) contactsDedupeSummary {
	if p == nil {
		return contactsDedupeSummary{}
	}
	return contactsDedupeSummary{
		Resource: p.ResourceName,
		Name:     primaryName(p),
		Emails:   uniqueContactsDedupeEmails([]*people.Person{p}),
		Phones:   uniqueContactsDedupePhones([]*people.Person{p}),
	}
}

func orderContactsDedupeMembers(primary *people.Person, members []*people.Person) []*people.Person {
	out := make([]*people.Person, 0, len(members))
	if primary != nil {
		out = append(out, primary)
	}
	for _, member := range members {
		if member == nil || contactsDedupeResource(member) == contactsDedupeResource(primary) {
			continue
		}
		out = append(out, member)
	}
	return out
}

func firstContactsDedupeName(primary *people.Person, members []*people.Person) string {
	if name := primaryName(primary); name != "" {
		return name
	}
	for _, member := range members {
		if name := primaryName(member); name != "" {
			return name
		}
	}
	return ""
}

func uniqueContactsDedupeEmails(members []*people.Person) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range members {
		if p == nil {
			continue
		}
		for _, email := range p.EmailAddresses {
			if email == nil {
				continue
			}
			if key := normalizeContactEmail(email.Value); key != "" && !seen[key] {
				seen[key] = true
				out = append(out, strings.TrimSpace(email.Value))
			}
		}
	}
	return out
}

func uniqueContactsDedupePhones(members []*people.Person) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range members {
		if p == nil {
			continue
		}
		for _, phone := range p.PhoneNumbers {
			if phone == nil {
				continue
			}
			if key := normalizeContactPhone(phone.Value); key != "" && !seen[key] {
				seen[key] = true
				out = append(out, strings.TrimSpace(phone.Value))
			}
		}
	}
	return out
}

func normalizeContactEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeContactPhone(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizeContactName(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func contactsDedupeResource(p *people.Person) string {
	if p == nil {
		return ""
	}
	return p.ResourceName
}

func sortedContactsDedupeKeys(keys map[string]bool) []string {
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

type contactsDedupeUnionFind struct {
	parent []int
	rank   []int
}

func newContactsDedupeUnionFind(n int) *contactsDedupeUnionFind {
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	return &contactsDedupeUnionFind{parent: parent, rank: make([]int, n)}
}

func (u *contactsDedupeUnionFind) find(x int) int {
	if u.parent[x] != x {
		u.parent[x] = u.find(u.parent[x])
	}
	return u.parent[x]
}

func (u *contactsDedupeUnionFind) union(a int, b int) {
	ra := u.find(a)
	rb := u.find(b)
	if ra == rb {
		return
	}
	if u.rank[ra] < u.rank[rb] {
		u.parent[ra] = rb
		return
	}
	if u.rank[ra] > u.rank[rb] {
		u.parent[rb] = ra
		return
	}
	u.parent[rb] = ra
	u.rank[ra]++
}
