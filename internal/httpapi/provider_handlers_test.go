package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sudoStream/internal/access"
	"sudoStream/internal/provider"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

var errProviderStub = errors.New("provider stub boom")

const providerTestLibraryID = "lib-1"

type stubProviderService struct {
	settings provider.Settings
	getErr   error
	patchErr error
	registry *provider.Registry
}

func (s *stubProviderService) GetSettings(
	_ context.Context,
	libraryID string,
) (provider.Settings, error) {
	if s.getErr != nil {
		return provider.Settings{}, s.getErr
	}

	settings := s.settings
	settings.LibraryID = libraryID

	return settings, nil
}

func (s *stubProviderService) PatchSettings(
	_ context.Context,
	libraryID string,
	input provider.Settings,
) (provider.Settings, error) {
	if s.patchErr != nil {
		return provider.Settings{}, s.patchErr
	}

	input.LibraryID = libraryID

	return input, nil
}

func (s *stubProviderService) Registry() *provider.Registry {
	if s.registry == nil {
		return provider.NewRegistry()
	}

	return s.registry
}

func filmLibrary(id string) access.Library {
	return access.Library{ID: id, Slug: "movies", RelPath: "movies", Name: "Movies", Type: access.LibraryTypeFilm}
}

//nolint:cyclop,funlen // multi-branch handler coverage mirrors maintenance_handlers_test.go
func TestProviderHandlers_CoverBranches(t *testing.T) {
	t.Parallel()

	allure.Test(t, "get/patch happy and error paths", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		lib := filmLibrary(providerTestLibraryID)
		stub := &stubProviderService{settings: provider.DefaultSettings(lib.ID)}
		admin := &adminHandler{
			providers: stub,
			access:    access.NewService(maintAccessStore{library: lib}),
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		ctx.Params = gin.Params{{Key: "id", Value: lib.ID}}
		admin.getLibraryProviders(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("get: %d %s", recorder.Code, recorder.Body.String())
		}

		payload, _ := json.Marshal(PatchProviderSettingsRequest{
			MetadataApplyMode:   provider.ApplyModeFillMissing,
			MetadataWriteTarget: provider.WriteTargetDB,
			SubtitleLanguages:   []string{},
		})
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(), http.MethodPatch, "/", bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "id", Value: lib.ID}}
		admin.patchLibraryProviders(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("patch: %d %s", recorder.Code, recorder.Body.String())
		}

		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(), http.MethodPatch, "/", bytes.NewReader([]byte("not json")),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "id", Value: lib.ID}}
		admin.patchLibraryProviders(ctx)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("want 400 invalid body, got %d", recorder.Code)
		}

		stub.patchErr = provider.ErrInvalidMetadataProvider
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(), http.MethodPatch, "/", bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "id", Value: lib.ID}}
		admin.patchLibraryProviders(ctx)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("want 400 invalid provider, got %d", recorder.Code)
		}

		stub.patchErr = errProviderStub
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(), http.MethodPatch, "/", bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "id", Value: lib.ID}}
		admin.patchLibraryProviders(ctx)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("want 500 unknown error, got %d", recorder.Code)
		}

		stub.getErr = errProviderStub
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		ctx.Params = gin.Params{{Key: "id", Value: lib.ID}}
		admin.getLibraryProviders(ctx)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("want 500 get failure, got %d", recorder.Code)
		}
	})

	allure.Test(t, "nil service, missing library, and unsupported library type", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		admin := &adminHandler{}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		ctx.Params = gin.Params{{Key: "id", Value: providerTestLibraryID}}
		admin.getLibraryProviders(ctx)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("want 503 nil providers, got %d", recorder.Code)
		}

		lib := filmLibrary(providerTestLibraryID)
		admin = &adminHandler{providers: &stubProviderService{}}
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		ctx.Params = gin.Params{{Key: "id", Value: lib.ID}}
		admin.getLibraryProviders(ctx)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("want 503 nil access, got %d", recorder.Code)
		}

		admin = &adminHandler{
			providers: &stubProviderService{},
			access:    access.NewService(maintAccessStore{library: lib, getErr: access.ErrLibraryNotFound}),
		}
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		ctx.Params = gin.Params{{Key: "id", Value: catalogMissingSlug}}
		admin.getLibraryProviders(ctx)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("want 404 library not found, got %d", recorder.Code)
		}

		musicLib := access.Library{ID: "lib-2", Slug: "music", RelPath: "music", Type: access.LibraryTypeMusic}
		admin = &adminHandler{
			providers: &stubProviderService{},
			access:    access.NewService(maintAccessStore{library: musicLib}),
		}
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		ctx.Params = gin.Params{{Key: "id", Value: musicLib.ID}}
		admin.getLibraryProviders(ctx)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("want 400 unsupported library type, got %d", recorder.Code)
		}
	})
}
