package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	searchconsoleapi "google.golang.org/api/searchconsole/v1"

	"github.com/openclaw/gogcli/internal/config"
)

const (
	defaultSearchConsoleRowLimit = int64(1000)
	maxSearchConsoleRowLimit     = int64(25000)

	searchConsoleGroupAnd          = "AND"
	searchConsoleTypeWeb           = "WEB"
	searchConsoleAggregationByPage = "BY_PAGE"
	searchConsoleDimensionQuery    = "QUERY"
	searchConsoleDimensionPage     = "PAGE"
)

type searchConsoleQueryPlan struct {
	SiteURL string
	Request *searchconsoleapi.SearchAnalyticsQueryRequest
}

func (c *SearchConsoleQueryCmd) plan(input io.Reader) (searchConsoleQueryPlan, error) {
	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return searchConsoleQueryPlan{}, usage("empty siteUrl")
	}
	request, err := c.buildRequest(input)
	if err != nil {
		return searchConsoleQueryPlan{}, err
	}
	return searchConsoleQueryPlan{
		SiteURL: siteURL,
		Request: request,
	}, nil
}

func (p searchConsoleQueryPlan) queryType() string {
	if p.Request == nil {
		return ""
	}
	if p.Request.Type != "" {
		return p.Request.Type
	}
	return p.Request.SearchType
}

func (c *SearchConsoleQueryCmd) buildRequest(input io.Reader) (*searchconsoleapi.SearchAnalyticsQueryRequest, error) {
	requestSpec := strings.TrimSpace(c.Request)
	if requestSpec != "" {
		return buildSearchConsoleRequestFromSpec(requestSpec, input)
	}

	from, err := parseSearchConsoleDate(c.From, "--from")
	if err != nil {
		return nil, err
	}
	to, err := parseSearchConsoleDate(c.To, "--to")
	if err != nil {
		return nil, err
	}
	if rangeErr := validateSearchConsoleDateRange(from, to); rangeErr != nil {
		return nil, rangeErr
	}

	if c.Max <= 0 || c.Max > maxSearchConsoleRowLimit {
		return nil, usagef("--max must be between 1 and %d", maxSearchConsoleRowLimit)
	}
	if c.Offset < 0 {
		return nil, usage("--offset must be >= 0")
	}

	dimensions, err := normalizeSearchConsoleDimensions(c.Dimensions)
	if err != nil {
		return nil, err
	}
	searchType, err := normalizeSearchConsoleType(c.Type)
	if err != nil {
		return nil, err
	}

	req := &searchconsoleapi.SearchAnalyticsQueryRequest{
		StartDate:  from,
		EndDate:    to,
		Dimensions: dimensions,
		Type:       searchType,
		RowLimit:   c.Max,
		StartRow:   c.Offset,
	}

	if value := strings.TrimSpace(c.Aggregation); value != "" {
		aggregation, err := normalizeSearchConsoleAggregation(value)
		if err != nil {
			return nil, err
		}
		req.AggregationType = aggregation
	}
	if value := strings.TrimSpace(c.DataState); value != "" {
		dataState, err := normalizeSearchConsoleDataState(value)
		if err != nil {
			return nil, err
		}
		req.DataState = dataState
	}

	if len(c.Filter) > 0 {
		filters := make([]*searchconsoleapi.ApiDimensionFilter, 0, len(c.Filter))
		for _, raw := range c.Filter {
			filter, err := parseSearchConsoleFilter(raw)
			if err != nil {
				return nil, err
			}
			filters = append(filters, filter)
		}
		req.DimensionFilterGroups = []*searchconsoleapi.ApiDimensionFilterGroup{
			{
				GroupType: searchConsoleGroupAnd,
				Filters:   filters,
			},
		}
	}

	return req, nil
}

