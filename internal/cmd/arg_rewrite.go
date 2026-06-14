package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
)

func rewriteDesirePathArgs(model *kong.Application, args []string) []string {
	// Some commands use `--fields` for API field masks. Agents also frequently
	// guess `--fields` to mean "select output fields", so we squat it everywhere
	// else by rewriting to the global `--select` flag.
	//
	// We avoid adding `--fields` as a real alias because Kong would treat it as a duplicate flag.
	out := make([]string, 0, len(args))
	for i, arg := range args {
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}
		if arg == "--fields" {
			if commandHasLocalFlagBefore(model, args, i, "fields") {
				out = append(out, arg)
			} else {
				out = append(out, "--select")
			}
			continue
		}
		if strings.HasPrefix(arg, "--fields=") {
			if commandHasLocalFlagBefore(model, args, i, "fields") {
				out = append(out, arg)
			} else {
				out = append(out, "--select="+strings.TrimPrefix(arg, "--fields="))
			}
			continue
		}
		out = append(out, arg)
	}
	return out
}

func commandHasLocalFlagBefore(model *kong.Application, args []string, flagIndex int, flagName string) bool {
	if model == nil || model.Node == nil || flagIndex < 0 || flagIndex > len(args) {
		return false
	}
	node := commandNodeBefore(model.Node, args[:flagIndex])
	return nodeHasLocalFlag(node, flagName)
}

func commandNodeBefore(root *kong.Node, args []string) *kong.Node {
	if root == nil {
		return nil
	}
	node := root
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			if flag := commandFlag(node, arg); flag != nil &&
				!strings.Contains(arg, "=") &&
				!flag.IsBool() &&
				!flag.IsCounter() &&
				i+1 < len(args) {
				i++
			}
			continue
		}

		if child := commandChild(node, arg); child != nil {
			node = child
		}
	}
	return node
}

func commandFlag(node *kong.Node, arg string) *kong.Flag {
	if node == nil {
		return nil
	}
	name := arg
	if before, _, ok := strings.Cut(name, "="); ok {
		name = before
	}
	lineage := make([]*kong.Node, 0, node.Depth()+2)
	for current := node; current != nil; current = current.Parent {
		lineage = append(lineage, current)
	}
	for i := len(lineage) - 1; i >= 0; i-- {
		for _, flag := range lineage[i].Flags {
			if name == "--"+flag.Name || flag.Short != 0 && name == "-"+string(flag.Short) {
				return flag
			}
			for _, alias := range flag.Aliases {
				if name == "--"+alias {
					return flag
				}
			}
		}
	}
	return nil
}

func commandChild(node *kong.Node, name string) *kong.Node {
	if node == nil {
		return nil
	}
	canonicalNames := make(map[string]struct{}, len(node.Children))
	for _, child := range node.Children {
		if child.Type == kong.CommandNode {
			canonicalNames[child.Name] = struct{}{}
			if child.Name == name {
				return child
			}
		}
	}
	if _, conflicts := canonicalNames[name]; conflicts {
		return nil
	}
	for _, child := range node.Children {
		if child.Type != kong.CommandNode {
			continue
		}
		for _, alias := range child.Aliases {
			if alias == name {
				return child
			}
		}
	}
	return nil
}

func nodeHasLocalFlag(node *kong.Node, name string) bool {
	if node == nil {
		return false
	}
	for _, flag := range node.Flags {
		if flag.Name == name {
			return true
		}
		for _, alias := range flag.Aliases {
			if alias == name {
				return true
			}
		}
	}
	return false
}
