package network

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

var errMemoryKVMissing = errors.New("missing")

func TestNormalizeSettings_Schemes(t *testing.T) {
	t.Parallel()

	allure.Test(t, "network settings accept http/https/socks5/socks5h and reject others", func(a *allure.Context) {
		t := a.T()
		for _, scheme := range []string{"http", "https", "socks5", "socks5h"} {
			got, err := NormalizeSettings(Settings{ProxyURL: scheme + "://127.0.0.1:7890"})
			if err != nil {
				t.Fatalf("%s: %v", scheme, err)
			}
			if got.ProxyURL == "" {
				t.Fatalf("%s: empty url", scheme)
			}
		}
		_, err := NormalizeSettings(Settings{ProxyURL: "vless://example"})
		if !errors.Is(err, ErrInvalidProxyURL) {
			t.Fatalf("want ErrInvalidProxyURL, got %v", err)
		}
	})
}

func TestNormalizeSettings_StripsUserinfoToFields(t *testing.T) {
	t.Parallel()

	allure.Test(t, "userinfo in proxy URL moves into dedicated auth fields", func(a *allure.Context) {
		t := a.T()
		const user = "alice"
		const pass = "secret"
		got, err := NormalizeSettings(Settings{
			ProxyURL: "http://" + user + ":" + pass + "@127.0.0.1:7890",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.ProxyUser != user || got.ProxyPassword != pass {
			t.Fatalf("auth fields: %+v", got)
		}
		parsed, err := url.Parse(got.ProxyURL)
		if err != nil || parsed.User != nil {
			t.Fatalf("url should not retain userinfo: %s", got.ProxyURL)
		}
		if !got.HasProxyPassword {
			t.Fatal("expected hasProxyPassword")
		}
	})
}

func TestPublicViewAndMergePassword(t *testing.T) {
	t.Parallel()

	allure.Test(t, "public view hides password; merge keeps stored secret", func(a *allure.Context) {
		t := a.T()
		stored := Settings{ProxyURL: "socks5h://127.0.0.1:1", ProxyPassword: "keep-me"}
		pub := PublicView(stored)
		if pub.ProxyPassword != "" || !pub.HasProxyPassword {
			t.Fatalf("public: %+v", pub)
		}
		merged := MergePassword(Settings{ProxyURL: stored.ProxyURL, ProxyPassword: ""}, stored)
		if merged.ProxyPassword != "keep-me" {
			t.Fatalf("merge: %+v", merged)
		}
	})
}

func TestHostBypassed(t *testing.T) {
	t.Parallel()

	allure.Test(t, "noProxy matches host and suffix rules", func(a *allure.Context) {
		t := a.T()
		if !hostBypassed("api.tvmaze.com", "tvmaze.com") {
			t.Fatal("suffix match")
		}
		if !hostBypassed("localhost", "localhost,127.0.0.1") {
			t.Fatal("exact match")
		}
		if hostBypassed("api.tvmaze.com", "example.com") {
			t.Fatal("should not bypass")
		}
	})
}

type memoryKV struct {
	data map[string][]byte
}

func (m *memoryKV) GetSettingValue(_ context.Context, key string) ([]byte, error) {
	raw, ok := m.data[key]
	if !ok {
		return nil, errMemoryKVMissing
	}

	return raw, nil
}

func (m *memoryKV) SaveSettingValue(_ context.Context, key string, value []byte) error {
	if m.data == nil {
		m.data = map[string][]byte{}
	}
	m.data[key] = value

	return nil
}

func TestKVSettingsStore_RoundTrip(t *testing.T) {
	t.Parallel()

	allure.Test(t, "network KV store loads defaults and persists settings", func(a *allure.Context) {
		t := a.T()
		store := NewKVSettingsStore(&memoryKV{})
		got, err := store.GetSettings(context.Background())
		if err != nil || got.ProxyURL != "" {
			t.Fatalf("default: %+v err=%v", got, err)
		}
		err = store.SaveSettings(context.Background(), Settings{
			ProxyURL:      "http://127.0.0.1:7890",
			ProxyUser:     "u",
			ProxyPassword: "p",
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err = store.GetSettings(context.Background())
		if err != nil || got.ProxyURL != "http://127.0.0.1:7890" || got.ProxyPassword != "p" {
			t.Fatalf("loaded: %+v err=%v", got, err)
		}
		raw, _ := json.Marshal(Settings{ProxyURL: "socks5h://127.0.0.1:9", ProxyPassword: "x"})
		_ = store.kv.SaveSettingValue(context.Background(), settingsKey, raw)
		got, err = store.GetSettings(context.Background())
		if err != nil || !got.IsSOCKS() {
			t.Fatalf("socks reload: %+v err=%v", got, err)
		}
	})
}

func TestLive_HTTPProxyRoutesRequest(t *testing.T) {
	t.Parallel()

	allure.Test(t, "live client sends HTTPS via HTTP CONNECT proxy", func(a *allure.Context) {
		t := a.T()
		var sawConnect bool
		proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodConnect {
				sawConnect = true
				hj, ok := writer.(http.Hijacker)
				if !ok {
					http.Error(writer, "no hijack", http.StatusInternalServerError)

					return
				}
				conn, _, err := hj.Hijack()
				if err != nil {
					return
				}
				_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
				_ = conn.Close()

				return
			}
			http.Error(writer, "unexpected", http.StatusBadRequest)
		}))
		t.Cleanup(proxy.Close)

		live := NewLive(Settings{ProxyURL: proxy.URL})
		client := live.HTTPClient()
		client.Timeout = 2 * time.Second
		req, err := http.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			"https://example.invalid/",
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if !sawConnect {
			t.Fatalf("expected CONNECT through proxy, err=%v", err)
		}
	})
}

func TestTestConnection_DirectSuccess(t *testing.T) {
	t.Parallel()

	allure.Test(t, "probe reports ok for direct 302 without following redirects", func(a *allure.Context) {
		t := a.T()
		upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Location", "/elsewhere")
			writer.WriteHeader(http.StatusFound)
		}))
		t.Cleanup(upstream.Close)

		result := TestConnection(t.Context(), Settings{}, ProbeOptions{
			URL:     upstream.URL,
			Timeout: 3 * time.Second,
		})
		if !result.OK || result.StatusCode != http.StatusFound {
			t.Fatalf("result: %+v", result)
		}
		if result.Via != "direct" {
			t.Fatalf("via: %s", result.Via)
		}
	})
}

