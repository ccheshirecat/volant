// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package httpapi

// This file contains the additional OpenAPI endpoints that need to be added to openapi.go
// Add these endpoint definitions right before the "return spec, nil" line in BuildOpenAPISpec()

/*
Add these endpoints to openapi.go before "return spec, nil":

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

*/
