package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	adsenseapi "google.golang.org/api/adsense/v2"
	gapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type AdSenseCmd struct {
	Accounts       AdSenseAccountsCmd       `cmd:"" name:"accounts" aliases:"account" help:"List/get AdSense accounts"`
	AdClients      AdSenseAdClientsCmd      `cmd:"" name:"adclients" aliases:"adclient" help:"List/get ad clients"`
	AdUnits        AdSenseAdUnitsCmd        `cmd:"" name:"adunits" aliases:"adunit" help:"List/get ad units"`
	CustomChannels AdSenseCustomChannelsCmd `cmd:"" name:"customchannels" aliases:"customchannel" help:"List/get custom channels"`
	UrlChannels    AdSenseUrlChannelsCmd    `cmd:"" name:"urlchannels" aliases:"urlchannel" help:"List/get URL channels"`
	Alerts         AdSenseAlertsCmd         `cmd:"" name:"alerts" help:"List account alerts"`
	Payments       AdSensePaymentsCmd       `cmd:"" name:"payments" help:"List account payments"`
	PolicyIssues   AdSensePolicyIssuesCmd   `cmd:"" name:"policyissues" aliases:"policyissue" help:"List/get policy issues"`
	Sites          AdSenseSitesCmd          `cmd:"" name:"sites" help:"List/get AdSense sites"`
	Reports        AdSenseReportsCmd        `cmd:"" name:"reports" aliases:"report" help:"Generate AdSense reports"`
}

// adSensePager is satisfied by the generated *...ListCall types: each exposes
// PageSize/PageToken returning itself and a typed Do.
type adSensePager[C any, R any] interface {
	PageSize(int64) C
	PageToken(string) C
	Do(...gapi.CallOption) (*R, error)
}

// adSenseFetchPage applies pagination to a generated list call and executes it.
func adSenseFetchPage[C adSensePager[C, R], R any](call C, pageSize int64, pageToken string) (*R, error) {
	call = call.PageSize(pageSize)
	if strings.TrimSpace(pageToken) != "" {
		call = call.PageToken(pageToken)
	}
	return call.Do()
}

// adSenseListPage lists one page (or, via collectAllPages, all pages) of an
// AdSense collection and renders it as a table or JSON.
type adSenseListPage[T any] struct {
	flags     *RootFlags
	max       int64
	page      string
	all       bool
	failEmpty bool
	jsonKey   string
	emptyMsg  string
	header    string
	fetch     func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]T, string, error)
	printRow  func(w io.Writer, item T)
}

func runAdSenseList[T any](ctx context.Context, p adSenseListPage[T]) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(p.flags)
	if err != nil {
		return err
	}
	if p.max <= 0 {
		return usage("--max must be > 0")
	}

	svc, err := adSenseService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]T, string, error) {
		return p.fetch(svc, p.max, pageToken)
	}

	var items []T
	nextPageToken := ""
	if p.all {
		all, collectErr := collectAllPages(p.page, fetch)
		if collectErr != nil {
			return collectErr
		}
		items = all
	} else {
		items, nextPageToken, err = fetch(p.page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			p.jsonKey:       items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(p.failEmpty)
		}
		return nil
	}

	if len(items) == 0 {
		u.Err().Println(p.emptyMsg)
		return failEmptyExit(p.failEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, p.header)
	for _, item := range items {
		p.printRow(w, item)
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

// runAdSenseGet fetches a single AdSense resource and renders it as key/value
// text or JSON.
func runAdSenseGet[T any](
	ctx context.Context,
	flags *RootFlags,
	rawName string,
	normalize func(string) (string, error),
	fetch func(svc *adsenseapi.Service, name string) (T, error),
	jsonKey string,
	kvs func(item T) []resultKV,
) error {
	u := ui.FromContext(ctx)
	name, err := normalize(rawName)
	if err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := adSenseService(ctx, account)
	if err != nil {
		return err
	}
	item, err := fetch(svc, name)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{jsonKey: item})
	}

	return writeResult(ctx, u, kvs(item)...)
}

func requireAdSenseResourceArg(name string) (string, error) {
	return requireAdSenseResource(name, "name")
}

