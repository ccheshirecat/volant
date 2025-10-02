package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ccheshirecat/volant/internal/pluginspec"
	orchestratorevents "github.com/ccheshirecat/volant/internal/server/orchestrator/events"
	"github.com/ccheshirecat/volant/internal/server/orchestrator/vmconfig"
)

// Client wraps REST access to the volantd API.
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
	Runtime       string `json:"runtime"`
	PID           *int64 `json:"pid,omitempty"`
	IPAddress     string `json:"ip_address"`
	MACAddress    string `json:"mac_address"`
	CPUCores      int    `json:"cpu_cores"`
	MemoryMB      int    `json:"memory_mb"`
	KernelCmdline string `json:"kernel_cmdline,omitempty"`
	SerialSocket  string `json:"serial_socket,omitempty"`
	ConsoleSocket string `json:"console_socket,omitempty"`
}

// CreateVMRequest contains creation parameters.
type CreateVMRequest struct {
	Name          string           `json:"name"`
	Plugin        string           `json:"plugin"`
	Runtime       string           `json:"runtime,omitempty"`
	CPUCores      int              `json:"cpu_cores"`
	MemoryMB      int              `json:"memory_mb"`
	KernelCmdline string           `json:"kernel_cmdline,omitempty"`
	APIHost       string           `json:"api_host,omitempty"`
	APIPort       string           `json:"api_port,omitempty"`
	Config        *vmconfig.Config `json:"config,omitempty"`
}

