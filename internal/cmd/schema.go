package cmd

import (
	"context"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/outfmt"
)

type SchemaCmd struct {
	Command       []string `arg:"" optional:"" name:"command" help:"Optional command path to describe (e.g. drive ls). Default: entire CLI"`
	IncludeHidden bool     `name:"include-hidden" help:"Include hidden commands and flags"`
}

type schemaDoc struct {
	SchemaVersion int         `json:"schema_version"`
	Build         string      `json:"build"`
	Command       *schemaNode `json:"command"`
}

type schemaNode struct {
	Type         string        `json:"type"`
	Name         string        `json:"name"`
	Aliases      []string      `json:"aliases,omitempty"`
	Help         string        `json:"help,omitempty"`
	Detail       string        `json:"detail,omitempty"`
	Path         string        `json:"path"`
	Usage        string        `json:"usage,omitempty"`
	Hidden       bool          `json:"hidden,omitempty"`
	Passthrough  bool          `json:"passthrough,omitempty"`
	DefaultCmd   string        `json:"default_cmd,omitempty"`
	Flags        []schemaFlag  `json:"flags,omitempty"`
	Positionals  []schemaArg   `json:"positionals,omitempty"`
	Subcommands  []*schemaNode `json:"subcommands,omitempty"`
	Requirements []string      `json:"requirements,omitempty"`
}

type schemaFlag struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Short       string   `json:"short,omitempty"`
	Help        string   `json:"help,omitempty"`
	Type        string   `json:"type"`
	Required    bool     `json:"required,omitempty"`
	Default     string   `json:"default,omitempty"`
	HasDefault  bool     `json:"has_default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Envs        []string `json:"envs,omitempty"`
	Hidden      bool     `json:"hidden,omitempty"`
	Negated     bool     `json:"negated,omitempty"`
}

type schemaArg struct {
	Name       string   `json:"name"`
	Help       string   `json:"help,omitempty"`
	Type       string   `json:"type"`
	Required   bool     `json:"required,omitempty"`
	Default    string   `json:"default,omitempty"`
	HasDefault bool     `json:"has_default,omitempty"`
	Enum       []string `json:"enum,omitempty"`
	Cumulative bool     `json:"cumulative,omitempty"`
}

func (c *SchemaCmd) Run(ctx context.Context, kctx *kong.Context) error {
	// Always emit schema untransformed, even if the caller enabled global JSON transforms.
	ctx = outfmt.WithJSONTransform(ctx, outfmt.JSONTransform{})

	root := kctx.Model.Node
	node := root

	cmdPath := splitCommandPath(c.Command)
	if len(cmdPath) > 0 {
		found, err := findCommandNode(root, cmdPath)
		if err != nil {
			return err
		}
		node = found
	}

	hide := !c.IncludeHidden

	doc := schemaDoc{
		SchemaVersion: 1,
		Build:         VersionString(),
		Command:       buildSchemaNode(node, hide),
	}

	return outfmt.WriteJSON(ctx, os.Stdout, doc)
}

func splitCommandPath(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		for _, tok := range strings.Fields(strings.TrimSpace(p)) {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			out = append(out, tok)
		}
	}
	return out
}

func findCommandNode(root *kong.Node, path []string) (*kong.Node, error) {
	cur := root
	for _, token := range path {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		next := findChildCommand(cur, token)
		if next == nil {
			return nil, usagef("unknown command %q under %q", token, strings.TrimSpace(cur.FullPath()))
		}

		cur = next
	}

	return cur, nil
}

func findChildCommand(parent *kong.Node, token string) *kong.Node {
	token = strings.ToLower(strings.TrimSpace(token))
	for _, child := range parent.Children {
		if child == nil || child.Type != kong.CommandNode {
			continue
		}
		if strings.ToLower(child.Name) == token {
			return child
		}
		for _, a := range child.Aliases {
			if strings.ToLower(strings.TrimSpace(a)) == token {
				return child
			}
		}
	}
	return nil
}

func buildSchemaNode(node *kong.Node, hide bool) *schemaNode {
	if node == nil {
		return nil
	}

	out := &schemaNode{
		Type:        schemaNodeType(node),
		Name:        node.Name,
		Aliases:     sortedStrings(node.Aliases),
		Help:        strings.TrimSpace(node.Help),
		Detail:      strings.TrimSpace(node.Detail),
		Path:        strings.TrimSpace(node.FullPath()),
		Usage:       strings.TrimSpace(node.Summary()),
		Hidden:      node.Hidden,
		Passthrough: node.Passthrough,
	}
	if node.DefaultCmd != nil {
		out.DefaultCmd = node.DefaultCmd.Name
	}

	out.Flags = schemaFlags(node, hide)
	out.Positionals = schemaPositionals(node)
	out.Requirements = schemaRequirements(node, hide)

	children := make([]*kong.Node, 0, len(node.Children))
	for _, child := range node.Children {
		if child == nil || child.Type != kong.CommandNode {
			continue
		}
		if hide && child.Hidden {
			continue
		}
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })

	for _, child := range children {
		out.Subcommands = append(out.Subcommands, buildSchemaNode(child, hide))
	}

	return out
}

func schemaNodeType(node *kong.Node) string {
	if node == nil {
		return ""
	}
	switch node.Type {
	case kong.ApplicationNode:
		return "application"
	case kong.CommandNode:
		return "command"
	case kong.ArgumentNode:
		return "argument"
	default:
		return "unknown"
	}
}

func schemaFlags(node *kong.Node, hide bool) []schemaFlag {
	out := []schemaFlag{}
	for _, group := range node.AllFlags(hide) {
		for _, f := range group {
			if f == nil {
				continue
			}
			out = append(out, schemaFlag{
				Name:        f.Name,
				Aliases:     sortedStrings(f.Aliases),
				Short:       flagShortString(f.Short),
				Help:        strings.TrimSpace(f.Help),
				Type:        reflectTypeString(f.Target),
				Required:    f.Required,
				Default:     strings.TrimSpace(f.Default),
				HasDefault:  f.HasDefault,
				Enum:        sortedStrings(f.EnumSlice()),
				Placeholder: strings.TrimSpace(f.FormatPlaceHolder()),
				Envs:        sortedStrings(f.Envs),
				Hidden:      f.Hidden,
				Negated:     f.Negated,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func schemaPositionals(node *kong.Node) []schemaArg {
	out := make([]schemaArg, 0, len(node.Positional))
	for _, p := range node.Positional {
		if p == nil {
			continue
		}
		out = append(out, schemaArg{
			Name:       p.Name,
			Help:       strings.TrimSpace(p.Help),
			Type:       reflectTypeString(p.Target),
			Required:   p.Required,
			Default:    strings.TrimSpace(p.Default),
			HasDefault: p.HasDefault,
			Enum:       sortedStrings(p.EnumSlice()),
			Cumulative: p.IsCumulative(),
		})
	}
	return out
}

func schemaRequirements(node *kong.Node, hide bool) []string {
	req := []string{}
	for _, group := range node.AllFlags(hide) {
		for _, f := range group {
			if f == nil || !f.Required {
				continue
			}
			req = append(req, "--"+f.Name)
		}
	}
	sort.Strings(req)
	return req
}

func flagShortString(r rune) string {
	if r == 0 {
		return ""
	}
	return string(r)
}

func reflectTypeString(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	return v.Type().String()
}

func sortedStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
