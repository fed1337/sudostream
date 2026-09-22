package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sudoStream/internal/access"
	"sudoStream/internal/maintenance"
	"sudoStream/internal/transcode"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

var errMaintStub = errors.New("maint stub boom")

type stubMaintenanceRunner struct {
	global       []maintenance.ActionStatus
	library      []maintenance.ActionStatus
	runs         []maintenance.Run
	runResult    maintenance.Run
	patchGlobal  error
	patchLibrary error
	runErr       error
	listErr      error
	globalErr    error
	libraryErr   error
}

func (s *stubMaintenanceRunner) GetGlobalMaintenance(
	_ context.Context,
) ([]maintenance.ActionStatus, error) {
	if s.globalErr != nil {
		return nil, s.globalErr
	}

	return s.global, nil
}

func (s *stubMaintenanceRunner) GetLibraryMaintenance(
	_ context.Context,
	_ string,
) ([]maintenance.ActionStatus, error) {
	if s.libraryErr != nil {
		return nil, s.libraryErr
	}

	return s.library, nil
}

func (s *stubMaintenanceRunner) PatchGlobalSchedules(
	_ context.Context,
	_ []maintenance.ScheduleInput,
) error {
	return s.patchGlobal
}

func (s *stubMaintenanceRunner) PatchLibrarySchedules(
	_ context.Context,
	_ string,
	_ []maintenance.ScheduleInput,
) error {
	return s.patchLibrary
}

func (s *stubMaintenanceRunner) RunNow(
	_ context.Context,
	_, _, _ string,
) (maintenance.Run, error) {
	if s.runErr != nil {
		return maintenance.Run{}, s.runErr
	}

	return s.runResult, nil
}

func (s *stubMaintenanceRunner) ListRuns(_ context.Context, _ int) ([]maintenance.Run, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}

	return s.runs, nil
}

func (s *stubMaintenanceRunner) CacheDirBytes() int64 {
	return 1234
}

type maintAccessStore struct {
	library access.Library
	getErr  error
}

func (s maintAccessStore) ListLibraries(_ context.Context) ([]access.Library, error) {
	return []access.Library{s.library}, nil
}

func (s maintAccessStore) GetLibrary(_ context.Context, libraryID string) (access.Library, error) {
	if s.getErr != nil {
		return access.Library{}, s.getErr
	}
	if libraryID == s.library.ID {
		return s.library, nil
	}

	return access.Library{}, access.ErrLibraryNotFound
}

func (s maintAccessStore) UpsertLibrary(
	_ context.Context,
	library access.Library,
) (access.Library, error) {
	return library, nil
}

func (s maintAccessStore) CreateLibrary(
	ctx context.Context,
	library access.Library,
) (access.Library, error) {
	return s.UpsertLibrary(ctx, library)
}

func (s maintAccessStore) AddRoot(
	ctx context.Context,
	libraryID, _ string,
) (access.Library, error) {
	return s.GetLibrary(ctx, libraryID)
}

func (s maintAccessStore) RemoveRoot(
	ctx context.Context,
	libraryID, _ string,
) (access.Library, error) {
	return s.GetLibrary(ctx, libraryID)
}

func (maintAccessStore) DeleteLibrary(_ context.Context, _ string) error { return nil }

func (s maintAccessStore) UpdateLibrary(
	ctx context.Context,
	libraryID string,
	name *string,
	libraryType *access.LibraryType,
) (access.Library, error) {
	library, err := s.GetLibrary(ctx, libraryID)
	if err != nil {
		return access.Library{}, err
	}
	if name != nil {
		library.Name = *name
	}
	if libraryType != nil {
		library.Type = *libraryType
	}

	return library, nil
}

func (maintAccessStore) GetUserGrantMap(
	_ context.Context,
	_ string,
) (map[string]access.LibraryPermissions, error) {
	return map[string]access.LibraryPermissions{}, nil
}

func (maintAccessStore) ReplaceUserGrants(
	_ context.Context,
	_ string,
	_ []access.GrantInput,
) error {
	return nil
}

type memoryTranscodeSettings struct {
	settings transcode.TranscodeSettings
	getErr   error
	saveErr  error
}

func (m *memoryTranscodeSettings) GetTranscodeSettings(
	_ context.Context,
) (transcode.TranscodeSettings, error) {
	if m.getErr != nil {
		return transcode.TranscodeSettings{}, m.getErr
	}

	return m.settings, nil
}

func (m *memoryTranscodeSettings) SaveTranscodeSettings(
	_ context.Context,
	settings transcode.TranscodeSettings,
) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.settings = settings

	return nil
}

