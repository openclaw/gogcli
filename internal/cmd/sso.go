package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/cloudidentity/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const ssoModeOff = "SSO_OFF"

var newInboundSSOService = googleapi.NewCloudIdentityInboundSSO

type SSOCmd struct {
	Settings    SSOSettingsCmd    `cmd:"" name:"settings" help:"Inbound SSO settings"`
	Assignments SSOAssignmentsCmd `cmd:"" name:"assignments" help:"Inbound SSO assignments"`
}

type SSOSettingsCmd struct {
	Get    SSOSettingsGetCmd    `cmd:"" name:"get" help:"Get inbound SSO settings"`
	Update SSOSettingsUpdateCmd `cmd:"" name:"update" help:"Update inbound SSO settings"`
}

type SSOSettingsGetCmd struct{}

func (c *SSOSettingsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newInboundSSOService(ctx, account)
	if err != nil {
		return err
	}

	profile, err := firstInboundSamlProfile(ctx, svc)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, profile)
	}

	u.Out().Printf("Profile:           %s\n", profile.Name)
	if profile.DisplayName != "" {
		u.Out().Printf("Display Name:      %s\n", profile.DisplayName)
	}
	if profile.IdpConfig != nil {
		if profile.IdpConfig.EntityId != "" {
			u.Out().Printf("Entity ID:         %s\n", profile.IdpConfig.EntityId)
		}
		if profile.IdpConfig.SingleSignOnServiceUri != "" {
			u.Out().Printf("SSO URL:           %s\n", profile.IdpConfig.SingleSignOnServiceUri)
		}
		if profile.IdpConfig.LogoutRedirectUri != "" {
			u.Out().Printf("Logout URL:        %s\n", profile.IdpConfig.LogoutRedirectUri)
		}
		if profile.IdpConfig.ChangePasswordUri != "" {
			u.Out().Printf("Change Password:   %s\n", profile.IdpConfig.ChangePasswordUri)
		}
	}
	if profile.SpConfig != nil {
		if profile.SpConfig.EntityId != "" {
			u.Out().Printf("SP Entity ID:      %s\n", profile.SpConfig.EntityId)
		}
		if profile.SpConfig.AssertionConsumerServiceUri != "" {
			u.Out().Printf("SP ACS URL:        %s\n", profile.SpConfig.AssertionConsumerServiceUri)
		}
	}
	return nil
}

type SSOSettingsUpdateCmd struct {
	SSOURL            string `name:"sso-url" help:"SSO URL (SingleSignOnService URI)"`
	LogoutURL         string `name:"logout-url" help:"Logout redirect URL"`
	ChangePasswordURL string `name:"change-password-url" help:"Change password URL"`
	Certificate       string `name:"certificate" help:"IdP signing certificate (PEM string or file path)"`
}

