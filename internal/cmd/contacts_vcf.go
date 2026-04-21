package cmd

import (
	"fmt"
	"strings"

	"google.golang.org/api/people/v1"
)

const (
	vcfRelHome  = "HOME"
	vcfRelWork  = "WORK"
	vcfRelOther = "OTHER"
	vcfRelCell  = "CELL"
	vcfRelMain  = "MAIN"
)

func exportToVcf(p *people.Person) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\r\n")
	b.WriteString("VERSION:3.0\r\n")
	if name := personFullName(p); name != "" {
		fmt.Fprintf(&b, "FN:%s\r\n", escapeVcfValue(name))
	}
	if name := personStructuredName(p); name != "" {
		fmt.Fprintf(&b, "N:%s\r\n", name)
	}
	for _, email := range allEmails(p) {
		fmt.Fprintf(&b, "EMAIL:%s\r\n", escapeVcfValue(email.value))
	}
	for _, phone := range allPhones(p) {
		fmt.Fprintf(&b, "TEL:%s\r\n", escapeVcfValue(phone.value))
	}
	for _, addr := range allAddresses(p) {
		fmt.Fprintf(&b, "ADR:%s\r\n", escapeVcfValue(addr))
	}
	if org, title := primaryOrganization(p); org != "" {
		fmt.Fprintf(&b, "ORG:%s\r\n", escapeVcfValue(org))
		if title != "" {
			fmt.Fprintf(&b, "TITLE:%s\r\n", escapeVcfValue(title))
		}
	}
	for _, url := range allURLs(p) {
		fmt.Fprintf(&b, "URL:%s\r\n", escapeVcfValue(url))
	}
	if bio := primaryBio(p); bio != "" {
		fmt.Fprintf(&b, "NOTE:%s\r\n", escapeVcfValue(bio))
	}
	if birthday := primaryBirthday(p); birthday != "" {
		fmt.Fprintf(&b, "BDAY:%s\r\n", escapeVcfValue(birthday))
	}
	b.WriteString("END:VCARD\r\n")
	return b.String()
}

func personFullName(p *people.Person) string {
	if p == nil || len(p.Names) == 0 || p.Names[0] == nil {
		return ""
	}
	if p.Names[0].DisplayName != "" {
		return p.Names[0].DisplayName
	}
	return strings.TrimSpace(strings.Join([]string{p.Names[0].GivenName, p.Names[0].FamilyName}, " "))
}

func personStructuredName(p *people.Person) string {
	if p == nil || len(p.Names) == 0 || p.Names[0] == nil {
		return ""
	}
	n := p.Names[0]
	parts := []string{
		nullString(n.FamilyName),
		nullString(n.GivenName),
		nullString(n.MiddleName),
		nullString(n.HonorificPrefix),
		nullString(n.HonorificSuffix),
	}
	return strings.Join(parts, ";")
}

func allEmails(p *people.Person) []emailInfo {
	if p == nil || len(p.EmailAddresses) == 0 {
		return nil
	}
	var emails []emailInfo
	for _, e := range p.EmailAddresses {
		if e == nil || e.Value == "" {
			continue
		}
		emails = append(emails, emailInfo{value: e.Value, relType: vcfRelType(e.Type)})
	}
	return emails
}

type emailInfo struct {
	value   string
	relType string
}

func allPhones(p *people.Person) []phoneInfo {
	if p == nil || len(p.PhoneNumbers) == 0 {
		return nil
	}
	var phones []phoneInfo
	for _, ph := range p.PhoneNumbers {
		if ph == nil || ph.Value == "" {
			continue
		}
		phones = append(phones, phoneInfo{value: ph.Value, relType: vcfRelType(ph.Type)})
	}
	return phones
}

type phoneInfo struct {
	value   string
	relType string
}

func vcfRelType(t string) string {
	switch strings.ToLower(t) {
	case "home":
		return vcfRelHome
	case "work":
		return vcfRelWork
	case "mobile", "cell":
		return vcfRelCell
	case "main":
		return vcfRelMain
	case "other":
		return vcfRelOther
	default:
		if t != "" {
			return strings.ToUpper(t)
		}
		return ""
	}
}

func escapeVcfValue(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

func nullString(s string) string {
	return s
}