// Accounts

type AdSenseAccountsCmd struct {
	List     AdSenseAccountsListCmd     `cmd:"" default:"withargs" aliases:"ls" help:"List accessible AdSense accounts"`
	Get      AdSenseAccountsGetCmd      `cmd:"" name:"get" aliases:"info,show" help:"Get a specific AdSense account"`
	Children AdSenseAccountsChildrenCmd `cmd:"" name:"children" aliases:"list-children" help:"List child accounts of a manager account"`
}

type AdSenseAccountsListCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max accounts per page" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AdSenseAccountsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAdSenseList(ctx, adSenseListPage[*adsenseapi.Account]{
		flags: flags, max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty,
		jsonKey:  "accounts",
		emptyMsg: "No AdSense accounts",
		header:   "ACCOUNT\tDISPLAY_NAME\tSTATE\tPREMIUM",
		fetch: func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]*adsenseapi.Account, string, error) {
			resp, err := adSenseFetchPage[*adsenseapi.AccountsListCall, adsenseapi.ListAccountsResponse](
				svc.Accounts.List().Context(ctx), pageSize, pageToken)
			if err != nil {
				return nil, "", err
			}
			return resp.Accounts, resp.NextPageToken, nil
		},
		printRow: printAdSenseAccountRow,
	})
}

func printAdSenseAccountRow(w io.Writer, item *adsenseapi.Account) {
	if item == nil {
		return
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%t\n",
		sanitizeTab(adSenseResourceID(item.Name)),
		sanitizeTab(item.DisplayName),
		sanitizeTab(item.State),
		item.Premium,
	)
}

type AdSenseAccountsGetCmd struct {
	Account string `arg:"" name:"account" help:"AdSense account (e.g. pub-1234567890123456 or accounts/pub-1234567890123456)"`
}

func (c *AdSenseAccountsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAdSenseGet(ctx, flags, c.Account, normalizeAdSenseAccount,
		func(svc *adsenseapi.Service, name string) (*adsenseapi.Account, error) {
			return svc.Accounts.Get(name).Context(ctx).Do()
		},
		"account",
		func(acct *adsenseapi.Account) []resultKV {
			return []resultKV{
				kv("name", acct.Name),
				kv("display_name", acct.DisplayName),
				kv("state", acct.State),
				kv("premium", acct.Premium),
				kv("time_zone", adSenseTimeZoneID(acct.TimeZone)),
			}
		},
	)
}

type AdSenseAccountsChildrenCmd struct {
	Parent    string `arg:"" name:"parent" help:"Parent AdSense manager account (e.g. pub-... or accounts/pub-...)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max accounts per page" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AdSenseAccountsChildrenCmd) Run(ctx context.Context, flags *RootFlags) error {
	parent, err := normalizeAdSenseAccount(c.Parent)
	if err != nil {
		return err
	}
	return runAdSenseList(ctx, adSenseListPage[*adsenseapi.Account]{
		flags: flags, max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty,
		jsonKey:  "accounts",
		emptyMsg: "No child accounts",
		header:   "ACCOUNT\tDISPLAY_NAME\tSTATE\tPREMIUM",
		fetch: func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]*adsenseapi.Account, string, error) {
			resp, err := adSenseFetchPage[*adsenseapi.AccountsListChildAccountsCall, adsenseapi.ListChildAccountsResponse](
				svc.Accounts.ListChildAccounts(parent).Context(ctx), pageSize, pageToken)
			if err != nil {
				return nil, "", err
			}
			return resp.Accounts, resp.NextPageToken, nil
		},
		printRow: printAdSenseAccountRow,
	})
}

// Ad clients

type AdSenseAdClientsCmd struct {
	List   AdSenseAdClientsListCmd   `cmd:"" default:"withargs" aliases:"ls" help:"List ad clients for an account"`
	Get    AdSenseAdClientsGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get a specific ad client"`
	AdCode AdSenseAdClientsAdCodeCmd `cmd:"" name:"adcode" help:"Get the ad code snippet for an ad client"`
}

