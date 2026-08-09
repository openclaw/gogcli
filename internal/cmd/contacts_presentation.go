package cmd

import (
	"strconv"
	"strings"

	"google.golang.org/api/people/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
)

type contactsDedupeRow struct {
	Group     int
	Action    string
	Person    *people.Person
	MatchedOn string
}

func contactColumns() []outfmt.Column[*people.Person] {
	return []outfmt.Column[*people.Person]{
		{Header: "RESOURCE", Value: func(person *people.Person) string { return person.ResourceName }},
		{Header: "NAME", Value: func(person *people.Person) string { return sanitizeTab(primaryName(person)) }},
		{Header: "EMAIL", Value: func(person *people.Person) string { return sanitizeTab(primaryEmail(person)) }},
		{Header: "PHONE", Value: func(person *people.Person) string { return sanitizeTab(primaryPhone(person)) }},
		{Header: "BIRTHDAY", Value: func(person *people.Person) string {
			return sanitizeTab(primaryBirthday(person))
		}},
	}
}

func directoryPersonColumns() []outfmt.Column[*people.Person] {
	return []outfmt.Column[*people.Person]{
		{Header: "RESOURCE", Value: func(person *people.Person) string { return person.ResourceName }},
		{Header: "NAME", Value: func(person *people.Person) string { return sanitizeTab(primaryName(person)) }},
		{Header: "EMAIL", Value: func(person *people.Person) string { return sanitizeTab(primaryEmail(person)) }},
	}
}

func otherContactColumns() []outfmt.Column[*people.Person] {
	return []outfmt.Column[*people.Person]{
		{Header: "RESOURCE", Value: func(person *people.Person) string { return person.ResourceName }},
		{Header: "NAME", Value: func(person *people.Person) string { return sanitizeTab(primaryName(person)) }},
		{Header: "EMAIL", Value: func(person *people.Person) string { return sanitizeTab(primaryEmail(person)) }},
		{Header: "PHONE", Value: func(person *people.Person) string { return sanitizeTab(primaryPhone(person)) }},
	}
}

func peopleRelationColumns() []outfmt.Column[*people.Relation] {
	return []outfmt.Column[*people.Relation]{
		{Header: "TYPE", Value: func(relation *people.Relation) string {
			typ := relation.Type
			if typ == "" {
				typ = relation.FormattedType
			}
			return sanitizeTab(typ)
		}},
		{Header: "PERSON", Value: func(relation *people.Relation) string {
			return sanitizeTab(relation.Person)
		}},
	}
}

func contactsDedupeColumns() []outfmt.Column[contactsDedupeRow] {
	return []outfmt.Column[contactsDedupeRow]{
		{Header: "GROUP", Value: func(row contactsDedupeRow) string { return strconv.Itoa(row.Group) }},
		{Header: "ACTION", Value: func(row contactsDedupeRow) string { return row.Action }},
		{Header: "RESOURCE", Value: func(row contactsDedupeRow) string {
			return sanitizeTab(contactsDedupeResource(row.Person))
		}},
		{Header: "NAME", Value: func(row contactsDedupeRow) string {
			return sanitizeTab(primaryName(row.Person))
		}},
		{Header: "EMAIL", Value: func(row contactsDedupeRow) string {
			return sanitizeTab(primaryEmail(row.Person))
		}},
		{Header: "PHONE", Value: func(row contactsDedupeRow) string {
			return sanitizeTab(primaryPhone(row.Person))
		}},
		{Header: "MATCHED_ON", Value: func(row contactsDedupeRow) string {
			return sanitizeTab(row.MatchedOn)
		}},
	}
}

func compactPeopleRows(rows []*people.Person) []*people.Person {
	filtered := make([]*people.Person, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func contactSearchRows(results []*people.SearchResult) []*people.Person {
	rows := make([]*people.Person, 0, len(results))
	for _, result := range results {
		if result != nil && result.Person != nil {
			rows = append(rows, result.Person)
		}
	}
	return rows
}

func contactsDedupeRows(groups []contactsDedupeGroup) []contactsDedupeRow {
	rows := make([]contactsDedupeRow, 0)
	for index, group := range groups {
		matchedOn := strings.Join(group.MatchedOn, ",")
		for _, member := range group.Members {
			action := "merge"
			if contactsDedupeResource(member) == contactsDedupeResource(group.Primary) {
				action = "keep"
			}
			rows = append(rows, contactsDedupeRow{
				Group:     index + 1,
				Action:    action,
				Person:    member,
				MatchedOn: matchedOn,
			})
		}
	}
	return rows
}
