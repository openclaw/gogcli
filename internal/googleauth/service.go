package googleauth

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Service string

const (
	ServiceGmail     Service = "gmail"
	ServiceCalendar  Service = "calendar"
	ServiceChat      Service = "chat"
	ServiceClassroom Service = "classroom"
	ServiceDrive     Service = "drive"
	ServiceDocs      Service = "docs"
	ServiceSlides    Service = "slides"
	ServiceContacts  Service = "contacts"
	ServiceTasks     Service = "tasks"
	ServicePeople    Service = "people"
	ServiceSheets    Service = "sheets"
	ServiceForms     Service = "forms"
	ServiceAppScript Service = "appscript"
	ServiceGroups    Service = "groups"
	ServiceKeep      Service = "keep"
)

const (
	scopeOpenID        = "openid"
	scopeEmail         = "email"
	scopeUserinfoEmail = "https://www.googleapis.com/auth/userinfo.email"
)

var (
	errUnknownService    = errors.New("unknown service")
	errInvalidDriveScope = errors.New("invalid drive scope")
)

type DriveScopeMode string

const (
	DriveScopeFull     DriveScopeMode = "full"
	DriveScopeReadonly DriveScopeMode = "readonly"
	DriveScopeFile     DriveScopeMode = "file"
)

type ScopeOptions struct {
	Readonly   bool
	DriveScope DriveScopeMode
}

type serviceInfo struct {
	scopes []string
	user   bool
	apis   []string
	note   string
}

var serviceOrder = []Service{
	ServiceGmail,
	ServiceCalendar,
	ServiceChat,
	ServiceClassroom,
	ServiceDrive,
	ServiceDocs,
	ServiceSlides,
	ServiceContacts,
	ServiceTasks,
	ServiceSheets,
	ServicePeople,
	ServiceForms,
	ServiceAppScript,
	ServiceGroups,
	ServiceKeep,
}

