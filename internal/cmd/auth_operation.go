package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type AuthCheckOperationCmd struct {
	Operation string `arg:"" name:"operation" help:"Command path with dots, e.g. docs.write or sheets.datasource.refresh"`
	Resource  string `name:"resource" help:"Doc, spreadsheet, or Drive file ID/URL to probe"`
	Probe     bool   `name:"probe" help:"Run metadata-only GET requests; never attempt the requested write"`
}

type authOperationSpec struct {
	scopes     []string
	write      bool
	probe      string
	extraScope string
}

func authOperationSpecification(operation string) (authOperationSpec, bool) {
	const scope = "https://www.googleapis.com/auth/"
	readDrive := []string{scope + "drive", scope + "drive.readonly", scope + "drive.file"}
	writeDrive := []string{scope + "drive", scope + "drive.file"}
	writeDocs := []string{scope + "documents", scope + "drive", scope + "drive.file"}
	writeSheets := []string{scope + "spreadsheets", scope + "drive", scope + "drive.file"}
	var spec authOperationSpec
	switch operation {
	case "docs.cat":
		spec = authOperationSpec{scopes: append([]string{scope + "documents.readonly"}, writeDocs...), probe: "docs"}
	case "docs.write":
		spec = authOperationSpec{scopes: writeDocs, write: true, probe: "docs"}
	case "sheets.get":
		spec = authOperationSpec{scopes: append([]string{scope + "spreadsheets.readonly"}, writeSheets...), probe: "sheets"}
	case "sheets.update":
		spec = authOperationSpec{scopes: writeSheets, write: true, probe: "sheets"}
	case "sheets.datasource.refresh":
		spec = authOperationSpec{scopes: writeSheets, write: true, probe: "sheets", extraScope: scope + "bigquery.readonly"}
	case "drive.get":
		spec = authOperationSpec{scopes: readDrive, probe: "drive"}
	case "drive.upload":
		spec = authOperationSpec{scopes: writeDrive, write: true, probe: "drive"}
	default:
		return spec, false
	}
	return spec, true
}

func (c *AuthCheckOperationCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	path := splitCommandPath([]string{strings.ReplaceAll(c.Operation, ".", " ")})
	node, err := findCommandNode(kctx.Model.Node, path)
	if err != nil {
		return err
	}
	path = commandNodePath(node)
	operation := strings.Join(path, ".")
	spec, supported := authOperationSpecification(operation)
	checks := []authDoctorCheck{{Name: "command", Status: "ok", Detail: operation}}
	add := func(name, status, detail, hint string) {
		checks = append(checks, authDoctorCheck{Name: name, Status: status, Detail: detail, Hint: hint})
	}
	policyErr := authOperationPolicy(flags, path)
	if policyErr != nil {
		add("policy", "blocked", policyErr.Error(), "")
	} else {
		add("policy", "ok", "command path is permitted by gog command rules", "")
	}
	if !supported {
		add("requirements", "unknown", "operation-specific scope and probe requirements are not modeled", "use gog schema for command details; auth doctor for general authentication")
		return writeAuthOperationResult(ctx, operation, checks)
	}
	if spec.write && flags != nil && flags.ReadOnly {
		add("readonly", "blocked", "--readonly blocks mutating API requests", "")
	}
	account, accountErr := requireAccount(flags)
	if accountErr != nil {
		add("account", "unknown", accountErr.Error(), "select --account")
		return writeAuthOperationResult(ctx, operation, checks)
	}
	client, clientErr := resolveClientForEmail(ctx, account, flags)
	if clientErr != nil {
		return clientErr
	}
	var recorded []string
	var storeErr error
	if flags == nil || strings.TrimSpace(flags.AccessToken) == "" {
		store, openErr := openAuthSecretsStore(ctx)
		storeErr = openErr
		if openErr == nil {
			token, tokenErr := store.GetToken(client, account)
			if tokenErr == nil {
				recorded = token.Scopes
			} else {
				storeErr = tokenErr
			}
		}
	}
	if storeErr != nil || len(recorded) == 0 {
		add("recorded_scopes", "unknown", "no readable OAuth scope metadata for the selected account/client", "direct tokens and service-account grants are not inferred from OAuth metadata")
	} else {
		status := "recorded_missing"
		if authOperationHasScope(recorded, spec.scopes) {
			status = "recorded_present"
		}
		add("recorded_scopes", status, "requires any of: "+strings.Join(spec.scopes, ", "), "stored scopes are metadata, not a live permission test")
		if spec.extraScope != "" {
			status = "recorded_missing"
			if authOperationHasScope(recorded, []string{spec.extraScope}) {
				status = "recorded_present"
			}
			add("extra_scope", status, spec.extraScope, "BigQuery IAM, project API enablement, and data-source credentials remain separate requirements")
		}
	}
	if c.Probe && (flags == nil || !flags.DryRun) && policyErr == nil {
		if strings.TrimSpace(c.Resource) == "" {
			return usage("--probe requires --resource")
		}
		if err := probeAuthOperation(ctx, flags, spec, normalizeGoogleID(c.Resource)); err != nil {
			add("resource_read", "failed", err.Error(), "a 403 may be caused by scopes, API enablement, policy, or resource access; inspect Google's error")
		} else {
			add("resource_read", "ok", "metadata GET succeeded for the selected account and resource", "")
		}
		if spec.write {
			_, svc, driveErr := requireDriveService(ctx, flags)
			if driveErr == nil {
				file, readErr := svc.Files.Get(normalizeGoogleID(c.Resource)).SupportsAllDrives(true).Fields("capabilities/canEdit").Context(ctx).Do()
				switch {
				case readErr != nil:
					driveErr = readErr
				case file.Capabilities == nil:
					add("resource_edit", "unknown", "Drive did not return edit capability", "")
				case file.Capabilities.CanEdit:
					add("resource_edit", "reported_allowed", "Drive reports canEdit=true", "protected ranges, operation-specific policies, and write execution remain unverified")
				default:
					add("resource_edit", "blocked", "Drive reports canEdit=false", "")
				}
			}
			if driveErr != nil {
				add("resource_edit", "unknown", driveErr.Error(), "Drive metadata permission is separate from Docs/Sheets permission")
			}
		}
	} else {
		add("resource_read", "not_checked", "no live probe performed", "use --probe --resource <id> for metadata-only GET checks")
	}
	if spec.extraScope != "" {
		add("bigquery_iam", "unknown", "BigQuery IAM and data-source execution were not tested", "")
	}
	add("execution", "not_checked", "the requested operation was not executed", "read probes do not prove write success")
	return writeAuthOperationResult(ctx, operation, checks)
}

