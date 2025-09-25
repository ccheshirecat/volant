package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/ccheshirecat/volant/internal/protocol/agui"
	"github.com/ccheshirecat/volant/internal/server/db"
	"github.com/ccheshirecat/volant/internal/server/eventbus"
	"github.com/ccheshirecat/volant/internal/server/orchestrator"
	orchestratorevents "github.com/ccheshirecat/volant/internal/server/orchestrator/events"
	"github.com/ccheshirecat/volant/internal/server/plugins"
)

const (
	agentDefaultPort         = 8080
	agentDevToolsDefaultPort = 9222
)

var hopHeaders = map[string]struct{}{
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailer":             {},
	"trailers":            {},
	"transfer-encoding":   {},
	"upgrade":             {},
}

func New(logger *slog.Logger, engine orchestrator.Engine, bus eventbus.Bus, plugins *plugins.Registry) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger(logger))

	if cidr := os.Getenv("VOLANT_API_ALLOW_CIDR"); cidr != "" {
		allowList := strings.Split(cidr, ",")
		r.Use(ipFilterMiddleware(logger, allowList))
	}

	if apiKey := os.Getenv("VOLANT_API_KEY"); apiKey != "" {
		r.Use(apiKeyMiddleware(apiKey))
	}

	api := &apiServer{
		logger:      logger,
		engine:      engine,
		bus:         bus,
		agentPort:   agentDefaultPort,
		agentClient: &http.Client{Timeout: 120 * time.Second},
		plugins:     plugins,
	}

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")
	{
		v1.GET("/system/status", api.systemStatus)

		v1.POST("/mcp", api.handleMCP)

		vms := v1.Group("/vms")
		{
			vms.GET("", api.listVMs)
			vms.POST("", api.createVM)
			vms.GET(":name", api.getVM)
			vms.DELETE(":name", api.deleteVM)
			vms.Any(":name/agent/*path", api.proxyAgent)
			vms.POST(":name/actions/navigate", api.handleNavigateAction)
			vms.POST(":name/actions/screenshot", api.handleScreenshotAction)
			vms.POST(":name/actions/exec", api.handleExecAction)
		}

		events := v1.Group("/events")
		{
			events.GET("/vms", api.streamVMEvents)
		}
	}

	r.GET("/ws/v1/vms/:name/devtools/*path", api.vmDevToolsWebSocket)
	r.GET("/ws/v1/vms/:name/logs", api.vmLogsWebSocket)
	r.GET("/ws/v1/agui", api.aguiWebSocket)

	return r
}

func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		args := []any{
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.String("latency", latency.String()),
			slog.String("client_ip", c.ClientIP()),
		}
		if len(c.Errors) > 0 {
			args = append(args, slog.String("error", c.Errors.String()))
			logger.Error("http request", args...)
		} else {
			logger.Info("http request", args...)
		}
	}
}

func ipFilterMiddleware(logger *slog.Logger, cidrs []string) gin.HandlerFunc {
	var networks []*net.IPNet
	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			logger.Warn("invalid CIDR", "cidr", raw, "error", err)
			continue
		}
		networks = append(networks, network)
	}
	if len(networks) == 0 {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		ip := net.ParseIP(c.ClientIP())
		if ip == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid client IP"})
			return
		}
		for _, network := range networks {
			if network.Contains(ip) {
				c.Next()
				return
			}
		}
		logger.Warn("request blocked by CIDR filter", "ip", ip.String())
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "access denied"})
	}
}

func apiKeyMiddleware(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		provided := c.GetHeader("X-Volant-API-Key")
		if provided == "" {
			provided = c.Query("api_key")
		}
		if provided != expected {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}
		c.Next()
	}
}

type apiServer struct {
	logger      *slog.Logger
	engine      orchestrator.Engine
	bus         eventbus.Bus
	plugins     *plugins.Registry
	agentPort   int
	agentClient *http.Client
}

type navigateActionRequest struct {
	URL string `json:"url" binding:"required"`
}

type screenshotActionRequest struct {
	FullPage bool   `json:"full_page"`
	Format   string `json:"format"`
	Quality  int    `json:"quality"`
}

type execActionRequest struct {
	Expression string `json:"expression" binding:"required"`
}

