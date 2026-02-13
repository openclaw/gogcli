package cmd

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/alecthomas/kong"
)

type completionFlag struct {
	takesValue bool
}

type completionNode struct {
	children map[string]*completionNode
	flags    map[string]completionFlag
}

var (
	completionRootOnce sync.Once
	completionRoot     *completionNode
	completionRootErr  error
)

func completeWords(cword int, words []string) ([]string, error) {
	if len(words) == 0 {
		return nil, nil
	}

	root, err := completionRootNode()
	if err != nil {
		return nil, err
	}

	cword = normalizeCword(cword, len(words))
	if cword < 0 {
		return nil, nil
	}

	start := completionStartIndex(words)

	node, terminatorIndex, needsValue := advanceCompletionNode(root, words, start, cword)
	if needsValue {
		return nil, nil
	}

	if shouldStopAfterTerminator(terminatorIndex, cword, words) {
		return nil, nil
	}

	if expectsFlagValue(node, cword, words, start) {
		return nil, nil
	}

	current := ""
	if cword < len(words) {
		current = words[cword]
	}

	suggestions := make([]string, 0)
	if strings.HasPrefix(current, "-") {
		suggestions = append(suggestions, matchingFlags(node, current)...)
	} else {
		suggestions = append(suggestions, matchingCommands(node, current)...)
		suggestions = append(suggestions, matchingFlags(node, current)...)
	}
	sort.Strings(suggestions)
	return suggestions, nil
}

func completionRootNode() (*completionNode, error) {
	completionRootOnce.Do(func() {
		parser, _, err := newParser(baseDescription())
		if err != nil {
			completionRootErr = err
			return
		}
		completionRoot = buildCompletionNode(parser.Model.Node)
	})
	return completionRoot, completionRootErr
}

func normalizeCword(cword int, wordCount int) int {
	if cword < 0 {
		cword = wordCount - 1
	}
	if cword < 0 {
		return -1
	}
	if cword > wordCount {
		cword = wordCount
	}
	return cword
}

func completionStartIndex(words []string) int {
	if len(words) == 0 {
		return 0
	}
	if isProgramName(words[0]) {
		return 1
	}
	return 0
}

func advanceCompletionNode(root *completionNode, words []string, start int, cword int) (*completionNode, int, bool) {
	node := root
	terminatorIndex := -1
	for i := start; i < cword && i < len(words); {
		word := words[i]
		if word == "--" {
			terminatorIndex = i
			break
		}
		if strings.HasPrefix(word, "-") {
			flagToken, hasValue := splitFlagToken(word)
			if hasValue {
				i++
				continue
			}
			if spec, ok := node.flags[flagToken]; ok && spec.takesValue {
				if i+1 == cword {
					return node, terminatorIndex, true
				}
				i += 2
				continue
			}
			i++
			continue
		}
		if child, ok := node.children[word]; ok {
			node = child
			i++
			continue
		}
		i++
	}

	return node, terminatorIndex, false
}

func shouldStopAfterTerminator(terminatorIndex int, cword int, words []string) bool {
	if terminatorIndex != -1 && cword >= terminatorIndex {
		return true
	}
	if cword < len(words) && words[cword] == "--" {
		return true
	}
	return false
}

func expectsFlagValue(node *completionNode, cword int, words []string, start int) bool {
	if cword <= start || cword > len(words) {
		return false
	}
	prev := words[cword-1]
	if strings.HasPrefix(prev, "-") {
		flagToken, hasValue := splitFlagToken(prev)
		if hasValue {
			return true
		}
		if spec, ok := node.flags[flagToken]; ok && spec.takesValue {
			return true
		}
	}
	return false
}

func isProgramName(word string) bool {
	base := filepath.Base(word)
	return strings.EqualFold(base, "rata") || strings.EqualFold(base, "rata.exe")
}

func buildCompletionNode(node *kong.Node) *completionNode {
	current := &completionNode{
		children: make(map[string]*completionNode),
		flags:    make(map[string]completionFlag),
	}

	for _, group := range node.AllFlags(true) {
		for _, flag := range group {
			addFlagTokens(current.flags, flag)
		}
	}

	for _, child := range node.Children {
		if child.Hidden {
			continue
		}
		childNode := buildCompletionNode(child)
		for _, name := range append([]string{child.Name}, child.Aliases...) {
			if name == "" {
				continue
			}
			if _, exists := current.children[name]; !exists {
				current.children[name] = childNode
			}
		}
	}

	return current
}

func addFlagTokens(flags map[string]completionFlag, flag *kong.Flag) {
	takesValue := !(flag.IsBool() || flag.IsCounter())
	addFlag(flags, "--"+flag.Name, takesValue)
	for _, alias := range flag.Aliases {
		addFlag(flags, "--"+alias, takesValue)
	}
	if flag.Short != 0 {
		addFlag(flags, "-"+string(flag.Short), takesValue)
	}
	if negated := negatedFlagName(flag); negated != "" {
		addFlag(flags, negated, false)
	}
}

func negatedFlagName(flag *kong.Flag) string {
	switch flag.Tag.Negatable {
	case "":
		return ""
	case "_":
		return "--no-" + flag.Name
	default:
		return "--" + flag.Tag.Negatable
	}
}

func addFlag(flags map[string]completionFlag, token string, takesValue bool) {
	if token == "" {
		return
	}
	if _, exists := flags[token]; exists {
		return
	}
	flags[token] = completionFlag{takesValue: takesValue}
}

func splitFlagToken(word string) (string, bool) {
	if idx := strings.Index(word, "="); idx != -1 {
		return word[:idx], true
	}
	return word, false
}

func matchingCommands(node *completionNode, prefix string) []string {
	results := make([]string, 0, len(node.children))
	for name := range node.children {
		if strings.HasPrefix(name, prefix) {
			results = append(results, name)
		}
	}
	return results
}

func matchingFlags(node *completionNode, prefix string) []string {
	results := make([]string, 0, len(node.flags))
	for name := range node.flags {
		if strings.HasPrefix(name, prefix) {
			results = append(results, name)
		}
	}
	return results
}
