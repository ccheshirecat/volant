package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "os"
    "strings"

    httpapi "github.com/ccheshirecat/volant/internal/server/httpapi"
    openapi3 "github.com/getkin/kin-openapi/openapi3"
)

func main() {
    var (
        outPath   string
        format    string
        serverURL string
    )

    flag.StringVar(&outPath, "output", "", "Output path (default stdout)")
    flag.StringVar(&format, "format", "json", "Output format: json")
    flag.StringVar(&serverURL, "server", "http://127.0.0.1:7777", "Server URL to include in OpenAPI servers list")
    flag.Parse()

    // Build spec using the same generator used by the HTTP handler
    spec, err := httpapi.BuildOpenAPISpec("")
    if err != nil {
        fatalf("build openapi: %v", err)
    }

    // Normalize servers
    serverURL = strings.TrimSpace(serverURL)
    if serverURL != "" {
        spec.Servers = openapi3.Servers{&openapi3.Server{URL: serverURL}}
    }

    // Marshal
    var data []byte
    switch strings.ToLower(format) {
    case "json":
        data, err = json.MarshalIndent(spec, "", "  ")
        if err != nil {
            fatalf("marshal json: %v", err)
        }
    default:
        fatalf("unsupported format: %s (only json supported)", format)
    }

    // Write
    if outPath == "" {
        os.Stdout.Write(data)
        return
    }
    if err := os.WriteFile(outPath, data, 0o644); err != nil {
        fatalf("write %s: %v", outPath, err)
    }
}

func fatalf(format string, args ...any) {
    fmt.Fprintf(os.Stderr, format+"\n", args...)
    os.Exit(1)
}
