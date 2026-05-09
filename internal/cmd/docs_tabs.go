package cmd

import (
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"
)

func flattenTabs(tabs []*docs.Tab) []*docs.Tab {
	var result []*docs.Tab
	for _, tab := range tabs {
		if tab == nil {
			continue
		}
		result = append(result, tab)
		if len(tab.ChildTabs) > 0 {
			result = append(result, flattenTabs(tab.ChildTabs)...)
		}
	}
	return result
}

func findTab(tabs []*docs.Tab, query string) (*docs.Tab, error) {
	query = strings.TrimSpace(query)
	for _, tab := range tabs {
		if tab.TabProperties != nil && tab.TabProperties.TabId == query {
			return tab, nil
		}
	}
	lower := strings.ToLower(query)
	for _, tab := range tabs {
		if tab.TabProperties != nil && strings.ToLower(tab.TabProperties.Title) == lower {
			return tab, nil
		}
	}
	names := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		if tab.TabProperties != nil {
			names = append(names, fmt.Sprintf("%q", tab.TabProperties.Title))
		}
	}
	if len(names) > 0 {
		return nil, fmt.Errorf("tab not found: %q (available: %s)", query, strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("tab not found: %q (no tabs returned by API)", query)
}

func tabTitle(tab *docs.Tab) string {
	if tab.TabProperties != nil && tab.TabProperties.Title != "" {
		return tab.TabProperties.Title
	}
	return "(untitled)"
}
