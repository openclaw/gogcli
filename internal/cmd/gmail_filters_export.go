package cmd

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/gmail/v1"
)

const (
	gmailFiltersAtomNamespace = "http://www.w3.org/2005/Atom"
	gmailFiltersAppsNamespace = "http://schemas.google.com/apps/2006"
)

var nowGmailFiltersExport = time.Now

type gmailFiltersXMLFeed struct {
	XMLName   xml.Name               `xml:"feed"`
	XMLNS     string                 `xml:"xmlns,attr"`
	XMLNSApps string                 `xml:"xmlns:apps,attr"`
	Title     string                 `xml:"title"`
	ID        string                 `xml:"id"`
	Updated   string                 `xml:"updated"`
	Author    gmailFiltersXMLAuthor  `xml:"author"`
	Entries   []gmailFiltersXMLEntry `xml:"entry"`
}

type gmailFiltersXMLAuthor struct {
	Name  string `xml:"name"`
	Email string `xml:"email"`
}

type gmailFiltersXMLEntry struct {
	Category   gmailFiltersXMLCategory   `xml:"category"`
	Title      string                    `xml:"title"`
	ID         string                    `xml:"id"`
	Updated    string                    `xml:"updated"`
	Content    string                    `xml:"content"`
	Properties []gmailFiltersXMLProperty `xml:"apps:property"`
}

type gmailFiltersXMLCategory struct {
	Term string `xml:"term,attr"`
}

type gmailFiltersXMLProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

func marshalGmailFiltersXML(account string, filters []*gmail.Filter, labelNames map[string]string) ([]byte, error) {
	nowTime := nowGmailFiltersExport().UTC()
	now := nowTime.Format(time.RFC3339)
	feed := gmailFiltersXMLFeed{
		XMLNS:     gmailFiltersAtomNamespace,
		XMLNSApps: gmailFiltersAppsNamespace,
		Title:     "Mail Filters",
		ID:        fmt.Sprintf("tag:mail.google.com,2008:filters:%d", nowTime.UnixMilli()),
		Updated:   now,
		Author: gmailFiltersXMLAuthor{
			Name:  strings.TrimSpace(account),
			Email: strings.TrimSpace(account),
		},
		Entries: make([]gmailFiltersXMLEntry, 0, len(filters)),
	}

	for _, filter := range filters {
		if filter == nil {
			continue
		}
		entry := gmailFiltersXMLEntry{
			Category: gmailFiltersXMLCategory{Term: "filter"},
			Title:    "Mail Filter",
			ID:       "tag:mail.google.com,2008:filter:" + strings.TrimSpace(filter.Id),
			Updated:  now,
		}
		entry.Properties = append(entry.Properties, gmailFilterCriteriaXMLProperties(filter.Criteria)...)
		entry.Properties = append(entry.Properties, gmailFilterActionXMLProperties(filter.Action, labelNames)...)
		feed.Entries = append(feed.Entries, entry)
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(feed); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func gmailFilterCriteriaXMLProperties(criteria *gmail.FilterCriteria) []gmailFiltersXMLProperty {
	if criteria == nil {
		return nil
	}

	var props []gmailFiltersXMLProperty
	props = appendXMLProperty(props, "from", criteria.From)
	props = appendXMLProperty(props, "to", criteria.To)
	props = appendXMLProperty(props, "subject", criteria.Subject)
	props = appendXMLProperty(props, "hasTheWord", criteria.Query)
	props = appendXMLProperty(props, "doesNotHaveTheWord", criteria.NegatedQuery)
	if criteria.HasAttachment {
		props = appendXMLProperty(props, "hasAttachment", "true")
	}
	if criteria.ExcludeChats {
		props = appendXMLProperty(props, "excludeChats", "true")
	}
	if criteria.Size > 0 {
		props = appendXMLProperty(props, "size", strconv.FormatInt(criteria.Size, 10))
		props = appendXMLProperty(props, "sizeUnit", "s_sb")
		switch strings.ToLower(strings.TrimSpace(criteria.SizeComparison)) {
		case "larger":
			props = appendXMLProperty(props, "sizeOperator", "s_sl")
		case "smaller":
			props = appendXMLProperty(props, "sizeOperator", "s_ss")
		}
	}
	return props
}

func gmailFilterActionXMLProperties(action *gmail.FilterAction, labelNames map[string]string) []gmailFiltersXMLProperty {
	if action == nil {
		return nil
	}

	var props []gmailFiltersXMLProperty
	for _, id := range action.AddLabelIds {
		switch strings.ToUpper(strings.TrimSpace(id)) {
		case "":
			continue
		case gmailSystemLabelStarred:
			props = appendXMLProperty(props, "shouldStar", "true")
		case gmailSystemLabelImportant:
			props = appendXMLProperty(props, "shouldAlwaysMarkAsImportant", "true")
		case gmailSystemLabelTrash:
			props = appendXMLProperty(props, "shouldTrash", "true")
		default:
			if smartLabel := gmailFilterSmartLabelXMLValue(id); smartLabel != "" {
				props = appendXMLProperty(props, "smartLabelToApply", smartLabel)
				continue
			}
			props = appendXMLProperty(props, "label", gmailFilterXMLLabelName(id, labelNames))
		}
	}
	for _, id := range action.RemoveLabelIds {
		switch strings.ToUpper(strings.TrimSpace(id)) {
		case "":
			continue
		case gmailSystemLabelInbox:
			props = appendXMLProperty(props, "shouldArchive", "true")
		case gmailSystemLabelUnread:
			props = appendXMLProperty(props, "shouldMarkAsRead", "true")
		case gmailSystemLabelSpam:
			props = appendXMLProperty(props, "shouldNeverSpam", "true")
		case gmailSystemLabelImportant:
			props = appendXMLProperty(props, "shouldNeverMarkAsImportant", "true")
		}
	}
	props = appendXMLProperty(props, "forwardTo", action.Forward)
	return props
}

func gmailFilterXMLLabelName(id string, labelNames map[string]string) string {
	trimmed := strings.TrimSpace(id)
	if labelNames == nil {
		return trimmed
	}
	if name := strings.TrimSpace(labelNames[trimmed]); name != "" {
		return name
	}
	return trimmed
}

func gmailFilterSmartLabelXMLValue(id string) string {
	switch strings.ToUpper(strings.TrimSpace(id)) {
	case "CATEGORY_PERSONAL":
		return "^smartlabel_personal"
	case "CATEGORY_SOCIAL":
		return "^smartlabel_social"
	case "CATEGORY_PROMOTIONS":
		return "^smartlabel_promo"
	case "CATEGORY_UPDATES":
		return "^smartlabel_notification"
	case "CATEGORY_FORUMS":
		return "^smartlabel_group"
	default:
		return ""
	}
}

func appendXMLProperty(props []gmailFiltersXMLProperty, name, value string) []gmailFiltersXMLProperty {
	value = strings.TrimSpace(value)
	if value == "" {
		return props
	}
	return append(props, gmailFiltersXMLProperty{Name: name, Value: value})
}
