package standard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"

	"github.com/ccheshirecat/volant/internal/cli/client"
)

func newBrowsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "browsers",
		Short: "Browser automation tooling",
	}

	cmd.AddCommand(newBrowsersProxyCmd())
	cmd.AddCommand(newBrowsersStreamCmd())
	return cmd
}

func newBrowsersStreamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stream <vm>",
		Short: "Print the remote DevTools WebSocket URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			info, err := api.GetAgentDevTools(ctx, args[0])
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", info.WebSocketURL)
			return nil
		},
	}

	return cmd
}

func newBrowsersProxyCmd() *cobra.Command {
	var bind string
	var port int

	cmd := &cobra.Command{
		Use:   "proxy <vm>",
		Short: "Expose a VM browser DevTools endpoint locally",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}

			vmName := args[0]

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			preflightCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			info, err := api.GetAgentDevTools(preflightCtx, vmName)
			if err != nil {
				return fmt.Errorf("devtools discovery failed: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Remote DevTools WebSocket: %s\n", info.WebSocketURL)
			if info.BrowserVersion != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Browser: %s\n", info.BrowserVersion)
			}

			return runBrowsersProxy(ctx, cmd, api, vmName, bind, port)
		},
	}

	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "Local address for the DevTools proxy")
	cmd.Flags().IntVar(&port, "port", 9223, "Local port for the DevTools proxy")

	return cmd
}

func runBrowsersProxy(ctx context.Context, cmd *cobra.Command, api *client.Client, vmName, bind string, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}

	baseURL := api.BaseURL()
	if baseURL == nil {
		return fmt.Errorf("client base url not configured")
	}

	proxy, err := newDevToolsProxy(baseURL, vmName, bind, port)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(bind, strconv.Itoa(port))
	server := &http.Server{
		Addr:    addr,
		Handler: proxy,
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	fmt.Fprintf(cmd.OutOrStdout(), "DevTools proxy listening on http://%s\n", addr)
	fmt.Fprintf(cmd.OutOrStdout(), "Open Chrome DevTools at http://%s/json/version\n", addr)
	fmt.Fprintln(cmd.OutOrStdout(), "Press Ctrl+C to stop.")

	for {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
			return nil
		case err := <-serverErr:
			if err == nil || errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("devtools proxy error: %w", err)
		}
	}
}

type devtoolsProxy struct {
	serverBase    *url.URL
	vmName        string
	httpProxy     *httputil.ReverseProxy
	localHostPort string
}

func newDevToolsProxy(serverBase *url.URL, vmName, bind string, port int) (*devtoolsProxy, error) {
	if serverBase == nil {
		return nil, fmt.Errorf("server base URL required")
	}
	if strings.TrimSpace(vmName) == "" {
		return nil, fmt.Errorf("vm name required")
	}

	baseCopy := *serverBase
	httpProxy := httputil.NewSingleHostReverseProxy(&baseCopy)

	p := &devtoolsProxy{
		serverBase:    &baseCopy,
		vmName:        vmName,
		httpProxy:     httpProxy,
		localHostPort: net.JoinHostPort(bind, strconv.Itoa(port)),
	}

	httpProxy.Director = p.director
	modResp := func(resp *http.Response) error {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("upstream error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		rewritten, err := rewriteDevToolsJSON(body, p.localHostPort)
		if err != nil {
			resp.Body = io.NopCloser(bytes.NewReader(body))
			resp.ContentLength = int64(len(body))
			resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
			return nil
		}

		resp.Body = io.NopCloser(bytes.NewReader(rewritten))
		resp.ContentLength = int64(len(rewritten))
		resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
		return nil
	}

	httpProxy.ModifyResponse = modResp
	httpProxy.ErrorHandler = p.errorHandler
	httpProxy.FlushInterval = 200 * time.Millisecond

	return p, nil
}

func (p *devtoolsProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if websocket.IsWebSocketUpgrade(r) {
		p.handleWebSocket(w, r)
		return
	}
	p.httpProxy.ServeHTTP(w, r)
}

func (p *devtoolsProxy) director(req *http.Request) {
	req.URL.Scheme = p.serverBase.Scheme
	req.URL.Host = p.serverBase.Host
	req.Host = p.serverBase.Host

	path := req.URL.Path
	if path == "" {
		path = "/"
	}
	req.URL.Path = fmt.Sprintf("/api/v1/vms/%s/agent%s", url.PathEscape(p.vmName), path)
	req.URL.RawPath = req.URL.Path
}

func (p *devtoolsProxy) modifyResponse(resp *http.Response) error {
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "application/json") {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	rewritten, err := rewriteDevToolsJSON(body, p.localHostPort)
	if err != nil {
		rewritten = body
	}

	resp.Body = io.NopCloser(bytes.NewReader(rewritten))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))

	return nil
}