// Deployment represents a VM deployment group.
type Deployment struct {
	Name            string          `json:"name"`
	DesiredReplicas int             `json:"desired_replicas"`
	ReadyReplicas   int             `json:"ready_replicas"`
	Config          vmconfig.Config `json:"config"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// CreateDeploymentRequest captures deployment creation inputs.
type CreateDeploymentRequest struct {
	Name     string          `json:"name"`
	Replicas int             `json:"replicas"`
	Config   vmconfig.Config `json:"config"`
}

const (
	VMEventTypeCreated = orchestratorevents.TypeVMCreated
	VMEventTypeRunning = orchestratorevents.TypeVMRunning
	VMEventTypeStopped = orchestratorevents.TypeVMStopped
	VMEventTypeCrashed = orchestratorevents.TypeVMCrashed
	VMEventTypeDeleted = orchestratorevents.TypeVMDeleted
	VMEventTypeLog     = orchestratorevents.TypeVMLog
)

const (
	VMLogStreamStdout = orchestratorevents.LogStreamStdout
	VMLogStreamStderr = orchestratorevents.LogStreamStderr
)

// VMEvent represents a lifecycle event streamed from the server.
type VMEvent = orchestratorevents.VMEvent

// VMLogEvent represents a single log line emitted by a VM or agent process.
type VMLogEvent struct {
	Name      string    `json:"name"`
	Stream    string    `json:"stream"`
	Line      string    `json:"line"`
	Timestamp time.Time `json:"timestamp"`
}

type DevToolsInfo struct {
	WebSocketURL   string `json:"websocket_url"`
	WebSocketPath  string `json:"websocket_path"`
	BrowserVersion string `json:"browser_version"`
	UserAgent      string `json:"user_agent"`
	Address        string `json:"address"`
	Port           int    `json:"port"`
}

type MCPRequest struct {
	Command string                 `json:"command"`
	Params  map[string]interface{} `json:"params"`
}

type MCPResponse struct {
	Result interface{} `json:"result"`
	Error  string      `json:"error"`
}

type Plugin = pluginspec.Manifest

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

func (c *Client) GetVMConfig(ctx context.Context, name string) (*vmconfig.Versioned, error) {
	path := "/api/v1/vms/" + url.PathEscape(name) + "/config"
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var config vmconfig.Versioned
	if err := c.do(req, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (c *Client) UpdateVMConfig(ctx context.Context, name string, patch vmconfig.Patch) (*vmconfig.Versioned, error) {
	path := "/api/v1/vms/" + url.PathEscape(name) + "/config"
	req, err := c.newRequest(ctx, http.MethodPatch, path, patch)
	if err != nil {
		return nil, err
	}
	var config vmconfig.Versioned
	if err := c.do(req, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (c *Client) UpdateVMConfigRaw(ctx context.Context, name string, raw []byte) (*vmconfig.Versioned, error) {
	path := "/api/v1/vms/" + url.PathEscape(name) + "/config"
	var payload any
	if len(raw) > 0 {
		payload = json.RawMessage(raw)
	}
	req, err := c.newRequest(ctx, http.MethodPatch, path, payload)
	if err != nil {
		return nil, err
	}
	var config vmconfig.Versioned
	if err := c.do(req, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (c *Client) GetVMConfigHistory(ctx context.Context, name string, limit int) ([]vmconfig.HistoryEntry, error) {
	path := "/api/v1/vms/" + url.PathEscape(name) + "/config/history"
	if limit > 0 {
		path = path + "?limit=" + strconv.Itoa(limit)
	}
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var entries []vmconfig.HistoryEntry
	if err := c.do(req, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) StartVM(ctx context.Context, name string) (*VM, error) {
	path := "/api/v1/vms/" + url.PathEscape(name) + "/start"
	req, err := c.newRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return nil, err
	}
	var vm VM
	if err := c.do(req, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

func (c *Client) StopVM(ctx context.Context, name string) (*VM, error) {
	path := "/api/v1/vms/" + url.PathEscape(name) + "/stop"
	req, err := c.newRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return nil, err
	}
	var vm VM
	if err := c.do(req, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

func (c *Client) RestartVM(ctx context.Context, name string) (*VM, error) {
	path := "/api/v1/vms/" + url.PathEscape(name) + "/restart"
	req, err := c.newRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return nil, err
	}
	var vm VM
	if err := c.do(req, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

func (c *Client) CreateDeployment(ctx context.Context, payload CreateDeploymentRequest) (*Deployment, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/deployments", payload)
	if err != nil {
		return nil, err
	}
	var deployment Deployment
	if err := c.do(req, &deployment); err != nil {
		return nil, err
	}
	return &deployment, nil
}

func (c *Client) ListDeployments(ctx context.Context) ([]Deployment, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/deployments", nil)
	if err != nil {
		return nil, err
	}
	var deployments []Deployment
	if err := c.do(req, &deployments); err != nil {
		return nil, err
	}
	return deployments, nil
}

func (c *Client) GetDeployment(ctx context.Context, name string) (*Deployment, error) {
	path := "/api/v1/deployments/" + url.PathEscape(name)
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var deployment Deployment
	if err := c.do(req, &deployment); err != nil {
		return nil, err
	}
	return &deployment, nil
}

func (c *Client) DeleteDeployment(ctx context.Context, name string) error {
	path := "/api/v1/deployments/" + url.PathEscape(name)
	req, err := c.newRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *Client) ScaleDeployment(ctx context.Context, name string, replicas int) (*Deployment, error) {
	path := "/api/v1/deployments/" + url.PathEscape(name)
	payload := map[string]int{"replicas": replicas}
	req, err := c.newRequest(ctx, http.MethodPatch, path, payload)
	if err != nil {
		return nil, err
	}
	var deployment Deployment
	if err := c.do(req, &deployment); err != nil {
		return nil, err
	}
	return &deployment, nil
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

func (c *Client) WatchVMLogs(ctx context.Context, name string, handler func(VMLogEvent)) error {
	if name == "" {
		return fmt.Errorf("client: vm name required")
	}
	if handler == nil {
		return fmt.Errorf("client: handler required")
	}

	path := fmt.Sprintf("/ws/v1/vms/%s/logs", url.PathEscape(name))
	wsURL := c.baseURL.ResolveReference(&url.URL{Path: path})
	switch wsURL.Scheme {
	case "http":
		wsURL.Scheme = "ws"
	case "https":
		wsURL.Scheme = "wss"
	case "ws", "wss":
	default:
		return fmt.Errorf("client: unsupported scheme %q", wsURL.Scheme)
	}

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 30 * time.Second,
	}

	conn, resp, err := dialer.DialContext(ctx, wsURL.String(), nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("client: watch vm logs dial: %w", err)
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			_ = conn.Close()
		case <-done:
		}
	}()

	for {
		var event VMLogEvent
		if err := conn.ReadJSON(&event); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || errors.Is(err, context.Canceled) {
				close(done)
				return nil
			}
			close(done)
			return fmt.Errorf("client: read vm log: %w", err)
		}
		handler(event)
	}
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			_ = conn.Close()
		case <-done:
		}
	}()

	for {
		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			close(done)
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("client: read agui event: %w", err)
		}
		if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
			handler(string(payload))
		}
	}
}

func (c *Client) AgentRequest(ctx context.Context, vmName, method, path string, body any, out any) error {
	if strings.TrimSpace(vmName) == "" {
		return fmt.Errorf("client: vm name required")
	}
	if method == "" {
		method = http.MethodGet
	}
	if path == "" {
		path = "/"
	}
	var rawQuery string
	if idx := strings.Index(path, "?"); idx >= 0 {
		rawQuery = path[idx+1:]
		path = path[:idx]
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	basePath := fmt.Sprintf("/api/v1/vms/%s/agent%s", url.PathEscape(vmName), path)

	resolved := c.baseURL.ResolveReference(&url.URL{
		Path:     basePath,
		RawQuery: rawQuery,
	})

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("client: encode body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, resolved.String(), &buf)
	if err != nil {
		return fmt.Errorf("client: agent request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.do(req, out)
}

func (c *Client) GetAgentDevTools(ctx context.Context, vmName string) (*DevToolsInfo, error) {
	var info DevToolsInfo
	if err := c.AgentRequest(ctx, vmName, http.MethodGet, "/v1/devtools", nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) BaseURL() *url.URL {
	if c.baseURL == nil {
		return nil
	}
	clone := *c.baseURL
	return &clone
}

func (c *Client) NavigateVM(ctx context.Context, name string, payload NavigateActionRequest) error {
	if payload.URL == "" {
		return fmt.Errorf("client: navigate url required")
	}
	return c.PluginActionVM(ctx, name, "browser", "navigate", payload)
}

func (c *Client) ScreenshotVM(ctx context.Context, name string, payload ScreenshotActionRequest) (*ScreenshotActionResponse, error) {
	var response ScreenshotActionResponse
	if err := c.PluginActionVM(ctx, name, "browser", "screenshot", payload, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) ScrapeVM(ctx context.Context, name string, payload ScrapeActionRequest) (*ScrapeActionResponse, error) {
	var response ScrapeActionResponse
	if err := c.PluginActionVM(ctx, name, "browser", "scrape", payload, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) EvaluateVM(ctx context.Context, name string, payload EvaluateActionRequest) (*EvaluateActionResponse, error) {
	var response EvaluateActionResponse
	if err := c.PluginActionVM(ctx, name, "browser", "evaluate", payload, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GraphQLVM(ctx context.Context, name string, payload GraphQLActionRequest) (GraphQLActionResponse, error) {
	var response GraphQLActionResponse
	if err := c.PluginActionVM(ctx, name, "browser", "graphql", payload, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) MCP(ctx context.Context, request MCPRequest) (*MCPResponse, error) {
	var response MCPResponse
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/mcp", request)
	if err != nil {
		return nil, err
	}
	if err := c.do(req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetVMOpenAPISpec(ctx context.Context, name string) ([]byte, string, error) {
	path := fmt.Sprintf("/api/v1/vms/%s/openapi", url.PathEscape(name))
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("client: do request: %w", err)
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, "", fmt.Errorf("client: read response: %w", readErr)
	}

	if resp.StatusCode >= 300 {
		var apiErr map[string]any
		if len(data) > 0 && json.Unmarshal(data, &apiErr) == nil {
			if msg, ok := apiErr["error"].(string); ok {
				return nil, "", fmt.Errorf("client: http %d: %s", resp.StatusCode, msg)
			}
		}
		return nil, "", fmt.Errorf("client: http %d", resp.StatusCode)
	}

	return data, resp.Header.Get("Content-Type"), nil
}

func (c *Client) ProxyVM(ctx context.Context, vmName, method, path string, query url.Values, body []byte, headers http.Header) (*http.Response, error) {
	if method == "" {
		method = http.MethodGet
	}
	targetPath := fmt.Sprintf("/api/v1/vms/%s/agent%s", url.PathEscape(vmName), ensureLeadingSlash(path))
	ref := &url.URL{Path: targetPath}
	if query != nil {
		ref.RawQuery = query.Encode()
	}
	resolved := c.baseURL.ResolveReference(ref)

	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	} else {
		reader = http.NoBody
	}

	req, err := http.NewRequestWithContext(ctx, method, resolved.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("client: new request: %w", err)
	}

	if headers != nil {
		for key, values := range headers {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}
	if len(body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client: do request: %w", err)
	}
	return resp, nil
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
	VMCount int     `json:"vm_count"`
	CPU     float64 `json:"cpu_percent"`
	MEM     float64 `json:"mem_percent"`
}

func ensureLeadingSlash(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
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

func (c *Client) ListPlugins(ctx context.Context) ([]pluginspec.Manifest, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/plugins", nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		Plugins []string `json:"plugins"`
	}
	if err := c.do(req, &response); err != nil {
		return nil, err
	}
	result := make([]pluginspec.Manifest, 0, len(response.Plugins))
	for _, name := range response.Plugins {
		plugin, err := c.GetPlugin(ctx, name)
		if err != nil {
			return nil, err
		}
		if plugin != nil {
			result = append(result, *plugin)
		}
	}
	return result, nil
}

func (c *Client) GetPlugin(ctx context.Context, name string) (*pluginspec.Manifest, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/plugins/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, err
	}
	var manifest pluginspec.Manifest
	if err := c.do(req, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (c *Client) DescribePlugin(ctx context.Context, name string) (*pluginspec.Manifest, error) {
	return c.GetPlugin(ctx, name)
}

func (c *Client) InstallPlugin(ctx context.Context, manifest pluginspec.Manifest) error {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/plugins", manifest)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *Client) RemovePlugin(ctx context.Context, name string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/plugins/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *Client) SetPluginEnabled(ctx context.Context, name string, enabled bool) error {
	payload := map[string]any{"enabled": enabled}
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/plugins/"+url.PathEscape(name)+"/enabled", payload)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}