func TestLive_StoreAndCurrent(t *testing.T) {
	t.Parallel()

	allure.Test(t, "live Store updates Current settings for hot reload", func(a *allure.Context) {
		t := a.T()
		live := NewLive(DefaultSettings())
		if live.Current().ProxyURL != "" {
			t.Fatalf("default: %+v", live.Current())
		}
		live.Store(Settings{ProxyURL: "socks5h://127.0.0.1:7890", ProxyUser: "u", ProxyPassword: "p"})
		got := live.Current()
		if !got.IsSOCKS() || got.ProxyUser != "u" || got.ProxyPassword != "p" {
			t.Fatalf("current: %+v", got)
		}
	})
}

func TestNewSocksDialer_Builds(t *testing.T) {
	t.Parallel()

	allure.Test(t, "socks dialer builds for socks5h with auth", func(a *allure.Context) {
		t := a.T()
		dialer, err := newSocksDialer(Settings{
			ProxyURL:      "socks5h://127.0.0.1:7890",
			ProxyUser:     "u",
			ProxyPassword: "p",
		})
		if err != nil || dialer == nil {
			t.Fatalf("dialer=%v err=%v", dialer, err)
		}
	})
}

func TestHTTPProxyURL_WithUserOnly(t *testing.T) {
	t.Parallel()

	allure.Test(t, "http proxy URL embeds username without password", func(a *allure.Context) {
		t := a.T()
		parsed, err := httpProxyURL(Settings{ProxyURL: "http://127.0.0.1:1", ProxyUser: "only"})
		if err != nil || parsed.User == nil || parsed.User.Username() != "only" {
			t.Fatalf("parsed=%v err=%v", parsed, err)
		}
	})
}
