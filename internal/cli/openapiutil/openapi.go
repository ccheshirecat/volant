package openapiutil

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Operation describes a single OpenAPI operation with resolved metadata.
type Operation struct {
	OperationID string
	Method      string
	Path        string
	Summary     string
	Description string
	Parameters  []Parameter
	RequestBody *openapi3.RequestBodyRef
}

// Parameter captures relevant parameter metadata from the spec.
type Parameter struct {
	Name        string
	In          string
	Required    bool
	Description string
}

// ParseDocument loads an OpenAPI document from raw bytes.
func ParseDocument(data []byte) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("openapi: load: %w", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("openapi: validate: %w", err)
	}
	return doc, nil
}

// ListOperations flattens all operations in the OpenAPI document.
func ListOperations(doc *openapi3.T) []Operation {
	var ops []Operation
	if doc == nil || doc.Paths == nil {
		return ops
	}

	for path, item := range doc.Paths {
		if item == nil {
			continue
		}

		pathParams := collectParameters(item.Parameters)

		for _, entry := range []struct {
			method string
			op     *openapi3.Operation
		}{
			{"GET", item.Get},
			{"PUT", item.Put},
			{"POST", item.Post},
			{"DELETE", item.Delete},
			{"OPTIONS", item.Options},
			{"HEAD", item.Head},
			{"PATCH", item.Patch},
			{"TRACE", item.Trace},
		} {
			if entry.op == nil {
				continue
			}
			params := append(collectParameters(entry.op.Parameters), pathParams...)
			ops = append(ops, Operation{
				OperationID: entry.op.OperationID,
				Method:      entry.method,
				Path:        path,
				Summary:     entry.op.Summary,
				Description: entry.op.Description,
				Parameters:  params,
				RequestBody: entry.op.RequestBody,
			})
		}
	}

	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].OperationID != "" && ops[j].OperationID != "" {
			return strings.Compare(ops[i].OperationID, ops[j].OperationID) < 0
		}
		if ops[i].Path != ops[j].Path {
			return strings.Compare(ops[i].Path, ops[j].Path) < 0
		}
		return strings.Compare(ops[i].Method, ops[j].Method) < 0
	})

	return ops
}

// FindOperation locates an operation by operationId (case insensitive) or "METHOD:PATH" token.
func FindOperation(doc *openapi3.T, token string) (*Operation, error) {
	ops := ListOperations(doc)
	normalized := strings.ToLower(strings.TrimSpace(token))
	for _, op := range ops {
		if op.OperationID != "" && strings.ToLower(op.OperationID) == normalized {
			return &op, nil
		}
	}

	if parts := strings.SplitN(normalized, ":", 2); len(parts) == 2 {
		method := strings.ToUpper(strings.TrimSpace(parts[0]))
		path := strings.TrimSpace(parts[1])
		for _, op := range ops {
			if strings.ToUpper(op.Method) == method && strings.EqualFold(op.Path, path) {
				return &op, nil
			}
		}
	}

	return nil, fmt.Errorf("operation %q not found", token)
}

// RequiredParameters returns the names of required parameters filtered by location.
func RequiredParameters(op *Operation, location string) []string {
	if op == nil {
		return nil
	}
	location = strings.ToLower(location)
	var result []string
	for _, param := range op.Parameters {
		if strings.ToLower(param.In) == location && param.Required {
			result = append(result, param.Name)
		}
	}
	return result
}

func collectParameters(refs openapi3.Parameters) []Parameter {
	params := make([]Parameter, 0, len(refs))
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		param := ref.Value
		if param == nil {
			continue
		}
		params = append(params, Parameter{
			Name:        param.Name,
			In:          param.In,
			Required:    param.Required,
			Description: param.Description,
		})
	}
	return params
}
