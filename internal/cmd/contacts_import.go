package cmd

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ContactsImportCmd struct {
	File string `name:"file" help:"CSV file path (or - for stdin)" required:""`
	User string `name:"user" help:"User email to import contacts for"`
}

func (c *ContactsImportCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	reader, closer, err := openCSVReader(c.File)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}

	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("read csv: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("empty csv")
	}

	header := normalizeCSVHeader(records[0])
	contacts := make([]*people.ContactToCreate, 0, len(records)-1)

	for _, row := range records[1:] {
		if len(row) == 0 {
			continue
		}
		entry := parseCSVRow(header, row)
		person := csvPerson(entry)
		if person == nil {
			continue
		}
		contacts = append(contacts, &people.ContactToCreate{ContactPerson: person})
	}

	if len(contacts) == 0 {
		return fmt.Errorf("no contacts found in csv")
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.People.BatchCreateContacts(&people.BatchCreateContactsRequest{Contacts: contacts}).Do()
	if err != nil {
		return fmt.Errorf("import contacts: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	u.Out().Printf("Imported %d contacts\n", len(contacts))
	return nil
}

type ContactsExportCmd struct {
	File string `name:"file" help:"CSV output path (or - for stdout)" required:""`
	User string `name:"user" help:"User email to export contacts for"`
}

func (c *ContactsExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	contacts := make([]*people.Person, 0)
	pageToken := ""
	for {
		call := svc.People.Connections.List(peopleMeResource).PersonFields(contactsReadMask).PageSize(200)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		var resp *people.ListConnectionsResponse
		resp, err = call.Do()
		if err != nil {
			return fmt.Errorf("list contacts: %w", err)
		}
		contacts = append(contacts, resp.Connections...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	writer, closeFn, err := openCSVWriter(c.File)
	if err != nil {
		return err
	}
	if closeFn != nil {
		defer closeFn.Close()
	}

	if err := writer.Write([]string{"name", "email", "phone"}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, person := range contacts {
		if person == nil {
			continue
		}
		record := []string{primaryName(person), primaryEmail(person), primaryPhone(person)}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("write csv: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"exported": len(contacts), "file": c.File})
	}

	ui.FromContext(ctx).Out().Printf("Exported %d contacts to %s\n", len(contacts), c.File)
	return nil
}

type ContactsDedupCmd struct {
	User  string `name:"user" help:"User email to dedup contacts for"`
	Apply bool   `name:"apply" help:"Delete duplicate contacts"`
}

func (c *ContactsDedupCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	contacts := make([]*people.Person, 0)
	pageToken := ""
	for {
		call := svc.People.Connections.List(peopleMeResource).PersonFields(contactsReadMask).PageSize(200)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return fmt.Errorf("list contacts: %w", err)
		}
		contacts = append(contacts, resp.Connections...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	byEmail := make(map[string][]*people.Person)
	for _, person := range contacts {
		if person == nil {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(primaryEmail(person)))
		if email == "" {
			continue
		}
		byEmail[email] = append(byEmail[email], person)
	}

	duplicates := make([]string, 0)
	deleteTargets := make([]string, 0)
	for email, peopleList := range byEmail {
		if len(peopleList) <= 1 {
			continue
		}
		duplicates = append(duplicates, email)
		if c.Apply {
			for _, person := range peopleList[1:] {
				if person != nil {
					deleteTargets = append(deleteTargets, person.ResourceName)
				}
			}
		}
	}

	if len(duplicates) == 0 {
		u.Err().Println("No duplicates found")
		return nil
	}

	if c.Apply {
		if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete %d duplicate contacts", len(deleteTargets))); err != nil {
			return err
		}
		if _, err := svc.People.BatchDeleteContacts(&people.BatchDeleteContactsRequest{ResourceNames: deleteTargets}).Do(); err != nil {
			return fmt.Errorf("delete duplicates: %w", err)
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"duplicates": duplicates,
			"deleted":    len(deleteTargets),
		})
	}

	if c.Apply {
		u.Out().Printf("Deleted %d duplicate contacts\n", len(deleteTargets))
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "DUPLICATE EMAIL")
	for _, email := range duplicates {
		fmt.Fprintf(w, "%s\n", email)
	}
	u.Err().Println("Run with --apply to delete duplicates")
	return nil
}

func openCSVReader(path string) (*csv.Reader, io.Closer, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, nil, fmt.Errorf("file is required")
	}
	if trimmed == "-" {
		return csv.NewReader(os.Stdin), nil, nil
	}
	f, err := os.Open(trimmed) //nolint:gosec // G304: user-provided file path is intentional
	if err != nil {
		return nil, nil, fmt.Errorf("open csv: %w", err)
	}
	return csv.NewReader(f), f, nil
}

func openCSVWriter(path string) (*csv.Writer, io.Closer, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, nil, fmt.Errorf("file is required")
	}
	if trimmed == "-" {
		return csv.NewWriter(os.Stdout), nil, nil
	}
	f, err := os.Create(trimmed) //nolint:gosec // G304: user-provided file path is intentional
	if err != nil {
		return nil, nil, fmt.Errorf("create csv: %w", err)
	}
	return csv.NewWriter(f), f, nil
}

func normalizeCSVHeader(header []string) []string {
	out := make([]string, len(header))
	for i, h := range header {
		out[i] = strings.ToLower(strings.TrimSpace(h))
	}
	return out
}

func parseCSVRow(header []string, row []string) map[string]string {
	out := make(map[string]string, len(header))
	for i, key := range header {
		if i >= len(row) {
			continue
		}
		out[key] = strings.TrimSpace(row[i])
	}
	return out
}

func csvPerson(entry map[string]string) *people.Person {
	if len(entry) == 0 {
		return nil
	}

	name := entry["name"]
	given := entry["given"]
	family := entry["family"]
	email := entry["email"]
	phone := entry["phone"]

	person := &people.Person{}
	if name != "" || given != "" || family != "" {
		person.Names = []*people.Name{{DisplayName: name, GivenName: given, FamilyName: family}}
	}
	if email != "" {
		person.EmailAddresses = []*people.EmailAddress{{Value: email}}
	}
	if phone != "" {
		person.PhoneNumbers = []*people.PhoneNumber{{Value: phone}}
	}

	if len(person.Names) == 0 && len(person.EmailAddresses) == 0 && len(person.PhoneNumbers) == 0 {
		return nil
	}
	return person
}