var serviceInfoByService = map[Service]serviceInfo{
	ServiceGmail: {
		scopes: []string{
			"https://www.googleapis.com/auth/gmail.modify",
			"https://www.googleapis.com/auth/gmail.settings.basic",
			"https://www.googleapis.com/auth/gmail.settings.sharing",
		},
		user: true,
		apis: []string{"Gmail API"},
	},
	ServiceCalendar: {
		scopes: []string{"https://www.googleapis.com/auth/calendar"},
		user:   true,
		apis:   []string{"Calendar API"},
	},
	ServiceChat: {
		scopes: []string{
			"https://www.googleapis.com/auth/chat.spaces",
			"https://www.googleapis.com/auth/chat.messages",
			"https://www.googleapis.com/auth/chat.memberships",
			"https://www.googleapis.com/auth/chat.users.readstate.readonly",
		},
		user: true,
		apis: []string{"Chat API"},
	},
	ServiceClassroom: {
		scopes: []string{
			"https://www.googleapis.com/auth/classroom.courses",
			"https://www.googleapis.com/auth/classroom.rosters",
			"https://www.googleapis.com/auth/classroom.coursework.students",
			"https://www.googleapis.com/auth/classroom.coursework.me",
			"https://www.googleapis.com/auth/classroom.courseworkmaterials",
			"https://www.googleapis.com/auth/classroom.announcements",
			"https://www.googleapis.com/auth/classroom.topics",
			"https://www.googleapis.com/auth/classroom.guardianlinks.students",
			"https://www.googleapis.com/auth/classroom.profile.emails",
			"https://www.googleapis.com/auth/classroom.profile.photos",
		},
		user: true,
		apis: []string{"Classroom API"},
	},
	ServiceDrive: {
		scopes: []string{"https://www.googleapis.com/auth/drive"},
		user:   true,
		apis:   []string{"Drive API"},
	},
	ServiceDocs: {
		// Docs commands are implemented via Drive APIs (export/copy/create),
		// but also request the Docs scope for parity/future use.
		scopes: []string{
			"https://www.googleapis.com/auth/drive",
			"https://www.googleapis.com/auth/documents",
		},
		user: true,
		apis: []string{"Docs API", "Drive API"},
		note: "Export/copy/create via Drive",
	},
	ServiceSlides: {
		// Slides commands use both Slides API and Drive API
		scopes: []string{
			"https://www.googleapis.com/auth/drive",
			"https://www.googleapis.com/auth/presentations",
		},
		user: true,
		apis: []string{"Slides API", "Drive API"},
		note: "Create/edit presentations",
	},
	ServiceContacts: {
		scopes: []string{
			"https://www.googleapis.com/auth/contacts",
			"https://www.googleapis.com/auth/contacts.other.readonly",
			"https://www.googleapis.com/auth/directory.readonly",
		},
		user: true,
		apis: []string{"People API"},
		note: "Contacts + other contacts + directory",
	},
	ServiceTasks: {
		scopes: []string{"https://www.googleapis.com/auth/tasks"},
		user:   true,
		apis:   []string{"Tasks API"},
	},
	ServicePeople: {
		// Needed for "people/me" requests.
		scopes: []string{"profile"},
		user:   true,
		apis:   []string{"People API"},
		note:   "OIDC profile scope",
	},
	ServiceSheets: {
		scopes: []string{
			"https://www.googleapis.com/auth/drive",
			"https://www.googleapis.com/auth/spreadsheets",
		},
		user: true,
		apis: []string{"Sheets API", "Drive API"},
		note: "Export via Drive",
	},
	ServiceForms: {
		scopes: []string{
			"https://www.googleapis.com/auth/forms.body",
			"https://www.googleapis.com/auth/forms.responses.readonly",
		},
		user: true,
		apis: []string{"Forms API"},
	},
	ServiceAppScript: {
		scopes: []string{
			"https://www.googleapis.com/auth/script.projects",
			"https://www.googleapis.com/auth/script.deployments",
			"https://www.googleapis.com/auth/script.processes",
		},
		user: true,
		apis: []string{"Apps Script API"},
	},
	ServiceGroups: {
		scopes: []string{"https://www.googleapis.com/auth/cloud-identity.groups.readonly"},
		user:   false,
		apis:   []string{"Cloud Identity API"},
		note:   "Workspace only",
	},
	ServiceKeep: {
		scopes: []string{"https://www.googleapis.com/auth/keep.readonly"},
		user:   false,
		apis:   []string{"Keep API"},
		note:   "Workspace only; service account (domain-wide delegation)",
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

func manageServices(services []Service) []Service {
	if len(services) == 0 {
		services = UserServices()
	}

	filtered := make([]Service, 0, len(services))
	for _, svc := range services {
		if svc == ServiceKeep {
			continue
		}
		filtered = append(filtered, svc)
	}

	if len(filtered) == 0 {
		return UserServices()
	}

	return filtered
}

func AllServices() []Service {
	out := make([]Service, len(serviceOrder))
	copy(out, serviceOrder)

	return out
}

func Scopes(service Service) ([]string, error) {
	info, ok := serviceInfoByService[service]
	if !ok {
		return nil, errUnknownService
	}

	return append([]string(nil), info.scopes...), nil
}

type ServiceInfo struct {
	Service Service  `json:"service"`
	User    bool     `json:"user"`
	Scopes  []string `json:"scopes"`
	APIs    []string `json:"apis,omitempty"`
	Note    string   `json:"note,omitempty"`
}

func ServicesInfo() []ServiceInfo {
	out := make([]ServiceInfo, 0, len(serviceOrder))
	for _, svc := range serviceOrder {
		info, ok := serviceInfoByService[svc]
		if !ok {
			continue
		}

		out = append(out, ServiceInfo{
			Service: svc,
			User:    info.user,
			Scopes:  append([]string(nil), info.scopes...),
			APIs:    append([]string(nil), info.apis...),
			Note:    info.note,
		})
	}

	return out
}

func ServicesMarkdown(infos []ServiceInfo) string {
	if len(infos) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("| Service | User | APIs | Scopes | Notes |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")

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

func ScopesForServices(services []Service) ([]string, error) {
	set := make(map[string]struct{})

	for _, svc := range services {
		scopes, err := Scopes(svc)
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

func ScopesForManage(services []Service) ([]string, error) {
	scopes, err := ScopesForServices(services)
	if err != nil {
		return nil, err
	}

	return mergeScopes(scopes, []string{scopeOpenID, scopeEmail, scopeUserinfoEmail}), nil
}

func ScopesForManageWithOptions(services []Service, opts ScopeOptions) ([]string, error) {
	scopes, err := scopesForServicesWithOptions(services, opts)
	if err != nil {
		return nil, err
	}

	return mergeScopes(scopes, []string{scopeOpenID, scopeEmail, scopeUserinfoEmail}), nil
}

func scopesForServicesWithOptions(services []Service, opts ScopeOptions) ([]string, error) {
	set := make(map[string]struct{})

	for _, svc := range services {
		scopes, err := scopesForServiceWithOptions(svc, opts)
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

	sort.Strings(out)

	return out, nil
}

func scopesForServiceWithOptions(service Service, opts ScopeOptions) ([]string, error) {
	driveScope := strings.TrimSpace(string(opts.DriveScope))
	switch driveScope {
	case "", string(DriveScopeFull), string(DriveScopeReadonly), string(DriveScopeFile):
	default:
		return nil, fmt.Errorf("%w %q (expected full|readonly|file)", errInvalidDriveScope, opts.DriveScope)
	}

	driveScopeValue := func() string {
		if opts.Readonly {
			return "https://www.googleapis.com/auth/drive.readonly"
		}

		switch opts.DriveScope {
		case DriveScopeFile:
			return "https://www.googleapis.com/auth/drive.file"
		case DriveScopeReadonly:
			return "https://www.googleapis.com/auth/drive.readonly"
		default:
			return "https://www.googleapis.com/auth/drive"
		}
	}

	switch service {
	case ServiceGmail:
		if opts.Readonly {
			return []string{"https://www.googleapis.com/auth/gmail.readonly"}, nil
		}

		return Scopes(service)
	case ServiceCalendar:
		if opts.Readonly {
			return []string{"https://www.googleapis.com/auth/calendar.readonly"}, nil
		}

		return Scopes(service)
	case ServiceChat:
		if opts.Readonly {
			return []string{
				"https://www.googleapis.com/auth/chat.spaces.readonly",
				"https://www.googleapis.com/auth/chat.messages.readonly",
				"https://www.googleapis.com/auth/chat.memberships.readonly",
				"https://www.googleapis.com/auth/chat.users.readstate.readonly",
			}, nil
		}

		return Scopes(service)
	case ServiceClassroom:
		if opts.Readonly {
			return []string{
				"https://www.googleapis.com/auth/classroom.courses.readonly",
				"https://www.googleapis.com/auth/classroom.rosters.readonly",
				"https://www.googleapis.com/auth/classroom.coursework.students.readonly",
				"https://www.googleapis.com/auth/classroom.coursework.me.readonly",
				"https://www.googleapis.com/auth/classroom.courseworkmaterials.readonly",
				"https://www.googleapis.com/auth/classroom.announcements.readonly",
				"https://www.googleapis.com/auth/classroom.topics.readonly",
				"https://www.googleapis.com/auth/classroom.guardianlinks.students.readonly",
				"https://www.googleapis.com/auth/classroom.profile.emails",
				"https://www.googleapis.com/auth/classroom.profile.photos",
			}, nil
		}

		return Scopes(service)
	case ServiceDrive:
		return []string{driveScopeValue()}, nil
	case ServiceDocs:
		docScope := "https://www.googleapis.com/auth/documents"
		if opts.Readonly {
			docScope = "https://www.googleapis.com/auth/documents.readonly"
		}

		return []string{driveScopeValue(), docScope}, nil
	case ServiceSlides:
		slidesScope := "https://www.googleapis.com/auth/presentations"
		if opts.Readonly {
			slidesScope = "https://www.googleapis.com/auth/presentations.readonly"
		}

		return []string{driveScopeValue(), slidesScope}, nil
	case ServiceContacts:
		contactsScope := "https://www.googleapis.com/auth/contacts"
		if opts.Readonly {
			contactsScope = "https://www.googleapis.com/auth/contacts.readonly"
		}

		return []string{
			contactsScope,
			"https://www.googleapis.com/auth/contacts.other.readonly",
			"https://www.googleapis.com/auth/directory.readonly",
		}, nil
	case ServiceTasks:
		if opts.Readonly {
			return []string{"https://www.googleapis.com/auth/tasks.readonly"}, nil
		}

		return Scopes(service)
	case ServicePeople:
		// No read-only equivalent; profile is already read-ish.
		return Scopes(service)
	case ServiceSheets:
		sheetsScope := "https://www.googleapis.com/auth/spreadsheets"
		if opts.Readonly {
			sheetsScope = "https://www.googleapis.com/auth/spreadsheets.readonly"
		}

		return []string{driveScopeValue(), sheetsScope}, nil
	case ServiceForms:
		formBodyScope := "https://www.googleapis.com/auth/forms.body"
		if opts.Readonly {
			formBodyScope = "https://www.googleapis.com/auth/forms.body.readonly"
		}

		return []string{
			formBodyScope,
			"https://www.googleapis.com/auth/forms.responses.readonly",
		}, nil
	case ServiceAppScript:
		if opts.Readonly {
			return []string{
				"https://www.googleapis.com/auth/script.projects.readonly",
				"https://www.googleapis.com/auth/script.deployments.readonly",
			}, nil
		}

		return Scopes(service)
	case ServiceGroups:
		return Scopes(service)
	case ServiceKeep:
		return Scopes(service)
	default:
		return nil, errUnknownService
	}
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