func buildSearchConsoleRequestFromSpec(spec string, input io.Reader) (*searchconsoleapi.SearchAnalyticsQueryRequest, error) {
	data, err := readSearchConsoleRequestBytes(spec, input)
	if err != nil {
		return nil, err
	}

	var req searchconsoleapi.SearchAnalyticsQueryRequest
	if unmarshalErr := json.Unmarshal(data, &req); unmarshalErr != nil {
		return nil, fmt.Errorf("decode search console request: %w", unmarshalErr)
	}

	if req.RowLimit == 0 {
		req.RowLimit = defaultSearchConsoleRowLimit
	}
	if req.RowLimit < 1 || req.RowLimit > maxSearchConsoleRowLimit {
		return nil, usagef("request.rowLimit must be between 1 and %d", maxSearchConsoleRowLimit)
	}
	if req.StartRow < 0 {
		return nil, usage("request.startRow must be >= 0")
	}
	if rangeErr := validateSearchConsoleDateRange(req.StartDate, req.EndDate); rangeErr != nil {
		return nil, rangeErr
	}

	if len(req.Dimensions) > 0 {
		dimensions, dimensionErr := normalizeSearchConsoleDimensionList(req.Dimensions)
		if dimensionErr != nil {
			return nil, dimensionErr
		}
		req.Dimensions = dimensions
	}

	if req.Type == "" && req.SearchType != "" {
		req.Type = req.SearchType
	}
	if req.Type == "" {
		req.Type = searchConsoleTypeWeb
	}
	searchType, err := normalizeSearchConsoleType(req.Type)
	if err != nil {
		return nil, err
	}
	req.Type = searchType
	req.SearchType = searchType

	if value := strings.TrimSpace(req.AggregationType); value != "" {
		aggregation, err := normalizeSearchConsoleAggregation(value)
		if err != nil {
			return nil, err
		}
		req.AggregationType = aggregation
	}
	if value := strings.TrimSpace(req.DataState); value != "" {
		dataState, err := normalizeSearchConsoleDataState(value)
		if err != nil {
			return nil, err
		}
		req.DataState = dataState
	}

	for _, group := range req.DimensionFilterGroups {
		if group == nil {
			continue
		}
		if strings.TrimSpace(group.GroupType) == "" {
			group.GroupType = searchConsoleGroupAnd
		}
		if !strings.EqualFold(strings.TrimSpace(group.GroupType), "and") {
			return nil, usagef("invalid request.groupType %q (expected and)", group.GroupType)
		}
		group.GroupType = searchConsoleGroupAnd
		for _, filter := range group.Filters {
			if filter == nil {
				continue
			}
			dimension, err := normalizeSearchConsoleDimension(filter.Dimension)
			if err != nil {
				return nil, err
			}
			operator, err := normalizeSearchConsoleFilterOperator(filter.Operator)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(filter.Expression) == "" {
				return nil, usage("request filter expression cannot be empty")
			}
			filter.Dimension = dimension
			filter.Operator = operator
		}
	}

	return &req, nil
}

func readSearchConsoleRequestBytes(spec string, input io.Reader) ([]byte, error) {
	spec = strings.TrimSpace(spec)
	switch {
	case spec == "", spec == "-", strings.HasPrefix(spec, "@"), strings.HasPrefix(spec, "{"), strings.HasPrefix(spec, "["):
		return resolveInlineOrFileBytes(spec, input)
	default:
		path, err := config.ExpandPath(spec)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(path) //nolint:gosec // user-provided path
	}
}

func parseSearchConsoleDate(value string, flagName string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", usagef("empty %s", flagName)
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return "", usagef("invalid %s (expected YYYY-MM-DD)", flagName)
	}
	return value, nil
}

func validateSearchConsoleDateRange(from string, to string) error {
	start, err := time.Parse("2006-01-02", strings.TrimSpace(from))
	if err != nil {
		return usage("invalid start date (expected YYYY-MM-DD)")
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(to))
	if err != nil {
		return usage("invalid end date (expected YYYY-MM-DD)")
	}
	if end.Before(start) {
		return usage("--to must be on or after --from")
	}
	return nil
}

func normalizeSearchConsoleType(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch {
	case strings.EqualFold(value, "web"):
		return searchConsoleTypeWeb, nil
	case strings.EqualFold(value, "image"):
		return "IMAGE", nil
	case strings.EqualFold(value, "video"):
		return "VIDEO", nil
	case strings.EqualFold(value, "news"):
		return "NEWS", nil
	case strings.EqualFold(value, "discover"):
		return "DISCOVER", nil
	case strings.EqualFold(strings.ReplaceAll(value, "_", ""), "googleNews"),
		strings.EqualFold(strings.ReplaceAll(value, "-", ""), "googleNews"):
		return "GOOGLE_NEWS", nil
	case value == "":
		return "", usage("empty --type")
	default:
		return "", usagef("invalid --type %q (expected WEB|IMAGE|VIDEO|NEWS|DISCOVER|GOOGLE_NEWS)", raw)
	}
}

func normalizeSearchConsoleAggregation(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch {
	case value == "":
		return "", nil
	case strings.EqualFold(value, "auto"):
		return "AUTO", nil
	case strings.EqualFold(strings.ReplaceAll(strings.ReplaceAll(value, "_", ""), "-", ""), "byProperty"):
		return "BY_PROPERTY", nil
	case strings.EqualFold(strings.ReplaceAll(strings.ReplaceAll(value, "_", ""), "-", ""), "byPage"):
		return searchConsoleAggregationByPage, nil
	case strings.EqualFold(strings.ReplaceAll(strings.ReplaceAll(value, "_", ""), "-", ""), "byNewsShowcasePanel"):
		return "BY_NEWS_SHOWCASE_PANEL", nil
	default:
		return "", usagef("invalid --aggregation %q (expected AUTO|BY_PROPERTY|BY_PAGE|BY_NEWS_SHOWCASE_PANEL)", raw)
	}
}

