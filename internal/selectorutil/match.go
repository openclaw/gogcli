package selectorutil

import (
	"sort"
	"strings"
)

type Match struct {
	ID   string
	Name string
}

func FindByIDOrCaseFoldName(input string, options []Match) (*Match, bool, []Match) {
	in := strings.TrimSpace(input)
	if in == "" {
		return nil, false, nil
	}

	for _, option := range options {
		if strings.TrimSpace(option.ID) == in {
			match := option
			return &match, true, nil
		}
	}

	var matches []Match

	for _, option := range options {
		name := strings.TrimSpace(option.Name)
		if name == "" || !strings.EqualFold(name, in) {
			continue
		}
		matches = append(matches, Match{
			ID:   strings.TrimSpace(option.ID),
			Name: name,
		})
	}

	switch len(matches) {
	case 0:
		return nil, false, nil
	case 1:
		match := matches[0]
		return &match, true, nil
	default:
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].Name == matches[j].Name {
				return matches[i].ID < matches[j].ID
			}

			return matches[i].Name < matches[j].Name
		})

		return nil, false, matches
	}
}