type AdSenseAdClientsListCmd struct {
	Account   string `arg:"" name:"account" help:"AdSense account (e.g. pub-... or accounts/pub-...)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max ad clients per page" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AdSenseAdClientsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	parent, err := normalizeAdSenseAccount(c.Account)
	if err != nil {
		return err
	}
	return runAdSenseList(ctx, adSenseListPage[*adsenseapi.AdClient]{
		flags: flags, max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty,
		jsonKey:  "adClients",
		emptyMsg: "No ad clients",
		header:   "AD_CLIENT\tPRODUCT_CODE\tSTATE\tREPORTING_DIMENSION_ID",
		fetch: func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]*adsenseapi.AdClient, string, error) {
			resp, err := adSenseFetchPage[*adsenseapi.AccountsAdclientsListCall, adsenseapi.ListAdClientsResponse](
				svc.Accounts.Adclients.List(parent).Context(ctx), pageSize, pageToken)
			if err != nil {
				return nil, "", err
			}
			return resp.AdClients, resp.NextPageToken, nil
		},
		printRow: func(w io.Writer, item *adsenseapi.AdClient) {
			if item == nil {
				return
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				sanitizeTab(adSenseResourceID(item.Name)),
				sanitizeTab(item.ProductCode),
				sanitizeTab(item.State),
				sanitizeTab(item.ReportingDimensionId),
			)
		},
	})
}

type AdSenseAdClientsGetCmd struct {
	Name string `arg:"" name:"name" help:"Full ad client resource name (e.g. accounts/pub-.../adclients/ca-...)"`
}

func (c *AdSenseAdClientsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAdSenseGet(ctx, flags, c.Name, requireAdSenseResourceArg,
		func(svc *adsenseapi.Service, name string) (*adsenseapi.AdClient, error) {
			return svc.Accounts.Adclients.Get(name).Context(ctx).Do()
		},
		"adClient",
		func(adClient *adsenseapi.AdClient) []resultKV {
			return []resultKV{
				kv("name", adClient.Name),
				kv("product_code", adClient.ProductCode),
				kv("state", adClient.State),
				kv("reporting_dimension_id", adClient.ReportingDimensionId),
			}
		},
	)
}

type AdSenseAdClientsAdCodeCmd struct {
	Name string `arg:"" name:"name" help:"Full ad client resource name (e.g. accounts/pub-.../adclients/ca-...)"`
}

func (c *AdSenseAdClientsAdCodeCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAdSenseGet(ctx, flags, c.Name, requireAdSenseResourceArg,
		func(svc *adsenseapi.Service, name string) (*adsenseapi.AdClientAdCode, error) {
			return svc.Accounts.Adclients.GetAdcode(name).Context(ctx).Do()
		},
		"adCode",
		func(adCode *adsenseapi.AdClientAdCode) []resultKV {
			return []resultKV{
				kv("ad_code", adCode.AdCode),
				kv("amp_head", adCode.AmpHead),
				kv("amp_body", adCode.AmpBody),
			}
		},
	)
}

// Ad units

type AdSenseAdUnitsCmd struct {
	List                 AdSenseAdUnitsListCmd                 `cmd:"" default:"withargs" aliases:"ls" help:"List ad units for an ad client"`
	Get                  AdSenseAdUnitsGetCmd                  `cmd:"" name:"get" aliases:"info,show" help:"Get a specific ad unit"`
	AdCode               AdSenseAdUnitsAdCodeCmd               `cmd:"" name:"adcode" help:"Get the ad code snippet for an ad unit"`
	LinkedCustomChannels AdSenseAdUnitsLinkedCustomChannelsCmd `cmd:"" name:"linkedcustomchannels" aliases:"customchannels" help:"List custom channels linked to an ad unit"`
}

type AdSenseAdUnitsListCmd struct {
	AdClient  string `arg:"" name:"adclient" help:"Full ad client resource name (e.g. accounts/pub-.../adclients/ca-...)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max ad units per page" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func printAdSenseAdUnitRow(w io.Writer, item *adsenseapi.AdUnit) {
	if item == nil {
		return
	}
	fmt.Fprintf(w, "%s\t%s\t%s\n",
		sanitizeTab(adSenseResourceID(item.Name)),
		sanitizeTab(item.DisplayName),
		sanitizeTab(item.State),
	)
}

// Run bodies below share the runAdSenseList/adSenseFetchPage shape by design;
// each wires a distinct AdSense sub-resource and can't be factored further
// without losing per-endpoint type safety.
//
//nolint:dupl
func (c *AdSenseAdUnitsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	parent, err := requireAdSenseResource(c.AdClient, "adclient")
	if err != nil {
		return err
	}
	return runAdSenseList(ctx, adSenseListPage[*adsenseapi.AdUnit]{
		flags: flags, max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty,
		jsonKey:  "adUnits",
		emptyMsg: "No ad units",
		header:   "AD_UNIT\tDISPLAY_NAME\tSTATE",
		fetch: func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]*adsenseapi.AdUnit, string, error) {
			resp, err := adSenseFetchPage[*adsenseapi.AccountsAdclientsAdunitsListCall, adsenseapi.ListAdUnitsResponse](
				svc.Accounts.Adclients.Adunits.List(parent).Context(ctx), pageSize, pageToken)
			if err != nil {
				return nil, "", err
			}
			return resp.AdUnits, resp.NextPageToken, nil
		},
		printRow: printAdSenseAdUnitRow,
	})
}

