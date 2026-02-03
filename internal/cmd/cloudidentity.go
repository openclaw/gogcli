package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/api/cloudidentity/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newCloudIdentityAdminService = googleapi.NewCloudIdentity

func cloudIdentityParent() string {
	if id := os.Getenv("GOG_CUSTOMER_ID"); id != "" {
		return "customers/" + id
	}
	return "customers/my_customer"
}

type CloudIdentityCmd struct {
	Groups   CloudIdentityGroupsCmd   `cmd:"" name:"groups" help:"Cloud Identity groups"`
	Members  CloudIdentityMembersCmd  `cmd:"" name:"members" help:"Cloud Identity group members"`
	Policies CloudIdentityPoliciesCmd `cmd:"" name:"policies" help:"Cloud Identity policies"`
}

type CloudIdentityGroupsCmd struct {
	List   CloudIdentityGroupsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List Cloud Identity groups"`
	Get    CloudIdentityGroupsGetCmd    `cmd:"" name:"get" help:"Get a Cloud Identity group"`
	Create CloudIdentityGroupsCreateCmd `cmd:"" name:"create" aliases:"add" help:"Create a Cloud Identity group"`
	Update CloudIdentityGroupsUpdateCmd `cmd:"" name:"update" help:"Update a Cloud Identity group"`
	Delete CloudIdentityGroupsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete a Cloud Identity group"`
}

type CloudIdentityGroupsListCmd struct {
	Parent string `name:"parent" help:"Customer parent (default: customers/my_customer, override with GOG_CUSTOMER_ID env var)"`
	Max    int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page   string `name:"page" help:"Page token"`
}