func authOperationHasScope(recorded, alternatives []string) bool {
	for _, scope := range recorded {
		for _, candidate := range alternatives {
			if scope == candidate {
				return true
			}
		}
	}
	return false
}

func authOperationPolicy(flags *RootFlags, path []string) error {
	profile, err := loadBakedSafetyProfile()
	if err != nil {
		return err
	}
	if !profile.allowsCommandPath(path) {
		return profile.commandPathError(path)
	}
	if flags == nil {
		return nil
	}
	if commandPathMatches(parseEnabledCommands(flags.DisableCommands), path) {
		return fmt.Errorf("operation is disabled by command policy")
	}
	allow, exact := parseEnabledCommands(flags.EnableCommands), parseEnabledCommands(flags.EnableCommandsExact)
	if allow["*"] || allow["all"] || exact["*"] || exact["all"] {
		return nil
	}
	if len(allow) > 0 || len(exact) > 0 {
		if !commandPathMatches(allow, path) && !commandPathMatchesExact(exact, path) {
			return fmt.Errorf("operation is not enabled by command policy")
		}
	}
	return nil
}

func probeAuthOperation(ctx context.Context, flags *RootFlags, spec authOperationSpec, id string) error {
	switch spec.probe {
	case "docs":
		svc, err := requireDocsService(ctx, flags)
		if err != nil {
			return err
		}
		_, err = svc.Documents.Get(id).Fields("documentId").Context(ctx).Do()
		return err
	case "sheets":
		account, err := requireAccount(flags)
		if err != nil {
			return err
		}
		svc, err := sheetsService(ctx, account)
		if err != nil {
			return err
		}
		_, err = svc.Spreadsheets.Get(id).Fields("spreadsheetId").Context(ctx).Do()
		return err
	case "drive":
		_, svc, err := requireDriveService(ctx, flags)
		if err != nil {
			return err
		}
		_, err = svc.Files.Get(id).SupportsAllDrives(true).Fields("id").Context(ctx).Do()
		return err
	}
	return fmt.Errorf("no metadata probe modeled")
}

func writeAuthOperationResult(ctx context.Context, operation string, checks []authDoctorCheck) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"operation": operation, "checks": checks})
	}
	for _, check := range checks {
		ui.FromContext(ctx).Out().Linef("%s\t%s\t%s\t%s", check.Name, check.Status, check.Detail, check.Hint)
	}
	return nil
}
