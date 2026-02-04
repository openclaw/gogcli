package cmd

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/groupssettings/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newGroupsSettings = googleapi.NewGroupsSettings

// Admin group management

type GroupsCreateCmd struct {
	Email       string `arg:"" name:"email" help:"Group email address"`
	Name        string `name:"name" required:"" help:"Display name"`
	Description string `name:"description" help:"Description"`
}

func (c *GroupsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	req := &admin.Group{
		Email:       c.Email,
		Name:        c.Name,
		Description: c.Description,
	}
	created, err := svc.Groups.Insert(req).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create group %s: %w", c.Email, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created group: %s\n", created.Email)
	return nil
}

type GroupsUpdateCmd struct {
	Group       string  `arg:"" name:"group" help:"Group email or ID"`
	Name        *string `name:"name" help:"New display name"`
	Description *string `name:"description" help:"Description"`
}

func (c *GroupsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	group := &admin.Group{}
	hasUpdates := false
	if c.Name != nil {
		group.Name = *c.Name
		hasUpdates = true
	}
	if c.Description != nil {
		group.Description = *c.Description
		if *c.Description == "" {
			group.ForceSendFields = append(group.ForceSendFields, "Description")
		}
		hasUpdates = true
	}
	if !hasUpdates {
		return usage("no updates specified")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	updated, err := svc.Groups.Update(c.Group, group).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update group %s: %w", c.Group, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u.Out().Printf("Updated group: %s\n", updated.Email)
	return nil
}

type GroupsDeleteCmd struct {
	Group string `arg:"" name:"group" help:"Group email or ID"`
}

func (c *GroupsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err = confirmDestructive(ctx, flags, fmt.Sprintf("delete group %s", c.Group)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Groups.Delete(c.Group).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete group %s: %w", c.Group, err)
	}

	u.Out().Printf("Deleted group: %s\n", c.Group)
	return nil
}

type GroupsSettingsCmd struct {
	Group             string  `arg:"" name:"group" help:"Group email"`
	WhoCanJoin        *string `name:"who-can-join" help:"Who can join (e.g., ANYONE_CAN_JOIN, INVITED_CAN_JOIN, CAN_REQUEST_TO_JOIN)"`
	WhoCanPost        *string `name:"who-can-post" help:"Who can post (e.g., ANYONE_CAN_POST, ALL_IN_DOMAIN_CAN_POST, OWNERS_ONLY, NONE_CAN_POST)"`
	WhoCanViewGroup   *string `name:"who-can-view-group" help:"Who can view group (e.g., ANYONE_CAN_VIEW, ALL_IN_DOMAIN_CAN_VIEW, ALL_MEMBERS_CAN_VIEW)"`
	WhoCanViewMembers *string `name:"who-can-view-membership" help:"Who can view membership (e.g., ALL_IN_DOMAIN_CAN_VIEW, ALL_MEMBERS_CAN_VIEW, OWNERS_ONLY)"`
}

func (c *GroupsSettingsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGroupsSettings(ctx, account)
	if err != nil {
		return err
	}

	hasUpdates := c.WhoCanJoin != nil || c.WhoCanPost != nil || c.WhoCanViewGroup != nil || c.WhoCanViewMembers != nil
	if !hasUpdates {
		settings, getErr := svc.Groups.Get(c.Group).Context(ctx).Do()
		if getErr != nil {
			return fmt.Errorf("get group settings %s: %w", c.Group, getErr)
		}
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(os.Stdout, settings)
		}
		u.Out().Printf("Group: %s\n", settings.Email)
		u.Out().Printf("WhoCanJoin:        %s\n", settings.WhoCanJoin)
		u.Out().Printf("WhoCanPostMessage: %s\n", settings.WhoCanPostMessage)
		u.Out().Printf("WhoCanViewGroup:   %s\n", settings.WhoCanViewGroup)
		u.Out().Printf("WhoCanViewMembers: %s\n", settings.WhoCanViewMembership)
		return nil
	}

	req := &groupssettings.Groups{}
	if c.WhoCanJoin != nil {
		req.WhoCanJoin = *c.WhoCanJoin
		req.ForceSendFields = append(req.ForceSendFields, "WhoCanJoin")
	}
	if c.WhoCanPost != nil {
		req.WhoCanPostMessage = *c.WhoCanPost
		req.ForceSendFields = append(req.ForceSendFields, "WhoCanPostMessage")
	}
	if c.WhoCanViewGroup != nil {
		req.WhoCanViewGroup = *c.WhoCanViewGroup
		req.ForceSendFields = append(req.ForceSendFields, "WhoCanViewGroup")
	}
	if c.WhoCanViewMembers != nil {
		req.WhoCanViewMembership = *c.WhoCanViewMembers
		req.ForceSendFields = append(req.ForceSendFields, "WhoCanViewMembership")
	}

	updated, err := svc.Groups.Update(c.Group, req).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update group settings %s: %w", c.Group, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u.Out().Printf("Updated settings for group: %s\n", updated.Email)
	return nil
}

