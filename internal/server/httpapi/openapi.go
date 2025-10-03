package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	openapi3 "github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"

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

	// /api/v1/vms
	spec.AddOperation("/api/v1/vms", http.MethodGet, func() *openapi3.Operation {
		op := openapi3.NewOperation()
		op.Summary = "List VMs"
		op.OperationID = "listVMs"
		op.Tags = []string{"vm"}
		op.Responses = openapi3.NewResponses()
		{
			resp := openapi3.NewResponse().WithDescription("Array of VMs")
			arr := &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}, Items: vmRespRef}
			resp.Content = openapi3.NewContentWithJSONSchema(arr)
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

	return spec, nil
}
