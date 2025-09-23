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
