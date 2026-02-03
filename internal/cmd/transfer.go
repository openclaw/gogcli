package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	datatransfer "google.golang.org/api/admin/datatransfer/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newDataTransferService = googleapi.NewDataTransfer

type TransferCmd struct {
	List         TransferListCmd         `cmd:"" name:"list" aliases:"ls" help:"List data transfers"`
	Get          TransferGetCmd          `cmd:"" name:"get" help:"Get data transfer"`
	Create       TransferCreateCmd       `cmd:"" name:"create" aliases:"add" help:"Create data transfer"`
	Applications TransferApplicationsCmd `cmd:"" name:"applications" help:"List transferable applications"`
}

type TransferListCmd struct {
	OldOwner string `name:"old-owner" help:"Filter by old owner email"`
	NewOwner string `name:"new-owner" help:"Filter by new owner email"`
	Status   string `name:"status" help:"Filter by status"`
	Max      int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page     string `name:"page" help:"Page token"`
}

func (c *TransferListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDataTransferService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Transfers.List().CustomerId(adminCustomerID)
	if c.OldOwner != "" {
		call = call.OldOwnerUserId(c.OldOwner)
	}
	if c.NewOwner != "" {
		call = call.NewOwnerUserId(c.NewOwner)
	}
	if c.Status != "" {
		call = call.Status(c.Status)
	}
	if c.Max > 0 {
		call = call.MaxResults(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list transfers: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.DataTransfers) == 0 {
		u.Err().Println("No transfers found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TRANSFER ID\tOLD OWNER\tNEW OWNER\tSTATUS\tAPPLICATIONS")
	for _, transfer := range resp.DataTransfers {
		if transfer == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n",
			sanitizeTab(transfer.Id),
			sanitizeTab(transfer.OldOwnerUserId),
			sanitizeTab(transfer.NewOwnerUserId),
			sanitizeTab(transfer.OverallTransferStatusCode),
			len(transfer.ApplicationDataTransfers),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type TransferGetCmd struct {
	TransferID string `arg:"" name:"transfer-id" help:"Transfer ID"`
}

func (c *TransferGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	transferID := strings.TrimSpace(c.TransferID)
	if transferID == "" {
		return usage("transfer ID is required")
	}

	svc, err := newDataTransferService(ctx, account)
	if err != nil {
		return err
	}

	transfer, err := svc.Transfers.Get(transferID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get transfer %s: %w", transferID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, transfer)
	}

	fmt.Fprintf(os.Stdout, "ID:        %s\n", transfer.Id)
	fmt.Fprintf(os.Stdout, "Old Owner: %s\n", transfer.OldOwnerUserId)
	fmt.Fprintf(os.Stdout, "New Owner: %s\n", transfer.NewOwnerUserId)
	fmt.Fprintf(os.Stdout, "Status:    %s\n", transfer.OverallTransferStatusCode)
	if len(transfer.ApplicationDataTransfers) > 0 {
		fmt.Fprintf(os.Stdout, "Apps:      %d\n", len(transfer.ApplicationDataTransfers))
	}
	return nil
}

type TransferCreateCmd struct {
	OldOwner    string `name:"old-owner" help:"Old owner email" required:""`
	NewOwner    string `name:"new-owner" help:"New owner email" required:""`
	Application string `name:"application" help:"Application ID" required:""`
	Parameters  string `name:"parameters" help:"Transfer parameters (JSON map or key=value list)"`
}

func (c *TransferCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	oldOwner := strings.TrimSpace(c.OldOwner)
	newOwner := strings.TrimSpace(c.NewOwner)
	if oldOwner == "" || newOwner == "" {
		return usage("--old-owner and --new-owner are required")
	}

	appID, err := strconv.ParseInt(strings.TrimSpace(c.Application), 10, 64)
	if err != nil {
		return usage("--application must be a numeric application ID")
	}

	params, err := parseTransferParams(c.Parameters)
	if err != nil {
		return err
	}

	svc, err := newDataTransferService(ctx, account)
	if err != nil {
		return err
	}

	transfer := &datatransfer.DataTransfer{
		OldOwnerUserId: oldOwner,
		NewOwnerUserId: newOwner,
		ApplicationDataTransfers: []*datatransfer.ApplicationDataTransfer{
			{
				ApplicationId:             appID,
				ApplicationTransferParams: params,
			},
		},
	}

	created, err := svc.Transfers.Insert(transfer).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create transfer: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created transfer: %s\n", created.Id)
	return nil
}

type TransferApplicationsCmd struct{}

func (c *TransferApplicationsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDataTransferService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Applications.List().Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list applications: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Applications) == 0 {
		u.Err().Println("No applications found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tPARAMS")
	for _, app := range resp.Applications {
		if app == nil {
			continue
		}
		fmt.Fprintf(w, "%d\t%s\t%d\n",
			app.Id,
			sanitizeTab(app.Name),
			len(app.TransferParams),
		)
	}
	return nil
}

func parseTransferParams(input string) ([]*datatransfer.ApplicationTransferParam, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, nil
	}

	payload, err := readValueOrFile(trimmed)
	if err != nil {
		return nil, err
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, nil
	}

	if strings.HasPrefix(payload, "[") {
		var params []*datatransfer.ApplicationTransferParam
		if err := json.Unmarshal([]byte(payload), &params); err == nil {
			return params, nil
		}
	}

	if strings.HasPrefix(payload, "{") {
		var paramsMap map[string][]string
		if err := json.Unmarshal([]byte(payload), &paramsMap); err == nil {
			return transferParamsFromMap(paramsMap), nil
		}
		var paramsSimple map[string]string
		if err := json.Unmarshal([]byte(payload), &paramsSimple); err == nil {
			paramsMap = make(map[string][]string, len(paramsSimple))
			for key, value := range paramsSimple {
				paramsMap[key] = []string{value}
			}
			return transferParamsFromMap(paramsMap), nil
		}
	}

	pairs := splitCSV(payload)
	paramsMap := make(map[string][]string, len(pairs))
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid parameter %q (expected key=value)", pair)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("invalid parameter %q (empty key)", pair)
		}
		valuePart := strings.TrimSpace(parts[1])
		if valuePart == "" {
			return nil, fmt.Errorf("invalid parameter %q (empty value)", pair)
		}
		values := strings.Split(valuePart, "|")
		for i := range values {
			values[i] = strings.TrimSpace(values[i])
		}
		paramsMap[key] = values
	}

	return transferParamsFromMap(paramsMap), nil
}

func transferParamsFromMap(paramsMap map[string][]string) []*datatransfer.ApplicationTransferParam {
	params := make([]*datatransfer.ApplicationTransferParam, 0, len(paramsMap))
	for key, values := range paramsMap {
		params = append(params, &datatransfer.ApplicationTransferParam{
			Key:   key,
			Value: values,
		})
	}
	return params
}