type AdSenseAdUnitsGetCmd struct {
	Name string `arg:"" name:"name" help:"Full ad unit resource name (e.g. accounts/pub-.../adclients/ca-.../adunits/...)"`
}

func (c *AdSenseAdUnitsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAdSenseGet(ctx, flags, c.Name, requireAdSenseResourceArg,
		func(svc *adsenseapi.Service, name string) (*adsenseapi.AdUnit, error) {
			return svc.Accounts.Adclients.Adunits.Get(name).Context(ctx).Do()
		},
		"adUnit",
		func(adUnit *adsenseapi.AdUnit) []resultKV {
			return []resultKV{
				kv("name", adUnit.Name),
				kv("display_name", adUnit.DisplayName),
				kv("state", adUnit.State),
				kv("reporting_dimension_id", adUnit.ReportingDimensionId),
			}
		},
	)
}

type AdSenseAdUnitsAdCodeCmd struct {
	Name string `arg:"" name:"name" help:"Full ad unit resource name (e.g. accounts/pub-.../adclients/ca-.../adunits/...)"`
}

func (c *AdSenseAdUnitsAdCodeCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAdSenseGet(ctx, flags, c.Name, requireAdSenseResourceArg,
		func(svc *adsenseapi.Service, name string) (*adsenseapi.AdUnitAdCode, error) {
			return svc.Accounts.Adclients.Adunits.GetAdcode(name).Context(ctx).Do()
		},
		"adCode",
		func(adCode *adsenseapi.AdUnitAdCode) []resultKV {
			return []resultKV{kv("ad_code", adCode.AdCode)}
		},
	)
}