//nolint:cyclop,funlen,goconst // multi-branch maintenance handler coverage
func TestMaintenanceHandlers_CoverBranches(t *testing.T) {
	t.Parallel()

	allure.Test(t, "global get/patch/run/list happy and error paths", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		now := time.Now().UTC()
		stub := &stubMaintenanceRunner{
			global: []maintenance.ActionStatus{{
				Action: maintenance.ActionLibrariesScan,
			}},
			runs: []maintenance.Run{
				{
					ID:        "run-1",
					Action:    maintenance.ActionLibrariesScan,
					Status:    maintenance.StatusSuccess,
					StartedAt: now,
				},
			},
			runResult: maintenance.Run{
				ID:        "run-2",
				Action:    maintenance.ActionLibrariesScan,
				Status:    maintenance.StatusRunning,
				StartedAt: now,
			},
		}
		admin := &adminHandler{maintenance: stub}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		admin.getMaintenance(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("get: %d", recorder.Code)
		}

		payload, _ := json.Marshal(PatchMaintenanceRequest{Schedules: []maintenance.ScheduleInput{{
			Action: maintenance.ActionLibrariesScan, Cron: "0 * * * *", Enabled: true,
		}}})
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/",
			bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		admin.patchMaintenance(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("patch: %d %s", recorder.Code, recorder.Body.String())
		}

		stub.patchGlobal = maintenance.ErrInvalidCron
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/",
			bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		admin.patchMaintenance(ctx)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("want 400 invalid cron, got %d", recorder.Code)
		}

		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/",
			nil,
		)
		ctx.Params = gin.Params{{Key: "action", Value: maintenance.ActionLibrariesScan}}
		admin.runMaintenance(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("run: %d", recorder.Code)
		}

		stub.runErr = maintenance.ErrConflict
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/",
			nil,
		)
		ctx.Params = gin.Params{{Key: "action", Value: maintenance.ActionLibrariesScan}}
		admin.runMaintenance(ctx)
		if recorder.Code != http.StatusConflict {
			t.Fatalf("want 409, got %d", recorder.Code)
		}

		stub.runErr = maintenance.ErrLibraryRequired
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/",
			nil,
		)
		ctx.Params = gin.Params{{Key: "action", Value: maintenance.ActionMetadataScan}}
		admin.runMaintenance(ctx)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("want 400 library required, got %d", recorder.Code)
		}

		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/?limit=10",
			nil,
		)
		admin.listMaintenanceRuns(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("list: %d", recorder.Code)
		}
	})

	allure.Test(t, "library maintenance and nil service", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		library := access.Library{
			ID:      "lib-1",
			Slug:    playTestLibraryPath,
			RelPath: playTestLibraryPath,
			Name:    playTestLibraryName,
		}
		stub := &stubMaintenanceRunner{
			library: []maintenance.ActionStatus{{Action: maintenance.ActionMetadataScan}},
		}
		admin := &adminHandler{
			maintenance: stub,
			access:      access.NewService(maintAccessStore{library: library}),
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		ctx.Params = gin.Params{{Key: "id", Value: library.ID}}
		admin.getLibraryMaintenance(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("get library: %d", recorder.Code)
		}

		payload, _ := json.Marshal(PatchMaintenanceRequest{Schedules: []maintenance.ScheduleInput{{
			Action: maintenance.ActionMetadataScan, Cron: "0 2 * * *", Enabled: true,
		}}})
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/",
			bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "id", Value: library.ID}}
		admin.patchLibraryMaintenance(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("patch library: %d %s", recorder.Code, recorder.Body.String())
		}

		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		ctx.Params = gin.Params{{Key: "id", Value: "missing"}}
		admin.getLibraryMaintenance(ctx)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", recorder.Code)
		}

		nilAdmin := &adminHandler{}
		for _, call := range []func(*gin.Context){
			nilAdmin.getMaintenance,
			nilAdmin.patchMaintenance,
			nilAdmin.getLibraryMaintenance,
			nilAdmin.patchLibraryMaintenance,
			nilAdmin.runMaintenance,
			nilAdmin.listMaintenanceRuns,
		} {
			recorder = httptest.NewRecorder()
			ctx, _ = gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/",
				bytes.NewReader([]byte(`{}`)),
			)
			ctx.Request.Header.Set("Content-Type", "application/json")
			call(ctx)
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("nil maint want 503, got %d", recorder.Code)
			}
		}

		_ = errMaintStub
	})
}

