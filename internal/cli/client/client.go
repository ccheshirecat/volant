package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	orchestratorevents "github.com/viperhq/viper/internal/server/orchestrator/events"
)

// Client wraps REST access to the viper-server API.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

// New creates a client with the provided base URL (e.g. http://127.0.0.1:7777).
func New(rawURL string) (*Client, error) {
	if rawURL == "" {
		rawURL = "http://127.0.0.1:7777"
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("client: parse url: %w", err)
	}
	return &Client{
		baseURL: parsed,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// VM represents the API response for a microVM.
type VM struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	PID           *int64 `json:"pid,omitempty"`
	IPAddress     string `json:"ip_address"`
	MACAddress    string `json:"mac_address"`
	CPUCores      int    `json:"cpu_cores"`
	MemoryMB      int    `json:"memory_mb"`
	KernelCmdline string `json:"kernel_cmdline,omitempty"`
}

// CreateVMRequest contains creation parameters.
type CreateVMRequest struct {
	Name          string `json:"name"`
	CPUCores      int    `json:"cpu_cores"`
	MemoryMB      int    `json:"memory_mb"`
	KernelCmdline string `json:"kernel_cmdline,omitempty"`
}

// VMEvent represents a lifecycle event streamed from the server.
type VMEvent = orchestratorevents.VMEvent

func (c *Client) ListVMs(ctx context.Context) ([]VM, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/vms", nil)
	if err != nil {
		return nil, err
	}
	var vms []VM
	if err := c.do(req, &vms); err != nil {
		return nil, err
	}
	return vms, nil
}

func (c *Client) GetVM(ctx context.Context, name string) (*VM, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/vms/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, err
	}
	var vm VM
	if err := c.do(req, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

func (c *Client) CreateVM(ctx context.Context, payload CreateVMRequest) (*VM, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/vms", payload)
	if err != nil {
		return nil, err
	}
	var vm VM
	if err := c.do(req, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

func (c *Client) DeleteVM(ctx context.Context, name string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/vms/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// WatchVMEvents streams VM lifecycle events and invokes handler for each payload until
// the context is cancelled or the server closes the connection.
func (c *Client) WatchVMEvents(ctx context.Context, handler func(VMEvent)) error {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/events/vms", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("client: watch events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("client: watch events http %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}

		var event VMEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return fmt.Errorf("client: decode event: %w", err)
		}
		if handler != nil {
			handler(event)
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return fmt.Errorf("client: event stream error: %w", err)
		}
	}

	return nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	resolved := c.baseURL.ResolveReference(&url.URL{Path: path})
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, fmt.Errorf("client: encode body: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, resolved.String(), &buf)
	if err != nil {
		return nil, fmt.Errorf("client: new request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("client: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var apiErr map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return fmt.Errorf("client: http %d", resp.StatusCode)
		}
		if msg, ok := apiErr["error"].(string); ok {
			return fmt.Errorf("client: http %d: %s", resp.StatusCode, msg)
		}
		return fmt.Errorf("client: http %d", resp.StatusCode)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("client: decode response: %w", err)
	}
	return nil
}

// SystemStatus represents the system metrics.
type SystemStatus struct {
	VMCount int `json:"vm_count"`
	CPU     float64 `json:"cpu_percent"`
	MEM     float64 `json:"mem_percent"`
}

// GetSystemStatus fetches system metrics.
func (c *Client) GetSystemStatus(ctx context.Context) (*SystemStatus, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/system/status", nil)
	if err != nil {
		return nil, err
	}
	var status SystemStatus
	if err := c.do(req, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