func normalizeSearchConsoleDataState(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch {
	case value == "":
		return "", nil
	case strings.EqualFold(value, "final"):
		return "FINAL", nil
	case strings.EqualFold(value, "all"):
		return "ALL", nil
	case strings.EqualFold(strings.ReplaceAll(value, "-", "_"), "hourly_all"):
		return "HOURLY_ALL", nil
	default:
		return "", usagef("invalid --data-state %q (expected FINAL|ALL|HOURLY_ALL)", raw)
	}
}

func normalizeSearchConsoleDimensions(raw string) ([]string, error) {
	parts := splitCommaList(raw)
	if len(parts) == 0 {
		return nil, nil
	}
	return normalizeSearchConsoleDimensionList(parts)
}

func normalizeSearchConsoleDimensionList(parts []string) ([]string, error) {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value, err := normalizeSearchConsoleDimension(part)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func normalizeSearchConsoleDimension(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch {
	case strings.EqualFold(value, "date"):
		return "DATE", nil
	case strings.EqualFold(value, "query"):
		return searchConsoleDimensionQuery, nil
	case strings.EqualFold(value, "page"):
		return searchConsoleDimensionPage, nil
	case strings.EqualFold(value, "country"):
		return "COUNTRY", nil
	case strings.EqualFold(value, "device"):
		return "DEVICE", nil
	case strings.EqualFold(strings.ReplaceAll(strings.ReplaceAll(value, "_", ""), "-", ""), "searchAppearance"):
		return "SEARCH_APPEARANCE", nil
	case strings.EqualFold(value, "hour"):
		return "HOUR", nil
	case value == "":
		return "", usage("empty dimension")
	default:
		return "", usagef("invalid dimension %q (expected DATE|QUERY|PAGE|COUNTRY|DEVICE|SEARCH_APPEARANCE|HOUR)", raw)
	}
}

func parseSearchConsoleFilter(raw string) (*searchconsoleapi.ApiDimensionFilter, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, usage("empty --filter")
	}

	first := strings.Index(raw, ":")
	if first <= 0 {
		return nil, usagef("invalid --filter %q (expected dimension:operator:expression)", raw)
	}
	rest := raw[first+1:]
	second := strings.Index(rest, ":")
	if second < 0 {
		return nil, usagef("invalid --filter %q (expected dimension:operator:expression)", raw)
	}

	dimension, err := normalizeSearchConsoleDimension(raw[:first])
	if err != nil {
		return nil, err
	}
	operator, err := normalizeSearchConsoleFilterOperator(rest[:second])
	if err != nil {
		return nil, err
	}
	expression := strings.TrimSpace(rest[second+1:])
	if expression == "" {
		return nil, usagef("invalid --filter %q (expected dimension:operator:expression)", raw)
	}

	return &searchconsoleapi.ApiDimensionFilter{
		Dimension:  dimension,
		Operator:   operator,
		Expression: expression,
	}, nil
}

func normalizeSearchConsoleFilterOperator(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch {
	case value == "":
		return "", usage("empty filter operator")
	case strings.EqualFold(value, "equals"):
		return "EQUALS", nil
	case strings.EqualFold(strings.ReplaceAll(strings.ReplaceAll(value, "_", ""), "-", ""), "notEquals"):
		return "NOT_EQUALS", nil
	case strings.EqualFold(value, "contains"):
		return "CONTAINS", nil
	case strings.EqualFold(strings.ReplaceAll(strings.ReplaceAll(value, "_", ""), "-", ""), "notContains"):
		return "NOT_CONTAINS", nil
	case strings.EqualFold(strings.ReplaceAll(strings.ReplaceAll(value, "_", ""), "-", ""), "includingRegex"):
		return "INCLUDING_REGEX", nil
	case strings.EqualFold(strings.ReplaceAll(strings.ReplaceAll(value, "_", ""), "-", ""), "excludingRegex"):
		return "EXCLUDING_REGEX", nil
	default:
		return "", usagef("invalid filter operator %q (expected EQUALS|NOT_EQUALS|CONTAINS|NOT_CONTAINS|INCLUDING_REGEX|EXCLUDING_REGEX)", raw)
	}
}