//nolint:funlen // syncLibrary + maintenance edge coverage
func TestAdminHandler_SyncLibraryAndMaintEdges(t *testing.T) {
	t.Parallel()

	allure.Test(t, "syncLibrary uses maintenance RunNow and conflict", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		library := access.Library{
			ID:      "lib-1",
			Slug:    playTestLibraryPath,
			RelPath: playTestLibraryPath,
			Name:    playTestLibraryName,
		}
		stub := &stubMaintenanceRunner{
			runResult: maintenance.Run{ID: "r1", Action: maintenance.ActionMetadataScan},
		}
		admin := &adminHandler{
			access:      access.NewService(maintAccessStore{library: library}),
			maintenance: stub,
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/",
			nil,
		)
		ctx.Params = gin.Params{{Key: "id", Value: library.ID}}
		admin.syncLibrary(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("sync ok: %d %s", recorder.Code, recorder.Body.String())
		}

		stub.runErr = maintenance.ErrConflict
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/",
			nil,
		)
		ctx.Params = gin.Params{{Key: "id", Value: library.ID}}
		admin.syncLibrary(ctx)
		if recorder.Code != http.StatusConflict {
			t.Fatalf("want 409, got %d", recorder.Code)
		}

		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/",
			nil,
		)
		ctx.Params = gin.Params{{Key: "id", Value: "missing"}}
		admin.syncLibrary(ctx)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", recorder.Code)
		}
	})

	allure.Test(t, "library maintenance edge branches", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		library := access.Library{
			ID:      "lib-1",
			Slug:    playTestLibraryPath,
			RelPath: playTestLibraryPath,
			Name:    playTestLibraryName,
		}
		stub := &stubMaintenanceRunner{
			library: []maintenance.ActionStatus{{Action: maintenance.ActionMetadataScan}},
		}
		admin := &adminHandler{
			access:      access.NewService(maintAccessStore{library: library}),
			maintenance: stub,
		}

		adminNoAccess := &adminHandler{maintenance: stub}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		ctx.Params = gin.Params{{Key: "id", Value: library.ID}}
		adminNoAccess.getLibraryMaintenance(ctx)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("nil access get want 503, got %d", recorder.Code)
		}
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/",
			bytes.NewReader([]byte(`{}`)),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "id", Value: library.ID}}
		adminNoAccess.patchLibraryMaintenance(ctx)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("nil access patch want 503, got %d", recorder.Code)
		}

		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/",
			bytes.NewReader([]byte(`{`)),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "id", Value: library.ID}}
		admin.patchLibraryMaintenance(ctx)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("bad json want 400, got %d", recorder.Code)
		}

		stub.patchLibrary = maintenance.ErrInvalidAction
		payload, _ := json.Marshal(PatchMaintenanceRequest{Schedules: []maintenance.ScheduleInput{{
			Action: "nope", Cron: "0 * * * *", Enabled: true,
		}}})
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/",
			bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "id", Value: library.ID}}
		admin.patchLibraryMaintenance(ctx)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid action want 400, got %d", recorder.Code)
		}

		stub.listErr = errMaintStub
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		admin.listMaintenanceRuns(ctx)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("list err want 500, got %d", recorder.Code)
		}

		stub.patchGlobal = nil
		stub.globalErr = errMaintStub
		payload, _ = json.Marshal(PatchMaintenanceRequest{Schedules: []maintenance.ScheduleInput{{
			Action: maintenance.ActionLibrariesScan, Cron: "0 * * * *", Enabled: true,
		}}})
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/",
			bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		admin.patchMaintenance(ctx)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("reload after patch want 500, got %d", recorder.Code)
		}
	})
}

func TestAdminHandler_TranscodeSettings(t *testing.T) {
	t.Parallel()

	allure.Test(t, "get and patch transcode settings branches", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		admin := &adminHandler{}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		admin.getTranscodeSettings(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("nil store get: %d", recorder.Code)
		}

		store := &memoryTranscodeSettings{settings: transcode.DefaultTranscodeSettings()}
		admin.transcodeSettings = store
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		admin.getTranscodeSettings(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("get: %d", recorder.Code)
		}

		store.getErr = errMaintStub
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		admin.getTranscodeSettings(ctx)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("get err want 500, got %d", recorder.Code)
		}

		admin.transcodeSettings = nil
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/",
			bytes.NewReader([]byte(`{"hwAccel":"off"}`)),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		admin.patchTranscodeSettings(ctx)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("nil patch want 500, got %d", recorder.Code)
		}

		admin.transcodeSettings = store
		store.getErr = nil
		defaults := transcode.DefaultTranscodeSettings()
		defaults.HwAccel = transcode.HwAccelNVENC
		payload, _ := json.Marshal(defaults)
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/",
			bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		admin.patchTranscodeSettings(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("patch: %d %s", recorder.Code, recorder.Body.String())
		}

		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/",
			bytes.NewReader([]byte(`{"hwAccel":"nope"}`)),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		admin.patchTranscodeSettings(ctx)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid accel want 400, got %d", recorder.Code)
		}
	})
}
