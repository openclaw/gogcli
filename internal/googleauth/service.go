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
	ServiceContacts Service = "contacts"
	ServiceTasks    Service = "tasks"
	ServicePeople   Service = "people"
	ServiceSheets   Service = "sheets"
)

var errUnknownService = errors.New("unknown service")

func ParseService(s string) (Service, error) {
	switch Service(strings.ToLower(strings.TrimSpace(s))) {
	case ServiceGmail, ServiceCalendar, ServiceDrive, ServiceContacts, ServiceTasks, ServicePeople, ServiceSheets:
		return Service(strings.ToLower(strings.TrimSpace(s))), nil
	default:
		return "", fmt.Errorf("%w %q (expected gmail|calendar|drive|contacts|tasks|people|sheets)", errUnknownService, s)
	}
}

func AllServices() []Service {
	return []Service{ServiceGmail, ServiceCalendar, ServiceDrive, ServiceContacts, ServiceTasks, ServicePeople, ServiceSheets}
}

func Scopes(service Service) ([]string, error) {
	switch service {
	case ServiceGmail:
		return []string{
			"https://mail.google.com/",
			"https://www.googleapis.com/auth/gmail.settings.basic",
		}, nil
	case ServiceCalendar:
		return []string{"https://www.googleapis.com/auth/calendar"}, nil
	case ServiceDrive:
		return []string{"https://www.googleapis.com/auth/drive"}, nil
	case ServiceContacts:
		return []string{
			"https://www.googleapis.com/auth/contacts",
			"https://www.googleapis.com/auth/contacts.other.readonly",
			"https://www.googleapis.com/auth/directory.readonly",
		}, nil
	case ServiceTasks:
		return []string{"https://www.googleapis.com/auth/tasks"}, nil
	case ServicePeople:
		// Needed for "people/me" requests.
		return []string{"profile"}, nil
	case ServiceSheets:
		return []string{"https://www.googleapis.com/auth/spreadsheets"}, nil
	default:
		return nil, errUnknownService
	}
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
