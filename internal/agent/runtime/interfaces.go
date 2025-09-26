package runtime

import "context"

// Runtime defines the lifecycle and routing hooks for agent runtimes.
type Runtime interface {
    Name() string
    Shutdown(ctx context.Context) error
}
