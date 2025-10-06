// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	openapi3 "github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"

	"github.com/volantvm/volant/internal/server/db"
	orchestratorevents "github.com/volantvm/volant/internal/server/orchestrator/events"
)

// serveOpenAPI returns an OpenAPI v3 JSON document generated from server types.
func (api *apiServer) serveOpenAPI(w http.ResponseWriter, r *http.Request) {
	baseURL := ""
	if r != nil && r.Host != "" {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}

	spec, err := BuildOpenAPISpec(baseURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build openapi: %v", err), http.StatusInternalServerError)
		return
	}
	data, err := json.Marshal(spec)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to marshal openapi: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// BuildOpenAPISpec constructs the OpenAPI spec. If baseURL is non-empty, it will be set as the server URL.
func BuildOpenAPISpec(baseURL string) (*openapi3.T, error) {
	// Initialize spec
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:       "Volant REST API",
			Version:     "v1",
			Description: "REST interface for the Volant microVM orchestration engine.",
		},
		Servers:    openapi3.Servers{},
		Paths:      openapi3.NewPaths(),
		Components: &openapi3.Components{Schemas: openapi3.Schemas{}},
	}
	if baseURL != "" {
		spec.Servers = append(spec.Servers, &openapi3.Server{URL: baseURL})
	}

	gen := openapi3gen.NewGenerator(
		openapi3gen.CreateComponentSchemas(openapi3gen.ExportComponentSchemasOptions{
			ExportComponentSchemas: true,
			ExportTopLevelSchema:   false,
			ExportGenerics:         true,
		}),
	)

	// Register common schemas from our request/response types
	// VM and deployment types
	vmRespRef, _ := gen.NewSchemaRefForValue(&vmResponse{}, spec.Components.Schemas)
	createVMReqRef, _ := gen.NewSchemaRefForValue(&createVMRequest{}, spec.Components.Schemas)
	sysStatusRef, _ := gen.NewSchemaRefForValue(&SystemStatusResponse{}, spec.Components.Schemas)
	mcpReqRef, _ := gen.NewSchemaRefForValue(&MCPRequest{}, spec.Components.Schemas)
	mcpRespRef, _ := gen.NewSchemaRefForValue(&MCPResponse{}, spec.Components.Schemas)
	// Phase 3 additions
	sysSummaryRef, _ := gen.NewSchemaRefForValue(&systemSummaryResponse{}, spec.Components.Schemas)
	pluginArtifactRef, _ := gen.NewSchemaRefForValue(&db.PluginArtifact{}, spec.Components.Schemas)
	upsertArtifactReqRef, _ := gen.NewSchemaRefForValue(&upsertArtifactRequest{}, spec.Components.Schemas)
	// Events
	vmEventRef, _ := gen.NewSchemaRefForValue(&orchestratorevents.VMEvent{}, spec.Components.Schemas)

	// Helper: standard error schema
	errorSchema := openapi3.NewSchemaRef("", &openapi3.Schema{
		Type: &openapi3.Types{openapi3.TypeObject},
		Properties: map[string]*openapi3.SchemaRef{
			"error": openapi3.NewSchemaRef("", openapi3.NewStringSchema()),
			"details": openapi3.NewSchemaRef("", &openapi3.Schema{
				Type:                 &openapi3.Types{openapi3.TypeObject},
				AdditionalProperties: openapi3.AdditionalProperties{Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())},
			}),
		},
	})
	spec.Components.Schemas["Error"] = errorSchema

	// /healthz
	spec.AddOperation("/healthz", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Health check"
		op.OperationID = "getHealth"
		op.Tags = []string{"health"}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Service is healthy")
			schema := openapi3.NewObjectSchema()
			schema.Properties = map[string]*openapi3.SchemaRef{
				"status": openapi3.NewSchemaRef("", openapi3.NewStringSchema()),
			}
			resp.Content = openapi3.NewContentWithJSONSchema(schema)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/system/status
	spec.AddOperation("/api/v1/system/status", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "System status summary"
		op.OperationID = "getSystemStatus"
		op.Tags = []string{"status"}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Current VM count and resource usage")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(sysStatusRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/system/summary
	spec.AddOperation("/api/v1/system/summary", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "System summary for dashboards"
		op.OperationID = "getSystemSummary"
		op.Tags = []string{"status"}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Counts and plugin info")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(sysSummaryRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vms
	spec.AddOperation("/api/v1/vms", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "List VMs"
		op.OperationID = "listVMs"
		op.Tags = []string{"vm"}
		// Query parameters
		op.Parameters = append(op.Parameters,
			&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "status", In: openapi3.ParameterInQuery, Description: "Filter by status (repeatable or comma-separated)", Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}},
			&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "runtime", In: openapi3.ParameterInQuery, Description: "Filter by runtime", Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}},
			&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "plugin", In: openapi3.ParameterInQuery, Description: "Filter by plugin name", Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}},
			&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "q", In: openapi3.ParameterInQuery, Description: "Free text search (name, ip, runtime)", Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}},
			&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "limit", In: openapi3.ParameterInQuery, Description: "Max items to return", Schema: openapi3.NewSchemaRef("", openapi3.NewIntegerSchema())}},
			&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "offset", In: openapi3.ParameterInQuery, Description: "Items to skip (for pagination)", Schema: openapi3.NewSchemaRef("", openapi3.NewIntegerSchema())}},
			&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "sort", In: openapi3.ParameterInQuery, Description: "Sort field (name,status,runtime,created_at,updated_at)", Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}},
			&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "order", In: openapi3.ParameterInQuery, Description: "Sort order (asc,desc)", Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}},
		)
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Array of VMs")
			arr := &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}, Items: vmRespRef}
			resp.Content = openapi3.NewContentWithJSONSchema(arr)
			// X-Total-Count header
			if resp.Headers == nil {
				resp.Headers = openapi3.Headers{}
			}
			resp.Headers["X-Total-Count"] = &openapi3.HeaderRef{Value: &openapi3.Header{Parameter: openapi3.Parameter{Schema: openapi3.NewSchemaRef("", openapi3.NewIntegerSchema())}}}
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())
	spec.AddOperation("/api/v1/vms", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Create VM"
		op.OperationID = "createVM"
		op.Tags = []string{"vm"}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchemaRef(createVMReqRef)}}
		op.Responses = openapi3.NewResponses()
		// 201
		{
			resp := openapi3.NewResponse().WithDescription("VM created")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(vmRespRef)
			op.Responses.Set("201", &openapi3.ResponseRef{Value: resp})
		}
		// 400
		{
			resp := openapi3.NewResponse().WithDescription("Bad request")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("400", &openapi3.ResponseRef{Value: resp})
		}
		// 409
		{
			resp := openapi3.NewResponse().WithDescription("Conflict")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("409", &openapi3.ResponseRef{Value: resp})
		}
		// 500
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vms/{name}
	nameParam := &openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "name", In: openapi3.ParameterInPath, Required: true, Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}}
	spec.AddOperation("/api/v1/vms/{name}", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Fetch VM by name"
		op.OperationID = "getVMByName"
		op.Tags = []string{"vm"}
		op.Parameters = openapi3.Parameters{nameParam}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("VM")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(vmRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Not found")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("404", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())
	spec.AddOperation("/api/v1/vms/{name}", http.MethodDelete, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Destroy VM"
		op.OperationID = "destroyVM"
		op.Tags = []string{"vm"}
		op.Parameters = openapi3.Parameters{nameParam}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("204", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Deleted")})
		op.Responses.Set("404", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Not found")})
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/events/vms (SSE)
	spec.AddOperation("/api/v1/events/vms", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Stream VM lifecycle events (SSE)"
		op.OperationID = "streamVMEvents"
		op.Tags = []string{"events"}
		op.Responses = openapi3.NewResponses()
		{
			desc := "SSE stream of VM events"
			resp := &openapi3.Response{Description: &desc, Content: openapi3.Content{"text/event-stream": {Schema: vmEventRef}}}
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/plugins
	pluginListSchema := openapi3.NewSchemaRef("", func() *openapi3.Schema {
		s := openapi3.NewObjectSchema()
		s.Properties = map[string]*openapi3.SchemaRef{
			"plugins": openapi3.NewSchemaRef("", &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}, Items: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}),
		}
		return s
	}())
	spec.AddOperation("/api/v1/plugins", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "List plugin manifests"
		op.OperationID = "listPlugins"
		op.Tags = []string{"plugins"}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Plugin identifiers")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(pluginListSchema)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/mcp
	spec.AddOperation("/api/v1/mcp", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Model Context Protocol endpoint"
		op.OperationID = "postMCPCommand"
		op.Tags = []string{"mcp"}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchemaRef(mcpReqRef)}}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("MCP response")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(mcpRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Bad request")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("400", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vms/{name}/start
	spec.AddOperation("/api/v1/vms/{name}/start", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Start a stopped VM"
		op.OperationID = "startVM"
		op.Tags = []string{"vm"}
		op.Parameters = openapi3.Parameters{nameParam}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("VM started")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(vmRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Not found")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("404", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vms/{name}/stop
	spec.AddOperation("/api/v1/vms/{name}/stop", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Stop a running VM"
		op.OperationID = "stopVM"
		op.Tags = []string{"vm"}
		op.Parameters = openapi3.Parameters{nameParam}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("VM stopped")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(vmRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Not found")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("404", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vms/{name}/restart
	spec.AddOperation("/api/v1/vms/{name}/restart", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Restart a VM"
		op.OperationID = "restartVM"
		op.Tags = []string{"vm"}
		op.Parameters = openapi3.Parameters{nameParam}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("VM restarted")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(vmRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Not found")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("404", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vms/{name}/config
	spec.AddOperation("/api/v1/vms/{name}/config", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Get VM configuration"
		op.OperationID = "getVMConfig"
		op.Tags = []string{"vm", "config"}
		op.Parameters = openapi3.Parameters{nameParam}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("VM configuration")
			schema := openapi3.NewObjectSchema()
			resp.Content = openapi3.NewContentWithJSONSchema(schema)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Not found")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("404", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	spec.AddOperation("/api/v1/vms/{name}/config", http.MethodPatch, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Update VM configuration"
		op.OperationID = "updateVMConfig"
		op.Tags = []string{"vm", "config"}
		op.Parameters = openapi3.Parameters{nameParam}
		schema := openapi3.NewObjectSchema()
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchema(schema)}}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Configuration updated")
			resp.Content = openapi3.NewContentWithJSONSchema(schema)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Bad request")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("400", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vms/{name}/config/history
	spec.AddOperation("/api/v1/vms/{name}/config/history", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Get VM configuration history"
		op.OperationID = "getVMConfigHistory"
		op.Tags = []string{"vm", "config"}
		op.Parameters = openapi3.Parameters{nameParam}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Configuration history")
			schema := &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}, Items: openapi3.NewSchemaRef("", openapi3.NewObjectSchema())}
			resp.Content = openapi3.NewContentWithJSONSchema(schema)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vms/{name}/openapi
	spec.AddOperation("/api/v1/vms/{name}/openapi", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Get VM plugin OpenAPI spec"
		op.OperationID = "getVMOpenAPI"
		op.Tags = []string{"vm"}
		op.Parameters = openapi3.Parameters{nameParam}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("OpenAPI specification")
			schema := openapi3.NewObjectSchema()
			resp.Content = openapi3.NewContentWithJSONSchema(schema)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/deployments
	deploymentRespRef, _ := gen.NewSchemaRefForValue(&deploymentResponse{}, spec.Components.Schemas)
	deploymentReqRef, _ := gen.NewSchemaRefForValue(&createDeploymentRequest{}, spec.Components.Schemas)

	spec.AddOperation("/api/v1/deployments", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "List deployments"
		op.OperationID = "listDeployments"
		op.Tags = []string{"deployment"}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Array of deployments")
			arr := &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}, Items: deploymentRespRef}
			resp.Content = openapi3.NewContentWithJSONSchema(arr)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	spec.AddOperation("/api/v1/deployments", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Create deployment"
		op.OperationID = "createDeployment"
		op.Tags = []string{"deployment"}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchemaRef(deploymentReqRef)}}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Deployment created")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(deploymentRespRef)
			op.Responses.Set("201", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	spec.AddOperation("/api/v1/deployments/{name}", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Get deployment"
		op.OperationID = "getDeployment"
		op.Tags = []string{"deployment"}
		op.Parameters = openapi3.Parameters{nameParam}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Deployment details")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(deploymentRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	spec.AddOperation("/api/v1/deployments/{name}", http.MethodPatch, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Scale deployment"
		op.OperationID = "scaleDeployment"
		op.Tags = []string{"deployment"}
		op.Parameters = openapi3.Parameters{nameParam}
		patchSchema := openapi3.NewObjectSchema()
		patchSchema.Properties = map[string]*openapi3.SchemaRef{
			"replicas": openapi3.NewSchemaRef("", openapi3.NewIntegerSchema()),
		}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchema(patchSchema)}}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Deployment scaled")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(deploymentRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	spec.AddOperation("/api/v1/deployments/{name}", http.MethodDelete, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Delete deployment"
		op.OperationID = "deleteDeployment"
		op.Tags = []string{"deployment"}
		op.Parameters = openapi3.Parameters{nameParam}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("204", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Deleted")})
		return op
	}())

	// /api/v1/plugins
	manifestSchema := openapi3.NewObjectSchema()
	manifestSchema.Description = "Plugin manifest (see plugin-manifest-v1.json schema)"
	spec.AddOperation("/api/v1/plugins", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "List plugins"
		op.OperationID = "listPlugins"
		op.Tags = []string{"plugins"}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("List of plugin names")
			listSchema := openapi3.NewObjectSchema()
			listSchema.Properties = map[string]*openapi3.SchemaRef{
				"plugins": openapi3.NewSchemaRef("", &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}, Items: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}),
			}
			resp.Content = openapi3.NewContentWithJSONSchema(listSchema)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	spec.AddOperation("/api/v1/plugins", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Install plugin"
		op.OperationID = "installPlugin"
		op.Tags = []string{"plugins"}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchema(manifestSchema)}}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("201", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Plugin installed")})
		op.Responses.Set("400", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Bad request").WithContent(openapi3.NewContentWithJSONSchemaRef(errorSchema))})
		return op
	}())

	pluginParam := &openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "plugin", In: openapi3.ParameterInPath, Required: true, Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}}
	spec.AddOperation("/api/v1/plugins/{plugin}", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Get plugin"
		op.OperationID = "getPlugin"
		op.Tags = []string{"plugins"}
		op.Parameters = openapi3.Parameters{pluginParam}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Plugin manifest")
			resp.Content = openapi3.NewContentWithJSONSchema(manifestSchema)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	spec.AddOperation("/api/v1/plugins/{plugin}", http.MethodDelete, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Remove plugin"
		op.OperationID = "removePlugin"
		op.Tags = []string{"plugins"}
		op.Parameters = openapi3.Parameters{pluginParam}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("204", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Plugin removed")})
		return op
	}())

	spec.AddOperation("/api/v1/plugins/{plugin}/enabled", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Set plugin enabled status"
		op.OperationID = "setPluginEnabled"
		op.Tags = []string{"plugins"}
		op.Parameters = openapi3.Parameters{pluginParam}
		enabledSchema := openapi3.NewObjectSchema()
		enabledSchema.Properties = map[string]*openapi3.SchemaRef{
			"enabled": openapi3.NewSchemaRef("", openapi3.NewBoolSchema()),
		}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchema(enabledSchema)}}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("200", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Status updated")})
		return op
	}())

	// Plugin artifacts
	spec.AddOperation("/api/v1/plugins/{plugin}/artifacts", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "List plugin artifacts"
		op.OperationID = "listPluginArtifacts"
		op.Tags = []string{"plugins", "artifacts"}
		op.Parameters = openapi3.Parameters{
			pluginParam,
			&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "version", In: openapi3.ParameterInQuery, Description: "Filter by version", Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}},
		}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Artifacts list")
			arr := &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}, Items: pluginArtifactRef}
			resp.Content = openapi3.NewContentWithJSONSchema(arr)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	spec.AddOperation("/api/v1/plugins/{plugin}/artifacts", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Create or update a plugin artifact"
		op.OperationID = "upsertPluginArtifact"
		op.Tags = []string{"plugins", "artifacts"}
		op.Parameters = openapi3.Parameters{pluginParam}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchemaRef(upsertArtifactReqRef)}}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("201", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Created")})
		op.Responses.Set("400", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Bad request").WithContent(openapi3.NewContentWithJSONSchemaRef(errorSchema))})
		return op
	}())

	spec.AddOperation("/api/v1/plugins/{plugin}/artifacts", http.MethodDelete, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Delete plugin artifacts"
		op.OperationID = "deletePluginArtifacts"
		op.Tags = []string{"plugins", "artifacts"}
		op.Parameters = openapi3.Parameters{
			pluginParam,
			&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "version", In: openapi3.ParameterInQuery, Description: "Delete artifacts for a specific version (optional)", Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}},
		}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("204", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Deleted")})
		return op
	}())

	artifactParam := &openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "artifact", In: openapi3.ParameterInPath, Required: true, Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}}
	spec.AddOperation("/api/v1/plugins/{plugin}/artifacts/{artifact}", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Get a plugin artifact by name and version"
		op.OperationID = "getPluginArtifact"
		op.Tags = []string{"plugins", "artifacts"}
		op.Parameters = openapi3.Parameters{
			pluginParam,
			artifactParam,
			&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "version", In: openapi3.ParameterInQuery, Required: true, Description: "Artifact version", Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())}},
		}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Artifact")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(pluginArtifactRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		op.Responses.Set("404", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Not found").WithContent(openapi3.NewContentWithJSONSchemaRef(errorSchema))})
		return op
	}())

	// WebSocket console endpoint (documented as HTTP GET upgrade)
	spec.AddOperation("/ws/v1/vms/{name}/console", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "WebSocket console for VM"
		op.Description = "Upgrades to a WebSocket connection streaming the VM serial console"
		op.OperationID = "vmConsoleWebSocket"
		op.Tags = []string{"vm", "console"}
		op.Parameters = openapi3.Parameters{nameParam}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("101", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Switching Protocols (WebSocket)")})
		op.Responses.Set("200", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("OK (non-upgrade)")})
		return op
	}())

	// /api/v1/system/info
	spec.AddOperation("/api/v1/system/info", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Get system information"
		op.OperationID = "getSystemInfo"
		op.Tags = []string{"system"}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("System information")
			schema := openapi3.NewObjectSchema()
			resp.Content = openapi3.NewContentWithJSONSchema(schema)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/events/vms
	spec.AddOperation("/api/v1/events/vms", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Stream VM lifecycle events"
		op.Description = "Server-Sent Events (SSE) stream of VM lifecycle events"
		op.OperationID = "streamVMEvents"
		op.Tags = []string{"events"}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Event stream (text/event-stream)")
			resp.Content = openapi3.NewContent()
			resp.Content["text/event-stream"] = &openapi3.MediaType{}
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// Register VFIO request/response schemas
	vfioDeviceInfoReqRef, _ := gen.NewSchemaRefForValue(&vfioDeviceInfoRequest{}, spec.Components.Schemas)
	vfioDeviceInfoRespRef, _ := gen.NewSchemaRefForValue(&vfioDeviceInfoResponse{}, spec.Components.Schemas)
	vfioValidateReqRef, _ := gen.NewSchemaRefForValue(&vfioValidateRequest{}, spec.Components.Schemas)
	vfioValidateRespRef, _ := gen.NewSchemaRefForValue(&vfioValidateResponse{}, spec.Components.Schemas)
	vfioIOMMUGroupRespRef, _ := gen.NewSchemaRefForValue(&vfioIOMMUGroupResponse{}, spec.Components.Schemas)
	vfioBindReqRef, _ := gen.NewSchemaRefForValue(&vfioBindRequest{}, spec.Components.Schemas)
	vfioBindRespRef, _ := gen.NewSchemaRefForValue(&vfioBindResponse{}, spec.Components.Schemas)
	vfioUnbindReqRef, _ := gen.NewSchemaRefForValue(&vfioUnbindRequest{}, spec.Components.Schemas)
	vfioUnbindRespRef, _ := gen.NewSchemaRefForValue(&vfioUnbindResponse{}, spec.Components.Schemas)
	vfioGroupPathsReqRef, _ := gen.NewSchemaRefForValue(&vfioGroupPathsRequest{}, spec.Components.Schemas)
	vfioGroupPathsRespRef, _ := gen.NewSchemaRefForValue(&vfioGroupPathsResponse{}, spec.Components.Schemas)

	// /api/v1/vfio/devices/info
	spec.AddOperation("/api/v1/vfio/devices/info", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Get VFIO device information"
		op.Description = "Retrieve detailed information about a specific PCI device including vendor, device ID, driver, IOMMU group, and NUMA node"
		op.OperationID = "getVFIODeviceInfo"
		op.Tags = []string{"vfio"}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchemaRef(vfioDeviceInfoReqRef)}}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Device information")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(vfioDeviceInfoRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Bad request")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("400", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vfio/devices/validate
	spec.AddOperation("/api/v1/vfio/devices/validate", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Validate VFIO devices"
		op.Description = "Validate PCI addresses and check against optional allowlist patterns"
		op.OperationID = "validateVFIODevices"
		op.Tags = []string{"vfio"}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchemaRef(vfioValidateReqRef)}}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Validation result")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(vfioValidateRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Bad request")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("400", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vfio/devices/iommu-groups
	spec.AddOperation("/api/v1/vfio/devices/iommu-groups", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Check IOMMU groups"
		op.Description = "Get IOMMU group information for specified PCI devices"
		op.OperationID = "checkVFIOIOMMUGroups"
		op.Tags = []string{"vfio"}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchemaRef(vfioValidateReqRef)}}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("IOMMU group information")
			arr := &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}, Items: vfioIOMMUGroupRespRef}
			resp.Content = openapi3.NewContentWithJSONSchema(arr)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Bad request")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("400", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vfio/devices/bind
	spec.AddOperation("/api/v1/vfio/devices/bind", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Bind devices to vfio-pci"
		op.Description = "Bind PCI devices to the vfio-pci driver for passthrough"
		op.OperationID = "bindVFIODevices"
		op.Tags = []string{"vfio"}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchemaRef(vfioBindReqRef)}}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Bind result")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(vfioBindRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Bad request")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("400", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vfio/devices/unbind
	spec.AddOperation("/api/v1/vfio/devices/unbind", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Unbind devices from vfio-pci"
		op.Description = "Unbind PCI devices from the vfio-pci driver and restore original driver"
		op.OperationID = "unbindVFIODevices"
		op.Tags = []string{"vfio"}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchemaRef(vfioUnbindReqRef)}}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Unbind result")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(vfioUnbindRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Bad request")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("400", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	// /api/v1/vfio/devices/group-paths
	spec.AddOperation("/api/v1/vfio/devices/group-paths", http.MethodPost, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "Get VFIO group paths"
		op.Description = "Get /dev/vfio/GROUP_NUMBER paths for specified PCI devices"
		op.OperationID = "getVFIOGroupPaths"
		op.Tags = []string{"vfio"}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.NewContentWithJSONSchemaRef(vfioGroupPathsReqRef)}}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("VFIO group paths")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(vfioGroupPathsRespRef)
			op.Responses.Set("200", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Bad request")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("400", &openapi3.ResponseRef{Value: resp})
		}
		{
			resp := openapi3.NewResponse().WithDescription("Internal error")
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errorSchema)
			op.Responses.Set("500", &openapi3.ResponseRef{Value: resp})
		}
		return op
	}())

	return spec, nil
}
