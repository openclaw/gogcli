package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	gapi "google.golang.org/api/googleapi"
	"google.golang.org/api/people/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

const (
	contactsBatchReadLimit   = 200
	contactsBatchDeleteLimit = 500
	contactsBatchPending     = "not_attempted"
	contactsBatchCompleted   = "completed"
	contactsBatchUnconfirmed = "unconfirmed"
)

type ContactsBatchCmd struct {
	Get    ContactsBatchGetCmd    `cmd:"" help:"Get exact contact resources in batches of up to 200"`
	Create ContactsBatchCreateCmd `cmd:"" help:"Create contacts from a JSON array, in batches of up to 200"`
	Update ContactsBatchUpdateCmd `cmd:"" help:"Update contacts from a JSON resource map, preserving supplied CONTACT etags"`
	Delete ContactsBatchDeleteCmd `cmd:"" help:"Delete exact contact resources in batches of up to 500 (requires confirmation)"`
}

type ContactsBatchGetCmd struct {
	ResourceNames []string `arg:"" name:"resourceName" help:"Exact resource names (people/...)"`
}

type ContactsBatchCreateCmd struct {
	FromFile string `name:"from-file" required:"" help:"JSON array of Person objects; - reads stdin (maximum 32 MiB)"`
}

type ContactsBatchUpdateCmd struct {
	FromFile string `name:"from-file" required:"" help:"JSON object keyed by resource name; each Person requires CONTACT source metadata and etag; - reads stdin (maximum 32 MiB)"`
}

type ContactsBatchDeleteCmd struct {
	ResourceNames []string `arg:"" name:"resourceName" help:"Exact contact resource names (people/...)"`
}

type contactsBatchPlan struct {
	ResourceNames []string `json:"resource_names,omitempty"`
	InputIndexes  []int    `json:"input_indexes,omitempty"`
	Request       any      `json:"request"`
}

type contactsBatchOutcome struct {
	Batch         int                 `json:"batch"`
	ResourceNames []string            `json:"resource_names,omitempty"`
	InputIndexes  []int               `json:"input_indexes,omitempty"`
	State         string              `json:"state"`
	Response      any                 `json:"response,omitempty"`
	Error         *contactsBatchError `json:"error,omitempty"`
}

type contactsBatchError struct {
	Message string `json:"message"`
	Code    int    `json:"http_status,omitempty"`
	Details []any  `json:"details,omitempty"`
}

type contactsBatchResult struct {
	Operation string                 `json:"operation"`
	Requested int                    `json:"requested"`
	Completed int                    `json:"completed"`
	Batches   []contactsBatchOutcome `json:"batches"`
}

type contactsBatchGetRequest struct {
	ResourceNames []string `json:"resourceNames"`
	PersonFields  string   `json:"personFields"`
	Sources       []string `json:"sources"`
}

func contactsBatchReadMask() string {
	return strings.Join(contactsDedupeMutableFields, ",") + ",metadata"
}

func (c *ContactsBatchGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	names, err := normalizeContactsBatchResources(c.ResourceNames)
	if err != nil {
		return err
	}
	var plans []contactsBatchPlan
	for start := 0; start < len(names); start += contactsBatchReadLimit {
		chunk := names[start:min(start+contactsBatchReadLimit, len(names))]
		plans = append(plans, contactsBatchPlan{ResourceNames: chunk, Request: contactsBatchGetRequest{
			ResourceNames: chunk, PersonFields: contactsBatchReadMask(), Sources: []string{contactsDedupeContactSource},
		}})
	}
	return runContactsBatch(ctx, flags, "get", plans)
}

func (c *ContactsBatchCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	data, err := readContactsBatchInput(ctx, c.FromFile)
	if err != nil {
		return err
	}
	contacts, err := parseContactsBatchCreate(data)
	if err != nil {
		return err
	}
	var plans []contactsBatchPlan
	for start := 0; start < len(contacts); start += contactsBatchReadLimit {
		end := min(start+contactsBatchReadLimit, len(contacts))
		indexes := make([]int, 0, end-start)
		for index := start; index < end; index++ {
			indexes = append(indexes, index)
		}
		plans = append(plans, contactsBatchPlan{InputIndexes: indexes, Request: &people.BatchCreateContactsRequest{
			Contacts: contacts[start:end], ReadMask: contactsBatchReadMask(), Sources: []string{contactsDedupeContactSource},
		}})
	}
	return runContactsBatch(ctx, flags, "create", plans)
}

