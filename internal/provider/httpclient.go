package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sudoStream/internal/observability"
	"sync"
	"time"
)

const (
	// DefaultHTTPUserAgent identifies sudoStream to upstream provider APIs.
	DefaultHTTPUserAgent   = "sudoStream/1.0"
	defaultHTTPUserAgent   = DefaultHTTPUserAgent
	defaultHTTPTimeout     = 30 * time.Second
	maxHTTPRetryAfter      = 90 * time.Second
	maxHTTPResponseBytes   = 8 << 20
	defaultRetryAfterSecs  = 2
	defaultRetryAfterDelay = defaultRetryAfterSecs * time.Second
)

var (
	errHTTPClientUnavailable = errors.New("provider http client unavailable")
	errProviderRateLimited   = errors.New("provider rate limited")
	errProviderResponseHuge  = errors.New("provider response too large")
	// ErrProviderUnavailable means the upstream rejected or blocked the client
	// (403/ban/exhausted 429). Enricher stops the rest of the library run.
	ErrProviderUnavailable = errors.New("provider unavailable")
)

// ThrottledClient is a minimal HTTP client with User-Agent and min-interval spacing.
type ThrottledClient struct {
	client      *http.Client
	userAgent   string
	provider    string
	minInterval time.Duration

	mu       sync.Mutex
	lastCall time.Time
}

// NewThrottledClient builds a client. minInterval of 0 disables client-side spacing.
func NewThrottledClient(minInterval time.Duration) *ThrottledClient {
	return &ThrottledClient{
		client:      &http.Client{Timeout: defaultHTTPTimeout},
		userAgent:   defaultHTTPUserAgent,
		minInterval: minInterval,
	}
}

// WithHTTPClient overrides the underlying client (tests).
func (c *ThrottledClient) WithHTTPClient(client *http.Client) *ThrottledClient {
	if client != nil {
		c.client = client
	}

	return c
}

// WithUserAgent overrides the User-Agent header.
func (c *ThrottledClient) WithUserAgent(agent string) *ThrottledClient {
	if strings.TrimSpace(agent) != "" {
		c.userAgent = strings.TrimSpace(agent)
	}

	return c
}

// WithProvider sets the provider key used for outbound HTTP metrics.
func (c *ThrottledClient) WithProvider(name string) *ThrottledClient {
	if strings.TrimSpace(name) != "" {
		c.provider = strings.TrimSpace(name)
	}

	return c
}

// Wait applies the client-side min-interval throttle (shared by HTTP and UDP callers).
func (c *ThrottledClient) Wait(ctx context.Context) {
	if c == nil {
		return
	}
	c.wait(ctx)
}

// Do waits for the throttle, sets User-Agent when missing, and executes the request.
func (c *ThrottledClient) Do(req *http.Request) (*http.Response, error) {
	if c == nil || c.client == nil {
		return nil, errHTTPClientUnavailable
	}

	c.wait(req.Context())
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider http do: %w", err)
	}
	if c.provider != "" {
		observability.RecordProviderHTTP(c.provider, resp.StatusCode)
	}

	return resp, nil
}

// DoWithRetry retries once on HTTP 429 using Retry-After when present.
func (c *ThrottledClient) DoWithRetry(req *http.Request) (*http.Response, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		return resp, nil
	}

	delay := retryAfterDelay(resp.Header.Get("Retry-After"))
	_ = resp.Body.Close()
	if delay <= 0 {
		return nil, errProviderRateLimited
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-req.Context().Done():
		return nil, fmt.Errorf("provider retry wait: %w", req.Context().Err())
	case <-timer.C:
	}

	retry, err := cloneRequest(req)
	if err != nil {
		return nil, err
	}

	return c.Do(retry)
}

// ReadLimited reads up to maxHTTPResponseBytes from body.
func ReadLimited(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxHTTPResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read provider response: %w", err)
	}
	if len(data) > maxHTTPResponseBytes {
		return nil, errProviderResponseHuge
	}

	return data, nil
}

func (c *ThrottledClient) wait(ctx context.Context) {
	if c.minInterval <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	wait := c.minInterval - time.Since(c.lastCall)
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
	}
	c.lastCall = time.Now()
}

func retryAfterDelay(header string) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return defaultRetryAfterDelay
	}

	seconds, err := strconv.Atoi(header)
	if err == nil {
		if seconds <= 0 {
			return 0
		}
		delay := time.Duration(seconds) * time.Second
		if delay > maxHTTPRetryAfter {
			return maxHTTPRetryAfter
		}

		return delay
	}

	return defaultRetryAfterDelay
}

func cloneRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("clone provider request body: %w", err)
		}
		clone.Body = body
	}

	return clone, nil
}
