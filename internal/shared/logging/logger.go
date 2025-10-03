// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package logging

import (
	"log/slog"
	"os"
)

// New returns a slog.Logger configured for structured, JSON-oriented output.
func New(subsystem string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: true})
	return slog.New(handler).With("subsystem", subsystem)
}