func (p *devtoolsProxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, http.ErrServerClosed) {
		return
	}
	http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
}

func (p *devtoolsProxy) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	target := p.serverBase.ResolveReference(&url.URL{
		Path:     fmt.Sprintf("/ws/v1/vms/%s/devtools%s", url.PathEscape(p.vmName), r.URL.Path),
		RawQuery: r.URL.RawQuery,
	})

	switch target.Scheme {
	case "http", "":
		target.Scheme = "ws"
	case "https":
		target.Scheme = "wss"
	}

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 30 * time.Second,
	}

	agentConn, resp, err := dialer.DialContext(r.Context(), target.String(), nil)
	if resp != nil {
		resp.Body.Close()
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("dial devtools target: %v", err), http.StatusBadGateway)
		return
	}
	defer agentConn.Close()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer clientConn.Close()

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		errCh <- relayWebSocket(agentConn, clientConn)
	}()

	go func() {
		defer wg.Done()
		errCh <- relayWebSocket(clientConn, agentConn)
	}()

	var proxyErr error
	select {
	case <-r.Context().Done():
		proxyErr = r.Context().Err()
	case proxyErr = <-errCh:
	}

	_ = agentConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	_ = clientConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))

	wg.Wait()

	if proxyErr != nil && !errors.Is(proxyErr, context.Canceled) && !errors.Is(proxyErr, io.EOF) &&
		!websocket.IsCloseError(proxyErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		// Best-effort logging via standard error path.
	}
}

func relayWebSocket(src, dst *websocket.Conn) error {
	for {
		msgType, payload, err := src.ReadMessage()
		if err != nil {
			return err
		}
		if err := dst.WriteMessage(msgType, payload); err != nil {
			return err
		}
	}
}

func rewriteDevToolsJSON(body []byte, hostPort string) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}

	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, nil
	}

	switch data := payload.(type) {
	case map[string]interface{}:
		rewriteDevToolsMap(data, hostPort)
	case []interface{}:
		for _, item := range data {
			if m, ok := item.(map[string]interface{}); ok {
				rewriteDevToolsMap(m, hostPort)
			}
		}
	default:
		return body, nil
	}

	rewritten, err := json.Marshal(payload)
	if err != nil {
		return body, err
	}
	return rewritten, nil
}

func rewriteDevToolsMap(m map[string]interface{}, hostPort string) {
	if wsURL, ok := stringValue(m["webSocketDebuggerUrl"]); ok {
		if rewritten, err := rewriteWebSocketAddress(wsURL, hostPort); err == nil {
			m["webSocketDebuggerUrl"] = rewritten
		}
	}
	if secureURL, ok := stringValue(m["webSocketSecureUrl"]); ok {
		if rewritten, err := rewriteWebSocketAddress(secureURL, hostPort); err == nil {
			m["webSocketSecureUrl"] = rewritten
		}
	}
	if frontend, ok := stringValue(m["devtoolsFrontendUrl"]); ok {
		if rewritten, err := rewriteFrontendURL(frontend, hostPort); err == nil {
			m["devtoolsFrontendUrl"] = rewritten
		}
	}
	if frontendCompat, ok := stringValue(m["devtoolsFrontendUrlCompat"]); ok {
		if rewritten, err := rewriteFrontendURL(frontendCompat, hostPort); err == nil {
			m["devtoolsFrontendUrlCompat"] = rewritten
		}
	}
}

func rewriteWebSocketAddress(raw, hostPort string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty websocket url")
	}

	u, err := url.Parse(raw)
	if err != nil {
		if strings.HasPrefix(raw, "/") {
			u = &url.URL{
				Scheme: "ws",
				Host:   hostPort,
				Path:   raw,
			}
		} else {
			return "", err
		}
	}

	switch strings.ToLower(u.Scheme) {
	case "", "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		u.Scheme = "ws"
	}

	u.Host = hostPort
	return u.String(), nil
}

func rewriteFrontendURL(raw, hostPort string) (string, error) {
	if raw == "" {
		return raw, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return raw, nil
	}

	q := u.Query()
	if wsParam := q.Get("ws"); wsParam != "" {
		wsURL := wsParam
		if !strings.HasPrefix(wsURL, "ws://") && !strings.HasPrefix(wsURL, "wss://") {
			wsURL = "ws://" + wsURL
		}
		rewrittenWS, err := rewriteWebSocketAddress(wsURL, hostPort)
		if err == nil {
			if parsed, perr := url.Parse(rewrittenWS); perr == nil {
				q.Set("ws", parsed.Host+parsed.RequestURI())
			}
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func stringValue(value interface{}) (string, bool) {
	if value == nil {
		return "", false
	}
	str, ok := value.(string)
	return str, ok
}
