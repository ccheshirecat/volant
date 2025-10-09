//go:build linux

package vsockproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/mdlayher/vsock"
)

type manager struct {
	logger   *slog.Logger
	bindAddr string
	mu       sync.Mutex
	proxies  map[string]*proxy
	closed   bool
}

type proxy struct {
	proto     string
	hostPort  uint16
	cid       uint32
	guestPort uint16
	listener  net.Listener
	cancel    context.CancelFunc
	done      chan struct{}
	logger    *slog.Logger
}

func newManager(opts Options) (Manager, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	bind := opts.BindAddress
	if bind == "" {
		bind = "0.0.0.0"
	}

	return &manager{
		logger:   logger.With("component", "vsockproxy"),
		bindAddr: bind,
		proxies:  make(map[string]*proxy),
	}, nil
}

func (m *manager) key(proto string, port uint16) string {
	return fmt.Sprintf("%s/%d", proto, port)
}

func (m *manager) Upsert(ctx context.Context, proto string, hostPort uint16, cid uint32, guestPort uint16) error {
	if proto != "tcp" {
		return fmt.Errorf("vsock proxy: protocol %q not supported", proto)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("vsock proxy: manager closed")
	}

	key := m.key(proto, hostPort)
	if existing, ok := m.proxies[key]; ok {
		existing.stop()
	}

	addr := fmt.Sprintf("%s:%d", m.bindAddr, hostPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("vsock proxy: listen %s: %w", addr, err)
	}

	childCtx, cancel := context.WithCancel(context.Background())
	px := &proxy{
		proto:     proto,
		hostPort:  hostPort,
		cid:       cid,
		guestPort: guestPort,
		listener:  listener,
		cancel:    cancel,
		done:      make(chan struct{}),
		logger:    m.logger.With("host_port", hostPort, "cid", cid, "guest_port", guestPort),
	}
	px.start(childCtx)
	m.proxies[key] = px
	return nil
}

func (m *manager) Remove(ctx context.Context, proto string, hostPort uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := m.key(proto, hostPort)
	proxy, ok := m.proxies[key]
	if !ok {
		return nil
	}
	proxy.stop()
	delete(m.proxies, key)
	return nil
}

func (m *manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	for key, proxy := range m.proxies {
		proxy.stop()
		delete(m.proxies, key)
	}
	return nil
}

func (p *proxy) start(ctx context.Context) {
	go func() {
		p.logger.Info("vsock proxy started")
		defer close(p.done)
		for {
			conn, err := p.listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
				nErr, ok := err.(net.Error)
				if ok && nErr.Temporary() {
					p.logger.Warn("temporary accept error", "error", err)
					continue
				}
				p.logger.Error("accept error", "error", err)
				return
			}
			go p.handleConnection(ctx, conn)
		}
	}()
}

func (p *proxy) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	vsockConn, err := vsock.DialContext(ctx, int(p.cid), int(p.guestPort), nil)
	if err != nil {
		p.logger.Error("vsock dial failed", "error", err)
		return
	}
	defer vsockConn.Close()

	var wg sync.WaitGroup
	copyStream := func(dst io.Writer, src io.Reader) {
		defer wg.Done()
		if _, err := io.Copy(dst, src); err != nil {
			p.logger.Warn("copy stream", "error", err)
		}
	}
	wg.Add(2)
	go copyStream(vsockConn, conn)
	go copyStream(conn, vsockConn)
	wg.Wait()
}

func (p *proxy) stop() {
	p.cancel()
	_ = p.listener.Close()
	<-p.done
	p.logger.Info("vsock proxy stopped")
}
