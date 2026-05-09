package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/api/people/v1"
)

func writeContactsVCard(w io.Writer, contacts []*people.Person, groupNames map[string]string) error {
	for _, p := range contacts {
		if p == nil {
			continue
		}
		if err := writeContactVCard(w, p, groupNames); err != nil {
			return err
		}
	}
	return nil
}

func writeContactVCard(w io.Writer, p *people.Person, groupNames map[string]string) error {
	lines := []string{
		"BEGIN:VCARD",
		"VERSION:4.0",
		"FN:" + vcardEscapeText(vcardFullName(p)),
	}

	if n := primaryNameEntry(p); n != nil {
		lines = append(lines, "N:"+vcardJoinStructured([]string{
			n.FamilyName,
			n.GivenName,
			n.MiddleName,
			n.HonorificPrefix,
			n.HonorificSuffix,
		}))
	}
	if nicknames := vcardNicknames(p); len(nicknames) > 0 {
		lines = append(lines, "NICKNAME:"+vcardJoinList(nicknames))
	}
	for _, e := range p.EmailAddresses {
		if e == nil || strings.TrimSpace(e.Value) == "" {
			continue
		}
		lines = append(lines, "EMAIL"+vcardTypeParam(e.Type)+":"+vcardEscapeText(strings.TrimSpace(e.Value)))
	}
	for _, ph := range p.PhoneNumbers {
		if ph == nil || strings.TrimSpace(ph.Value) == "" {
			continue
		}
		lines = append(lines, "TEL"+vcardPhoneTypeParam(ph.Type)+":"+vcardEscapeText(strings.TrimSpace(ph.Value)))
	}
	for _, a := range p.Addresses {
		if a == nil || vcardAddressEmpty(a) {
			continue
		}
		lines = append(lines, "ADR"+vcardTypeParam(a.Type)+":"+vcardJoinStructured([]string{
			a.PoBox,
			a.ExtendedAddress,
			a.StreetAddress,
			a.City,
			a.Region,
			a.PostalCode,
			firstNonEmpty(a.Country, a.CountryCode),
		}))
	}
	if bday := vcardBirthday(p); bday != "" {
		lines = append(lines, "BDAY:"+bday)
	}
	if org := primaryOrganizationEntry(p); org != nil {
		if org.Name != "" || org.Department != "" {
			lines = append(lines, "ORG:"+vcardJoinStructured([]string{org.Name, org.Department}))
		}
		if org.Title != "" {
			lines = append(lines, "TITLE:"+vcardEscapeText(org.Title))
		}
	}
	for _, u := range p.Urls {
		if u != nil && strings.TrimSpace(u.Value) != "" {
			lines = append(lines, "URL:"+vcardEscapeText(strings.TrimSpace(u.Value)))
		}
	}
	if note := primaryBio(p); note != "" {
		lines = append(lines, "NOTE:"+vcardEscapeText(note))
	}
	if categories := vcardCategories(p, groupNames); len(categories) > 0 {
		lines = append(lines, "CATEGORIES:"+vcardJoinList(categories))
	}
	lines = append(lines, "END:VCARD")

	for _, line := range lines {
		if err := writeFoldedVCardLine(w, line); err != nil {
			return err
		}
	}
	return nil
}

func primaryNameEntry(p *people.Person) *people.Name {
	if p == nil {
		return nil
	}
	var first *people.Name
	for _, n := range p.Names {
		if n == nil {
			continue
		}
		if first == nil {
			first = n
		}
		if n.Metadata != nil && n.Metadata.Primary {
			return n
		}
	}
	return first
}

func primaryOrganizationEntry(p *people.Person) *people.Organization {
	if p == nil {
		return nil
	}
	var first *people.Organization
	for _, org := range p.Organizations {
		if org == nil {
			continue
		}
		if first == nil {
			first = org
		}
		if org.Metadata != nil && org.Metadata.Primary {
			return org
		}
	}
	return first
}

