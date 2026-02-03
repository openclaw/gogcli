package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SchemasCmd struct {
	List   SchemasListCmd   `cmd:"" name:"list" aliases:"ls" help:"List schemas"`
	Get    SchemasGetCmd    `cmd:"" name:"get" help:"Get schema"`
	Create SchemasCreateCmd `cmd:"" name:"create" aliases:"add" help:"Create schema"`
	Update SchemasUpdateCmd `cmd:"" name:"update" help:"Update schema"`
	Delete SchemasDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete schema"`
}

type SchemasListCmd struct{}

func (c *SchemasListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Schemas.List(adminCustomerID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list schemas: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Schemas) == 0 {
		u.Err().Println("No schemas found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tFIELDS\tID")
	for _, schema := range resp.Schemas {
		if schema == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%d\t%s\n",
			sanitizeTab(schema.SchemaName),
			len(schema.Fields),
			sanitizeTab(schema.SchemaId),
		)
	}
	return nil
}

type SchemasGetCmd struct {
	Name string `arg:"" name:"name" help:"Schema name or ID"`
}

func (c *SchemasGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("schema name is required")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	schema, err := svc.Schemas.Get(adminCustomerID, name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get schema %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, schema)
	}

	u.Out().Printf("Name:   %s\n", schema.SchemaName)
	u.Out().Printf("ID:     %s\n", schema.SchemaId)
	if schema.DisplayName != "" {
		u.Out().Printf("Display: %s\n", schema.DisplayName)
	}
	if len(schema.Fields) > 0 {
		u.Out().Printf("Fields: %d\n", len(schema.Fields))
		for _, field := range schema.Fields {
			if field == nil {
				continue
			}
			u.Out().Printf("- %s (%s)\n", field.FieldName, field.FieldType)
		}
	}
	return nil
}

type SchemasCreateCmd struct {
	Name   string   `arg:"" name:"name" help:"Schema name"`
	Fields []string `name:"field" help:"Field spec NAME:TYPE (repeatable)"`
}

func (c *SchemasCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("schema name is required")
	}

	fields, err := parseSchemaFields(c.Fields)
	if err != nil {
		return err
	}
	if len(fields) == 0 {
		return usage("--field is required")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	schema := &admin.Schema{
		SchemaName:  name,
		DisplayName: name,
		Fields:      fields,
	}

	created, err := svc.Schemas.Insert(adminCustomerID, schema).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create schema %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created schema: %s (%s)\n", created.SchemaName, created.SchemaId)
	return nil
}

type SchemasUpdateCmd struct {
	Name        string   `arg:"" name:"name" help:"Schema name or ID"`
	AddFields   []string `name:"add-field" help:"Field spec NAME:TYPE (repeatable)"`
	RemoveField []string `name:"remove-field" help:"Field name to remove (repeatable)"`
}

func (c *SchemasUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("schema name is required")
	}

	if len(c.AddFields) == 0 && len(c.RemoveField) == 0 {
		return usage("no updates specified")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	schema, err := svc.Schemas.Get(adminCustomerID, name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get schema %s: %w", name, err)
	}

	fieldMap := make(map[string]*admin.SchemaFieldSpec, len(schema.Fields))
	order := make([]string, 0, len(schema.Fields))
	for _, field := range schema.Fields {
		if field == nil {
			continue
		}
		fieldMap[field.FieldName] = field
		order = append(order, field.FieldName)
	}

	newFields, err := parseSchemaFields(c.AddFields)
	if err != nil {
		return err
	}
	for _, field := range newFields {
		if fieldMap[field.FieldName] != nil {
			return fmt.Errorf("field %s already exists", field.FieldName)
		}
		fieldMap[field.FieldName] = field
		order = append(order, field.FieldName)
	}

	for _, remove := range c.RemoveField {
		remove = strings.TrimSpace(remove)
		if remove == "" {
			continue
		}
		if _, ok := fieldMap[remove]; !ok {
			return fmt.Errorf("field %s not found", remove)
		}
		delete(fieldMap, remove)
	}

	updatedFields := make([]*admin.SchemaFieldSpec, 0, len(fieldMap))
	seen := make(map[string]struct{}, len(fieldMap))
	for _, name := range order {
		if field, ok := fieldMap[name]; ok {
			updatedFields = append(updatedFields, field)
			seen[name] = struct{}{}
		}
	}
	if len(seen) != len(fieldMap) {
		remaining := make([]string, 0, len(fieldMap)-len(seen))
		for key := range fieldMap {
			if _, ok := seen[key]; !ok {
				remaining = append(remaining, key)
			}
		}
		sort.Strings(remaining)
		for _, key := range remaining {
			updatedFields = append(updatedFields, fieldMap[key])
		}
	}

	schema.Fields = updatedFields
	updated, err := svc.Schemas.Update(adminCustomerID, name, schema).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update schema %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u.Out().Printf("Updated schema: %s\n", updated.SchemaName)
	return nil
}

type SchemasDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Schema name or ID"`
}

func (c *SchemasDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("schema name is required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete schema %s", name)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Schemas.Delete(adminCustomerID, name).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete schema %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"schema": name, "deleted": true})
	}

	u.Out().Printf("Deleted schema: %s\n", name)
	return nil
}

func parseSchemaFields(fields []string) ([]*admin.SchemaFieldSpec, error) {
	out := make([]*admin.SchemaFieldSpec, 0, len(fields))
	for _, spec := range fields {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		parts := strings.SplitN(spec, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid field spec %q (expected NAME:TYPE)", spec)
		}
		name := strings.TrimSpace(parts[0])
		if name == "" {
			return nil, fmt.Errorf("invalid field spec %q (empty name)", spec)
		}
		fieldType, ok := normalizeSchemaFieldType(parts[1])
		if !ok {
			return nil, fmt.Errorf("invalid field type %q", parts[1])
		}
		out = append(out, &admin.SchemaFieldSpec{
			FieldName:   name,
			FieldType:   fieldType,
			DisplayName: name,
		})
	}
	return out, nil
}

func normalizeSchemaFieldType(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "bool", "boolean":
		return "BOOL", true
	case "date":
		return "DATE", true
	case "double":
		return "DOUBLE", true
	case "email":
		return "EMAIL", true
	case "int", "int64":
		return "INT64", true
	case "phone":
		return "PHONE", true
	case "string", "str":
		return "STRING", true
	default:
		return "", false
	}
}
