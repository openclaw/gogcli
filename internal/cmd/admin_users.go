package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	admin "google.golang.org/api/admin/directory/v1"
	ggoogleapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// AdminUsersCmd manages Workspace users.
type AdminUsersCmd struct {
	List    AdminUsersListCmd    `cmd:"" name:"list" aliases:"ls" help:"List users in a domain"`
	Get     AdminUsersGetCmd     `cmd:"" name:"get" aliases:"info,show" help:"Get user details"`
	Create  AdminUsersCreateCmd  `cmd:"" name:"create" aliases:"add,new" help:"Create a new user"`
	Delete  AdminUsersDeleteCmd  `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a user account"`
	Suspend AdminUsersSuspendCmd `cmd:"" name:"suspend" help:"Suspend a user account"`
}

type AdminUsersListCmd struct {
	Domain    string `name:"domain" help:"Domain to list users from (e.g., example.com)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AdminUsersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	domain := strings.TrimSpace(c.Domain)
	if domain == "" {
		return usage("domain required (e.g., --domain example.com)")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	svc, err := adminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	fetch := func(pageToken string) ([]*admin.User, string, error) {
		call := svc.Users.List().
			Domain(domain).
			MaxResults(c.Max).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, fetchErr := call.Do()
		if fetchErr != nil {
			return nil, "", wrapAdminDirectoryError(fetchErr, account)
		}
		return resp.Users, resp.NextPageToken, nil
	}

	users, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Email     string `json:"email"`
			Name      string `json:"name,omitempty"`
			Suspended bool   `json:"suspended"`
			Admin     bool   `json:"admin"`
		}
		items := make([]item, 0, len(users))
		for _, user := range users {
			if user == nil {
				continue
			}
			name := ""
			if user.Name != nil {
				name = user.Name.FullName
			}
			items = append(items, item{
				Email:     user.PrimaryEmail,
				Name:      name,
				Suspended: user.Suspended,
				Admin:     user.IsAdmin,
			})
		}
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"users":         items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(users) == 0 {
		u.Err().Println("No users found")
		return failEmptyExit(c.FailEmpty)
	}

	if err := outfmt.WriteTable(ctx, stdoutWriter(ctx), nonNilAdminRows(users), adminUserColumns()); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type AdminUsersGetCmd struct {
	UserEmail string `arg:"" name:"userEmail" help:"User email (e.g., user@example.com)"`
}

func (c *AdminUsersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	userEmail := strings.TrimSpace(c.UserEmail)
	if userEmail == "" {
		return usage("user email required")
	}

	svc, err := adminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	user, err := svc.Users.Get(userEmail).Context(ctx).Do()
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Email       string   `json:"email"`
			Name        string   `json:"name,omitempty"`
			GivenName   string   `json:"givenName,omitempty"`
			FamilyName  string   `json:"familyName,omitempty"`
			Suspended   bool     `json:"suspended"`
			Admin       bool     `json:"admin"`
			Aliases     []string `json:"aliases,omitempty"`
			OrgUnitPath string   `json:"orgUnitPath,omitempty"`
			Creation    string   `json:"creationTime,omitempty"`
			LastLogin   string   `json:"lastLoginTime,omitempty"`
		}
		var aliases []string
		if user.Aliases != nil {
			aliases = user.Aliases
		}
		name := ""
		givenName := ""
		familyName := ""
		if user.Name != nil {
			name = user.Name.FullName
			givenName = user.Name.GivenName
			familyName = user.Name.FamilyName
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), item{
			Email:       user.PrimaryEmail,
			Name:        name,
			GivenName:   givenName,
			FamilyName:  familyName,
			Suspended:   user.Suspended,
			Admin:       user.IsAdmin,
			Aliases:     aliases,
			OrgUnitPath: user.OrgUnitPath,
			Creation:    user.CreationTime,
			LastLogin:   user.LastLoginTime,
		})
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintf(w, "Email:\t%s\n", user.PrimaryEmail)
	if user.Name != nil {
		fmt.Fprintf(w, "Name:\t%s\n", user.Name.FullName)
		fmt.Fprintf(w, "Given Name:\t%s\n", user.Name.GivenName)
		fmt.Fprintf(w, "Family Name:\t%s\n", user.Name.FamilyName)
	}
	fmt.Fprintf(w, "Suspended:\t%t\n", user.Suspended)
	fmt.Fprintf(w, "Admin:\t%t\n", user.IsAdmin)
	fmt.Fprintf(w, "Org Unit:\t%s\n", user.OrgUnitPath)
	fmt.Fprintf(w, "Created:\t%s\n", user.CreationTime)
	fmt.Fprintf(w, "Last Login:\t%s\n", user.LastLoginTime)
	if len(user.Aliases) > 0 {
		fmt.Fprintf(w, "Aliases:\t%s\n", strings.Join(user.Aliases, ", "))
	}
	return nil
}

type AdminUsersCreateCmd struct {
	Email         string `arg:"" name:"email" help:"User email (e.g., user@example.com)"`
	GivenName     string `name:"given" aliases:"first-name,given-name,fn" help:"Given (first) name"`
	FamilyName    string `name:"family" aliases:"last-name,family-name,ln" help:"Family (last) name"`
	Password      string `name:"password" aliases:"pass" help:"Initial password (generated if omitted)"`
	ChangePwd     bool   `name:"change-password" help:"Require password change on first login"`
	OrgUnit       string `name:"org-unit" aliases:"ou" help:"Organization unit path"`
	Suspended     bool   `name:"suspended" help:"Create user in suspended state"`
	Archived      bool   `name:"archived" help:"Create user in archived state"`
	RecoveryEmail string `name:"recovery-email" help:"Recovery email address"`
	RecoveryPhone string `name:"recovery-phone" help:"Recovery phone number in E.164 format"`
	HashFunction  string `name:"hash-function" help:"Password hash function when --password is pre-hashed (MD5, SHA-1, crypt)"`
	Admin         bool   `name:"admin" help:"Not supported; assign admin roles separately after user creation"`
}

func (c *AdminUsersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	plan, err := newAdminUserCreatePlan(adminUserCreateInput{
		Email:         c.Email,
		GivenName:     c.GivenName,
		FamilyName:    c.FamilyName,
		Password:      c.Password,
		ChangePwd:     c.ChangePwd,
		OrgUnit:       c.OrgUnit,
		Suspended:     c.Suspended,
		Archived:      c.Archived,
		RecoveryEmail: c.RecoveryEmail,
		RecoveryPhone: c.RecoveryPhone,
		HashFunction:  c.HashFunction,
		Admin:         c.Admin,
	})
	if err != nil {
		return err
	}
	if dryRunErr := dryRunExit(ctx, flags, "admin.users.create", plan.dryRunPayload()); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	password := plan.Password
	if plan.GeneratePassword {
		password, err = generateAdminUserPassword(16)
		if err != nil {
			return fmt.Errorf("generate password: %w", err)
		}
	}

	svc, err := adminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	created, err := svc.Users.Insert(plan.insertRequest(password)).Context(ctx).Do()
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}
	if plan.StatePatch != nil {
		userKey := created.PrimaryEmail
		if strings.TrimSpace(userKey) == "" {
			userKey = plan.Email
		}
		updated, patchErr := patchAdminUserState(ctx, svc, userKey, plan.StatePatch)
		if patchErr != nil {
			return wrapAdminDirectoryError(patchErr, account)
		}
		created = updated
	}

	if outfmt.IsJSON(ctx) {
		result := map[string]any{
			"email":     created.PrimaryEmail,
			"id":        created.Id,
			"suspended": created.Suspended,
			"archived":  created.Archived,
		}
		if plan.GeneratePassword {
			result["generatedPassword"] = password
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("Created user: %s (ID: %s)", created.PrimaryEmail, created.Id)
	if plan.GeneratePassword {
		u.Out().Linef("Generated password: %s", password)
	}
	return nil
}

func patchAdminUserState(ctx context.Context, svc *admin.Service, userKey string, patch *admin.User) (*admin.User, error) {
	const attempts = 6
	var lastErr error
	for attempt := range attempts {
		updated, err := svc.Users.Patch(userKey, patch).Context(ctx).Do()
		if err == nil {
			return updated, nil
		}
		lastErr = err
		var googleErr *ggoogleapi.Error
		if !errors.As(err, &googleErr) || googleErr.Code != 404 || attempt == attempts-1 {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil, lastErr
}

type AdminUsersDeleteCmd struct {
	UserEmail string `arg:"" name:"userEmail" help:"User email to delete"`
}

func (c *AdminUsersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	userEmail := strings.TrimSpace(c.UserEmail)
	if userEmail == "" {
		return usage("user email required")
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "admin.users.delete", map[string]any{
		"email": userEmail,
	}, fmt.Sprintf("delete user %s", userEmail)); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	svc, err := adminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	if err := svc.Users.Delete(userEmail).Context(ctx).Do(); err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"email":   userEmail,
			"deleted": true,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("Deleted user: %s", userEmail)
	return nil
}

type AdminUsersSuspendCmd struct {
	UserEmail string `arg:"" name:"userEmail" help:"User email to suspend"`
}

func (c *AdminUsersSuspendCmd) Run(ctx context.Context, flags *RootFlags) error {
	userEmail := strings.TrimSpace(c.UserEmail)
	if userEmail == "" {
		return usage("user email required")
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "admin.users.suspend", map[string]any{
		"email":     userEmail,
		"suspended": true,
	}, fmt.Sprintf("suspend user %s", userEmail)); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	svc, err := adminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	updated, err := svc.Users.Update(userEmail, &admin.User{Suspended: true}).Context(ctx).Do()
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"email":     updated.PrimaryEmail,
			"suspended": updated.Suspended,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("Suspended user: %s", updated.PrimaryEmail)
	return nil
}
