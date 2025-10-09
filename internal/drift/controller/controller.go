package controller

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/volantvm/volant/internal/drift/dataplane"
	"github.com/volantvm/volant/internal/drift/routes"
	"github.com/volantvm/volant/internal/drift/vsockproxy"
)

// Controller coordinates persistent route state with dataplane programming.
type Controller struct {
	store routes.Store
	dp    dataplane.Interface
	vsock vsockproxy.Manager
}

// New constructs a Controller.
func New(store routes.Store, dp dataplane.Interface, vsock vsockproxy.Manager) *Controller {
	return &Controller{store: store, dp: dp, vsock: vsock}
}

// ValidationError marks input validation failures.
type ValidationError struct{ Err error }

func (e ValidationError) Error() string { return e.Err.Error() }
func (e ValidationError) Unwrap() error { return e.Err }

// RuntimeUnavailableError indicates the required runtime manager is absent.
type RuntimeUnavailableError struct{ Component string }

func (e RuntimeUnavailableError) Error() string { return fmt.Sprintf("%s unavailable", e.Component) }

// List returns all routes.
func (c *Controller) List(ctx context.Context) ([]routes.Route, error) {
	return c.store.List(ctx)
}

// Upsert validates, applies to dataplane, and persists the route.
func (c *Controller) Upsert(ctx context.Context, route routes.Route) (routes.Route, error) {
	normalized, err := normalize(route)
	if err != nil {
		return routes.Route{}, ValidationError{Err: err}
	}

	if err := c.applyRuntime(ctx, normalized); err != nil {
		return routes.Route{}, err
	}

	if err := c.store.Upsert(ctx, normalized); err != nil {
		_ = c.removeRuntime(ctx, normalized)
		return routes.Route{}, err
	}

	return normalized, nil
}

// Delete removes a route from both dataplane and persistent store.
func (c *Controller) Delete(ctx context.Context, hostPort uint16, protocol string) error {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if !validProtocol(protocol) {
		return fmt.Errorf("protocol %q not supported", protocol)
	}

	route, err := c.store.Get(ctx, hostPort, protocol)
	if err != nil {
		return err
	}

	if err := c.removeRuntime(ctx, *route); err != nil {
		return err
	}

	return c.store.Delete(ctx, hostPort, protocol)
}

// Restore replays persisted routes into runtime managers.
func (c *Controller) Restore(ctx context.Context) error {
	items, err := c.store.List(ctx)
	if err != nil {
		return err
	}
	for _, route := range items {
		if err := c.applyRuntime(ctx, route); err != nil {
			var unavailable RuntimeUnavailableError
			if errors.As(err, &unavailable) {
				continue
			}
			return fmt.Errorf("restore route %d/%s: %w", route.HostPort, route.Protocol, err)
		}
	}
	return nil
}

func normalize(route routes.Route) (routes.Route, error) {
	if route.HostPort == 0 {
		return routes.Route{}, fmt.Errorf("host_port must be > 0")
	}

	normalized := route
	normalized.Protocol = strings.ToLower(strings.TrimSpace(route.Protocol))
	if !validProtocol(normalized.Protocol) {
		return routes.Route{}, fmt.Errorf("protocol %q not supported", route.Protocol)
	}

	normalized.Backend.Type = routes.BackendType(strings.ToLower(strings.TrimSpace(string(route.Backend.Type))))
	if normalized.Backend.Port == 0 {
		return routes.Route{}, fmt.Errorf("backend.port must be > 0")
	}

	switch normalized.Backend.Type {
	case routes.BackendBridge:
		ip := strings.TrimSpace(route.Backend.IP)
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.To4() == nil {
			return routes.Route{}, fmt.Errorf("backend.ip must be valid ipv4")
		}
		normalized.Backend.IP = parsed.String()
	case routes.BackendVsock:
		if route.Backend.CID == 0 {
			return routes.Route{}, fmt.Errorf("backend.cid must be > 0 for vsock routes")
		}
		if normalized.Protocol != "tcp" {
			return routes.Route{}, fmt.Errorf("vsock routes require tcp protocol")
		}
		normalized.Backend.IP = ""
	default:
		return routes.Route{}, fmt.Errorf("backend type %q not supported", route.Backend.Type)
	}

	return normalized, nil
}

func validProtocol(proto string) bool {
	switch proto {
	case "tcp", "udp":
		return true
	default:
		return false
	}
}

func protocolNumber(proto string) uint8 {
	switch proto {
	case "tcp":
		return 6
	case "udp":
		return 17
	default:
		return 0
	}
}

func (c *Controller) applyRuntime(ctx context.Context, route routes.Route) error {
	switch route.Backend.Type {
	case routes.BackendBridge:
		if c.dp == nil {
			return RuntimeUnavailableError{Component: "bridge dataplane"}
		}
		dstIP := net.ParseIP(route.Backend.IP)
		if err := c.dp.ApplyBridge(ctx, protocolNumber(route.Protocol), route.HostPort, dstIP, route.Backend.Port); err != nil {
			return fmt.Errorf("apply bridge dataplane: %w", err)
		}
	case routes.BackendVsock:
		if c.vsock == nil {
			return RuntimeUnavailableError{Component: "vsock proxy"}
		}
		if err := c.vsock.Upsert(ctx, route.Protocol, route.HostPort, route.Backend.CID, route.Backend.Port); err != nil {
			return fmt.Errorf("apply vsock proxy: %w", err)
		}
	}
	return nil
}

func (c *Controller) removeRuntime(ctx context.Context, route routes.Route) error {
	switch route.Backend.Type {
	case routes.BackendBridge:
		if c.dp == nil {
			return nil
		}
		if err := c.dp.Remove(ctx, protocolNumber(route.Protocol), route.HostPort); err != nil {
			return fmt.Errorf("remove bridge dataplane: %w", err)
		}
	case routes.BackendVsock:
		if c.vsock == nil {
			return nil
		}
		if err := c.vsock.Remove(ctx, route.Protocol, route.HostPort); err != nil {
			return fmt.Errorf("remove vsock proxy: %w", err)
		}
	}
	return nil
}