type scrapeActionRequest struct {
	Selector  string `json:"selector" binding:"required"`
	Attribute string `json:"attribute"`
}

type evaluateActionRequest struct {
	Expression   string `json:"expression" binding:"required"`
	AwaitPromise bool   `json:"await_promise"`
}

type graphqlActionRequest struct {
	Endpoint  string                 `json:"endpoint"`
	Query     string                 `json:"query" binding:"required"`
	Variables map[string]interface{} `json:"variables"`
}

type createVMRequest struct {
	Name          string `json:"name" binding:"required"`
	Runtime       string `json:"runtime"`
	CPUCores      int    `json:"cpu_cores" binding:"required,min=1"`
	MemoryMB      int    `json:"memory_mb" binding:"required,min=64"`
	KernelCmdline string `json:"kernel_cmdline"`
}

type vmResponse struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	Runtime       string     `json:"runtime"`
	PID           *int64     `json:"pid,omitempty"`
	IPAddress     string     `json:"ip_address"`
	MACAddress    string     `json:"mac_address"`
	CPUCores      int        `json:"cpu_cores"`
	MemoryMB      int        `json:"memory_mb"`
	KernelCmdline string     `json:"kernel_cmdline,omitempty"`
	CreatedAt     *time.Time `json:"created_at,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

func vmToResponse(vm *db.VM) vmResponse {
	if vm == nil {
		return vmResponse{}
	}
	resp := vmResponse{
		ID:            vm.ID,
		Name:          vm.Name,
		Status:        string(vm.Status),
		Runtime:       vm.Runtime,
		PID:           vm.PID,
		IPAddress:     vm.IPAddress,
		MACAddress:    vm.MACAddress,
		CPUCores:      vm.CPUCores,
		MemoryMB:      vm.MemoryMB,
		KernelCmdline: vm.KernelCmdline,
	}
	if !vm.CreatedAt.IsZero() {
		t := vm.CreatedAt
		resp.CreatedAt = &t
	}
	if !vm.UpdatedAt.IsZero() {
		t := vm.UpdatedAt
		resp.UpdatedAt = &t
	}
	return resp
}

func (api *apiServer) listVMs(c *gin.Context) {
	vms, err := api.engine.ListVMs(c.Request.Context())
	if err != nil {
		api.logger.Error("list vms", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list vms"})
		return
	}
	resp := make([]vmResponse, 0, len(vms))
	for i := range vms {
		vm := vms[i]
		resp = append(resp, vmToResponse(&vm))
	}
	c.JSON(http.StatusOK, resp)
}

func (api *apiServer) getVM(c *gin.Context) {
	name := c.Param("name")
	vm, err := api.engine.GetVM(c.Request.Context(), name)
	if err != nil {
		api.logger.Error("get vm", "vm", name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch vm"})
		return
	}
	if vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "vm not found"})
		return
	}
	c.JSON(http.StatusOK, vmToResponse(vm))
}

func (api *apiServer) createVM(c *gin.Context) {
	var req createVMRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	vm, err := api.engine.CreateVM(c.Request.Context(), orchestrator.CreateVMRequest{
		Name:              req.Name,
		Runtime:           req.Runtime,
		CPUCores:          req.CPUCores,
		MemoryMB:          req.MemoryMB,
		KernelCmdlineHint: req.KernelCmdline,
	})
	if err != nil {
		api.logger.Error("create vm", "vm", req.Name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	// Emit event for async notification
	api.bus.Publish(c.Request.Context(), orchestratorevents.TopicVMEvents, orchestratorevents.VMEvent{
		Type:      orchestratorevents.TypeVMCreated,
		Name:      vm.Name,
		Timestamp: time.Now().UTC(),
		Message:   "VM created",
	})
	c.JSON(http.StatusCreated, vmToResponse(vm))
}

func (api *apiServer) deleteVM(c *gin.Context) {
	name := c.Param("name")
	if err := api.engine.DestroyVM(c.Request.Context(), name); err != nil {
		api.logger.Error("destroy vm", "vm", name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (api *apiServer) streamVMEvents(c *gin.Context) {
	if api.bus == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event streaming not available"})
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	ctx := c.Request.Context()
	eventsCh := make(chan any, 16)
	unsubscribe, err := api.bus.Subscribe(orchestratorevents.TopicVMEvents, eventsCh)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to subscribe"})
		return
	}
	defer unsubscribe()

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-eventsCh:
			if payload == nil {
				continue
			}
			vmEvent, ok := payload.(orchestratorevents.VMEvent)
			if !ok {
				continue
			}
			data, err := json.Marshal(vmEvent)
			if err != nil {
				api.logger.Error("marshal vm event", "error", err)
				continue
			}
			if _, err := c.Writer.Write([]byte("event: " + vmEvent.Type + "\n")); err != nil {
				return
			}
			if _, err := c.Writer.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (api *apiServer) systemStatus(c *gin.Context) {
	vms, err := api.engine.ListVMs(c.Request.Context())
	if err != nil {
		api.logger.Error("system status", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch status"})
		return
	}
	resp := SystemStatusResponse{
		VMCount: len(vms),
		CPU:     0.0, // Placeholder; integrate real metrics later
		MEM:     0.0, // Placeholder
	}
	c.JSON(http.StatusOK, resp)
}

type SystemStatusResponse struct {
	VMCount int     `json:"vm_count"`
	CPU     float64 `json:"cpu_percent"`
	MEM     float64 `json:"mem_percent"`
}

type MCPRequest struct {
	Command string                 `json:"command"`
	Params  map[string]interface{} `json:"params"`
}

type MCPResponse struct {
	Result interface{} `json:"result"`
	Error  string      `json:"error,omitempty"`
}

func (api *apiServer) handleMCP(c *gin.Context) {
	var req MCPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MCPResponse{Error: err.Error()})
		return
	}

	ctx := c.Request.Context()
	var result interface{}
	var err error

	switch req.Command {
	case "hype.vms.list":
		vms, e := api.engine.ListVMs(ctx)
		if e != nil {
			err = e
		} else {
			vmList := make([]map[string]interface{}, len(vms))
			for i, vm := range vms {
				vmList[i] = map[string]interface{}{
					"id":         vm.ID,
					"name":       vm.Name,
					"status":     vm.Status,
					"ip_address": vm.IPAddress,
					"cpu_cores":  vm.CPUCores,
					"memory_mb":  vm.MemoryMB,
				}
			}
			result = vmList
		}
	case "hype.vms.create":
		name, ok := req.Params["name"].(string)
		if !ok {
			err = fmt.Errorf("name param required")
		} else {
			vm, e := api.engine.CreateVM(ctx, orchestrator.CreateVMRequest{
				Name:     name,
				CPUCores: 2,
				MemoryMB: 2048,
			})
			if e != nil {
				err = e
			} else {
				result = map[string]interface{}{
					"id":         vm.ID,
					"name":       vm.Name,
					"status":     vm.Status,
					"ip_address": vm.IPAddress,
					"cpu_cores":  vm.CPUCores,
					"memory_mb":  vm.MemoryMB,
				}
				// Emit event for async notification
				api.bus.Publish(ctx, orchestratorevents.TopicVMEvents, orchestratorevents.VMEvent{
					Type:      orchestratorevents.TypeVMCreated,
					Name:      vm.Name,
					Timestamp: time.Now().UTC(),
					Message:   "VM created via MCP",
				})
			}
		}
	case "hype.system.get_capabilities":
		result = map[string]interface{}{
			"capabilities": []map[string]interface{}{
				{
					"name":        "hype.vms.create",
					"description": "Create a new microVM",
					"params": map[string]interface{}{
						"name":      "string (required)",
						"cpu_cores": "int (default 2)",
						"memory_mb": "int (default 2048)",
					},
				},
				{
					"name":        "hype.vms.list",
					"description": "List all microVMs",
					"params":      map[string]interface{}{},
				},
			},
		}
	default:
		err = fmt.Errorf("unknown command: %s", req.Command)
	}

	resp := MCPResponse{Result: result}
	if err != nil {
		resp.Error = err.Error()
	}
	c.JSON(http.StatusOK, resp)
}

func (api *apiServer) aguiWebSocket(c *gin.Context) {
	conn, err := (&websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}).Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		api.logger.Error("agui ws upgrade", "error", err)
		return
	}
	defer conn.Close()

	ctx := c.Request.Context()
	eventsCh := make(chan any, 16)
	unsubscribe, err := api.bus.Subscribe(orchestratorevents.TopicVMEvents, eventsCh)
	if err != nil {
		api.logger.Error("agui ws subscribe", "error", err)
		return
	}
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-eventsCh:
			if payload == nil {
				continue
			}
			vmEvent, ok := payload.(orchestratorevents.VMEvent)
			if !ok {
				continue
			}
			// Translate to AG-UI event
			var aguiEvent interface{}
			switch vmEvent.Type {
			case orchestratorevents.TypeVMCreated:
				aguiEvent = agui.RunStartedEvent{
					ID:   vmEvent.Name,
					Name: "VM " + vmEvent.Name + " started",
				}
			case orchestratorevents.TypeVMRunning:
				aguiEvent = agui.TextMessageEvent{
					Type: "text",
					Text: "VM " + vmEvent.Name + " is running",
				}
			case orchestratorevents.TypeVMStopped:
				aguiEvent = agui.RunFinishedEvent{
					Output: "VM " + vmEvent.Name + " stopped",
				}
			default:
				continue
			}
			if err := conn.WriteJSON(aguiEvent); err != nil {
				return
			}
		}
	}
}

type devToolsInfo struct {
	WebSocketURL   string `json:"websocket_url"`
	WebSocketPath  string `json:"websocket_path"`
	BrowserVersion string `json:"browser_version"`
	UserAgent      string `json:"user_agent"`
	Address        string `json:"address"`
	Port           int    `json:"port"`
}

type agentLogEvent struct {
	Stream    string    `json:"stream"`
	Line      string    `json:"line"`
	Timestamp time.Time `json:"timestamp"`
}

type vmLogPayload struct {
	Name      string    `json:"name"`
	Stream    string    `json:"stream"`
	Line      string    `json:"line"`
	Timestamp time.Time `json:"timestamp"`
}

func (api *apiServer) proxyAgent(c *gin.Context) {
	if api.agentClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent proxy unavailable"})
		return
	}

	name := c.Param("name")
	vm, err := api.engine.GetVM(c.Request.Context(), name)
	if err != nil {
		api.logger.Error("proxy agent get vm", "vm", name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve vm"})
		return
	}
	if vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "vm not found"})
		return
	}
	if vm.Status != db.VMStatusRunning {
		c.JSON(http.StatusConflict, gin.H{"error": "vm not running"})
		return
	}
	if vm.IPAddress == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "vm ip address unavailable"})
		return
	}
	if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "websocket upgrade not supported"})
		return
	}

	proxyPath := c.Param("path")
	if proxyPath == "" {
		proxyPath = "/"
	}
	target := api.agentURL(vm, proxyPath)
	if raw := c.Request.URL.RawQuery; raw != "" {
		target = target + "?" + raw
	}

	var bodyReader io.Reader = http.NoBody
	if c.Request.Body != nil {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		if err := c.Request.Body.Close(); err != nil {
			api.logger.Debug("proxy agent body close", "vm", vm.Name, "error", err)
		}
		if len(bodyBytes) > 0 {
			bodyReader = bytes.NewReader(bodyBytes)
		}
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, target, bodyReader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create proxy request"})
		return
	}

	req.Header = make(http.Header)
	copyHeaders(req.Header, c.Request.Header)
	req.Header.Del("Accept-Encoding")
	req.Host = fmt.Sprintf("%s:%d", vm.IPAddress, api.agentPort)

	resp, err := api.agentClient.Do(req)
	if err != nil {
		api.logger.Error("proxy agent request", "vm", vm.Name, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	for key := range c.Writer.Header() {
		c.Writer.Header().Del(key)
	}
	copyHeaders(c.Writer.Header(), resp.Header)
	c.Status(resp.StatusCode)

	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		api.logger.Debug("proxy agent copy", "vm", vm.Name, "error", err)
	}
	c.Abort()
}

func (api *apiServer) fetchDevToolsInfo(ctx context.Context, vm *db.VM) (*devToolsInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.agentURL(vm, "/v1/devtools"), nil)
	if err != nil {
		return nil, fmt.Errorf("devtools request: %w", err)
	}
	resp, err := api.agentClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("devtools request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("devtools response status %d", resp.StatusCode)
	}

	var info devToolsInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode devtools response: %w", err)
	}

	if info.Port == 0 {
		if parsed, err := url.Parse(info.WebSocketURL); err == nil && parsed.Port() != "" {
			if p, convErr := strconv.Atoi(parsed.Port()); convErr == nil {
				info.Port = p
			}
		}
		if info.Port == 0 {
			info.Port = agentDevToolsDefaultPort
		}
	}

	if info.WebSocketPath == "" {
		if parsed, err := url.Parse(info.WebSocketURL); err == nil && parsed.Path != "" {
			info.WebSocketPath = parsed.Path
		}
	}

	return &info, nil
}

func (api *apiServer) vmDevToolsWebSocket(c *gin.Context) {
	if api.agentClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent proxy unavailable"})
		return
	}

	ctx := c.Request.Context()
	name := c.Param("name")

	vm, err := api.engine.GetVM(ctx, name)
	if err != nil {
		api.logger.Error("devtools ws get vm", "vm", name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve vm"})
		return
	}
	if vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "vm not found"})
		return
	}
	if vm.Status != db.VMStatusRunning || vm.IPAddress == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "vm not ready"})
		return
	}

	info, err := api.fetchDevToolsInfo(ctx, vm)
	if err != nil {
		api.logger.Error("devtools info", "vm", vm.Name, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "devtools metadata unavailable"})
		return
	}

	wsPath := c.Param("path")
	if wsPath == "" || wsPath == "/" {
		wsPath = info.WebSocketPath
	}
	if wsPath == "" {
		wsPath = "/devtools/browser"
	}
	if !strings.HasPrefix(wsPath, "/") {
		wsPath = "/" + wsPath
	}

	targetURL, err := url.Parse(info.WebSocketURL)
	if err != nil || targetURL.Host == "" {
		targetURL = &url.URL{
			Scheme: "ws",
			Host:   net.JoinHostPort(vm.IPAddress, strconv.Itoa(info.Port)),
		}
	}
	if targetURL.Scheme == "" {
		targetURL.Scheme = "ws"
	}
	switch targetURL.Scheme {
	case "http":
		targetURL.Scheme = "ws"
	case "https":
		targetURL.Scheme = "wss"
	}

	targetURL.Path = wsPath
	targetURL.RawQuery = c.Request.URL.RawQuery

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 30 * time.Second,
	}
	agentConn, resp, err := dialer.DialContext(ctx, targetURL.String(), nil)
	if resp != nil {
		resp.Body.Close()
	}
	if err != nil {
		api.logger.Error("devtools ws dial", "vm", vm.Name, "target", targetURL.String(), "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to connect devtools", "target": targetURL.String()})
		return
	}
	defer agentConn.Close()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		api.logger.Error("devtools ws upgrade", "vm", vm.Name, "error", err)
		return
	}
	defer clientConn.Close()

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go pumpWebSocket(ctx, api.logger, "agent->client", agentConn, clientConn, &wg, errCh)
	go pumpWebSocket(ctx, api.logger, "client->agent", clientConn, agentConn, &wg, errCh)

	var proxyErr error
	select {
	case <-ctx.Done():
		proxyErr = ctx.Err()
	case proxyErr = <-errCh:
	}

	_ = agentConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	_ = clientConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))

	wg.Wait()

	if proxyErr != nil && !errors.Is(proxyErr, context.Canceled) && !errors.Is(proxyErr, net.ErrClosed) && !errors.Is(proxyErr, io.EOF) && !websocket.IsCloseError(proxyErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		api.logger.Debug("devtools proxy closed", "vm", vm.Name, "error", proxyErr)
	}
}

func pumpWebSocket(ctx context.Context, logger *slog.Logger, direction string, src, dst *websocket.Conn, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()
	for {
		msgType, payload, err := src.ReadMessage()
		if err != nil {
			errCh <- fmt.Errorf("%s read: %w", direction, err)
			return
		}
		if writeErr := dst.WriteMessage(msgType, payload); writeErr != nil {
			errCh <- fmt.Errorf("%s write: %w", direction, writeErr)
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func proxyWebSocket(src, dst *websocket.Conn) error {
	for {
		messageType, payload, err := src.ReadMessage()
		if err != nil {
			return err
		}
		if err := dst.WriteMessage(messageType, payload); err != nil {
			return err
		}
	}
}

func (api *apiServer) vmLogsWebSocket(c *gin.Context) {
	if api.agentClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent proxy unavailable"})
		return
	}

	conn, err := (&websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}).Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		api.logger.Error("vm logs ws upgrade", "error", err)
		return
	}
	defer conn.Close()

	ctx := c.Request.Context()
	name := c.Param("name")

	vm, err := api.engine.GetVM(ctx, name)
	if err != nil {
		api.logger.Error("vm logs get vm", "vm", name, "error", err)
		writeWebSocketClose(conn, websocket.CloseInternalServerErr, "failed to resolve vm")
		return
	}
	if vm == nil {
		writeWebSocketClose(conn, websocket.CloseNormalClosure, "vm not found")
		return
	}
	if vm.Status != db.VMStatusRunning || vm.IPAddress == "" {
		writeWebSocketClose(conn, websocket.CloseTryAgainLater, "vm not ready")
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.agentURL(vm, "/v1/logs/stream"), nil)
	if err != nil {
		writeWebSocketClose(conn, websocket.CloseInternalServerErr, "stream request failed")
		return
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := api.agentClient.Do(req)
	if err != nil {
		api.logger.Error("vm logs stream", "vm", vm.Name, "error", err)
		writeWebSocketClose(conn, websocket.CloseTryAgainLater, "agent unreachable")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeWebSocketClose(conn, websocket.CloseTryAgainLater, fmt.Sprintf("agent returned %d", resp.StatusCode))
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var builder strings.Builder
	flush := func() bool {
		if builder.Len() == 0 {
			return true
		}
		payload := builder.String()
		builder.Reset()

		var raw agentLogEvent
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			api.logger.Debug("agent log decode", "vm", vm.Name, "error", err)
			return true
		}

		event := vmLogPayload{
			Name:      vm.Name,
			Stream:    raw.Stream,
			Line:      raw.Line,
			Timestamp: raw.Timestamp,
		}
		if err := conn.WriteJSON(event); err != nil {
			return false
		}

		if api.bus != nil {
			e := orchestratorevents.VMEvent{
				Type:      orchestratorevents.TypeVMLog,
				Name:      vm.Name,
				Status:    orchestratorevents.VMStatusRunning,
				IPAddress: vm.IPAddress,
				Timestamp: raw.Timestamp,
				Message:   raw.Line,
				Stream:    raw.Stream,
				Line:      raw.Line,
			}
			if err := api.bus.Publish(ctx, orchestratorevents.TopicVMEvents, e); err != nil {
				api.logger.Debug("publish vm log", "vm", vm.Name, "error", err)
			}
		}
		return true
	}

	for {
		if err := ctx.Err(); err != nil {
			writeWebSocketClose(conn, websocket.CloseNormalClosure, err.Error())
			return
		}

		if !scanner.Scan() {
			_ = flush()
			if err := scanner.Err(); err != nil && ctx.Err() == nil {
				api.logger.Debug("vm log stream ended", "vm", vm.Name, "error", err)
			}
			writeWebSocketClose(conn, websocket.CloseNormalClosure, "stream closed")
			return
		}

		line := scanner.Text()
		if line == "" {
			if !flush() {
				writeWebSocketClose(conn, websocket.CloseAbnormalClosure, "client closed")
				return
			}
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(data)
	}
}

func (api *apiServer) agentURL(vm *db.VM, path string) string {
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fmt.Sprintf("http://%s:%d%s", vm.IPAddress, api.agentPort, path)
}

func copyHeaders(dst, src http.Header) {
	for key := range dst {
		if _, hop := hopHeaders[strings.ToLower(key)]; hop {
			dst.Del(key)
		}
	}
	for key, values := range src {
		if _, hop := hopHeaders[strings.ToLower(key)]; hop {
			continue
		}
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func writeWebSocketClose(conn *websocket.Conn, code int, message string) {
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, message), time.Now().Add(time.Second))
}

func (api *apiServer) agentAction(c *gin.Context, vm *db.VM, method, path string, body any, out any) error {
	if method == "" {
		method = http.MethodPost
	}

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode request"})
			return err
		}
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), method, api.agentURL(vm, path), bytes.NewReader(buf.Bytes()))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create agent request"})
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := api.agentClient.Do(req)
	if err != nil {
		api.logger.Error("agent action", "vm", vm.Name, "path", path, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			c.JSON(resp.StatusCode, gin.H{"error": http.StatusText(resp.StatusCode)})
			return fmt.Errorf("agent returned %d", resp.StatusCode)
		}
		message, _ := payload["error"].(string)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		c.JSON(resp.StatusCode, gin.H{"error": message})
		return fmt.Errorf("agent returned %d", resp.StatusCode)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode agent response"})
		return err
	}
	return nil
}

func (api *apiServer) resolveVM(c *gin.Context) (*db.VM, bool) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "vm name required"})
		return nil, false
	}

	vm, err := api.engine.GetVM(c.Request.Context(), name)
	if err != nil {
		api.logger.Error("resolve vm", "vm", name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve vm"})
		return nil, false
	}
	if vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "vm not found"})
		return nil, false
	}
	if vm.Status != db.VMStatusRunning || vm.IPAddress == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "vm not ready"})
		return nil, false
	}

	return vm, true
}

func statusFromError(err error) int {
	switch {
	case errors.Is(err, orchestrator.ErrVMNotFound):
		return http.StatusNotFound
	case errors.Is(err, orchestrator.ErrVMExists):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func (api *apiServer) actionNavigate(c *gin.Context) {
	var req navigateActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vm, ok := api.resolveVM(c)
	if !ok {
		return
	}

	if err := api.agentAction(c, vm, http.MethodPost, "/v1/browser/navigate", req, nil); err != nil {
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (api *apiServer) actionScreenshot(c *gin.Context) {
	var req screenshotActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vm, ok := api.resolveVM(c)
	if !ok {
		return
	}

	var resp map[string]interface{}
	if err := api.agentAction(c, vm, http.MethodPost, "/v1/browser/screenshot", req, &resp); err != nil {
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (api *apiServer) actionEvaluate(c *gin.Context) {
	var req evaluateActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vm, ok := api.resolveVM(c)
	if !ok {
		return
	}

	var resp map[string]interface{}
	if err := api.agentAction(c, vm, http.MethodPost, "/v1/browser/evaluate", req, &resp); err != nil {
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (api *apiServer) handleNavigateAction(c *gin.Context) {
	var req navigateActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vm, ok := api.resolveVM(c)
	if !ok {
		return
	}

	if err := api.agentAction(c, vm, http.MethodPost, "/v1/browser/navigate", req, nil); err != nil {
		return
	}
	c.Status(http.StatusAccepted)
}

func (api *apiServer) handleScreenshotAction(c *gin.Context) {
	var req screenshotActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vm, ok := api.resolveVM(c)
	if !ok {
		return
	}

	var resp map[string]any
	if err := api.agentAction(c, vm, http.MethodPost, "/v1/browser/screenshot", req, &resp); err != nil {
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (api *apiServer) handleExecAction(c *gin.Context) {
	var req execActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vm, ok := api.resolveVM(c)
	if !ok {
		return
	}

	var resp map[string]any
	if err := api.agentAction(c, vm, http.MethodPost, "/v1/browser/evaluate", req, &resp); err != nil {
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (api *apiServer) actionScrape(c *gin.Context) {
	var req scrapeActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vm, ok := api.resolveVM(c)
	if !ok {
		return
	}

	var resp map[string]interface{}
	if err := api.agentAction(c, vm, http.MethodPost, "/v1/dom/scrape", req, &resp); err != nil {
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (api *apiServer) actionGraphQL(c *gin.Context) {
	var req graphqlActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vm, ok := api.resolveVM(c)
	if !ok {
		return
	}

	var resp map[string]interface{}
	if err := api.agentAction(c, vm, http.MethodPost, "/v1/browser/graphql", req, &resp); err != nil {
		return
	}
	c.JSON(http.StatusOK, resp)
}