type AdSenseAdUnitsLinkedCustomChannelsCmd struct {
	AdUnit    string `arg:"" name:"adunit" help:"Full ad unit resource name (e.g. accounts/pub-.../adclients/ca-.../adunits/...)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max custom channels per page" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

//nolint:dupl // see comment above AdSenseAdUnitsListCmd.Run
func (c *AdSenseAdUnitsLinkedCustomChannelsCmd) Run(ctx context.Context, flags *RootFlags) error {
	parent, err := requireAdSenseResource(c.AdUnit, "adunit")
	if err != nil {
		return err
	}
	return runAdSenseList(ctx, adSenseListPage[*adsenseapi.CustomChannel]{
		flags: flags, max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty,
		jsonKey:  "customChannels",
		emptyMsg: "No linked custom channels",
		header:   "CUSTOM_CHANNEL\tDISPLAY_NAME\tACTIVE",
		fetch: func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]*adsenseapi.CustomChannel, string, error) {
			resp, err := adSenseFetchPage[*adsenseapi.AccountsAdclientsAdunitsListLinkedCustomChannelsCall, adsenseapi.ListLinkedCustomChannelsResponse](
				svc.Accounts.Adclients.Adunits.ListLinkedCustomChannels(parent).Context(ctx), pageSize, pageToken)
			if err != nil {
				return nil, "", err
			}
			return resp.CustomChannels, resp.NextPageToken, nil
		},
		printRow: printAdSenseCustomChannelRow,
	})
}

// Custom channels

type AdSenseCustomChannelsCmd struct {
	List          AdSenseCustomChannelsListCmd          `cmd:"" default:"withargs" aliases:"ls" help:"List custom channels for an ad client"`
	Get           AdSenseCustomChannelsGetCmd           `cmd:"" name:"get" aliases:"info,show" help:"Get a specific custom channel"`
	LinkedAdUnits AdSenseCustomChannelsLinkedAdUnitsCmd `cmd:"" name:"linkedadunits" aliases:"adunits" help:"List ad units linked to a custom channel"`
}

type AdSenseCustomChannelsListCmd struct {
	AdClient  string `arg:"" name:"adclient" help:"Full ad client resource name (e.g. accounts/pub-.../adclients/ca-...)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max custom channels per page" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func printAdSenseCustomChannelRow(w io.Writer, item *adsenseapi.CustomChannel) {
	if item == nil {
		return
	}
	fmt.Fprintf(w, "%s\t%s\t%t\n",
		sanitizeTab(adSenseResourceID(item.Name)),
		sanitizeTab(item.DisplayName),
		item.Active,
	)
}

//nolint:dupl // see comment above AdSenseAdUnitsListCmd.Run
func (c *AdSenseCustomChannelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	parent, err := requireAdSenseResource(c.AdClient, "adclient")
	if err != nil {
		return err
	}
	return runAdSenseList(ctx, adSenseListPage[*adsenseapi.CustomChannel]{
		flags: flags, max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty,
		jsonKey:  "customChannels",
		emptyMsg: "No custom channels",
		header:   "CUSTOM_CHANNEL\tDISPLAY_NAME\tACTIVE",
		fetch: func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]*adsenseapi.CustomChannel, string, error) {
			resp, err := adSenseFetchPage[*adsenseapi.AccountsAdclientsCustomchannelsListCall, adsenseapi.ListCustomChannelsResponse](
				svc.Accounts.Adclients.Customchannels.List(parent).Context(ctx), pageSize, pageToken)
			if err != nil {
				return nil, "", err
			}
			return resp.CustomChannels, resp.NextPageToken, nil
		},
		printRow: printAdSenseCustomChannelRow,
	})
}

type AdSenseCustomChannelsGetCmd struct {
	Name string `arg:"" name:"name" help:"Full custom channel resource name (e.g. accounts/pub-.../adclients/ca-.../customchannels/...)"`
}

func (c *AdSenseCustomChannelsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAdSenseGet(ctx, flags, c.Name, requireAdSenseResourceArg,
		func(svc *adsenseapi.Service, name string) (*adsenseapi.CustomChannel, error) {
			return svc.Accounts.Adclients.Customchannels.Get(name).Context(ctx).Do()
		},
		"customChannel",
		func(channel *adsenseapi.CustomChannel) []resultKV {
			return []resultKV{
				kv("name", channel.Name),
				kv("display_name", channel.DisplayName),
				kv("active", channel.Active),
				kv("reporting_dimension_id", channel.ReportingDimensionId),
			}
		},
	)
}