func (c *SSOSettingsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newInboundSSOService(ctx, account)
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.SSOURL) == "" && strings.TrimSpace(c.LogoutURL) == "" && strings.TrimSpace(c.ChangePasswordURL) == "" && strings.TrimSpace(c.Certificate) == "" {
		return usage("no updates specified")
	}

	profile, err := firstInboundSamlProfile(ctx, svc)
	if err != nil {
		return err
	}

	updateMask := make([]string, 0, 3)
	patch := &cloudidentity.InboundSamlSsoProfile{}

	idpConfig := &cloudidentity.SamlIdpConfig{}
	if strings.TrimSpace(c.SSOURL) != "" {
		idpConfig.SingleSignOnServiceUri = strings.TrimSpace(c.SSOURL)
		updateMask = append(updateMask, "idpConfig.singleSignOnServiceUri")
	}
	if strings.TrimSpace(c.LogoutURL) != "" {
		idpConfig.LogoutRedirectUri = strings.TrimSpace(c.LogoutURL)
		updateMask = append(updateMask, "idpConfig.logoutRedirectUri")
	}
	if strings.TrimSpace(c.ChangePasswordURL) != "" {
		idpConfig.ChangePasswordUri = strings.TrimSpace(c.ChangePasswordURL)
		updateMask = append(updateMask, "idpConfig.changePasswordUri")
	}
	if len(updateMask) > 0 {
		patch.IdpConfig = idpConfig
	}

	var patchOp *cloudidentity.Operation
	if len(updateMask) > 0 {
		patchOp, err = svc.InboundSamlSsoProfiles.Patch(profile.Name, patch).
			UpdateMask(strings.Join(updateMask, ",")).
			Context(ctx).
			Do()
		if err != nil {
			return fmt.Errorf("update inbound sso profile: %w", err)
		}
	}

	var certOp *cloudidentity.Operation
	if strings.TrimSpace(c.Certificate) != "" {
		pemData, err := readValueOrFile(c.Certificate)
		if err != nil {
			return fmt.Errorf("read certificate: %w", err)
		}
		if strings.TrimSpace(pemData) == "" {
			return usage("certificate is empty")
		}
		certOp, err = svc.InboundSamlSsoProfiles.IdpCredentials.Add(profile.Name, &cloudidentity.AddIdpCredentialRequest{
			PemData: pemData,
		}).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("add idp credential: %w", err)
		}
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{"profile": profile.Name}
		if patchOp != nil {
			payload["update"] = patchOp
		}
		if certOp != nil {
			payload["certificate"] = certOp
		}
		return outfmt.WriteJSON(os.Stdout, payload)
	}

	if patchOp != nil {
		u.Out().Printf("Updated inbound SSO profile: %s\n", profile.Name)
		if patchOp.Name != "" {
			u.Out().Printf("Update operation: %s\n", patchOp.Name)
		}
	}
	if certOp != nil {
		if patchOp == nil {
			u.Out().Printf("Updated inbound SSO profile: %s\n", profile.Name)
		}
		if certOp.Name != "" {
			u.Out().Printf("Certificate operation: %s\n", certOp.Name)
		}
	}
	return nil
}

type SSOAssignmentsCmd struct {
	List   SSOAssignmentsListCmd   `cmd:"" name:"list" help:"List inbound SSO assignments"`
	Create SSOAssignmentsCreateCmd `cmd:"" name:"create" help:"Create inbound SSO assignment"`
	Delete SSOAssignmentsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete inbound SSO assignment"`
}

type SSOAssignmentsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *SSOAssignmentsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newInboundSSOService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.InboundSsoAssignments.List()
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list inbound sso assignments: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.InboundSsoAssignments) == 0 {
		u.Err().Println("No inbound SSO assignments found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ASSIGNMENT ID\tMODE\tTARGET\tPROFILE")
	for _, assignment := range resp.InboundSsoAssignments {
		if assignment == nil {
			continue
		}
		target := assignment.TargetOrgUnit
		if target == "" {
			target = assignment.TargetGroup
		}
		profile := ""
		if assignment.SamlSsoInfo != nil {
			profile = assignment.SamlSsoInfo.InboundSamlSsoProfile
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(assignment.Name),
			sanitizeTab(assignment.SsoMode),
			sanitizeTab(target),
			sanitizeTab(profile),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type SSOAssignmentsCreateCmd struct {
	OrgUnit string `name:"org-unit" aliases:"ou" help:"Org unit path or ID" required:""`
	Mode    string `name:"mode" help:"SSO mode: SSO_OFF|SSO_ON|NONE" enum:"SSO_OFF,SSO_ON,NONE" required:""`
}

func (c *SSOAssignmentsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	orgUnit := strings.TrimSpace(c.OrgUnit)
	if orgUnit == "" {
		return usage("--org-unit is required")
	}

	svc, err := newInboundSSOService(ctx, account)
	if err != nil {
		return err
	}

	targetOrgUnit, err := resolveOrgUnitResource(ctx, flags, orgUnit)
	if err != nil {
		return err
	}

	mode := strings.ToUpper(strings.TrimSpace(c.Mode))
	if mode == "NONE" {
		return clearInboundSSOAssignments(ctx, svc, targetOrgUnit)
	}

	ssoMode, err := mapInboundSSOMode(mode)
	if err != nil {
		return err
	}

	assignment := &cloudidentity.InboundSsoAssignment{
		TargetOrgUnit: targetOrgUnit,
		SsoMode:       ssoMode,
	}

	op, err := svc.InboundSsoAssignments.Create(assignment).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create inbound sso assignment: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Created inbound SSO assignment for %s\n", targetOrgUnit)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

type SSOAssignmentsDeleteCmd struct {
	AssignmentID string `arg:"" name:"assignment-id" help:"Assignment resource name"`
}

func (c *SSOAssignmentsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.AssignmentID) == "" {
		return usage("assignment ID is required")
	}

	if err = confirmDestructive(ctx, flags, fmt.Sprintf("delete inbound SSO assignment %s", c.AssignmentID)); err != nil {
		return err
	}

	svc, err := newInboundSSOService(ctx, account)
	if err != nil {
		return err
	}

	op, err := svc.InboundSsoAssignments.Delete(c.AssignmentID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("delete inbound sso assignment %s: %w", c.AssignmentID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Deleted inbound SSO assignment: %s\n", c.AssignmentID)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

func firstInboundSamlProfile(ctx context.Context, svc *cloudidentity.Service) (*cloudidentity.InboundSamlSsoProfile, error) {
	resp, err := svc.InboundSamlSsoProfiles.List().Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list inbound sso profiles: %w", err)
	}
	if len(resp.InboundSamlSsoProfiles) == 0 {
		return nil, fmt.Errorf("no inbound SAML SSO profiles found")
	}
	return resp.InboundSamlSsoProfiles[0], nil
}

func mapInboundSSOMode(mode string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case ssoModeOff:
		return ssoModeOff, nil
	case "SSO_ON":
		return "DOMAIN_WIDE_SAML_IF_ENABLED", nil
	default:
		return "", usage("mode must be SSO_OFF, SSO_ON, or NONE")
	}
}

func resolveOrgUnitResource(ctx context.Context, flags *RootFlags, orgUnit string) (string, error) {
	if strings.HasPrefix(orgUnit, "orgUnits/") {
		return orgUnit, nil
	}

	account, err := requireAccount(flags)
	if err != nil {
		return "", err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return "", err
	}

	ou, err := svc.Orgunits.Get(adminCustomerID(), orgUnit).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("resolve org unit %s: %w", orgUnit, err)
	}
	if strings.TrimSpace(ou.OrgUnitId) == "" {
		return "", fmt.Errorf("org unit %s has no ID", orgUnit)
	}
	return "orgUnits/" + ou.OrgUnitId, nil
}

func clearInboundSSOAssignments(ctx context.Context, svc *cloudidentity.Service, targetOrgUnit string) error {
	u := ui.FromContext(ctx)
	resp, err := svc.InboundSsoAssignments.List().Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list inbound sso assignments: %w", err)
	}

	deleted := make([]string, 0)
	for _, assignment := range resp.InboundSsoAssignments {
		if assignment == nil || assignment.TargetOrgUnit != targetOrgUnit {
			continue
		}
		if _, err := svc.InboundSsoAssignments.Delete(assignment.Name).Context(ctx).Do(); err != nil {
			return fmt.Errorf("delete inbound sso assignment %s: %w", assignment.Name, err)
		}
		deleted = append(deleted, assignment.Name)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"targetOrgUnit": targetOrgUnit,
			"deleted":       deleted,
		})
	}

	if len(deleted) == 0 {
		u.Err().Printf("No inbound SSO assignments found for %s\n", targetOrgUnit)
		return nil
	}

	u.Out().Printf("Deleted %d inbound SSO assignments for %s\n", len(deleted), targetOrgUnit)
	return nil
}

// readValueOrFile interprets value as either a literal string or a file reference.
// Prefix with "@" to explicitly read from a file path (e.g. "@/path/to/cert.pem").
// JSON values (starting with "{" or "[") are returned as-is. As a fallback,
// if the value happens to match an existing file path on disk it will be read;
// prefer the "@" prefix to avoid ambiguity.
func readValueOrFile(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, "@") {
		path := strings.TrimSpace(strings.TrimPrefix(trimmed, "@"))
		if path == "" {
			return "", fmt.Errorf("empty @file path")
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: user-provided file path is intentional
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return trimmed, nil
	}
	if info, err := os.Stat(trimmed); err == nil && !info.IsDir() {
		data, err := os.ReadFile(trimmed) //nolint:gosec // G304: user-provided file path is intentional
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return trimmed, nil
}