type GroupsMembersAddCmd struct {
	Group string `arg:"" name:"group" help:"Group email or ID"`
	Email string `arg:"" name:"email" help:"Member email"`
	Role  string `name:"role" default:"MEMBER" enum:"MEMBER,MANAGER,OWNER" help:"Member role"`
}

func (c *GroupsMembersAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	role, err := normalizeGroupRole(c.Role)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	member := &admin.Member{Email: c.Email, Role: role}
	created, err := svc.Members.Insert(c.Group, member).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("add member %s to %s: %w", c.Email, c.Group, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Added %s to %s as %s\n", created.Email, c.Group, created.Role)
	return nil
}

type GroupsMembersRemoveCmd struct {
	Group string `arg:"" name:"group" help:"Group email or ID"`
	Email string `arg:"" name:"email" help:"Member email"`
}

func (c *GroupsMembersRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err = confirmDestructive(ctx, flags, fmt.Sprintf("remove %s from %s", c.Email, c.Group)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Members.Delete(c.Group, c.Email).Context(ctx).Do(); err != nil {
		return fmt.Errorf("remove member %s from %s: %w", c.Email, c.Group, err)
	}

	u.Out().Printf("Removed %s from %s\n", c.Email, c.Group)
	return nil
}

type GroupsMembersSyncCmd struct {
	Group string `arg:"" name:"group" help:"Group email or ID"`
	File  string `name:"file" required:"" help:"CSV file with member emails"`
}

func (c *GroupsMembersSyncCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	desiredEmails, err := readCSVEmails(c.File)
	if err != nil {
		return err
	}
	if len(desiredEmails) == 0 {
		return usage("no member emails found in CSV")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	currentMembers, err := listGroupMembers(ctx, svc, c.Group)
	if err != nil {
		return err
	}

	desired := make(map[string]struct{}, len(desiredEmails))
	for _, email := range desiredEmails {
		desired[strings.ToLower(email)] = struct{}{}
	}

	toAdd := make([]string, 0)
	for email := range desired {
		if _, ok := currentMembers[email]; !ok {
			toAdd = append(toAdd, email)
		}
	}
	toRemove := make([]string, 0)
	for email := range currentMembers {
		if _, ok := desired[email]; !ok {
			toRemove = append(toRemove, email)
		}
	}

	sort.Strings(toAdd)
	sort.Strings(toRemove)

	if len(toAdd) == 0 && len(toRemove) == 0 {
		u.Out().Printf("Group %s already in sync (%d members)\n", c.Group, len(currentMembers))
		return nil
	}

	if len(toRemove) > 0 {
		if err := confirmDestructive(ctx, flags, fmt.Sprintf("sync members for %s (add %d, remove %d)", c.Group, len(toAdd), len(toRemove))); err != nil {
			return err
		}
	}

	for _, email := range toAdd {
		member := &admin.Member{Email: email, Role: "MEMBER"}
		if _, err := svc.Members.Insert(c.Group, member).Context(ctx).Do(); err != nil {
			return fmt.Errorf("add member %s: %w", email, err)
		}
	}
	for _, email := range toRemove {
		if err := svc.Members.Delete(c.Group, email).Context(ctx).Do(); err != nil {
			return fmt.Errorf("remove member %s: %w", email, err)
		}
	}

	u.Out().Printf("Synced group %s: added %d, removed %d\n", c.Group, len(toAdd), len(toRemove))
	return nil
}

func normalizeGroupRole(role string) (string, error) {
	role = strings.TrimSpace(strings.ToUpper(role))
	switch role {
	case "MEMBER", "MANAGER", "OWNER":
		return role, nil
	default:
		return "", usage("invalid role (expected MEMBER, MANAGER, OWNER)")
	}
}

func readCSVEmails(path string) ([]string, error) {
	f, err := os.Open(path) //nolint:gosec // G304: user-provided file path is intentional
	if err != nil {
		return nil, fmt.Errorf("open CSV: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read CSV: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	header := records[0]
	idx := findEmailColumn(header)
	start := 0
	if idx >= 0 {
		start = 1
	} else {
		idx = 0
	}

	seen := make(map[string]struct{})
	out := make([]string, 0, len(records)-start)
	for _, row := range records[start:] {
		if idx >= len(row) {
			continue
		}
		email := strings.TrimSpace(row[idx])
		if email == "" {
			continue
		}
		key := strings.ToLower(email)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, email)
	}
	return out, nil
}

func findEmailColumn(header []string) int {
	for i, col := range header {
		name := strings.TrimSpace(strings.ToLower(col))
		switch name {
		case "email", "emailaddress", "member", "member_email":
			return i
		}
	}
	return -1
}

func listGroupMembers(ctx context.Context, svc *admin.Service, group string) (map[string]struct{}, error) {
	members := make(map[string]struct{})
	call := svc.Members.List(group).MaxResults(200)
	for {
		resp, err := call.Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("list members for %s: %w", group, err)
		}
		for _, member := range resp.Members {
			if member == nil || member.Email == "" {
				continue
			}
			members[strings.ToLower(member.Email)] = struct{}{}
		}
		if resp.NextPageToken == "" {
			break
		}
		call = call.PageToken(resp.NextPageToken)
	}
	return members, nil
}
