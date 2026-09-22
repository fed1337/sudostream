package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	xproxy "golang.org/x/net/proxy"
)

const (
	defaultClientTimeout     = 30 * time.Second
	maxIdleConns             = 100
	idleConnTimeout          = 90 * time.Second
	tlsHandshakeTimeout      = 10 * time.Second
	expectContinueTimeout    = 1 * time.Second
	probeBodyLimitBytes      = 64 << 10
	httpStatusOKMin          = 200
	httpStatusOKMaxExclusive = 400
)

// Live holds hot-reloadable settings for provider outbound HTTP.
type Live struct {
	cfg    atomic.Pointer[Settings]
	client *http.Client
}

// NewLive builds a shared HTTP client whose Proxy/DialContext read the latest settings.
func NewLive(initial Settings) *Live {
	live := &Live{}
	normalized, err := NormalizeSettings(initial)
	if err != nil {
		normalized = DefaultSettings()
	}
	live.cfg.Store(&normalized)
	transport := &http.Transport{
		Proxy:                 live.proxyForRequest,
		DialContext:           live.dialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          maxIdleConns,
		IdleConnTimeout:       idleConnTimeout,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ExpectContinueTimeout: expectContinueTimeout,
	}
	live.client = &http.Client{
		Timeout:   defaultClientTimeout,
		Transport: transport,
	}

	return live
}

// HTTPClient returns the shared client (safe for concurrent use).
func (l *Live) HTTPClient() *http.Client {
	if l == nil || l.client == nil {
		return &http.Client{Timeout: defaultClientTimeout}
	}

	return l.client
}

// Store replaces the active settings (call after successful save).
func (l *Live) Store(settings Settings) {
	if l == nil {
		return
	}
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		return
	}
	l.cfg.Store(&normalized)
}

// Current returns a copy of the active settings.
func (l *Live) Current() Settings {
	if l == nil {
		return DefaultSettings()
	}
	ptr := l.cfg.Load()
	if ptr == nil {
		return DefaultSettings()
	}

	return *ptr
}

func (l *Live) load() Settings {
	if l == nil {
		return DefaultSettings()
	}
	ptr := l.cfg.Load()
	if ptr == nil {
		return DefaultSettings()
	}

	return *ptr
}

func (l *Live) proxyForRequest(req *http.Request) (*url.URL, error) {
	settings := l.load()
	if settings.ProxyURL == "" {
		proxyURL, err := http.ProxyFromEnvironment(req)
		if err != nil {
			return nil, fmt.Errorf("proxy from environment: %w", err)
		}

		return proxyURL, nil
	}
	if settings.IsSOCKS() {
		//nolint:nilnil // http.Transport treats (nil, nil) as "dial direct"
		return nil, nil
	}
	host := ""
	if req != nil && req.URL != nil {
		host = req.URL.Hostname()
	}
	if hostBypassed(host, settings.NoProxy) {
		//nolint:nilnil // http.Transport treats (nil, nil) as "dial direct"
		return nil, nil
	}

	return httpProxyURL(settings)
}

func (l *Live) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	settings := l.load()
	host, _, splitErr := net.SplitHostPort(address)
	if splitErr != nil {
		host = address
	}
	if settings.ProxyURL == "" || !settings.IsSOCKS() || hostBypassed(host, settings.NoProxy) {
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, fmt.Errorf("direct dial: %w", err)
		}

		return conn, nil
	}

	socks, err := newSocksDialer(settings)
	if err != nil {
		return nil, err
	}
	if ctxDialer, ok := socks.(xproxy.ContextDialer); ok {
		conn, dialErr := ctxDialer.DialContext(ctx, network, address)
		if dialErr != nil {
			return nil, fmt.Errorf("socks dial: %w", dialErr)
		}

		return conn, nil
	}

	return dialWithContext(ctx, socks, network, address)
}

// ClientFor builds a one-shot HTTP client from settings (test probe / isolated use).
func ClientFor(settings Settings, timeout time.Duration) (*http.Client, error) {
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = defaultClientTimeout
	}
	live := NewLive(normalized)
	client := live.HTTPClient()
	client.Timeout = timeout

	return client, nil
}

func httpProxyURL(settings Settings) (*url.URL, error) {
	parsed, err := url.Parse(settings.ProxyURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidProxyURL, err)
	}
	if settings.ProxyUser != "" {
		if settings.ProxyPassword != "" {
			parsed.User = url.UserPassword(settings.ProxyUser, settings.ProxyPassword)
		} else {
			parsed.User = url.User(settings.ProxyUser)
		}
	}

	return parsed, nil
}

func newSocksDialer(settings Settings) (xproxy.Dialer, error) { //nolint:ireturn // x/net proxy API is interface-typed
	parsed, err := url.Parse(settings.ProxyURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidProxyURL, err)
	}
	if settings.ProxyUser != "" {
		parsed.User = url.UserPassword(settings.ProxyUser, settings.ProxyPassword)
	}

	dialer, err := xproxy.FromURL(parsed, xproxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("socks dialer: %w", err)
	}

	return dialer, nil
}

func dialWithContext(
	ctx context.Context,
	dialer xproxy.Dialer,
	network, address string,
) (net.Conn, error) {
	type dialResult struct {
		conn net.Conn
		err  error
	}
	done := make(chan dialResult, 1)
	go func() {
		conn, err := dialer.Dial(network, address)
		done <- dialResult{conn: conn, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("socks dial canceled: %w", ctx.Err())
	case res := <-done:
		if res.err != nil {
			return nil, fmt.Errorf("socks dial: %w", res.err)
		}

		return res.conn, nil
	}
}

func hostBypassed(host, noProxy string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	noProxy = strings.TrimSpace(noProxy)
	if host == "" || noProxy == "" {
		return false
	}
	for part := range strings.SplitSeq(noProxy, ",") {
		if matchNoProxyRule(host, strings.ToLower(strings.TrimSpace(part))) {
			return true
		}
	}

	return false
}

func matchNoProxyRule(host, rule string) bool {
	if rule == "" {
		return false
	}
	if rule == "*" {
		return true
	}
	if after, ok := strings.CutPrefix(rule, "."); ok {
		return strings.HasSuffix(host, rule) || host == after
	}

	return host == rule || strings.HasSuffix(host, "."+rule)
}
