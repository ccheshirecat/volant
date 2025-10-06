// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

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

	"github.com/volantvm/volant/internal/pluginspec"
	"github.com/volantvm/volant/internal/server/db"
	"github.com/volantvm/volant/internal/server/devicemanager"
	"github.com/volantvm/volant/internal/server/eventbus"
	"github.com/volantvm/volant/internal/server/orchestrator"
	orchestratorevents "github.com/volantvm/volant/internal/server/orchestrator/events"
	"github.com/volantvm/volant/internal/server/orchestrator/vmconfig"
	"github.com/volantvm/volant/internal/server/plugins"
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

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "http://localhost:3000")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Volant-API-Key")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func New(logger *slog.Logger, engine orchestrator.Engine, bus eventbus.Bus, plugins *plugins.Registry) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())
	r.Use(requestLogger(logger))

	// CORS (optional, for browser-based UI)
	if raw := os.Getenv("VOLANT_CORS_ORIGINS"); raw != "" {
		origins := strings.Split(raw, ",")
		r.Use(corsMiddleware(logger, origins))
	}

	if cidr := os.Getenv("VOLANT_API_ALLOW_CIDR"); cidr != "" {
		allowList := strings.Split(cidr, ",")
		r.Use(ipFilterMiddleware(logger, allowList))
	}

	if apiKey := os.Getenv("VOLANT_API_KEY"); apiKey != "" {
		r.Use(apiKeyMiddleware(apiKey))
	}

	if err := loadStoredPlugins(engine, logger, plugins); err != nil {
		logger.Warn("load stored plugins", "error", err)
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

	// Serve OpenAPI spec at /openapi (JSON)
	r.GET("/openapi", func(c *gin.Context) {
		api.serveOpenAPI(c.Writer, c.Request)
	})

	v1 := r.Group("/api/v1")
	{
		v1.GET("/system/status", api.systemStatus)
		v1.GET("/system/info", api.systemInfo)
		v1.GET("/system/summary", api.systemSummary)
		v1.POST("/mcp", api.handleMCP)

		vms := v1.Group("/vms")
		{
			vms.GET("", api.listVMs)
			vms.POST("", api.createVM)
			vms.GET(":name", api.getVM)
			vms.GET(":name/config", api.getVMConfig)
			vms.GET(":name/config/history", api.getVMConfigHistory)
			vms.PATCH(":name/config", api.updateVMConfig)
			vms.DELETE(":name", api.deleteVM)
			vms.POST(":name/start", api.startVM)
			vms.POST(":name/stop", api.stopVM)
			vms.POST(":name/restart", api.restartVM)
			vms.GET(":name/openapi", api.getVMOpenAPI)
			vms.Any(":name/agent/*path", api.proxyAgent)
			vms.POST(":name/actions/:plugin/:action", api.postVMPluginAction)
		}

		deployments := v1.Group("/deployments")
		{
			deployments.GET("", api.listDeployments)
			deployments.POST("", api.createDeployment)
			deployments.GET(":name", api.getDeployment)
			deployments.PATCH(":name", api.patchDeployment)
			deployments.DELETE(":name", api.deleteDeployment)
		}

		pluginsGroup := v1.Group("/plugins")
		{
			pluginsGroup.GET("", api.listPlugins)
			pluginsGroup.POST("", api.installPlugin)
			pluginsGroup.GET(":plugin", api.describePlugin)
			pluginsGroup.GET(":plugin/manifest", api.getPluginManifest)
			pluginsGroup.DELETE(":plugin", api.removePlugin)
			pluginsGroup.POST(":plugin/enabled", api.setPluginEnabled)
			pluginsGroup.POST(":plugin/actions/:action", api.postPluginAction)

			// Plugin artifacts API
			pluginsGroup.GET(":plugin/artifacts", api.listPluginArtifacts)
			pluginsGroup.POST(":plugin/artifacts", api.upsertPluginArtifact)
			pluginsGroup.DELETE(":plugin/artifacts", api.deletePluginArtifacts)
			pluginsGroup.GET(":plugin/artifacts/:artifact", api.getPluginArtifact)
		}

		events := v1.Group("/events")
		{
			events.GET("/vms", api.streamVMEvents)
		}

		vfio := v1.Group("/vfio")
		{
			vfio.POST("/devices/info", api.getVFIODeviceInfo)
			vfio.POST("/devices/validate", api.validateVFIODevices)
			vfio.POST("/devices/iommu-groups", api.checkVFIOIOMMUGroups)
			vfio.POST("/devices/bind", api.bindVFIODevices)
			vfio.POST("/devices/unbind", api.unbindVFIODevices)
			vfio.POST("/devices/group-paths", api.getVFIOGroupPaths)
		}
	}

	r.GET("/ws/v1/vms/:name/devtools/*path", api.vmDevToolsWebSocket)
	r.GET("/ws/v1/vms/:name/console", api.vmConsoleWebSocket)
	r.GET("/ws/v1/vms/:name/logs", api.vmLogsWebSocket)

	return r
}

