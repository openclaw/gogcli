package discoveryapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	discovery "google.golang.org/api/discovery/v1"
)

const DefaultBaseURL = "https://www.googleapis.com/discovery/v1"

var (
	ErrInvalidDescription = errors.New("invalid Discovery description")
	ErrDiscoveryRequest   = errors.New("discovery request failed")
	ErrMethodNotFound     = errors.New("discovery method not found")
	ErrMissingParameter   = errors.New("missing required parameter")
	ErrUnknownParameter   = errors.New("unknown parameter")
	ErrUntrustedAPIURL    = errors.New("untrusted Discovery API URL")
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type Method struct {
	ID       string               `json:"id"`
	Resource string               `json:"resource,omitempty"`
	Name     string               `json:"name"`
	Spec     discovery.RestMethod `json:"spec"`
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}

	return &http.Client{Timeout: 30 * time.Second}
}

func (c Client) baseURL() string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return DefaultBaseURL
	}

	return base
}

func (c Client) List(ctx context.Context, preferred bool) (json.RawMessage, error) {
	u := c.baseURL() + "/apis"
	if preferred {
		u += "?preferred=true"
	}

	return c.get(ctx, u)
}

func (c Client) Description(ctx context.Context, api, version string) (*discovery.RestDescription, error) {
	if strings.TrimSpace(api) == "" || strings.TrimSpace(version) == "" {
		return nil, ErrInvalidDescription
	}

	u := c.baseURL() + "/apis/" + url.PathEscape(api) + "/" + url.PathEscape(version) + "/rest"

	raw, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}

	var description discovery.RestDescription
	if err := json.Unmarshal(raw, &description); err != nil {
		return nil, fmt.Errorf("decode Discovery document: %w", err)
	}

	return &description, nil
}

func (c Client) get(ctx context.Context, requestURL string) (json.RawMessage, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build Discovery request: %w", err)
	}

	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch Discovery document: %w", err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("read Discovery response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrDiscoveryRequest, response.StatusCode, strings.TrimSpace(string(raw)))
	}

	return raw, nil
}

func Methods(description *discovery.RestDescription) []Method {
	if description == nil {
		return nil
	}

	var methods []Method
	appendMethods(&methods, "", description.Methods, description.Resources)
	sort.Slice(methods, func(i, j int) bool { return methods[i].ID < methods[j].ID })

	return methods
}

func appendMethods(out *[]Method, resource string, methods map[string]discovery.RestMethod, resources map[string]discovery.RestResource) {
	for name, spec := range methods {
		id := spec.Id
		if id == "" {
			id = strings.Trim(strings.Join([]string{resource, name}, "."), ".")
		}
		*out = append(*out, Method{ID: id, Resource: resource, Name: name, Spec: spec})
	}

	for name, nested := range resources {
		nestedResource := strings.Trim(strings.Join([]string{resource, name}, "."), ".")
		appendMethods(out, nestedResource, nested.Methods, nested.Resources)
	}
}

func FindMethod(description *discovery.RestDescription, id string) (Method, error) {
	wanted := strings.TrimSpace(id)
	for _, method := range Methods(description) {
		if method.ID == wanted || strings.Trim(strings.Join([]string{method.Resource, method.Name}, "."), ".") == wanted {
			return method, nil
		}
	}

	return Method{}, fmt.Errorf("%w: %q", ErrMethodNotFound, id)
}

func BuildURL(description *discovery.RestDescription, method Method, params map[string]any) (string, error) {
	if description == nil {
		return "", ErrInvalidDescription
	}

	path := method.Spec.Path
	query := url.Values{}

	parameterSchemas := make(map[string]discovery.JsonSchema, len(description.Parameters)+len(method.Spec.Parameters))
	for name, schema := range description.Parameters {
		parameterSchemas[name] = schema
	}

	for name, schema := range method.Spec.Parameters {
		parameterSchemas[name] = schema
	}

	for name := range params {
		if _, ok := parameterSchemas[name]; !ok {
			return "", fmt.Errorf("%w %q", ErrUnknownParameter, name)
		}
	}

	for name, schema := range parameterSchemas {
		value, ok := params[name]
		if !ok {
			if schema.Required {
				return "", fmt.Errorf("%w %q", ErrMissingParameter, name)
			}

			continue
		}

		text := fmt.Sprint(value)
		if schema.Location == "path" {
			path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(text))
			path = strings.ReplaceAll(path, "{+"+name+"}", escapeReservedPath(text))

			continue
		}

		if values, ok := value.([]any); ok {
			for _, item := range values {
				query.Add(name, fmt.Sprint(item))
			}

			continue
		}

		query.Set(name, text)
	}

	base := description.RootUrl
	if base == "" {
		base = description.BaseUrl
	} else {
		base = strings.TrimRight(base, "/") + "/" + strings.Trim(description.ServicePath, "/")
	}

	u := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
	if encoded := query.Encode(); encoded != "" {
		u += "?" + encoded
	}

	return u, nil
}

// ValidateGoogleAPIURL ensures OAuth credentials are only attached to Google API hosts.
func ValidateGoogleAPIURL(requestURL string) error {
	u, err := url.Parse(requestURL)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUntrustedAPIURL, err)
	}

	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if u.Scheme != "https" || u.User != nil || host == "" || (host != "googleapis.com" && !strings.HasSuffix(host, ".googleapis.com")) {
		return fmt.Errorf("%w: %q", ErrUntrustedAPIURL, requestURL)
	}

	return nil
}

func escapeReservedPath(value string) string {
	parts := strings.Split(value, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}

	return strings.Join(parts, "/")
}

func NewRequest(ctx context.Context, method, requestURL string, body json.RawMessage) (*http.Request, error) {
	var reader io.Reader
	if len(bytes.TrimSpace(body)) > 0 {
		reader = bytes.NewReader(body)
	}

	request, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	if reader != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	return request, nil
}
