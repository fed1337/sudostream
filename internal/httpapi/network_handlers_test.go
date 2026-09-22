package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sudoStream/internal/network"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

const (
	testProxyURL      = "http://127.0.0.1:7890"
	testProxyPassword = "secret"
)

var errNetworkTestKVMissing = errors.New("missing")

type memoryNetworkKV struct {
	data map[string][]byte
}

func (m *memoryNetworkKV) GetSettingValue(_ context.Context, key string) ([]byte, error) {
	raw, ok := m.data[key]
	if !ok {
		return nil, errNetworkTestKVMissing
	}

	return raw, nil
}

func (m *memoryNetworkKV) SaveSettingValue(_ context.Context, key string, value []byte) error {
	if m.data == nil {
		m.data = map[string][]byte{}
	}
	m.data[key] = append([]byte(nil), value...)

	return nil
}

func TestAdminNetworkSettings_GetPatchKeepsPassword(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	allure.Test(t, "admin network settings get/patch keep password and never echo it", func(a *allure.Context) {
		t := a.T()
		store := network.NewKVSettingsStore(&memoryNetworkKV{})
		live := network.NewLive(network.DefaultSettings())
		admin := &adminHandler{networkSettings: store, networkLive: live}

		seed := network.Settings{
			ProxyURL:      testProxyURL,
			ProxyUser:     "u",
			ProxyPassword: testProxyPassword,
		}
		err := store.SaveSettings(context.Background(), seed)
		if err != nil {
			t.Fatal(err)
		}
		live.Store(seed)

		assertPublicGet(t, admin, testProxyURL)
		assertPatchKeepsPassword(t, admin, store, live)
	})
}

func assertPublicGet(t *testing.T, admin *adminHandler, wantURL string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/api/admin/network/settings",
		nil,
	)
	admin.getNetworkSettings(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("get status: %d", recorder.Code)
	}
	var got network.Settings
	_ = json.Unmarshal(recorder.Body.Bytes(), &got)
	if got.ProxyPassword != "" || !got.HasProxyPassword || got.ProxyURL != wantURL {
		t.Fatalf("get body: %+v", got)
	}
}

func assertPatchKeepsPassword(
	t *testing.T,
	admin *adminHandler,
	store *network.KVSettingsStore,
	live *network.Live,
) {
	t.Helper()
	payload, err := json.Marshal(network.Settings{
		ProxyURL:  testProxyURL,
		ProxyUser: "u",
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPatch,
		"/api/admin/network/settings",
		bytes.NewReader(payload),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	admin.patchNetworkSettings(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("patch status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var patched network.Settings
	_ = json.Unmarshal(recorder.Body.Bytes(), &patched)
	if patched.ProxyPassword != "" || !patched.HasProxyPassword {
		t.Fatalf("patch body: %+v", patched)
	}
	stored, err := store.GetSettings(context.Background())
	if err != nil || stored.ProxyPassword != testProxyPassword {
		t.Fatalf("stored: %+v err=%v", stored, err)
	}
	if live.Current().ProxyPassword != testProxyPassword {
		t.Fatalf("live: %+v", live.Current())
	}
}

func TestAdminNetworkSettings_TestRejectsBadScheme(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	allure.Test(t, "admin network test returns 400 for unsupported proxy scheme", func(a *allure.Context) {
		t := a.T()
		admin := &adminHandler{networkSettings: network.NewKVSettingsStore(&memoryNetworkKV{})}
		payload, _ := json.Marshal(network.Settings{ProxyURL: "vless://x"})
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/api/admin/network/settings/test",
			bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		admin.testNetworkSettings(ctx)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status: %d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}