func (c *CloudIdentityGroupsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newCloudIdentityAdminService(ctx, account)
	if err != nil {
		return err
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		parent = cloudIdentityParent()
	}

	call := svc.Groups.List().Parent(parent).PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Groups) == 0 {
		u.Err().Println("No groups found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "EMAIL\tNAME\tRESOURCE")
	for _, group := range resp.Groups {
		if group == nil {
			continue
		}
		email := ""
		if group.GroupKey != nil {
			email = group.GroupKey.Id
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(email),
			sanitizeTab(group.DisplayName),
			sanitizeTab(group.Name),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type CloudIdentityGroupsGetCmd struct {
	Group string `arg:"" name:"group" help:"Group email or resource name"`
}

func (c *CloudIdentityGroupsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	groupKey := strings.TrimSpace(c.Group)
	if groupKey == "" {
		return usage("group is required")
	}

	svc, err := newCloudIdentityAdminService(ctx, account)
	if err != nil {
		return err
	}

	groupName, err := resolveGroupName(ctx, svc, groupKey)
	if err != nil {
		return err
	}

	group, err := svc.Groups.Get(groupName).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get group %s: %w", groupKey, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, group)
	}

	fmt.Fprintf(os.Stdout, "Name:        %s\n", group.Name)
	if group.GroupKey != nil {
		fmt.Fprintf(os.Stdout, "Email:       %s\n", group.GroupKey.Id)
	}
	if group.DisplayName != "" {
		fmt.Fprintf(os.Stdout, "Display Name: %s\n", group.DisplayName)
	}
	if group.Parent != "" {
		fmt.Fprintf(os.Stdout, "Parent:      %s\n", group.Parent)
	}
	if len(group.Labels) > 0 {
		labels := make([]string, 0, len(group.Labels))
		for key := range group.Labels {
			labels = append(labels, key)
		}
		sort.Strings(labels)
		fmt.Fprintf(os.Stdout, "Labels:      %s\n", strings.Join(labels, ", "))
	}
	return nil
}

type CloudIdentityGroupsCreateCmd struct {
	Email        string `name:"email" help:"Group email" required:""`
	DisplayName  string `name:"display-name" help:"Display name"`
	Parent       string `name:"parent" help:"Customer parent (default: customers/my_customer, override with GOG_CUSTOMER_ID env var)"`
	DynamicQuery string `name:"dynamic-query" help:"Dynamic group membership query"`
}

func (c *CloudIdentityGroupsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("--email is required")
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		parent = cloudIdentityParent()
	}

	svc, err := newCloudIdentityAdminService(ctx, account)
	if err != nil {
		return err
	}

	display := strings.TrimSpace(c.DisplayName)
	if display == "" {
		display = email
	}

	labels := map[string]string{"cloudidentity.googleapis.com/groups.discussion_forum": ""}
	group := &cloudidentity.Group{
		Parent:      parent,
		GroupKey:    &cloudidentity.EntityKey{Id: email},
		DisplayName: display,
		Labels:      labels,
	}
	if strings.TrimSpace(c.DynamicQuery) != "" {
		group.DynamicGroupMetadata = &cloudidentity.DynamicGroupMetadata{
			Queries: []*cloudidentity.DynamicGroupQuery{{
				Query:        strings.TrimSpace(c.DynamicQuery),
				ResourceType: "USER",
			}},
		}
		group.Labels = map[string]string{"cloudidentity.googleapis.com/groups.dynamic": ""}
	}

	op, err := svc.Groups.Create(group).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create group: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Created group %s\n", email)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

type CloudIdentityGroupsUpdateCmd struct {
	Group       string `arg:"" name:"group" help:"Group email or resource name"`
	DisplayName string `name:"display-name" help:"New display name"`
}

func (c *CloudIdentityGroupsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	groupKey := strings.TrimSpace(c.Group)
	if groupKey == "" {
		return usage("group is required")
	}
	if strings.TrimSpace(c.DisplayName) == "" {
		return usage("--display-name is required")
	}

	svc, err := newCloudIdentityAdminService(ctx, account)
	if err != nil {
		return err
	}

	groupName, err := resolveGroupName(ctx, svc, groupKey)
	if err != nil {
		return err
	}

	patch := &cloudidentity.Group{DisplayName: strings.TrimSpace(c.DisplayName)}
	op, err := svc.Groups.Patch(groupName, patch).UpdateMask("displayName").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update group %s: %w", groupKey, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Updated group %s\n", groupKey)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

type CloudIdentityGroupsDeleteCmd struct {
	Group string `arg:"" name:"group" help:"Group email or resource name"`
}

func (c *CloudIdentityGroupsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	groupKey := strings.TrimSpace(c.Group)
	if groupKey == "" {
		return usage("group is required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete group %s", groupKey)); err != nil {
		return err
	}

	svc, err := newCloudIdentityAdminService(ctx, account)
	if err != nil {
		return err
	}

	groupName, err := resolveGroupName(ctx, svc, groupKey)
	if err != nil {
		return err
	}

	op, err := svc.Groups.Delete(groupName).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("delete group %s: %w", groupKey, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Deleted group %s\n", groupKey)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

type CloudIdentityMembersCmd struct {
	List   CloudIdentityMembersListCmd   `cmd:"" name:"list" aliases:"ls" help:"List group members"`
	Add    CloudIdentityMembersAddCmd    `cmd:"" name:"add" help:"Add a member to a group"`
	Remove CloudIdentityMembersRemoveCmd `cmd:"" name:"remove" aliases:"rm" help:"Remove a member from a group"`
}

type CloudIdentityMembersListCmd struct {
	Group string `arg:"" name:"group" help:"Group email or resource name"`
	Max   int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page  string `name:"page" help:"Page token"`
}

func (c *CloudIdentityMembersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	groupKey := strings.TrimSpace(c.Group)
	if groupKey == "" {
		return usage("group is required")
	}

	svc, err := newCloudIdentityAdminService(ctx, account)
	if err != nil {
		return err
	}

	groupName, err := resolveGroupName(ctx, svc, groupKey)
	if err != nil {
		return err
	}

	call := svc.Groups.Memberships.List(groupName).PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list members: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Memberships) == 0 {
		u.Err().Println("No members found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "MEMBER\tROLE\tRESOURCE")
	for _, membership := range resp.Memberships {
		if membership == nil {
			continue
		}
		memberEmail := ""
		if membership.PreferredMemberKey != nil {
			memberEmail = membership.PreferredMemberKey.Id
		}
		roles := membershipRoleNames(membership.Roles)
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(memberEmail),
			sanitizeTab(strings.Join(roles, ",")),
			sanitizeTab(membership.Name),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type CloudIdentityMembersAddCmd struct {
	Group string `arg:"" name:"group" help:"Group email or resource name"`
	Email string `name:"email" help:"Member email" required:""`
	Role  string `name:"role" default:"MEMBER" enum:"MEMBER,MANAGER,OWNER" help:"Member role"`
}

func (c *CloudIdentityMembersAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	groupKey := strings.TrimSpace(c.Group)
	if groupKey == "" {
		return usage("group is required")
	}
	memberEmail := strings.TrimSpace(c.Email)
	if memberEmail == "" {
		return usage("--email is required")
	}

	svc, err := newCloudIdentityAdminService(ctx, account)
	if err != nil {
		return err
	}

	groupName, err := resolveGroupName(ctx, svc, groupKey)
	if err != nil {
		return err
	}

	membership := &cloudidentity.Membership{
		PreferredMemberKey: &cloudidentity.EntityKey{Id: memberEmail},
		Roles:              []*cloudidentity.MembershipRole{{Name: c.Role}},
	}
	op, err := svc.Groups.Memberships.Create(groupName, membership).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("add member %s: %w", memberEmail, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Added %s to %s as %s\n", memberEmail, groupKey, c.Role)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

type CloudIdentityMembersRemoveCmd struct {
	Group string `arg:"" name:"group" help:"Group email or resource name"`
	Email string `name:"email" help:"Member email" required:""`
}

func (c *CloudIdentityMembersRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	groupKey := strings.TrimSpace(c.Group)
	if groupKey == "" {
		return usage("group is required")
	}
	memberEmail := strings.TrimSpace(c.Email)
	if memberEmail == "" {
		return usage("--email is required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove %s from %s", memberEmail, groupKey)); err != nil {
		return err
	}

	svc, err := newCloudIdentityAdminService(ctx, account)
	if err != nil {
		return err
	}

	groupName, err := resolveGroupName(ctx, svc, groupKey)
	if err != nil {
		return err
	}

	membershipName, err := resolveMembershipName(ctx, svc, groupName, memberEmail)
	if err != nil {
		return err
	}

	op, err := svc.Groups.Memberships.Delete(membershipName).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("remove member %s: %w", memberEmail, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Removed %s from %s\n", memberEmail, groupKey)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

type CloudIdentityPoliciesCmd struct {
	List CloudIdentityPoliciesListCmd `cmd:"" name:"list" aliases:"ls" help:"List Cloud Identity policies"`
}

type CloudIdentityPoliciesListCmd struct {
	Filter string `name:"filter" help:"Filter expression"`
	Max    int64  `name:"max" aliases:"limit" default:"50" help:"Max results"`
	Page   string `name:"page" help:"Page token"`
}

func (c *CloudIdentityPoliciesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newCloudIdentityAdminService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Policies.List().PageSize(c.Max)
	if strings.TrimSpace(c.Filter) != "" {
		call = call.Filter(strings.TrimSpace(c.Filter))
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list policies: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Policies) == 0 {
		u.Err().Println("No policies found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tSETTING\tTYPE")
	for _, policy := range resp.Policies {
		if policy == nil {
			continue
		}
		settingType := ""
		if policy.Setting != nil {
			settingType = policy.Setting.Type
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(policy.Name),
			sanitizeTab(settingType),
			sanitizeTab(policy.Type),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

func resolveGroupName(ctx context.Context, svc *cloudidentity.Service, groupKey string) (string, error) {
	key := strings.TrimSpace(groupKey)
	if key == "" {
		return "", fmt.Errorf("group key is required")
	}
	if strings.HasPrefix(key, "groups/") {
		return key, nil
	}
	lookup, err := svc.Groups.Lookup().GroupKeyId(key).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("lookup group %s: %w", key, err)
	}
	if lookup.Name == "" {
		return "", fmt.Errorf("lookup group %s: empty response", key)
	}
	return lookup.Name, nil
}

func resolveMembershipName(ctx context.Context, svc *cloudidentity.Service, groupName, memberEmail string) (string, error) {
	lookup, err := svc.Groups.Memberships.Lookup(groupName).MemberKeyId(memberEmail).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("lookup membership %s: %w", memberEmail, err)
	}
	if lookup.Name == "" {
		return "", fmt.Errorf("lookup membership %s: empty response", memberEmail)
	}
	return lookup.Name, nil
}

func membershipRoleNames(roles []*cloudidentity.MembershipRole) []string {
	if len(roles) == 0 {
		return nil
	}
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		if role == nil || role.Name == "" {
			continue
		}
		out = append(out, role.Name)
	}
	if len(out) == 0 {
		return out
	}
	sort.Strings(out)
	return out
}
