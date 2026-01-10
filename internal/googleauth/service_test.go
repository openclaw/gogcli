package googleauth

import "testing"

func TestParseService(t *testing.T) {
	tests := []struct {
		in   string
		want Service
	}{
		{"gmail", ServiceGmail},
		{"GMAIL", ServiceGmail},
		{"calendar", ServiceCalendar},
		{"drive", ServiceDrive},
		{"docs", ServiceDocs},
		{"contacts", ServiceContacts},
		{"tasks", ServiceTasks},
		{"people", ServicePeople},
		{"sheets", ServiceSheets},
		{"groups", ServiceGroups},
		{"keep", ServiceKeep},
	}
	for _, tt := range tests {
		got, err := ParseService(tt.in)
		if err != nil {
			t.Fatalf("ParseService(%q) err: %v", tt.in, err)
		}

		if got != tt.want {
			t.Fatalf("ParseService(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseService_Invalid(t *testing.T) {
	if _, err := ParseService("nope"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestExtractCodeAndState(t *testing.T) {
	code, state, err := extractCodeAndState("http://localhost:1/?code=abc&state=xyz")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if code != "abc" || state != "xyz" {
		t.Fatalf("unexpected: code=%q state=%q", code, state)
	}
}

func TestExtractCodeAndState_Errors(t *testing.T) {
	if _, _, err := extractCodeAndState("not a url"); err == nil {
		t.Fatalf("expected error")
	}

	if _, _, err := extractCodeAndState("http://localhost:1/?state=xyz"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAllServices(t *testing.T) {
	svcs := AllServices()
	if len(svcs) != 10 {
		t.Fatalf("unexpected: %v", svcs)
	}
	seen := make(map[Service]bool)

	for _, s := range svcs {
		seen[s] = true
	}

	for _, want := range []Service{ServiceGmail, ServiceCalendar, ServiceDrive, ServiceDocs, ServiceContacts, ServiceTasks, ServicePeople, ServiceSheets, ServiceGroups, ServiceKeep} {
		if !seen[want] {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestUserServices(t *testing.T) {
	svcs := UserServices()
	if len(svcs) != 8 {
		t.Fatalf("unexpected: %v", svcs)
	}

	seenDocs := false
	for _, s := range svcs {
		switch s {
		case ServiceDocs:
			seenDocs = true
		case ServiceKeep:
			t.Fatalf("unexpected keep in user services")
		}
	}

	if !seenDocs {
		t.Fatalf("missing docs in user services")
	}
}

func TestUserServiceCSV(t *testing.T) {
	want := "gmail,calendar,drive,docs,contacts,tasks,sheets,people"
	if got := UserServiceCSV(); got != want {
		t.Fatalf("unexpected user services csv: %q", got)
	}
}

func TestServiceOrderCoverage(t *testing.T) {
	seen := make(map[Service]bool)
	for _, svc := range serviceOrder {
		seen[svc] = true

		if _, ok := serviceInfoByService[svc]; !ok {
			t.Fatalf("missing info for %q", svc)
		}
	}

	for svc := range serviceInfoByService {
		if !seen[svc] {
			t.Fatalf("service %q missing from order", svc)
		}
	}
}

func TestServicesInfo_Metadata(t *testing.T) {
	infos := ServicesInfo()
	if len(infos) != len(serviceOrder) {
		t.Fatalf("unexpected services info length: %d", len(infos))
	}

	docsInfo, foundDocs := findServiceInfo(infos, ServiceDocs)

	if !foundDocs {
		t.Fatalf("missing docs info")
	}

	if len(docsInfo.APIs) == 0 {
		t.Fatalf("docs APIs missing")
	}

	for _, want := range []string{
		"https://www.googleapis.com/auth/drive",
		"https://www.googleapis.com/auth/documents",
	} {
		if !containsScope(docsInfo.Scopes, want) {
			t.Fatalf("docs missing scope %q", want)
		}
	}

	if markdown := ServicesMarkdown(infos); markdown == "" {
		t.Fatalf("expected markdown output")
	}
}

func findServiceInfo(infos []ServiceInfo, svc Service) (ServiceInfo, bool) {
	for _, info := range infos {
		if info.Service == svc {
			return info, true
		}
	}

	return ServiceInfo{}, false
}

func containsScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}

	return false
}

func TestScopesForServices_UnionSorted(t *testing.T) {
	scopes, err := ScopesForServices([]Service{ServiceContacts, ServiceGmail, ServiceTasks, ServicePeople, ServiceContacts}, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(scopes) < 3 {
		t.Fatalf("unexpected scopes: %v", scopes)
	}
	// Ensure stable sorting.
	for i := 1; i < len(scopes); i++ {
		if scopes[i-1] > scopes[i] {
			t.Fatalf("not sorted: %v", scopes)
		}
	}
	// Ensure expected scopes are included.
	want := []string{
		"https://mail.google.com/",
		"https://www.googleapis.com/auth/contacts",
		"https://www.googleapis.com/auth/contacts.other.readonly",
		"https://www.googleapis.com/auth/directory.readonly",
		"https://www.googleapis.com/auth/tasks",
		"profile",
	}
	for _, w := range want {
		found := false
		for _, s := range scopes {
			if s == w {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("missing scope %q in %v", w, scopes)
		}
	}
}

func TestScopes_DocsIncludesDriveAndDocsScopes(t *testing.T) {
	scopes, err := Scopes(ServiceDocs, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	for _, want := range []string{
		"https://www.googleapis.com/auth/drive",
		"https://www.googleapis.com/auth/documents",
	} {
		found := false
		for _, scope := range scopes {
			if scope == want {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("missing %q in %v", want, scopes)
		}
	}
}

func TestScopes_UnknownService(t *testing.T) {
	if _, err := Scopes(Service("nope"), false); err == nil {
		t.Fatalf("expected error")
	}
}

func TestScopes_Readonly(t *testing.T) {
	tests := []struct {
		service Service
		want    []string
	}{
		{ServiceGmail, []string{"https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/gmail.settings.basic"}},
		{ServiceDrive, []string{"https://www.googleapis.com/auth/drive.readonly"}},
		{ServiceCalendar, []string{"https://www.googleapis.com/auth/calendar.readonly"}},
	}

	for _, tt := range tests {
		scopes, err := Scopes(tt.service, true)
		if err != nil {
			t.Fatalf("Scopes(%q, true) err: %v", tt.service, err)
		}

		if len(scopes) != len(tt.want) {
			t.Fatalf("Scopes(%q, true) len=%d, want %d (%v)", tt.service, len(scopes), len(tt.want), scopes)
		}

		for _, wantScope := range tt.want {
			if !containsScope(scopes, wantScope) {
				t.Fatalf("Scopes(%q, true) missing %q", tt.service, wantScope)
			}
		}
	}
}

func TestScopes_ReadonlyFallback(t *testing.T) {
	// ServiceGroups has no readonlyScopes defined, should return default scopes
	scopes, err := Scopes(ServiceGroups, true)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	want := "https://www.googleapis.com/auth/cloud-identity.groups.readonly"
	if !containsScope(scopes, want) {
		t.Fatalf("missing %q", want)
	}
}