func (c *ContactsBatchUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	data, err := readContactsBatchInput(ctx, c.FromFile)
	if err != nil {
		return err
	}
	requests, err := parseContactsBatchUpdates(data)
	if err != nil {
		return err
	}
	plans := make([]contactsBatchPlan, 0, len(requests))
	for _, request := range requests {
		names := make([]string, 0, len(request.Contacts))
		for name := range request.Contacts {
			names = append(names, name)
		}
		sort.Strings(names)
		plans = append(plans, contactsBatchPlan{ResourceNames: names, Request: request})
	}
	return runContactsBatch(ctx, flags, "update", plans)
}

func (c *ContactsBatchDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	names, err := normalizeContactsBatchResources(c.ResourceNames)
	if err != nil {
		return err
	}
	var plans []contactsBatchPlan
	for start := 0; start < len(names); start += contactsBatchDeleteLimit {
		chunk := names[start:min(start+contactsBatchDeleteLimit, len(names))]
		plans = append(plans, contactsBatchPlan{ResourceNames: chunk, Request: &people.BatchDeleteContactsRequest{ResourceNames: chunk}})
	}
	return runContactsBatch(ctx, flags, "delete", plans)
}

func runContactsBatch(ctx context.Context, flags *RootFlags, operation string, plans []contactsBatchPlan) error {
	if operation != "get" {
		if err := enforceContactsBatchMutationPolicy(flags, operation); err != nil {
			return err
		}
	}
	result := contactsBatchResult{Operation: operation, Batches: make([]contactsBatchOutcome, len(plans))}
	for i, plan := range plans {
		result.Requested += len(plan.ResourceNames) + len(plan.InputIndexes)
		result.Batches[i] = contactsBatchOutcome{
			Batch: i + 1, ResourceNames: plan.ResourceNames, InputIndexes: plan.InputIndexes, State: contactsBatchPending,
		}
	}
	payload := map[string]any{"count": result.Requested, "batches": plans}
	if operation == "delete" {
		if err := dryRunAndConfirmDestructive(ctx, flags, "contacts.batch.delete", payload,
			fmt.Sprintf("delete %d contacts in %d batch(es)", result.Requested, len(plans))); err != nil {
			return err
		}
	} else if err := dryRunExit(ctx, flags, "contacts.batch."+operation, payload); err != nil {
		return err
	}
	if operation != "get" && googleapi.ReadOnly(ctx) {
		return googleapi.ErrReadOnly
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := peopleContactsService(ctx, account)
	if err != nil {
		return err
	}
	for i, plan := range plans {
		ui.FromContext(ctx).Err().Linef("Contacts %s: batch %d/%d", operation, i+1, len(plans))
		response, callErr := submitContactsBatch(ctx, svc, plan.Request)
		outcome := &result.Batches[i]
		outcome.Response = response
		if callErr != nil {
			outcome.State = contactsBatchUnconfirmed
			outcome.Error = &contactsBatchError{Message: callErr.Error()}
			var apiErr *gapi.Error
			if errors.As(callErr, &apiErr) {
				outcome.Error.Code = apiErr.Code
				outcome.Error.Details = apiErr.Details
			}
			if err := writeContactsBatchResult(ctx, result); err != nil {
				return errors.Join(callErr, err)
			}
			if operation != "get" {
				return contactsBatchMutationError(fmt.Errorf("batch %d/%d: %w", i+1, len(plans), callErr))
			}
			return fmt.Errorf("contacts batch get stopped at batch %d/%d: %w", i+1, len(plans), callErr)
		}
		outcome.State = contactsBatchCompleted
		result.Completed += len(plan.ResourceNames) + len(plan.InputIndexes)
	}
	return writeContactsBatchResult(ctx, result)
}

func enforceContactsBatchMutationPolicy(flags *RootFlags, operation string) error {
	// Bulk writes must not bypass a denial of the corresponding single-contact write.
	path := []string{"contacts", operation}
	if bakedSafetyDenyMatch(path) || (flags != nil && commandPathMatches(parseEnabledCommands(flags.DisableCommands), path)) {
		return usagef("contacts batch %s is blocked by the contacts.%s command policy", operation, operation)
	}
	return nil
}

func contactsBatchMutationError(err error) error {
	code := ExitCode(stableExitCode(err))
	if code == exitCodeRetryable {
		code = 1
	}
	return &ExitError{Code: code, Err: fmt.Errorf("contacts mutation outcome is unconfirmed; inspect the affected contacts before rerunning: %w", err)}
}

func submitContactsBatch(ctx context.Context, svc *people.Service, request any) (any, error) {
	writeCtx := googleapi.WithoutRetries(ctx)
	switch request := request.(type) {
	case contactsBatchGetRequest:
		response, err := svc.People.GetBatchGet().ResourceNames(request.ResourceNames...).PersonFields(request.PersonFields).
			Sources(request.Sources...).Context(ctx).Do()
		if err == nil {
			_, err = validateContactsBatchGet(request.ResourceNames, response)
		}
		return response, err
	case *people.BatchCreateContactsRequest:
		response, err := svc.People.BatchCreateContacts(request).Context(writeCtx).Do()
		if err == nil {
			if response == nil || len(response.CreatedPeople) != len(request.Contacts) {
				err = fmt.Errorf("create response does not account for every requested contact")
			} else {
				for _, person := range response.CreatedPeople {
					if err = validateContactsBatchPersonResponse(person); err != nil {
						break
					}
				}
			}
		}
		return response, err
	case *people.BatchUpdateContactsRequest:
		response, err := svc.People.BatchUpdateContacts(request).Context(writeCtx).Do()
		if err == nil {
			if response == nil || len(response.UpdateResult) != len(request.Contacts) {
				err = fmt.Errorf("update response does not account for every requested contact")
			} else {
				for name := range request.Contacts {
					person := response.UpdateResult[name]
					if err = validateContactsBatchPersonResponse(&person); err != nil {
						err = fmt.Errorf("contact %s: %w", name, err)
						break
					}
				}
			}
		}
		return response, err
	case *people.BatchDeleteContactsRequest:
		return svc.People.BatchDeleteContacts(request).Context(writeCtx).Do()
	default:
		return nil, fmt.Errorf("unsupported contacts batch request %T", request)
	}
}

func validateContactsBatchPersonResponse(response *people.PersonResponse) error {
	if response == nil {
		return fmt.Errorf("missing person response")
	}
	if response.Status != nil && response.Status.Code != 0 {
		return fmt.Errorf("person response status %d: %s", response.Status.Code, response.Status.Message)
	}
	if response.HttpStatusCode != 0 && (response.HttpStatusCode < 200 || response.HttpStatusCode >= 300) {
		return fmt.Errorf("person response HTTP status %d", response.HttpStatusCode)
	}
	if response.Person == nil || response.Person.ResourceName == "" {
		return fmt.Errorf("missing contact in person response")
	}
	return nil
}

func validateContactsBatchGet(names []string, response *people.GetPeopleResponse) (map[string]*people.Person, error) {
	if response == nil || len(response.Responses) != len(names) {
		return nil, fmt.Errorf("get response does not account for every requested contact")
	}
	requested := make(map[string]bool, len(names))
	for _, name := range names {
		requested[name] = true
	}
	contacts := make(map[string]*people.Person, len(names))
	for _, item := range response.Responses {
		if err := validateContactsBatchPersonResponse(item); err != nil {
			return nil, err
		}
		name := item.RequestedResourceName
		if !requested[name] || contacts[name] != nil {
			return nil, fmt.Errorf("unexpected or repeated resource %q in get response", name)
		}
		contacts[name] = item.Person
	}
	return contacts, nil
}

func writeContactsBatchResult(ctx context.Context, result contactsBatchResult) error {
	if outfmt.IsJSON(ctx) {
		// Partial progress and unattempted inputs are part of the primary result.
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), outfmt.PrimaryResult(result))
	}
	return outfmt.WriteTable(ctx, stdoutWriter(ctx), result.Batches, []outfmt.Column[contactsBatchOutcome]{
		{Header: "BATCH", Value: func(b contactsBatchOutcome) string { return strconv.Itoa(b.Batch) }},
		{Header: "STATE", Value: func(b contactsBatchOutcome) string { return b.State }},
		{Header: "COUNT", Value: func(b contactsBatchOutcome) string { return strconv.Itoa(len(b.ResourceNames) + len(b.InputIndexes)) }},
		{Header: "RESOURCES", Value: contactsBatchResourceOutput},
		{Header: "ERROR", Value: func(b contactsBatchOutcome) string {
			if b.Error != nil {
				return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(b.Error.Message)
			}
			return ""
		}},
	})
}

func contactsBatchResourceOutput(batch contactsBatchOutcome) string {
	names := batch.ResourceNames
	if response, ok := batch.Response.(*people.BatchCreateContactsResponse); ok && response != nil {
		for _, item := range response.CreatedPeople {
			if item != nil && item.Person != nil {
				names = append(names, item.Person.ResourceName)
			}
		}
	}
	return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(strings.Join(names, ","))
}
