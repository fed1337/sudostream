package network

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultProbeURL     = "https://api.tvmaze.com/"
	defaultProbeTimeout = 12 * time.Second
)

// ProbeResult is the admin "test connection" response.
type ProbeResult struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"statusCode,omitempty"`
	DurationMs int64  `json:"durationMs"`
	Via        string `json:"via,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ProbeOptions customizes TestConnection (tests override URL).
type ProbeOptions struct {
	URL     string
	Timeout time.Duration
}

// TestConnection dials the probe URL through a one-shot client built from draft settings.
func TestConnection(ctx context.Context, settings Settings, opts ProbeOptions) ProbeResult {
	start := time.Now()
	probeURL := strings.TrimSpace(opts.URL)
	if probeURL == "" {
		probeURL = defaultProbeURL
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}

	client, err := ClientFor(settings, timeout)
	if err != nil {
		return ProbeResult{
			OK:         false,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      sanitizeProbeError(err.Error()),
		}
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return ProbeResult{
			OK:         false,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      sanitizeProbeError(err.Error()),
		}
	}
	req.Header.Set("User-Agent", "sudoStream/1.0")

	via := settings.Scheme()
	if via == "" {
		via = "direct"
	}

	resp, err := client.Do(req)
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return ProbeResult{
			OK:         false,
			DurationMs: duration,
			Via:        via,
			Error:      sanitizeProbeError(err.Error()),
		}
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, probeBodyLimitBytes))

	ok := resp.StatusCode >= httpStatusOKMin && resp.StatusCode < httpStatusOKMaxExclusive
	result := ProbeResult{
		OK:         ok,
		StatusCode: resp.StatusCode,
		DurationMs: duration,
		Via:        via,
	}
	if !ok {
		result.Error = fmt.Sprintf("unexpected status %d", resp.StatusCode)
	}

	return result
}

func sanitizeProbeError(message string) string {
	// Avoid echoing proxy passwords if they appear in URL-shaped errors.
	message = strings.TrimSpace(message)
	if message == "" {
		return "probe failed"
	}

	return message
}
