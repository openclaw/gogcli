package cmd

import (
	"sort"
	"strings"

	"github.com/steipete/gogcli/internal/drivereport"
)

type driveDuSummary = drivereport.Summary

func sortDriveInventory(items []driveTreeItem, sortBy string, order string) {
	sortBy = strings.ToLower(strings.TrimSpace(sortBy))
	order = strings.ToLower(strings.TrimSpace(order))
	desc := order == "desc"

	less := func(i, j int) bool { return false }
	switch sortBy {
	case "size":
		less = func(i, j int) bool { return items[i].Size < items[j].Size }
	case "modified":
		less = func(i, j int) bool { return items[i].ModifiedTime < items[j].ModifiedTime }
	default:
		less = func(i, j int) bool { return items[i].Path < items[j].Path }
	}

	sort.Slice(items, func(i, j int) bool {
		if desc {
			return less(j, i)
		}
		return less(i, j)
	})
}
