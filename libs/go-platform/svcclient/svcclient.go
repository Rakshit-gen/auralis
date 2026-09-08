// Package svcclient is a small JSON HTTP client for synchronous
// service-to-service calls. It attaches the shared service token and propagates
// the correlation id so a request can be traced across services.
package svcclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/logging"
)

// Client calls one downstream service.
type Client struct {
	base   string
	token  string
	hc     *http.Client
	caller string
}

// New builds a client for baseURL (for example http://content:8082).
func New(baseURL, serviceToken, callerName string) *Client {
	return &Client{
		base:   baseURL,
		token:  serviceToken,
		caller: callerName,
		hc:     &http.Client{Timeout: 5 * time.Second},
	}
}

// Get performs a GET and decodes the JSON body into out. A non-2xx response is
// returned as a dependency error, except 404 which returns ErrNotFound.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// Post performs a POST with a JSON body.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

// ErrNotFound is returned for a 404 response.
var ErrNotFound = errcodes.New(http.StatusNotFound, errcodes.NotFound, "downstream resource not found")

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auralis-Service-Token", c.token)
	req.Header.Set("X-Auralis-Service", c.caller)
	if f := logging.FromContext(ctx); f.CorrelationID != "" {
		req.Header.Set("X-Correlation-Id", f.CorrelationID)
		req.Header.Set("X-Auralis-Request-Id", f.RequestID)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return errcodes.New(http.StatusBadGateway, errcodes.Unavailable,
			fmt.Sprintf("%s is unavailable", c.base))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		logging.L(ctx).Warn("downstream call failed",
			"url", c.base+path, "status", resp.StatusCode, "body", string(snippet))
		return errcodes.New(http.StatusBadGateway, errcodes.Unavailable,
			fmt.Sprintf("downstream returned %d", resp.StatusCode))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