func vcardFullName(p *people.Person) string {
	if name := primaryName(p); name != "" {
		return name
	}
	return firstNonEmpty(primaryEmail(p), primaryPhone(p), primaryOrganizationName(p), p.ResourceName, "Unnamed Contact")
}

func primaryOrganizationName(p *people.Person) string {
	if org, _ := primaryOrganization(p); org != "" {
		return org
	}
	return ""
}

func vcardNicknames(p *people.Person) []string {
	var out []string
	for _, n := range p.Nicknames {
		if n != nil && strings.TrimSpace(n.Value) != "" {
			out = append(out, strings.TrimSpace(n.Value))
		}
	}
	return out
}

func vcardBirthday(p *people.Person) string {
	b := primaryBirthdayEntry(p)
	if b == nil || b.Date == nil {
		return ""
	}
	d := b.Date
	switch {
	case d.Year > 0 && d.Month > 0 && d.Day > 0:
		return fmt.Sprintf("%04d%02d%02d", d.Year, d.Month, d.Day)
	case d.Month > 0 && d.Day > 0:
		return fmt.Sprintf("--%02d%02d", d.Month, d.Day)
	case d.Year > 0:
		return fmt.Sprintf("%04d", d.Year)
	}
	return ""
}

func primaryBirthdayEntry(p *people.Person) *people.Birthday {
	if p == nil {
		return nil
	}
	var first *people.Birthday
	for _, b := range p.Birthdays {
		if b == nil {
			continue
		}
		if first == nil {
			first = b
		}
		if b.Metadata != nil && b.Metadata.Primary {
			return b
		}
	}
	return first
}

func vcardCategories(p *people.Person, groupNames map[string]string) []string {
	if len(groupNames) == 0 || p == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, m := range p.Memberships {
		if m == nil || m.ContactGroupMembership == nil {
			continue
		}
		name := groupNames[m.ContactGroupMembership.ContactGroupResourceName]
		if strings.TrimSpace(name) == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func vcardAddressEmpty(a *people.Address) bool {
	return firstNonEmpty(a.PoBox, a.ExtendedAddress, a.StreetAddress, a.City, a.Region, a.PostalCode, a.Country, a.CountryCode) == ""
}

func vcardJoinStructured(values []string) string {
	escaped := make([]string, len(values))
	for i, v := range values {
		escaped[i] = vcardEscapeText(v)
	}
	return strings.Join(escaped, ";")
}

func vcardJoinList(values []string) string {
	escaped := make([]string, 0, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			escaped = append(escaped, vcardEscapeText(trimmed))
		}
	}
	return strings.Join(escaped, ",")
}

func vcardEscapeText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	return s
}

func vcardTypeParam(value string) string {
	value = sanitizeVCardParam(value)
	if value == "" {
		return ""
	}
	return ";TYPE=" + value
}

func vcardPhoneTypeParam(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch value {
	case "mobile":
		return ";TYPE=cell"
	case "workMobile":
		return ";TYPE=work,cell"
	case "homeFax":
		return ";TYPE=home,fax"
	case "workFax":
		return ";TYPE=work,fax"
	case "otherFax":
		return ";TYPE=fax"
	case "workPager":
		return ";TYPE=work,pager"
	case "googleVoice", "main":
		return ";TYPE=voice"
	default:
		return vcardTypeParam(value)
	}
}

func sanitizeVCardParam(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func writeFoldedVCardLine(w io.Writer, line string) error {
	for first := true; line != ""; first = false {
		limit := 75
		prefix := ""
		if !first {
			limit = 74
			prefix = " "
		}
		part, rest := splitUTF8ByteLimit(line, limit)
		if _, err := io.WriteString(w, prefix+part+"\r\n"); err != nil {
			return err
		}
		line = rest
	}
	return nil
}

func splitUTF8ByteLimit(s string, limit int) (string, string) {
	if len(s) <= limit {
		return s, ""
	}
	used := 0
	for i, r := range s {
		n := utf8.RuneLen(r)
		if n < 0 {
			n = 1
		}
		if used+n > limit {
			return s[:i], s[i:]
		}
		used += n
	}
	return s, ""
}