func loadStoredPlugins(engine orchestrator.Engine, logger *slog.Logger, registry *plugins.Registry) error {
	if engine == nil || registry == nil {
		return nil
	}
	store := engine.Store()
	if store == nil {
		return nil
	}
	return store.WithTx(context.Background(), func(q db.Queries) error {
		entries, err := q.Plugins().List(context.Background())
		if err != nil {
			return err
		}
		for _, entry := range entries {
			var manifest pluginspec.Manifest
			if len(entry.Metadata) > 0 {
				if err := json.Unmarshal(entry.Metadata, &manifest); err != nil {
					logger.Warn("decode plugin manifest", "plugin", entry.Name, "error", err)
					continue
				}
			} else {
				manifest = pluginspec.Manifest{Name: entry.Name, Version: entry.Version}
			}
			manifest.Enabled = entry.Enabled
			manifest.Normalize()
			registry.Register(manifest)
		}
		return nil
	})
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

// corsMiddleware enables configurable CORS for browser-based clients.
// Allowed origins are provided via configuration; methods/headers use sane defaults.
func corsMiddleware(logger *slog.Logger, allowed []string) gin.HandlerFunc {
	normalized := make([]string, 0, len(allowed))
	allowAll := false
	for _, o := range allowed {
		v := strings.TrimSpace(o)
		if v == "" {
			continue
		}
		if v == "*" {
			allowAll = true
		}
		normalized = append(normalized, v)
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		// Set CORS headers if origin is allowed
		if origin != "" {
			allowedOrigin := ""
			if allowAll {
				allowedOrigin = "*"
			} else {
				lo := strings.ToLower(origin)
				for _, o := range normalized {
					if strings.EqualFold(o, origin) || strings.EqualFold(o, lo) {
						allowedOrigin = origin
						break
					}
				}
			}
			if allowedOrigin != "" {
				c.Header("Access-Control-Allow-Origin", allowedOrigin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, X-Volant-API-Key")
				c.Header("Access-Control-Expose-Headers", "Content-Type, X-Total-Count")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
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

type createDeploymentRequest struct {
	Name     string          `json:"name" binding:"required"`
	Replicas int             `json:"replicas"`
	Config   vmconfig.Config `json:"config" binding:"required"`
}

type patchDeploymentRequest struct {
	Replicas *int `json:"replicas" binding:"required"`
}

type deploymentResponse struct {
	Name            string          `json:"name"`
	DesiredReplicas int             `json:"desired_replicas"`
	ReadyReplicas   int             `json:"ready_replicas"`
	Config          vmconfig.Config `json:"config"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type createVMRequest struct {
	Name          string           `json:"name" binding:"required"`
	Plugin        string           `json:"plugin"`
	Runtime       string           `json:"runtime"`
	CPUCores      int              `json:"cpu_cores"`
	MemoryMB      int              `json:"memory_mb"`
	KernelCmdline string           `json:"kernel_cmdline"`
	APIHost       string           `json:"api_host"`
	APIPort       string           `json:"api_port"`
	Config        *vmconfig.Config `json:"config,omitempty"`
}

type vfioDeviceInfoRequest struct {
	PCIAddress string `json:"pci_address" binding:"required"`
}

type vfioDeviceInfoResponse struct {
	Address    string `json:"address"`
	Vendor     string `json:"vendor"`
	Device     string `json:"device"`
	Driver     string `json:"driver"`
	IOMMUGroup string `json:"iommu_group"`
	NumaNode   string `json:"numa_node"`
}

type vfioValidateRequest struct {
	PCIAddresses []string `json:"pci_addresses" binding:"required"`
	Allowlist    []string `json:"allowlist,omitempty"`
}

type vfioValidateResponse struct {
	Valid   bool     `json:"valid"`
	Message string   `json:"message,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

type vfioIOMMUGroupResponse struct {
	ID      string   `json:"id"`
	Devices []string `json:"devices"`
}

type vfioBindRequest struct {
	PCIAddresses []string `json:"pci_addresses" binding:"required"`
}

type vfioBindResponse struct {
	Success      bool     `json:"success"`
	Message      string   `json:"message,omitempty"`
	BoundDevices []string `json:"bound_devices,omitempty"`
}

type vfioUnbindRequest struct {
	PCIAddresses []string `json:"pci_addresses" binding:"required"`
}

type vfioUnbindResponse struct {
	Success        bool     `json:"success"`
	Message        string   `json:"message,omitempty"`
	UnboundDevices []string `json:"unbound_devices,omitempty"`
}

type vfioGroupPathsRequest struct {
	PCIAddresses []string `json:"pci_addresses" binding:"required"`
}

type vfioGroupPathsResponse struct {
	GroupPaths []string `json:"group_paths"`
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
	KernelCmdline string     `json:"kernel_cmdline"`
	SerialSocket  string     `json:"serial_socket"`
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
		SerialSocket:  vm.SerialSocket,
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

func deploymentToResponse(dep orchestrator.Deployment) deploymentResponse {
	return deploymentResponse{
		Name:            dep.Name,
		DesiredReplicas: dep.DesiredReplicas,
		ReadyReplicas:   dep.ReadyReplicas,
		Config:          dep.Config,
		CreatedAt:       dep.CreatedAt,
		UpdatedAt:       dep.UpdatedAt,
	}
}

func (api *apiServer) listVMs(c *gin.Context) {
	// Parse filters
	statuses := make(map[string]bool)
	if arr := c.QueryArray("status"); len(arr) > 0 {
		for _, s := range arr {
			for _, part := range strings.Split(s, ",") {
				v := strings.TrimSpace(strings.ToLower(part))
				if v != "" {
					statuses[v] = true
				}
			}
		}
	}
	runtimeFilter := strings.TrimSpace(c.Query("runtime"))
	pluginFilter := strings.TrimSpace(c.Query("plugin"))
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	limit := -1
	offset := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			limit = n
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			offset = n
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset"})
			return
		}
	}
	sortField := strings.ToLower(strings.TrimSpace(c.Query("sort")))
	if sortField == "" {
		sortField = "created_at"
	}
	order := strings.ToLower(strings.TrimSpace(c.Query("order")))
	if order != "desc" {
		order = "asc"
	}

	// Load all VMs then apply filters in-memory
	vms, err := api.engine.ListVMs(c.Request.Context())
	if err != nil {
		api.logger.Error("list vms", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list vms"})
		return
	}

	// Optional plugin filter requires reading VM config

	// Filter
	filtered := make([]db.VM, 0, len(vms))
	for i := range vms {
		vm := vms[i]
		if len(statuses) > 0 && !statuses[strings.ToLower(string(vm.Status))] {
			continue
		}
		if runtimeFilter != "" && !strings.EqualFold(vm.Runtime, runtimeFilter) {
			continue
		}
		if q != "" {
			if !strings.Contains(strings.ToLower(vm.Name), q) && !strings.Contains(strings.ToLower(vm.IPAddress), q) && !strings.Contains(strings.ToLower(vm.Runtime), q) {
				continue
			}
		}
		// plugin filter via config lookup
		if pluginFilter != "" {
			versioned, cfgErr := api.engine.GetVMConfig(c.Request.Context(), vm.Name)
			if cfgErr != nil || versioned == nil {
				continue
			}
			effPlugin := strings.TrimSpace(versioned.Config.Plugin)
			if effPlugin == "" && versioned.Config.Manifest != nil {
				effPlugin = strings.TrimSpace(versioned.Config.Manifest.Name)
			}
			if !strings.EqualFold(effPlugin, pluginFilter) {
				continue
			}
		}
		filtered = append(filtered, vm)
	}

	// Sort
	less := func(i, j int) bool { return true }
	switch sortField {
	case "name":
		less = func(i, j int) bool { return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name) }
	case "status":
		less = func(i, j int) bool { return string(filtered[i].Status) < string(filtered[j].Status) }
	case "runtime":
		less = func(i, j int) bool {
			return strings.ToLower(filtered[i].Runtime) < strings.ToLower(filtered[j].Runtime)
		}
	case "updated_at":
		less = func(i, j int) bool { return filtered[i].UpdatedAt.Before(filtered[j].UpdatedAt) }
	default: // created_at
		less = func(i, j int) bool { return filtered[i].CreatedAt.Before(filtered[j].CreatedAt) }
	}
	// Simple stable sort
	for i := 0; i < len(filtered); i++ {
		for j := i + 1; j < len(filtered); j++ {
			if less(j, i) {
				filtered[i], filtered[j] = filtered[j], filtered[i]
			}
		}
	}
	if order == "desc" {
		for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
			filtered[i], filtered[j] = filtered[j], filtered[i]
		}
	}

	total := len(filtered)
	// Paginate
	start := offset
	if start > total {
		start = total
	}
	end := total
	if limit >= 0 {
		if start+limit < end {
			end = start + limit
		}
	}
	page := filtered[start:end]

	// Build response and include X-Total-Count
	resp := make([]vmResponse, 0, len(page))
	for i := range page {
		vm := page[i]
		resp = append(resp, vmToResponse(&vm))
	}
	c.Header("X-Total-Count", strconv.Itoa(total))
	c.JSON(http.StatusOK, resp)
}

// System summary for dashboard
type systemSummaryResponse struct {
	TotalVMs      int             `json:"total_vms"`
	ByStatus      map[string]int  `json:"by_status"`
	ByRuntime     map[string]int  `json:"by_runtime"`
	TotalPlugins  int             `json:"total_plugins"`
	EnabledPlugin int             `json:"enabled_plugins"`
	Plugins       []pluginSummary `json:"plugins"`
}

type pluginSummary struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
}

func (api *apiServer) systemSummary(c *gin.Context) {
	vms, err := api.engine.ListVMs(c.Request.Context())
	if err != nil {
		api.logger.Error("system summary list vms", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list vms"})
		return
	}
	byStatus := map[string]int{}
	byRuntime := map[string]int{}
	for i := range vms {
		vm := vms[i]
		byStatus[strings.ToLower(string(vm.Status))]++
		if strings.TrimSpace(vm.Runtime) != "" {
			byRuntime[strings.ToLower(vm.Runtime)]++
		}
	}
	var pluginsList []pluginSummary
	totalPlugins := 0
	enabled := 0
	if api.plugins != nil {
		names := api.plugins.List()
		totalPlugins = len(names)
		for _, name := range names {
			if manifest, ok := api.plugins.Get(name); ok {
				pluginsList = append(pluginsList, pluginSummary{Name: manifest.Name, Version: manifest.Version, Enabled: manifest.Enabled})
				if manifest.Enabled {
					enabled++
				}
			}
		}
	}
	resp := systemSummaryResponse{
		TotalVMs:      len(vms),
		ByStatus:      byStatus,
		ByRuntime:     byRuntime,
		TotalPlugins:  totalPlugins,
		EnabledPlugin: enabled,
		Plugins:       pluginsList,
	}
	c.JSON(http.StatusOK, resp)
}

func (api *apiServer) listDeployments(c *gin.Context) {
	deployments, err := api.engine.ListDeployments(c.Request.Context())
	if err != nil {
		api.logger.Error("list deployments", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list deployments"})
		return
	}
	resp := make([]deploymentResponse, 0, len(deployments))
	for _, d := range deployments {
		resp = append(resp, deploymentToResponse(d))
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
	pluginName := strings.TrimSpace(req.Plugin)
	if req.Config != nil && strings.TrimSpace(req.Config.Plugin) != "" {
		configPlugin := strings.TrimSpace(req.Config.Plugin)
		if pluginName != "" && !strings.EqualFold(pluginName, configPlugin) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "plugin mismatch between request and config"})
			return
		}
		pluginName = configPlugin
	}
	if pluginName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plugin is required"})
		return
	}
	manifest, ok := api.plugins.Get(pluginName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("plugin %s not found", pluginName)})
		return
	}
	if !manifest.Enabled {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("plugin %s disabled", pluginName)})
		return
	}
	labels := cloneLabelMap(manifest.Labels)
	manifestCopy := manifest
	manifestCopy.Labels = labels
	manifestCopy.Normalize()

	runtimeName := strings.TrimSpace(req.Runtime)
	if req.Config != nil && strings.TrimSpace(req.Config.Runtime) != "" {
		runtimeName = strings.TrimSpace(req.Config.Runtime)
	}
	if runtimeName == "" {
		runtimeName = manifestCopy.Runtime
	}
	if runtimeName == "" {
		runtimeName = manifestCopy.Name
	}
	if runtimeName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "runtime not specified and plugin manifest missing runtime"})
		return
	}

	cpu := req.CPUCores
	if req.Config != nil && req.Config.Resources.CPUCores > 0 {
		cpu = req.Config.Resources.CPUCores
	}
	if cpu <= 0 {
		cpu = 2
	}
	mem := req.MemoryMB
	if req.Config != nil && req.Config.Resources.MemoryMB > 0 {
		mem = req.Config.Resources.MemoryMB
	}
	if mem <= 0 {
		mem = 2048
	}

	kernelExtra := strings.TrimSpace(req.KernelCmdline)
	if req.Config != nil && strings.TrimSpace(req.Config.KernelCmdline) != "" {
		kernelExtra = strings.TrimSpace(req.Config.KernelCmdline)
	}

	apiHost := strings.TrimSpace(req.APIHost)
	apiPort := strings.TrimSpace(req.APIPort)
	if req.Config != nil {
		if host := strings.TrimSpace(req.Config.API.Host); host != "" {
			apiHost = host
		}
		if port := strings.TrimSpace(req.Config.API.Port); port != "" {
			apiPort = port
		}
	}

	var configClone *vmconfig.Config
	if req.Config != nil {
		clone := req.Config.Clone()
		clone.Plugin = pluginName
		clone.Runtime = runtimeName
		clone.Resources = vmconfig.Resources{CPUCores: cpu, MemoryMB: mem}
		clone.KernelCmdline = kernelExtra
		clone.API = vmconfig.API{Host: apiHost, Port: apiPort}
		if clone.Manifest == nil {
			manifestForConfig := manifestCopy
			clone.Manifest = &manifestForConfig
		} else {
			manifestForConfig := *clone.Manifest
			manifestForConfig.Normalize()
			clone.Manifest = &manifestForConfig
		}
		configClone = &clone
	}

	vm, err := api.engine.CreateVM(c.Request.Context(), orchestrator.CreateVMRequest{
		Name:              req.Name,
		Plugin:            pluginName,
		Runtime:           runtimeName,
		CPUCores:          cpu,
		MemoryMB:          mem,
		APIHost:           apiHost,
		APIPort:           apiPort,
		KernelCmdlineHint: kernelExtra,
		Manifest:          &manifestCopy,
		Config:            configClone,
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

func (api *apiServer) createDeployment(c *gin.Context) {
	var req createDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	deployment, err := api.engine.CreateDeployment(c.Request.Context(), orchestrator.CreateDeploymentRequest{
		Name:     req.Name,
		Replicas: req.Replicas,
		Config:   req.Config,
	})
	if err != nil {
		api.logger.Error("create deployment", "deployment", req.Name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, deploymentToResponse(*deployment))
}

func (api *apiServer) getVMConfig(c *gin.Context) {
	name := c.Param("name")
	config, err := api.engine.GetVMConfig(c.Request.Context(), name)
	if err != nil {
		api.logger.Error("get vm config", "vm", name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	if config == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "vm config not found"})
		return
	}
	c.JSON(http.StatusOK, config)
}

func (api *apiServer) updateVMConfig(c *gin.Context) {
	name := c.Param("name")
	var patch vmconfig.Patch
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	config, err := api.engine.UpdateVMConfig(c.Request.Context(), name, patch)
	if err != nil {
		api.logger.Error("update vm config", "vm", name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, config)
}

func (api *apiServer) getVMConfigHistory(c *gin.Context) {
	name := c.Param("name")
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil || val < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		limit = val
	}
	entries, err := api.engine.GetVMConfigHistory(c.Request.Context(), name, limit)
	if err != nil {
		api.logger.Error("vm config history", "vm", name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, entries)
}

func (api *apiServer) startVM(c *gin.Context) {
	name := c.Param("name")
	vm, err := api.engine.StartVM(c.Request.Context(), name)
	if err != nil {
		api.logger.Error("start vm", "vm", name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, vmToResponse(vm))
}

func (api *apiServer) stopVM(c *gin.Context) {
	name := c.Param("name")
	vm, err := api.engine.StopVM(c.Request.Context(), name)
	if err != nil {
		api.logger.Error("stop vm", "vm", name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, vmToResponse(vm))
}

func (api *apiServer) restartVM(c *gin.Context) {
	name := c.Param("name")
	vm, err := api.engine.RestartVM(c.Request.Context(), name)
	if err != nil {
		api.logger.Error("restart vm", "vm", name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, vmToResponse(vm))
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

func (api *apiServer) getDeployment(c *gin.Context) {
	name := c.Param("name")
	deployment, err := api.engine.GetDeployment(c.Request.Context(), name)
	if err != nil {
		api.logger.Error("get deployment", "deployment", name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, deploymentToResponse(*deployment))
}

func (api *apiServer) patchDeployment(c *gin.Context) {
	name := c.Param("name")
	var req patchDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Replicas == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "replicas field required"})
		return
	}
	deployment, err := api.engine.ScaleDeployment(c.Request.Context(), name, *req.Replicas)
	if err != nil {
		api.logger.Error("scale deployment", "deployment", name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, deploymentToResponse(*deployment))
}

func (api *apiServer) deleteDeployment(c *gin.Context) {
	name := c.Param("name")
	if err := api.engine.DeleteDeployment(c.Request.Context(), name); err != nil {
		api.logger.Error("delete deployment", "deployment", name, "error", err)
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
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (api *apiServer) systemInfo(c *gin.Context) {
	listenAddr := ""
	advertiseAddr := ""
	hostIP := ""
	if api.engine != nil {
		listenAddr = api.engine.ControlPlaneListenAddr()
		advertiseAddr = api.engine.ControlPlaneAdvertiseAddr()
		hostIP = api.engine.HostIP().String()
	}

	c.JSON(http.StatusOK, gin.H{
		"status":             "ok",
		"api_listen_addr":    listenAddr,
		"api_advertise_addr": advertiseAddr,
		"host_ip":            hostIP,
	})
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
	case "volant.vms.list":
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
	case "volant.vms.create":
		name, ok := req.Params["name"].(string)
		if !ok {
			err = fmt.Errorf("name param required")
		} else {
			runtime := "browser"
			if raw, exists := req.Params["runtime"].(string); exists {
				runtime = strings.TrimSpace(raw)
			}
			manifest, ok := api.plugins.Get(runtime)
			if !ok || !manifest.Enabled {
				err = fmt.Errorf("runtime %s unavailable", runtime)
				break
			}
			manifestCopy := manifest
			hostIP := api.engine.HostIP().String()
			_, portStr, _ := net.SplitHostPort(api.engine.ControlPlaneAdvertiseAddr())
			vm, e := api.engine.CreateVM(ctx, orchestrator.CreateVMRequest{
				Name:     name,
				Plugin:   runtime,
				Runtime:  runtime,
				CPUCores: manifest.Resources.CPUCores,
				MemoryMB: manifest.Resources.MemoryMB,
				Manifest: &manifestCopy,
				APIHost:  hostIP,
				APIPort:  portStr,
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
	case "volant.system.get_capabilities":
		result = map[string]interface{}{
			"capabilities": []map[string]interface{}{
				{
					"name":        "volant.vms.create",
					"description": "Create a new microVM",
					"params": map[string]interface{}{
						"name":      "string (required)",
						"cpu_cores": "int (default 2)",
						"memory_mb": "int (default 2048)",
					},
				},
				{
					"name":        "volant.vms.list",
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

// getVMOpenAPI serves the VM plugin's OpenAPI document.
// Precedence: 1) agent http://<vm.ip>:8080/v1/openapi, 2) manifest.OpenAPI URL, else 404.
func (api *apiServer) getVMOpenAPI(c *gin.Context) {
	name := c.Param("name")
	if strings.TrimSpace(name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "vm name required"})
		return
	}

	// Try agent first if VM is running with an IP
	vm, err := api.engine.GetVM(c.Request.Context(), name)
	if err != nil {
		api.logger.Error("get vm openapi", "vm", name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve vm"})
		return
	}

	// helper to write a raw document
	writeDoc := func(contentType string, data []byte) {
		if contentType == "" {
			contentType = http.DetectContentType(data)
		}
		// Prefer JSON/YAML types when possible
		lower := strings.ToLower(contentType)
		if !strings.Contains(lower, "json") && !strings.Contains(lower, "yaml") && !strings.Contains(lower, "yml") {
			// best-effort guess by content
			ct := "application/json"
			trimmed := strings.TrimSpace(string(data))
			if len(trimmed) > 0 && trimmed[0] != '{' && trimmed[0] != '[' {
				ct = "application/yaml"
			}
			contentType = ct
		}
		c.Header("Content-Type", contentType)
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(data)
	}

	if vm != nil && vm.Status == db.VMStatusRunning && strings.TrimSpace(vm.IPAddress) != "" {
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, api.agentURL(vm, "/v1/openapi"), nil)
		if err == nil {
			resp, err := api.agentClient.Do(req)
			if err == nil && resp != nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					data, _ := io.ReadAll(resp.Body)
					writeDoc(resp.Header.Get("Content-Type"), data)
					return
				}
			}
		}
	}

	// Fallback to manifest.OpenAPI URL from stored VM config
	versioned, err := api.engine.GetVMConfig(c.Request.Context(), name)
	if err != nil {
		api.logger.Error("get vm config for openapi", "vm", name, "error", err)
		c.JSON(statusFromError(err), gin.H{"error": err.Error()})
		return
	}
	if versioned == nil || versioned.Config.Manifest == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "openapi spec unavailable"})
		return
	}
	ref := strings.TrimSpace(versioned.Config.Manifest.OpenAPI)
	if ref == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "openapi spec unavailable"})
		return
	}

	// Support http(s), file://, and absolute path
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, ref, nil)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch manifest openapi"})
			return
		}
		resp, err := api.agentClient.Do(req)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch manifest openapi"})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("manifest openapi returned %d", resp.StatusCode)})
			return
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read manifest openapi"})
			return
		}
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			lower := strings.ToLower(ref)
			switch {
			case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
				ct = "application/yaml"
			default:
				ct = "application/json"
			}
		}
		writeDoc(ct, data)
		return
	}

	// file:// or absolute path
	path := ref
	if strings.HasPrefix(ref, "file://") {
		if u, err := url.Parse(ref); err == nil {
			path = u.Path
		}
	}
	if !strings.HasPrefix(path, "/") {
		// not a supported scheme/path
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid openapi reference in manifest"})
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "openapi file not found"})
		return
	}
	ct := "application/json"
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
		ct = "application/yaml"
	}
	writeDoc(ct, data)
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
	case errors.Is(err, orchestrator.ErrDeploymentNotFound):
		return http.StatusNotFound
	case errors.Is(err, orchestrator.ErrDeploymentExists):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func (api *apiServer) postVMPluginAction(c *gin.Context) {
	vmName := c.Param("name")
	api.dispatchPluginAction(c, vmName)
}

