package googleauth

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Service string

const (
	ServiceGmail    Service = "gmail"
	ServiceCalendar Service = "calendar"
	ServiceDrive    Service = "drive"
	ServiceDocs     Service = "docs"
	ServiceContacts Service = "contacts"
	ServiceTasks    Service = "tasks"
	ServicePeople   Service = "people"
	ServiceSheets   Service = "sheets"
	ServiceGroups   Service = "groups"
	ServiceKeep     Service = "keep"
)

const (
	scopeOpenID        = "openid"
	scopeEmail         = "email"
	scopeUserinfoEmail = "https://www.googleapis.com/auth/userinfo.email"
)

var errUnknownService = errors.New("unknown service")

type serviceInfo struct {
	scopes         []string
	readonlyScopes []string

	user bool
	apis []string
	note string
}

var serviceOrder = []Service{
	ServiceGmail,
	ServiceCalendar,
	ServiceDrive,
	ServiceDocs,
	ServiceContacts,
	ServiceTasks,
	ServiceSheets,
	ServicePeople,
	ServiceGroups,
	ServiceKeep,
}

var serviceInfoByService = map[Service]serviceInfo{
	ServiceGmail: {
		scopes: []string{
			"https://mail.google.com/",
			"https://www.googleapis.com/auth/gmail.settings.basic",
		},
		readonlyScopes: []string{
			"https://www.googleapis.com/auth/gmail.readonly",
		},
		user: true,
		apis: []string{"Gmail API"},
	},
	ServiceCalendar: {
		scopes:         []string{"https://www.googleapis.com/auth/calendar"},
		readonlyScopes: []string{"https://www.googleapis.com/auth/calendar.readonly"},
		user:           true,
		apis:           []string{"Calendar API"},
	},
	ServiceDrive: {
		scopes:         []string{"https://www.googleapis.com/auth/drive"},
		readonlyScopes: []string{"https://www.googleapis.com/auth/drive.readonly"},
		user:           true,
		apis:           []string{"Drive API"},
	},
	ServiceDocs: {
		// Docs commands are implemented via Drive APIs (export/copy/create),
		// but also request the Docs scope for parity/future use.
		scopes: []string{
			"https://www.googleapis.com/auth/drive",
			"https://www.googleapis.com/auth/documents",
		},
		readonlyScopes: []string{
			"https://www.googleapis.com/auth/drive.readonly",
			"https://www.googleapis.com/auth/documents.readonly",
		},
		user: true,
		apis: []string{"Docs API", "Drive API"},
		note: "Export/copy/create via Drive",
	},
	ServiceContacts: {
		scopes: []string{
			"https://www.googleapis.com/auth/contacts",
			"https://www.googleapis.com/auth/contacts.other.readonly",
			"https://www.googleapis.com/auth/directory.readonly",
		},
		readonlyScopes: []string{
			"https://www.googleapis.com/auth/contacts.readonly",
			"https://www.googleapis.com/auth/contacts.other.readonly",
			"https://www.googleapis.com/auth/directory.readonly",
		},
		user: true,
		apis: []string{"People API"},
		note: "Contacts + other contacts + directory",
	},
	ServiceTasks: {
		scopes:         []string{"https://www.googleapis.com/auth/tasks"},
		readonlyScopes: []string{"https://www.googleapis.com/auth/tasks.readonly"},
		user:           true,
		apis:           []string{"Tasks API"},
	},
	ServicePeople: {
		// Needed for "people/me" requests.
		scopes: []string{"profile"},
		user:   true,
		apis:   []string{"People API"},
		note:   "OIDC profile scope",
	},
	ServiceSheets: {
		scopes:         []string{"https://www.googleapis.com/auth/spreadsheets"},
		readonlyScopes: []string{"https://www.googleapis.com/auth/spreadsheets.readonly"},
		user:           true,
		apis:           []string{"Sheets API", "Drive API"},
		note:   "Export via Drive",
	},
	ServiceGroups: {
		scopes: []string{"https://www.googleapis.com/auth/cloud-identity.groups.readonly"},
		user:   false,
		apis:   []string{"Cloud Identity API"},
		note:   "Workspace only",
	},
	ServiceKeep: {
		scopes:         []string{"https://www.googleapis.com/auth/keep"},
		readonlyScopes: []string{"https://www.googleapis.com/auth/keep.readonly"},
		user:           false,
		apis:           []string{"Keep API"},
		note:           "Workspace only; service account",
	},
}