type AdSenseCustomChannelsLinkedAdUnitsCmd struct {
	CustomChannel string `arg:"" name:"customchannel" help:"Full custom channel resource name (e.g. accounts/pub-.../adclients/ca-.../customchannels/...)"`
	Max           int64  `name:"max" aliases:"limit" help:"Max ad units per page" default:"50"`
	Page          string `name:"page" aliases:"cursor" help:"Page token"`
	All           bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty     bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

//nolint:dupl // see comment above AdSenseAdUnitsListCmd.Run
func (c *AdSenseCustomChannelsLinkedAdUnitsCmd) Run(ctx context.Context, flags *RootFlags) error {
	parent, err := requireAdSenseResource(c.CustomChannel, "customchannel")
	if err != nil {
		return err
	}
	return runAdSenseList(ctx, adSenseListPage[*adsenseapi.AdUnit]{
		flags: flags, max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty,
		jsonKey:  "adUnits",
		emptyMsg: "No linked ad units",
		header:   "AD_UNIT\tDISPLAY_NAME\tSTATE",
		fetch: func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]*adsenseapi.AdUnit, string, error) {
			resp, err := adSenseFetchPage[*adsenseapi.AccountsAdclientsCustomchannelsListLinkedAdUnitsCall, adsenseapi.ListLinkedAdUnitsResponse](
				svc.Accounts.Adclients.Customchannels.ListLinkedAdUnits(parent).Context(ctx), pageSize, pageToken)
			if err != nil {
				return nil, "", err
			}
			return resp.AdUnits, resp.NextPageToken, nil
		},
		printRow: printAdSenseAdUnitRow,
	})
}

// URL channels

type AdSenseUrlChannelsCmd struct {
	List AdSenseUrlChannelsListCmd `cmd:"" default:"withargs" aliases:"ls" help:"List URL channels for an ad client"`
	Get  AdSenseUrlChannelsGetCmd  `cmd:"" name:"get" aliases:"info,show" help:"Get a specific URL channel"`
}

type AdSenseUrlChannelsListCmd struct {
	AdClient  string `arg:"" name:"adclient" help:"Full ad client resource name (e.g. accounts/pub-.../adclients/ca-...)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max URL channels per page" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

//nolint:dupl // see comment above AdSenseAdUnitsListCmd.Run
func (c *AdSenseUrlChannelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	parent, err := requireAdSenseResource(c.AdClient, "adclient")
	if err != nil {
		return err
	}
	return runAdSenseList(ctx, adSenseListPage[*adsenseapi.UrlChannel]{
		flags: flags, max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty,
		jsonKey:  "urlChannels",
		emptyMsg: "No URL channels",
		header:   "URL_CHANNEL\tURI_PATTERN",
		fetch: func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]*adsenseapi.UrlChannel, string, error) {
			resp, err := adSenseFetchPage[*adsenseapi.AccountsAdclientsUrlchannelsListCall, adsenseapi.ListUrlChannelsResponse](
				svc.Accounts.Adclients.Urlchannels.List(parent).Context(ctx), pageSize, pageToken)
			if err != nil {
				return nil, "", err
			}
			return resp.UrlChannels, resp.NextPageToken, nil
		},
		printRow: func(w io.Writer, item *adsenseapi.UrlChannel) {
			if item == nil {
				return
			}
			fmt.Fprintf(w, "%s\t%s\n",
				sanitizeTab(adSenseResourceID(item.Name)),
				sanitizeTab(item.UriPattern),
			)
		},
	})
}

type AdSenseUrlChannelsGetCmd struct {
	Name string `arg:"" name:"name" help:"Full URL channel resource name (e.g. accounts/pub-.../adclients/ca-.../urlchannels/...)"`
}

func (c *AdSenseUrlChannelsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAdSenseGet(ctx, flags, c.Name, requireAdSenseResourceArg,
		func(svc *adsenseapi.Service, name string) (*adsenseapi.UrlChannel, error) {
			return svc.Accounts.Adclients.Urlchannels.Get(name).Context(ctx).Do()
		},
		"urlChannel",
		func(channel *adsenseapi.UrlChannel) []resultKV {
			return []resultKV{
				kv("name", channel.Name),
				kv("uri_pattern", channel.UriPattern),
				kv("reporting_dimension_id", channel.ReportingDimensionId),
			}
		},
	)
}

// Alerts

type AdSenseAlertsCmd struct {
	List AdSenseAlertsListCmd `cmd:"" default:"withargs" aliases:"ls" help:"List account alerts"`
}

type AdSenseAlertsListCmd struct {
	Account   string `arg:"" name:"account" help:"AdSense account (e.g. pub-... or accounts/pub-...)"`
	Language  string `name:"language" aliases:"lang" help:"Language code for alert messages (e.g. en-US)"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AdSenseAlertsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	parent, err := normalizeAdSenseAccount(c.Account)
	if err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := adSenseService(ctx, account)
	if err != nil {
		return err
	}
	call := svc.Accounts.Alerts.List(parent).Context(ctx)
	if v := strings.TrimSpace(c.Language); v != "" {
		call = call.LanguageCode(v)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	rows := resp.Alerts
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"alerts": rows}); err != nil {
			return err
		}
		if len(rows) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(rows) == 0 {
		u.Err().Println("No AdSense alerts")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "SEVERITY\tTYPE\tMESSAGE")
	for _, item := range rows {
		if item == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(item.Severity),
			sanitizeTab(item.Type),
			sanitizeTab(item.Message),
		)
	}
	return nil
}

