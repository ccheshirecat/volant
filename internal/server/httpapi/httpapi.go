package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/viperhq/viper/internal/protocol/agui"
	"github.com/viperhq/viper/internal/server/db"
	"github.com/viperhq/viper/internal/server/eventbus"
	"github.com/viperhq/viper/internal/server/orchestrator"
	orchestratorevents "github.com/viperhq/viper/internal/server/orchestrator/events"
	"github.com/gorilla/websocket"

	"github.com/viperhq/viper/internal/protocol/agui"
)

// New constructs the HTTP API router backed by the orchestrator engine.
func New(logger *slog.Logger, engine orchestrator.Engine, bus eventbus.Bus) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger(logger))

	if cidr := os.Getenv("VIPER_API_ALLOW_CIDR"); cidr != "" {
		allowList := strings.Split(cidr, ",")
		r.Use(ipFilterMiddleware(logger, allowList))
	}

	if apiKey := os.Getenv("VIPER_API_KEY"); apiKey != "" {
		r.Use(apiKeyMiddleware(apiKey))
	}

	api := &apiServer{logger: logger, engine: engine, bus: bus}

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
		}

		events := v1.Group("/events")
		{
			events.GET("/vms", api.streamVMEvents)
		}
	}

	r.GET("/ws/v1/agui", api.aguiWebSocket)

	return r
}

// requestLogger adapts slog to Gin's middleware interface.
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
		provided := c.GetHeader("X-Viper-API-Key")
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
	logger *slog.Logger
	engine orchestrator.Engine
	bus    eventbus.Bus
}

type createVMRequest struct {
	Name          string `json:"name" binding:"required"`
	CPUCores      int    `json:"cpu_cores" binding:"required,min=1"`
	MemoryMB      int    `json:"memory_mb" binding:"required,min=64"`
	KernelCmdline string `json:"kernel_cmdline"`
}

type vmResponse struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
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
	api.bus.Publish(orchestratorevents.VMEvent{
		Type:      orchestratorevents.VMTypeCreated,
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

// MCPRequest is the incoming MCP request format.
type MCPRequest struct {
	Command string                 `json:"command"`
	Params  map[string]interface{} `json:"params"`
}

// MCPResponse is the outgoing MCP response format.
type MCPResponse struct {
	Result interface{} `json:"result"`
	Error  string      `json:"error,omitempty"`
}

// handleMCP handles MCP requests.
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
	case "viper.vms.list":
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
	case "viper.vms.create":
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
				api.bus.Publish(orchestratorevents.VMEvent{
					Type:      orchestratorevents.VMTypeCreated,
					Name:      vm.Name,
					Timestamp: time.Now().UTC(),
					Message:   "VM created via MCP",
				})
			}
		}
	case "viper.system.get_capabilities":
		result = map[string]interface{}{
			"capabilities": []map[string]interface{}{
				{
					"name":        "viper.vms.create",
					"description": "Create a new microVM",
					"params": map[string]interface{}{
						"name": "string (required)",
						"cpu_cores": "int (default 2)",
						"memory_mb": "int (default 2048)",
					},
				},
				{
					"name":        "viper.vms.list",
					"description": "List all microVMs",
					"params": map[string]interface{}{},
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

// aguiWebSocket handles AG-UI WebSocket connections.
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
			case orchestratorevents.VMTypeCreated:
				aguiEvent = agui.RunStartedEvent{
					ID:   vmEvent.Name,
					Name: "VM " + vmEvent.Name + " started",
				}
			case orchestratorevents.VMTypeRunning:
				aguiEvent = agui.TextMessageEvent{
					Type: "text",
					Text: "VM " + vmEvent.Name + " is running",
				}
			case orchestratorevents.VMTypeStopped:
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

// SystemStatusResponse is the response for system metrics.
type SystemStatusResponse struct {
	VMCount int     `json:"vm_count"`
	CPU     float64 `json:"cpu_percent"`
	MEM     float64 `json:"mem_percent"`
}

// systemStatus returns basic system metrics.
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

// MCPRequest is the incoming MCP request format.
type MCPRequest struct {
	Command string                 `json:"command"`
	Params  map[string]interface{} `json:"params"`
}

// MCPResponse is the outgoing MCP response format.
type MCPResponse struct {
	Result interface{} `json:"result"`
	Error  string      `json:"error,omitempty"`
}

// handleMCP handles MCP requests.
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
	case "viper.vms.list":
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
	case "viper.vms.create":
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
				api.bus.Publish(orchestratorevents.VMEvent{
					Type:      orchestratorevents.VMTypeCreated,
					Name:      vm.Name,
					Timestamp: time.Now().UTC(),
					Message:   "VM created via MCP",
				})
			}
		}
	case "viper.system.get_capabilities":
		result = map[string]interface{}{
			"capabilities": []map[string]interface{}{
				{
					"name":        "viper.vms.create",
					"description": "Create a new microVM",
					"params": map[string]interface{}{
						"name": "string (required)",
						"cpu_cores": "int (default 2)",
						"memory_mb": "int (default 2048)",
					},
				},
				{
					"name":        "viper.vms.list",
					"description": "List all microVMs",
					"params": map[string]interface{}{},
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

// aguiWebSocket handles AG-UI WebSocket connections.
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
			case orchestratorevents.VMTypeCreated:
				aguiEvent = agui.RunStartedEvent{
					ID:   vmEvent.Name,
					Name: "VM " + vmEvent.Name + " started",
				}
			case orchestratorevents.VMTypeRunning:
				aguiEvent = agui.TextMessageEvent{
					Type: "text",
					Text: "VM " + vmEvent.Name + " is running",
				}
			case orchestratorevents.VMTypeStopped:
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
