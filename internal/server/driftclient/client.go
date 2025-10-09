package driftclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/volantvm/volant/internal/drift/routes"
)

// ErrDisabled indicates the Drift client is not configured.
var ErrDisabled = errors.New("drift client disabled")

// APIError captures error responses returned by the Drift daemon.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("driftd returned status %d", e.Status)
}

// Client interacts with the Drift control daemon.
type Client struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
}

// New creates a configured Drift client.
func New(endpoint, apiKey string, client *http.Client) (*Client, error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return nil, ErrDisabled
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse drift endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("drift endpoint must include scheme and host")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	base := *parsed
	base.Path = strings.TrimRight(parsed.Path, "/")
	return &Client{baseURL: &base, apiKey: strings.TrimSpace(apiKey), httpClient: client}, nil
}

// Enabled reports whether the client is usable.
func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != nil
}

// ListRoutes retrieves all configured routes from Drift.
func (c *Client) ListRoutes(ctx context.Context) ([]routes.Route, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/routes", nil)
	if err != nil {
		return nil, err
	}
	var result []routes.Route
	if err := c.do(req, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// UpsertRoute creates or updates a route via Drift.
func (c *Client) UpsertRoute(ctx context.Context, route routes.Route) (routes.Route, error) {
	if !c.Enabled() {
		return routes.Route{}, ErrDisabled
	}
	body, err := json.Marshal(route)
	if err != nil {
		return routes.Route{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/routes", bytes.NewReader(body))
	if err != nil {
		return routes.Route{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	var result routes.Route
	if err := c.do(req, &result); err != nil {
		return routes.Route{}, err
	}
	return result, nil
}

// DeleteRoute removes a route from Drift.
func (c *Client) DeleteRoute(ctx context.Context, protocol string, hostPort uint16) error {
	if !c.Enabled() {
		return ErrDisabled
	}
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	pathSuffix := fmt.Sprintf("/routes/%s/%d", protocol, hostPort)
	req, err := c.newRequest(ctx, http.MethodDelete, pathSuffix, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *Client) newRequest(ctx context.Context, method, suffix string, body io.Reader) (*http.Request, error) {
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	full := *c.baseURL
	full.Path = path.Clean(c.baseURL.Path + suffix)
	req, err := http.NewRequestWithContext(ctx, method, full.String(), body)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return req, nil
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || resp.StatusCode == http.StatusNoContent {
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}

	apiErr := &APIError{Status: resp.StatusCode}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil && payload.Error != "" {
		apiErr.Message = payload.Error
	}
	return apiErr
}