// Payments

type AdSensePaymentsCmd struct {
	List AdSensePaymentsListCmd `cmd:"" default:"withargs" aliases:"ls" help:"List account payments"`
}

type AdSensePaymentsListCmd struct {
	Account   string `arg:"" name:"account" help:"AdSense account (e.g. pub-... or accounts/pub-...)"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AdSensePaymentsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	parent, err := normalizeAdSenseAccount(c.Account)
	if err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := adSenseService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Accounts.Payments.List(parent).Context(ctx).Do()
	if err != nil {
		return err
	}

	rows := resp.Payments
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"payments": rows}); err != nil {
			return err
		}
		if len(rows) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(rows) == 0 {
		u.Err().Println("No AdSense payments")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tDATE\tAMOUNT")
	for _, item := range rows {
		if item == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(adSenseResourceID(item.Name)),
			sanitizeTab(adSenseDateString(item.Date)),
			sanitizeTab(item.Amount),
		)
	}
	return nil
}

// Policy issues

type AdSensePolicyIssuesCmd struct {
	List AdSensePolicyIssuesListCmd `cmd:"" default:"withargs" aliases:"ls" help:"List policy issues for an account"`
	Get  AdSensePolicyIssuesGetCmd  `cmd:"" name:"get" aliases:"info,show" help:"Get a specific policy issue"`
}

type AdSensePolicyIssuesListCmd struct {
	Account   string `arg:"" name:"account" help:"AdSense account (e.g. pub-... or accounts/pub-...)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max policy issues per page" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AdSensePolicyIssuesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	parent, err := normalizeAdSenseAccount(c.Account)
	if err != nil {
		return err
	}
	return runAdSenseList(ctx, adSenseListPage[*adsenseapi.PolicyIssue]{
		flags: flags, max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty,
		jsonKey:  "policyIssues",
		emptyMsg: "No policy issues",
		header:   "NAME\tSITE\tENTITY_TYPE\tACTION\tAD_REQUESTS\tTOPICS",
		fetch: func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]*adsenseapi.PolicyIssue, string, error) {
			resp, err := adSenseFetchPage[*adsenseapi.AccountsPolicyIssuesListCall, adsenseapi.ListPolicyIssuesResponse](
				svc.Accounts.PolicyIssues.List(parent).Context(ctx), pageSize, pageToken)
			if err != nil {
				return nil, "", err
			}
			return resp.PolicyIssues, resp.NextPageToken, nil
		},
		printRow: func(w io.Writer, item *adsenseapi.PolicyIssue) {
			if item == nil {
				return
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
				sanitizeTab(adSenseResourceID(item.Name)),
				sanitizeTab(item.Site),
				sanitizeTab(item.EntityType),
				sanitizeTab(item.Action),
				item.AdRequestCount,
				sanitizeTab(adSensePolicyTopics(item.PolicyTopics)),
			)
		},
	})
}

type AdSensePolicyIssuesGetCmd struct {
	Name string `arg:"" name:"name" help:"Full policy issue resource name (e.g. accounts/pub-.../policyIssues/...)"`
}