func (api *apiServer) dispatchPluginAction(c *gin.Context, vmName string) {
	if api.plugins == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin registry unavailable"})
		return
	}

	pluginName := c.Param("plugin")
	actionName := c.Param("action")
	manifest, action, err := api.plugins.ResolveAction(pluginName, actionName)
	if err != nil {
		api.logger.Error("resolve plugin action", "plugin", pluginName, "action", actionName, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var vm *db.VM
	if vmName != "" {
		var ok bool
		vm, ok = api.resolveVMByName(c, vmName)
		if !ok {
			return
		}
		if manifest.Runtime != vm.Runtime {
			c.JSON(http.StatusConflict, gin.H{"error": "vm runtime does not match plugin"})
			return
		}
	}

	targetPath := action.Path
	if !strings.HasPrefix(targetPath, "/") {
		targetPath = "/" + targetPath
	}

	method := action.Method
	if method == "" {
		method = http.MethodPost
	}

	var respBody map[string]any
	if vm != nil {
		if err := api.agentAction(c, vm, method, targetPath, payload, &respBody); err != nil {
			return
		}
	} else {
		resp, err := api.forwardPluginAction(c.Request.Context(), manifest, method, targetPath, payload)
		if err != nil {
			api.logger.Error("plugin action forward", "plugin", pluginName, "action", actionName, "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		respBody = resp
	}

	if respBody == nil {
		c.Status(http.StatusAccepted)
		return
	}
	c.JSON(http.StatusOK, respBody)
}

func (api *apiServer) resolveVMByName(c *gin.Context, name string) (*db.VM, bool) {
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

func (api *apiServer) forwardPluginAction(ctx context.Context, manifest pluginspec.Manifest, method, path string, payload map[string]any) (map[string]any, error) {
	// placeholder for future non-VM plugin action dispatch (e.g. pooled runtimes)
	return map[string]any{"status": "accepted"}, nil
}

func (api *apiServer) listPlugins(c *gin.Context) {
	if api.plugins == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin registry unavailable"})
		return
	}

	names := api.plugins.List()
	c.JSON(http.StatusOK, gin.H{"plugins": names})
}

func (api *apiServer) describePlugin(c *gin.Context) {
	if api.plugins == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin registry unavailable"})
		return
	}

	pluginName := c.Param("plugin")
	manifest, ok := api.plugins.Get(pluginName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
		return
	}
	c.JSON(http.StatusOK, manifest)
}

func (api *apiServer) postPluginAction(c *gin.Context) {
	api.dispatchPluginAction(c, "")
}

func (api *apiServer) installPlugin(c *gin.Context) {
	if api.plugins == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin registry unavailable"})
		return
	}

	var manifest pluginspec.Manifest
	if err := c.ShouldBindJSON(&manifest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	manifest.Normalize()
	if err := manifest.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := api.persistPluginManifest(c.Request.Context(), manifest, true); err != nil {
		api.logger.Error("install plugin", "plugin", manifest.Name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	api.plugins.Register(manifest)
	c.Status(http.StatusCreated)
}

func (api *apiServer) removePlugin(c *gin.Context) {
	name := c.Param("plugin")
	if strings.TrimSpace(name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plugin name required"})
		return
	}

	if err := api.deletePluginManifest(c.Request.Context(), name); err != nil {
		api.logger.Error("remove plugin", "plugin", name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

func (api *apiServer) setPluginEnabled(c *gin.Context) {
	if api.plugins == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin registry unavailable"})
		return
	}

	name := c.Param("plugin")
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := api.togglePlugin(c.Request.Context(), name, payload.Enabled); err != nil {
		api.logger.Error("toggle plugin", "plugin", name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

func (api *apiServer) persistPluginManifest(ctx context.Context, manifest pluginspec.Manifest, enabled bool) error {
	store := api.engine.Store()
	if store == nil {
		return fmt.Errorf("store not configured")
	}

	return store.WithTx(ctx, func(q db.Queries) error {
		data, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		return q.Plugins().Upsert(ctx, db.Plugin{
			Name:     manifest.Name,
			Version:  manifest.Version,
			Enabled:  enabled,
			Metadata: data,
		})
	})
}

func (api *apiServer) deletePluginManifest(ctx context.Context, name string) error {
	store := api.engine.Store()
	if store == nil {
		return fmt.Errorf("store not configured")
	}

	return store.WithTx(ctx, func(q db.Queries) error {
		return q.Plugins().Delete(ctx, name)
	})
}

func (api *apiServer) togglePlugin(ctx context.Context, name string, enabled bool) error {
	store := api.engine.Store()
	if store == nil {
		return fmt.Errorf("store not configured")
	}

	return store.WithTx(ctx, func(q db.Queries) error {
		if err := q.Plugins().SetEnabled(ctx, name, enabled); err != nil {
			return err
		}

		manifest, ok := api.plugins.Get(name)
		if !ok {
			return nil
		}

		manifest.Enabled = enabled
		if enabled {
			api.plugins.Register(manifest)
		}
		return nil
	})
}

func cloneLabelMap(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	dup := make(map[string]string, len(labels))
	for key, value := range labels {
		dup[key] = value
	}
	return dup
}

func (api *apiServer) mountManifestRoutes(router *gin.RouterGroup, vm *db.VM, manifest pluginspec.Manifest) {
	for _, action := range manifest.Actions {
		method := strings.ToUpper(strings.TrimSpace(action.Method))
		if method == "" {
			method = http.MethodPost
		}
		path := strings.TrimSpace(action.Path)
		if path == "" {
			continue
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		actionPath := path
		actionMethod := method

		router.Handle(actionMethod, actionPath, func(c *gin.Context) {
			var payload map[string]any
			if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if err := api.agentAction(c, vm, actionMethod, actionPath, payload, nil); err != nil {
				return
			}
			c.Status(http.StatusAccepted)
		})
	}
}

func (api *apiServer) handleManifestAction(ctx context.Context, w http.ResponseWriter, req *http.Request, vm *db.VM, manifest pluginspec.Manifest, actionName string, action pluginspec.Action) {
	// Placeholder: full implementation forthcoming
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write([]byte("manifest action proxy not implemented"))
}

func (api *apiServer) getPluginManifest(c *gin.Context) {
	if api.plugins == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin registry unavailable"})
		return
	}

	pluginName := c.Param("plugin")
	manifest, ok := api.plugins.Get(pluginName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
		return
	}
	if !manifest.Enabled {
		c.JSON(http.StatusConflict, gin.H{"error": "plugin disabled"})
		return
	}
	c.JSON(http.StatusOK, manifest)
}

// Plugin Artifacts API
// GET /api/v1/plugins/:plugin/artifacts?version=...
func (api *apiServer) listPluginArtifacts(c *gin.Context) {
	plugin := strings.TrimSpace(c.Param("plugin"))
	if plugin == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plugin name required"})
		return
	}
	store := api.engine.Store()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store not configured"})
		return
	}

	version := strings.TrimSpace(c.Query("version"))
	var (
		result []db.PluginArtifact
		err    error
	)
	if version != "" {
		result, err = store.Queries().PluginArtifacts().ListByPluginVersion(c.Request.Context(), plugin, version)
	} else {
		result, err = store.Queries().PluginArtifacts().ListByPlugin(c.Request.Context(), plugin)
	}
	if err != nil {
		api.logger.Error("list plugin artifacts", "plugin", plugin, "version", version, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list artifacts"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GET /api/v1/plugins/:plugin/artifacts/:artifact?version=...
func (api *apiServer) getPluginArtifact(c *gin.Context) {
	plugin := strings.TrimSpace(c.Param("plugin"))
	artifact := strings.TrimSpace(c.Param("artifact"))
	version := strings.TrimSpace(c.Query("version"))
	if plugin == "" || artifact == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plugin and artifact required"})
		return
	}
	if version == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version query required"})
		return
	}
	store := api.engine.Store()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store not configured"})
		return
	}
	rec, err := store.Queries().PluginArtifacts().Get(c.Request.Context(), plugin, version, artifact)
	if err != nil {
		api.logger.Error("get plugin artifact", "plugin", plugin, "artifact", artifact, "version", version, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get artifact"})
		return
	}
	if rec == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}
	c.JSON(http.StatusOK, rec)
}

type upsertArtifactRequest struct {
	Version      string `json:"version" binding:"required"`
	ArtifactName string `json:"artifact_name" binding:"required"`
	Kind         string `json:"kind"`
	SourceURL    string `json:"source_url"`
	Checksum     string `json:"checksum"`
	Format       string `json:"format"`
	LocalPath    string `json:"local_path"`
	SizeBytes    int64  `json:"size_bytes"`
}

// POST /api/v1/plugins/:plugin/artifacts
func (api *apiServer) upsertPluginArtifact(c *gin.Context) {
	plugin := strings.TrimSpace(c.Param("plugin"))
	if plugin == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plugin name required"})
		return
	}
	var req upsertArtifactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	art := db.PluginArtifact{
		PluginName:   plugin,
		Version:      strings.TrimSpace(req.Version),
		ArtifactName: strings.TrimSpace(req.ArtifactName),
		Kind:         strings.TrimSpace(req.Kind),
		SourceURL:    strings.TrimSpace(req.SourceURL),
		Checksum:     strings.TrimSpace(req.Checksum),
		Format:       strings.TrimSpace(req.Format),
		LocalPath:    strings.TrimSpace(req.LocalPath),
		SizeBytes:    req.SizeBytes,
	}
	if art.Version == "" || art.ArtifactName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version and artifact_name required"})
		return
	}
	store := api.engine.Store()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store not configured"})
		return
	}
	if err := store.WithTx(c.Request.Context(), func(q db.Queries) error {
		return q.PluginArtifacts().Upsert(c.Request.Context(), art)
	}); err != nil {
		api.logger.Error("upsert plugin artifact", "plugin", plugin, "artifact", art.ArtifactName, "version", art.Version, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upsert artifact"})
		return
	}
	c.Status(http.StatusCreated)
}

// DELETE /api/v1/plugins/:plugin/artifacts?version=...
func (api *apiServer) deletePluginArtifacts(c *gin.Context) {
	plugin := strings.TrimSpace(c.Param("plugin"))
	if plugin == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plugin name required"})
		return
	}
	version := strings.TrimSpace(c.Query("version"))
	store := api.engine.Store()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store not configured"})
		return
	}
	var err error
	if version != "" {
		err = store.WithTx(c.Request.Context(), func(q db.Queries) error {
			return q.PluginArtifacts().DeleteByPluginVersion(c.Request.Context(), plugin, version)
		})
	} else {
		err = store.WithTx(c.Request.Context(), func(q db.Queries) error {
			return q.PluginArtifacts().DeleteByPlugin(c.Request.Context(), plugin)
		})
	}
	if err != nil {
		api.logger.Error("delete plugin artifacts", "plugin", plugin, "version", version, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete artifacts"})
		return
	}
	c.Status(http.StatusNoContent)
}

// /ws/v1/vms/:name/console -> bridge to VM serial socket
func (api *apiServer) vmConsoleWebSocket(c *gin.Context) {
	vm, ok := api.resolveVM(c)
	if !ok {
		return
	}
	serial := strings.TrimSpace(vm.SerialSocket)
	if serial == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "serial socket unavailable"})
		return
	}

	// Establish Unix domain socket connection to serial
	unixConn, err := net.Dial("unix", serial)
	if err != nil {
		api.logger.Error("console dial unix", "vm", vm.Name, "socket", serial, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "serial socket unavailable"})
		return
	}
	defer unixConn.Close()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		api.logger.Error("console ws upgrade", "vm", vm.Name, "error", err)
		return
	}
	defer wsConn.Close()

	ctx := c.Request.Context()
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	// Pipe: VM serial -> WS (binary frames)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := unixConn.Read(buf)
			if n > 0 {
				if writeErr := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					errCh <- writeErr
					return
				}
			}
			if readErr != nil {
				errCh <- readErr
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	// Pipe: WS -> VM serial
	go func() {
		defer wg.Done()
		for {
			msgType, payload, readErr := wsConn.ReadMessage()
			if readErr != nil {
				errCh <- readErr
				return
			}
			// Accept both text and binary frames
			_ = msgType
			if _, writeErr := unixConn.Write(payload); writeErr != nil {
				errCh <- writeErr
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	var bridgeErr error
	select {
	case <-ctx.Done():
		bridgeErr = ctx.Err()
	case bridgeErr = <-errCh:
	}

	_ = wsConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	wg.Wait()

	if bridgeErr != nil && !errors.Is(bridgeErr, net.ErrClosed) && !errors.Is(bridgeErr, io.EOF) && !websocket.IsCloseError(bridgeErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		api.logger.Debug("console bridge closed", "vm", vm.Name, "error", bridgeErr)
	}
}

// VFIO Device Management Handlers

func (api *apiServer) getVFIODeviceInfo(c *gin.Context) {
	var req vfioDeviceInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get VFIOManager from the engine
	vfioMgr := devicemanager.NewVFIOManager(api.logger)

	deviceInfo, err := vfioMgr.GetDeviceInfo(req.PCIAddress)
	if err != nil {
		api.logger.Error("failed to get device info", "pci_address", req.PCIAddress, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := vfioDeviceInfoResponse{
		Address:    deviceInfo.Address,
		Vendor:     deviceInfo.Vendor,
		Device:     deviceInfo.Device,
		Driver:     deviceInfo.Driver,
		IOMMUGroup: deviceInfo.IOMMUGroup,
		NumaNode:   deviceInfo.NumaNode,
	}

	c.JSON(http.StatusOK, resp)
}

func (api *apiServer) validateVFIODevices(c *gin.Context) {
	var req vfioValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vfioMgr := devicemanager.NewVFIOManager(api.logger)

	err := vfioMgr.ValidateDevices(req.PCIAddresses, req.Allowlist)
	if err != nil {
		api.logger.Warn("device validation failed", "devices", req.PCIAddresses, "error", err)
		c.JSON(http.StatusOK, vfioValidateResponse{
			Valid:   false,
			Message: err.Error(),
			Errors:  []string{err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, vfioValidateResponse{
		Valid:   true,
		Message: "All devices are valid and available for passthrough",
	})
}

func (api *apiServer) checkVFIOIOMMUGroups(c *gin.Context) {
	var req vfioValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vfioMgr := devicemanager.NewVFIOManager(api.logger)

	groups, err := vfioMgr.CheckIOMMUGroups(req.PCIAddresses)
	if err != nil {
		api.logger.Error("failed to check IOMMU groups", "devices", req.PCIAddresses, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var response []vfioIOMMUGroupResponse
	for _, group := range groups {
		response = append(response, vfioIOMMUGroupResponse{
			ID:      group.ID,
			Devices: group.Devices,
		})
	}

	c.JSON(http.StatusOK, response)
}

func (api *apiServer) bindVFIODevices(c *gin.Context) {
	var req vfioBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vfioMgr := devicemanager.NewVFIOManager(api.logger)

	err := vfioMgr.BindDevices(req.PCIAddresses)
	if err != nil {
		api.logger.Error("failed to bind devices", "devices", req.PCIAddresses, "error", err)
		c.JSON(http.StatusInternalServerError, vfioBindResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	api.logger.Info("successfully bound devices to vfio-pci", "devices", req.PCIAddresses)
	c.JSON(http.StatusOK, vfioBindResponse{
		Success:      true,
		Message:      "Devices successfully bound to vfio-pci driver",
		BoundDevices: req.PCIAddresses,
	})
}

func (api *apiServer) unbindVFIODevices(c *gin.Context) {
	var req vfioUnbindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vfioMgr := devicemanager.NewVFIOManager(api.logger)

	err := vfioMgr.UnbindDevices(req.PCIAddresses)
	if err != nil {
		api.logger.Error("failed to unbind devices", "devices", req.PCIAddresses, "error", err)
		c.JSON(http.StatusInternalServerError, vfioUnbindResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	api.logger.Info("successfully unbound devices from vfio-pci", "devices", req.PCIAddresses)
	c.JSON(http.StatusOK, vfioUnbindResponse{
		Success:        true,
		Message:        "Devices successfully unbound from vfio-pci driver",
		UnboundDevices: req.PCIAddresses,
	})
}

func (api *apiServer) getVFIOGroupPaths(c *gin.Context) {
	var req vfioGroupPathsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vfioMgr := devicemanager.NewVFIOManager(api.logger)

	groupPaths, err := vfioMgr.GetVFIOGroupPaths(req.PCIAddresses)
	if err != nil {
		api.logger.Error("failed to get VFIO group paths", "devices", req.PCIAddresses, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, vfioGroupPathsResponse{
		GroupPaths: groupPaths,
	})
}
