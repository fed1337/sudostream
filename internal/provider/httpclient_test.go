package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestThrottledClient_DoSetsUserAgent(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ThrottledClient sets User-Agent on outbound requests", func(a *allure.Context) {
		t := a.T()
		var gotUA string
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			gotUA = request.Header.Get("User-Agent")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`ok`))
		}))
		t.Cleanup(server.Close)

		client := NewThrottledClient(0).WithHTTPClient(server.Client()).WithUserAgent("sudoStream-test")
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatalf("request: %v", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		body, err := ReadLimited(resp.Body)
		closeErr := resp.Body.Close()
		if closeErr != nil {
			t.Fatalf("close body: %v", closeErr)
		}
		if err != nil || string(body) != "ok" {
			t.Fatalf("body: %q %v", body, err)
		}
		if gotUA != "sudoStream-test" {
			t.Fatalf("user-agent: %q", gotUA)
		}
	})
}

func TestThrottledClient_DoWithRetryRateLimited(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DoWithRetry fails when Retry-After is zero", func(a *allure.Context) {
		t := a.T()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Retry-After", "0")
			writer.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(server.Close)

		client := NewThrottledClient(0).WithHTTPClient(server.Client())
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatalf("request: %v", err)
		}

		resp, err := client.DoWithRetry(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, errProviderRateLimited) {
			t.Fatalf("want errProviderRateLimited, got %v", err)
		}
	})
}

func TestReadLimited_RejectsHugeBody(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ReadLimited rejects oversized provider payloads", func(a *allure.Context) {
		t := a.T()
		huge := strings.NewReader(strings.Repeat("a", maxHTTPResponseBytes+2))
		_, err := ReadLimited(huge)
		if !errors.Is(err, errProviderResponseHuge) {
			t.Fatalf("want errProviderResponseHuge, got %v", err)
		}
	})
}

func TestRetryAfterDelay(t *testing.T) {
	t.Parallel()

	allure.Test(t, "retryAfterDelay parses seconds and caps max", func(a *allure.Context) {
		t := a.T()
		if got := retryAfterDelay(""); got != defaultRetryAfterDelay {
			t.Fatalf("empty: %v", got)
		}
		if got := retryAfterDelay("3"); got != 3*time.Second {
			t.Fatalf("seconds: %v", got)
		}
		if got := retryAfterDelay("9999"); got != maxHTTPRetryAfter {
			t.Fatalf("cap: %v", got)
		}
		if got := retryAfterDelay("0"); got != 0 {
			t.Fatalf("zero: %v", got)
		}
		if got := retryAfterDelay("not-a-number"); got != defaultRetryAfterDelay {
			t.Fatalf("fallback: %v", got)
		}
	})
}

func TestThrottledClient_DoWithRetrySucceedsAfter429(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DoWithRetry waits Retry-After then succeeds", func(a *allure.Context) {
		t := a.T()
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			calls++
			if calls == 1 {
				writer.Header().Set("Retry-After", "1")
				writer.WriteHeader(http.StatusTooManyRequests)

				return
			}
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`ok`))
		}))
		t.Cleanup(server.Close)

		client := NewThrottledClient(0).WithHTTPClient(server.Client())
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatalf("request: %v", err)
		}

		resp, err := client.DoWithRetry(req)
		if err != nil {
			t.Fatalf("DoWithRetry: %v", err)
		}
		body, readErr := ReadLimited(resp.Body)
		closeErr := resp.Body.Close()
		if closeErr != nil {
			t.Fatalf("close: %v", closeErr)
		}
		if readErr != nil || string(body) != "ok" || resp.StatusCode != http.StatusOK || calls != 2 {
			t.Fatalf("status=%d calls=%d body=%q err=%v", resp.StatusCode, calls, body, readErr)
		}
	})
}

func TestThrottledClient_WaitRespectsMinInterval(t *testing.T) {
	t.Parallel()

	allure.Test(t, "Wait applies min-interval spacing", func(a *allure.Context) {
		t := a.T()
		client := NewThrottledClient(20 * time.Millisecond)
		start := time.Now()
		client.Wait(context.Background())
		client.Wait(context.Background())
		if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
			t.Fatalf("expected throttle delay, elapsed=%v", elapsed)
		}

		var nilClient *ThrottledClient
		nilClient.Wait(context.Background())

		req, reqErr := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"http://127.0.0.1/",
			nil,
		)
		if reqErr != nil {
			t.Fatalf("request: %v", reqErr)
		}
		resp, err := (&ThrottledClient{}).Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, errHTTPClientUnavailable) {
			t.Fatalf("nil underlying client Do: %v", err)
		}
	})
}

func TestThrottledClient_DoWithRetryCanceledDuringWait(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DoWithRetry returns when context cancels during Retry-After", func(a *allure.Context) {
		t := a.T()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Retry-After", "30")
			writer.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(server.Close)

		ctx, cancel := context.WithCancel(context.Background())
		client := NewThrottledClient(0).WithHTTPClient(server.Client())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatalf("request: %v", err)
		}

		errCh := make(chan error, 1)
		go func() {
			resp, doErr := client.DoWithRetry(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
			errCh <- doErr
		}()
		time.Sleep(20 * time.Millisecond)
		cancel()

		select {
		case doErr := <-errCh:
			if doErr == nil {
				t.Fatal("want cancel error")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for cancel")
		}
	})
}