func (c *AdSensePolicyIssuesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAdSenseGet(ctx, flags, c.Name, requireAdSenseResourceArg,
		func(svc *adsenseapi.Service, name string) (*adsenseapi.PolicyIssue, error) {
			return svc.Accounts.PolicyIssues.Get(name).Context(ctx).Do()
		},
		"policyIssue",
		func(issue *adsenseapi.PolicyIssue) []resultKV {
			return []resultKV{
				kv("name", issue.Name),
				kv("site", issue.Site),
				kv("entity_type", issue.EntityType),
				kv("action", issue.Action),
				kv("ad_request_count", issue.AdRequestCount),
				kv("topics", adSensePolicyTopics(issue.PolicyTopics)),
				kv("first_detected_date", adSenseDateString(issue.FirstDetectedDate)),
				kv("last_detected_date", adSenseDateString(issue.LastDetectedDate)),
			}
		},
	)
}

// Sites

type AdSenseSitesCmd struct {
	List AdSenseSitesListCmd `cmd:"" default:"withargs" aliases:"ls" help:"List AdSense sites for an account"`
	Get  AdSenseSitesGetCmd  `cmd:"" name:"get" aliases:"info,show" help:"Get a specific AdSense site"`
}

type AdSenseSitesListCmd struct {
	Account   string `arg:"" name:"account" help:"AdSense account (e.g. pub-... or accounts/pub-...)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max sites per page" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AdSenseSitesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	parent, err := normalizeAdSenseAccount(c.Account)
	if err != nil {
		return err
	}
	return runAdSenseList(ctx, adSenseListPage[*adsenseapi.Site]{
		flags: flags, max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty,
		jsonKey:  "sites",
		emptyMsg: "No AdSense sites",
		header:   "SITE\tDOMAIN\tSTATE\tAUTO_ADS",
		fetch: func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]*adsenseapi.Site, string, error) {
			resp, err := adSenseFetchPage[*adsenseapi.AccountsSitesListCall, adsenseapi.ListSitesResponse](
				svc.Accounts.Sites.List(parent).Context(ctx), pageSize, pageToken)
			if err != nil {
				return nil, "", err
			}
			return resp.Sites, resp.NextPageToken, nil
		},
		printRow: func(w io.Writer, item *adsenseapi.Site) {
			if item == nil {
				return
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%t\n",
				sanitizeTab(adSenseResourceID(item.Name)),
				sanitizeTab(item.Domain),
				sanitizeTab(item.State),
				item.AutoAdsEnabled,
			)
		},
	})
}

type AdSenseSitesGetCmd struct {
	Name string `arg:"" name:"name" help:"Full site resource name (e.g. accounts/pub-.../sites/...)"`
}

func (c *AdSenseSitesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAdSenseGet(ctx, flags, c.Name, requireAdSenseResourceArg,
		func(svc *adsenseapi.Service, name string) (*adsenseapi.Site, error) {
			return svc.Accounts.Sites.Get(name).Context(ctx).Do()
		},
		"site",
		func(site *adsenseapi.Site) []resultKV {
			return []resultKV{
				kv("name", site.Name),
				kv("domain", site.Domain),
				kv("state", site.State),
				kv("auto_ads_enabled", site.AutoAdsEnabled),
				kv("reporting_dimension_id", site.ReportingDimensionId),
			}
		},
	)
}

// Shared helpers

func normalizeAdSenseAccount(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", usage("empty account")
	}
	if strings.HasPrefix(value, "accounts/") {
		return value, nil
	}
	return "accounts/" + strings.TrimPrefix(value, "/"), nil
}

func requireAdSenseResource(raw, argName string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", usagef("empty %s", argName)
	}
	return value, nil
}

func adSenseResourceID(resource string) string {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return ""
	}
	if i := strings.LastIndex(resource, "/"); i >= 0 && i+1 < len(resource) {
		return resource[i+1:]
	}
	return resource
}

func adSenseTimeZoneID(tz *adsenseapi.TimeZone) string {
	if tz == nil {
		return ""
	}
	return tz.Id
}

func adSenseDateString(d *adsenseapi.Date) string {
	if d == nil || d.Year == 0 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

func adSensePolicyTopics(topics []*adsenseapi.PolicyTopic) string {
	if len(topics) == 0 {
		return ""
	}
	parts := make([]string, 0, len(topics))
	for _, topic := range topics {
		if topic == nil {
			continue
		}
		parts = append(parts, topic.Topic)
	}
	return strings.Join(parts, ",")
}
