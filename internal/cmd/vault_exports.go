package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/storage/v1"
	"google.golang.org/api/vault/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type VaultExportsCmd struct {
	List     VaultExportsListCmd     `cmd:"" name:"list" aliases:"ls" help:"List exports"`
	Get      VaultExportsGetCmd      `cmd:"" name:"get" help:"Get export"`
	Create   VaultExportsCreateCmd   `cmd:"" name:"create" aliases:"add" help:"Create export"`
	Download VaultExportsDownloadCmd `cmd:"" name:"download" help:"Download export files"`
}

type VaultExportsListCmd struct {
	MatterID string `name:"matter" required:"" help:"Matter ID"`
	Max      int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page     string `name:"page" help:"Page token"`
}

func (c *VaultExportsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Matters.Exports.List(c.MatterID).PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list exports: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Exports) == 0 {
		u.Err().Println("No exports found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "EXPORT ID\tNAME\tSTATUS\tCREATED")
	for _, exp := range resp.Exports {
		if exp == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(exp.Id),
			sanitizeTab(exp.Name),
			sanitizeTab(exp.Status),
			sanitizeTab(exp.CreateTime),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type VaultExportsGetCmd struct {
	MatterID string `name:"matter" required:"" help:"Matter ID"`
	ExportID string `arg:"" name:"export-id" help:"Export ID"`
}

func (c *VaultExportsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	exp, err := svc.Matters.Exports.Get(c.MatterID, c.ExportID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get export %s: %w", c.ExportID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, exp)
	}

	fmt.Fprintf(os.Stdout, "Export ID: %s\n", exp.Id)
	fmt.Fprintf(os.Stdout, "Name:      %s\n", exp.Name)
	fmt.Fprintf(os.Stdout, "Status:    %s\n", exp.Status)
	fmt.Fprintf(os.Stdout, "Created:   %s\n", exp.CreateTime)
	if exp.Query != nil {
		fmt.Fprintf(os.Stdout, "Corpus:    %s\n", exp.Query.Corpus)
	}
	return nil
}

type VaultExportsCreateCmd struct {
	MatterID string `name:"matter" required:"" help:"Matter ID"`
	Name     string `name:"name" required:"" help:"Export name"`
	Query    string `name:"query" help:"Search query terms"`
}

func (c *VaultExportsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	query := &vault.Query{
		Corpus:       "MAIL",
		SearchMethod: "ENTIRE_ORG",
		DataScope:    "ALL_DATA",
	}
	if strings.TrimSpace(c.Query) != "" {
		query.Terms = c.Query
	}

	export := &vault.Export{
		Name:  c.Name,
		Query: query,
	}

	created, err := svc.Matters.Exports.Create(c.MatterID, export).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create export: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	fmt.Fprintf(os.Stdout, "Created export: %s (%s)\n", created.Name, created.Id)
	return nil
}

type VaultExportsDownloadCmd struct {
	MatterID string `name:"matter" required:"" help:"Matter ID"`
	ExportID string `arg:"" name:"export-id" help:"Export ID"`
	Output   string `name:"output" required:"" help:"Output directory"`
}

func (c *VaultExportsDownloadCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	exp, err := svc.Matters.Exports.Get(c.MatterID, c.ExportID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get export %s: %w", c.ExportID, err)
	}
	if exp.CloudStorageSink == nil || len(exp.CloudStorageSink.Files) == 0 {
		return fmt.Errorf("export %s has no Cloud Storage files", c.ExportID)
	}

	if err := os.MkdirAll(c.Output, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	storageSvc, err := newStorageService(ctx, account)
	if err != nil {
		return err
	}

	results := make([]map[string]string, 0, len(exp.CloudStorageSink.Files))
	for _, file := range exp.CloudStorageSink.Files {
		if file == nil {
			continue
		}
		path, err := downloadExportFile(ctx, storageSvc, file, c.Output)
		if err != nil {
			return err
		}
		results = append(results, map[string]string{
			"bucket": file.BucketName,
			"object": file.ObjectName,
			"path":   path,
		})
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"files": results})
	}

	for _, item := range results {
		u.Out().Printf("Downloaded %s/%s -> %s\n", item["bucket"], item["object"], item["path"])
	}
	return nil
}

func downloadExportFile(ctx context.Context, svc *storage.Service, file *vault.CloudStorageFile, outputDir string) (string, error) {
	if file.BucketName == "" || file.ObjectName == "" {
		return "", fmt.Errorf("invalid storage file metadata")
	}

	resp, err := svc.Objects.Get(file.BucketName, file.ObjectName).Context(ctx).Download()
	if err != nil {
		return "", fmt.Errorf("download %s/%s: %w", file.BucketName, file.ObjectName, err)
	}
	defer resp.Body.Close()

	name := filepath.Base(file.ObjectName)
	if name == "." || name == "/" || name == "" {
		name = "export"
	}
	path := filepath.Join(outputDir, name)

	out, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return path, nil
}
