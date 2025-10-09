package routes

import (
	"context"
	"fmt"
)

// ErrNotFound indicates the requested route does not exist.
var ErrNotFound = fmt.Errorf("route not found")

// Store abstracts persistent management of routing entries.
type Store interface {
	List(ctx context.Context) ([]Route, error)
	Get(ctx context.Context, hostPort uint16, protocol string) (*Route, error)
	Upsert(ctx context.Context, route Route) error
	Delete(ctx context.Context, hostPort uint16, protocol string) error
}
