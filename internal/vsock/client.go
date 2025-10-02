package vsock

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/mdlayher/vsock"
)

// Client provides HTTP-over-vsock communication for VM actions.
type Client struct {
	cid     uint32
	port    uint32
	timeout time.Duration
}

// NewClient creates a vsock client for the specified CID and port.
// The port should match the port exposed by the volant agent in the guest.
func NewClient(cid, port uint32) *Client {
	return &Client{
		cid:     cid,
		port:    port,
		timeout: 30 * time.Second,
	}
}

// SetTimeout configures the request timeout.
func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

// Do sends an HTTP request over vsock and returns the response.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Create vsock connection
	conn, err := vsock.Dial(c.cid, c.port, nil)
	if err != nil {
		return nil, fmt.Errorf("vsock dial: %w", err)
	}

	// Set deadline
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.timeout)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close()
		return nil, fmt.Errorf("set deadline: %w", err)
	}

	// Write HTTP request
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Read HTTP response
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Wrap connection to ensure it's closed when response body is closed
	resp.Body = &connCloser{
		ReadCloser: resp.Body,
		conn:       conn,
	}

	return resp, nil
}

// DoJSON sends an HTTP request with JSON body and decodes JSON response.
func (c *Client) DoJSON(ctx context.Context, method, path string, reqBody, respBody interface{}) error {
	// Build URL
	u := fmt.Sprintf("http://vsock/%s", path)
	
	// Encode request body if provided
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = io.NopCloser(io.Reader(&jsonReader{data: data}))
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	// Send request
	resp, err := c.Do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}

	// Decode response if output provided
	if respBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

// connCloser wraps a ReadCloser and also closes the underlying connection.
type connCloser struct {
	io.ReadCloser
	conn net.Conn
}

func (c *connCloser) Close() error {
	err1 := c.ReadCloser.Close()
	err2 := c.conn.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// jsonReader implements io.Reader for byte slices.
type jsonReader struct {
	data []byte
	pos  int
}

func (r *jsonReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