func ParseService(s string) (Service, error) {
	parsed := Service(strings.ToLower(strings.TrimSpace(s)))
	if _, ok := serviceInfoByService[parsed]; ok {
		return parsed, nil
	}

	return "", fmt.Errorf("%w %q (expected %s)", errUnknownService, s, serviceNames(AllServices(), "|"))
}

// UserServices are the default OAuth services intended for consumer ("regular") accounts.
func UserServices() []Service {
	return filteredServices(func(info serviceInfo) bool { return info.user })
}

func AllServices() []Service {
	out := make([]Service, len(serviceOrder))
	copy(out, serviceOrder)

	return out
}

func Scopes(service Service, readonly bool) ([]string, error) {
	info, ok := serviceInfoByService[service]
	if !ok {
		return nil, errUnknownService
	}

	if readonly && len(info.readonlyScopes) > 0 {
		return append([]string(nil), info.readonlyScopes...), nil
	}
	return append([]string(nil), info.scopes...), nil
}

type ServiceInfo struct {
	Service        Service  `json:"service"`
	User           bool     `json:"user"`
	Scopes         []string `json:"scopes"`
	ReadonlyScopes []string `json:"readonly_scopes,omitempty"`
	APIs           []string `json:"apis,omitempty"`
	Note           string   `json:"note,omitempty"`
}

func ServicesInfo() []ServiceInfo {
	out := make([]ServiceInfo, 0, len(serviceOrder))
	for _, svc := range serviceOrder {
		info, ok := serviceInfoByService[svc]
		if !ok {
			continue
		}

		out = append(out, ServiceInfo{
			Service:        svc,
			User:           info.user,
			Scopes:         append([]string(nil), info.scopes...),
			ReadonlyScopes: append([]string(nil), info.readonlyScopes...),
			APIs:           append([]string(nil), info.apis...),
			Note:           info.note,
		})
	}

	return out
}

func ServicesMarkdown(infos []ServiceInfo) string {
	if len(infos) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("| Service | User | APIs | Scopes | Readonly Scopes | Notes |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")

	for _, info := range infos {
		userLabel := "no"
		if info.User {
			userLabel = "yes"
		}

		b.WriteString("| ")
		b.WriteString(string(info.Service))
		b.WriteString(" | ")
		b.WriteString(userLabel)
		b.WriteString(" | ")
		b.WriteString(strings.Join(info.APIs, ", "))
		b.WriteString(" | ")
		b.WriteString(markdownScopes(info.Scopes))
		b.WriteString(" | ")
		b.WriteString(markdownScopes(info.ReadonlyScopes))
		b.WriteString(" | ")
		b.WriteString(info.Note)
		b.WriteString(" |\n")
	}

	return b.String()
}

func markdownScopes(scopes []string) string {
	if len(scopes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(scopes))

	for _, scope := range scopes {
		parts = append(parts, "`"+scope+"`")
	}

	return strings.Join(parts, "<br>")
}

func ScopesForServices(services []Service, readonly bool) ([]string, error) {
	set := make(map[string]struct{})

	for _, svc := range services {
		scopes, err := Scopes(svc, readonly)
		if err != nil {
			return nil, err
		}

		for _, s := range scopes {
			set[s] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))

	for s := range set {
		out = append(out, s)
	}
	// stable ordering (useful for tests + auth URL diffs)
	sort.Strings(out)

	return out, nil
}

func ScopesForManage(services []Service, readonly bool) ([]string, error) {
	scopes, err := ScopesForServices(services, readonly)
	if err != nil {
		return nil, err
	}

	return mergeScopes(scopes, []string{scopeOpenID, scopeEmail, scopeUserinfoEmail}), nil
}

func mergeScopes(scopes []string, extras []string) []string {
	set := make(map[string]struct{}, len(scopes)+len(extras))

	for _, s := range scopes {
		if s == "" {
			continue
		}

		set[s] = struct{}{}
	}

	for _, s := range extras {
		if s == "" {
			continue
		}

		set[s] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}

	sort.Strings(out)

	return out
}

func UserServiceCSV() string {
	return serviceNames(UserServices(), ",")
}

func serviceNames(services []Service, sep string) string {
	names := make([]string, 0, len(services))
	for _, svc := range services {
		names = append(names, string(svc))
	}

	return strings.Join(names, sep)
}

func filteredServices(include func(info serviceInfo) bool) []Service {
	out := make([]Service, 0, len(serviceOrder))
	for _, svc := range serviceOrder {
		info, ok := serviceInfoByService[svc]
		if !ok {
			continue
		}

		if include == nil || include(info) {
			out = append(out, svc)
		}
	}

	return out
}
