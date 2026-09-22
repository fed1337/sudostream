package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/transcode"
	"sudoStream/internal/trash"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func TestAdminTrash_ListRestoreDelete(t *testing.T) { //nolint:cyclop
	t.Parallel()

	allure.Test(t, "admin trash list, restore, and delete forever", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		root := t.TempDir()
		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("media: %v", err)
		}
		clip := filepath.Join(root, "movies", "clip.mp4")
		err = os.MkdirAll(filepath.Dir(clip), 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(clip, []byte("video"), 0o600)
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		svc := trash.NewService(media, newTrashMemoryStore(), nil, nil)
		admin := &adminHandler{trash: svc}
		router := gin.New()
		router.GET("/api/admin/trash", admin.listTrash)
		router.POST("/api/admin/trash/:id/restore", admin.restoreTrash)
		router.DELETE("/api/admin/trash/:id", admin.deleteTrashForever)
		router.GET("/api/admin/trash/settings", admin.getTrashSettings)

		item, err := svc.Move(context.Background(), "movies/clip.mp4", "")
		if err != nil {
			t.Fatalf("move: %v", err)
		}

		listRec := httptest.NewRecorder()
		listReq := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/admin/trash",
			nil,
		)
		router.ServeHTTP(listRec, listReq)
		if listRec.Code != http.StatusOK {
			t.Fatalf("list: %d %s", listRec.Code, listRec.Body.String())
		}
		var listed TrashListResponse
		err = json.Unmarshal(listRec.Body.Bytes(), &listed)
		if err != nil || len(listed.Items) != 1 {
			t.Fatalf("list body: %v %+v", err, listed)
		}

		restoreRec := httptest.NewRecorder()
		restoreReq := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/admin/trash/"+item.ID+"/restore",
			nil,
		)
		router.ServeHTTP(restoreRec, restoreReq)
		if restoreRec.Code != http.StatusOK {
			t.Fatalf("restore: %d %s", restoreRec.Code, restoreRec.Body.String())
		}
		_, err = os.Stat(clip)
		if err != nil {
			t.Fatalf("restored file: %v", err)
		}

		item, err = svc.Move(context.Background(), "movies/clip.mp4", "")
		if err != nil {
			t.Fatalf("move 2: %v", err)
		}
		delRec := httptest.NewRecorder()
		delReq := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodDelete,
			"/api/admin/trash/"+item.ID,
			nil,
		)
		router.ServeHTTP(delRec, delReq)
		if delRec.Code != http.StatusNoContent {
			t.Fatalf("delete forever: %d %s", delRec.Code, delRec.Body.String())
		}

		settingsRec := httptest.NewRecorder()
		settingsReq := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/admin/trash/settings",
			nil,
		)
		router.ServeHTTP(settingsRec, settingsReq)
		if settingsRec.Code != http.StatusOK {
			t.Fatalf("settings: %d", settingsRec.Code)
		}
	})
}

func TestAdminTrash_EmptyPatchUnavailableAndErrors(t *testing.T) { //nolint:cyclop,funlen
	t.Parallel()

	allure.Test(
		t,
		"admin empty trash, patch settings, and map restore errors",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			root := t.TempDir()
			media, err := mediafs.New(root)
			if err != nil {
				t.Fatalf("media: %v", err)
			}
			clip := filepath.Join(root, "movies", "clip.mp4")
			err = os.MkdirAll(filepath.Dir(clip), 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			err = os.WriteFile(clip, []byte("video"), 0o600)
			if err != nil {
				t.Fatalf("write: %v", err)
			}

			svc := trash.NewService(media, newTrashMemoryStore(), nil, &trashSettingsKV{})
			admin := &adminHandler{trash: svc}
			router := gin.New()
			router.GET("/api/admin/trash", admin.listTrash)
			router.DELETE("/api/admin/trash", admin.emptyTrash)
			router.POST("/api/admin/trash/:id/restore", admin.restoreTrash)
			router.PATCH("/api/admin/trash/settings", admin.patchTrashSettings)

			unavailable := &adminHandler{}
			unavailableRouter := gin.New()
			unavailableRouter.GET("/api/admin/trash", unavailable.listTrash)
			unavailableRouter.DELETE("/api/admin/trash", unavailable.emptyTrash)
			unavailableRouter.PATCH("/api/admin/trash/settings", unavailable.patchTrashSettings)

			unavailRec := httptest.NewRecorder()
			unavailReq := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/admin/trash",
				nil,
			)
			unavailableRouter.ServeHTTP(unavailRec, unavailReq)
			if unavailRec.Code != http.StatusServiceUnavailable {
				t.Fatalf("unavailable list: %d", unavailRec.Code)
			}

			_, err = svc.Move(context.Background(), "movies/clip.mp4", "")
			if err != nil {
				t.Fatalf("move: %v", err)
			}

			emptyRec := httptest.NewRecorder()
			emptyReq := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodDelete,
				"/api/admin/trash",
				nil,
			)
			router.ServeHTTP(emptyRec, emptyReq)
			if emptyRec.Code != http.StatusNoContent {
				t.Fatalf("empty: %d %s", emptyRec.Code, emptyRec.Body.String())
			}

			patchRec := httptest.NewRecorder()
			patchReq := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPatch,
				"/api/admin/trash/settings",
				bytes.NewBufferString(`{"retentionDays":14}`),
			)
			patchReq.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(patchRec, patchReq)
			if patchRec.Code != http.StatusOK {
				t.Fatalf("patch: %d %s", patchRec.Code, patchRec.Body.String())
			}

			invalidRec := httptest.NewRecorder()
			invalidReq := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPatch,
				"/api/admin/trash/settings",
				bytes.NewBufferString(`{"retentionDays":-3}`),
			)
			invalidReq.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(invalidRec, invalidReq)
			if invalidRec.Code != http.StatusBadRequest {
				t.Fatalf("invalid retention: %d", invalidRec.Code)
			}

			missingRec := httptest.NewRecorder()
			missingReq := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPost,
				"/api/admin/trash/missing/restore",
				nil,
			)
			router.ServeHTTP(missingRec, missingReq)
			if missingRec.Code != http.StatusNotFound {
				t.Fatalf("missing restore: %d", missingRec.Code)
			}

			cleanup := trashCleanup{}
			err = cleanup.OnPermanentDelete(context.Background(), trash.Item{
				OriginalRelPath: "movies/clip.mp4",
				TrashRelPath:    ".trash/id/movies/clip.mp4",
			})
			if err != nil {
				t.Fatalf("cleanup: %v", err)
			}
		},
	)
}

func TestAdminTrash_ErrorMappingConflictAndCleanup(t *testing.T) { //nolint:cyclop,funlen,gocognit
	t.Parallel()

	allure.Test(
		t,
		"trash restore conflict, error mapping, and cache cleanup",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			root := t.TempDir()
			media, err := mediafs.New(root)
			if err != nil {
				t.Fatalf("media: %v", err)
			}
			clip := filepath.Join(root, "movies", "clip.mp4")
			err = os.MkdirAll(filepath.Dir(clip), 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			err = os.WriteFile(clip, []byte("video"), 0o600)
			if err != nil {
				t.Fatalf("write: %v", err)
			}

			svc := trash.NewService(media, newTrashMemoryStore(), nil, &trashSettingsKV{})
			admin := &adminHandler{trash: svc}
			router := gin.New()
			router.POST("/api/admin/trash/:id/restore", admin.restoreTrash)
			router.PATCH("/api/admin/trash/settings", admin.patchTrashSettings)

			unavailable := &adminHandler{}
			unavailableRouter := gin.New()
			unavailableRouter.POST("/api/admin/trash/:id/restore", unavailable.restoreTrash)
			unavailableRouter.DELETE("/api/admin/trash/:id", unavailable.deleteTrashForever)
			unavailableRouter.DELETE("/api/admin/trash", unavailable.emptyTrash)
			unavailableRouter.PATCH("/api/admin/trash/settings", unavailable.patchTrashSettings)

			unavailRec := httptest.NewRecorder()
			unavailReq := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPost,
				"/api/admin/trash/id/restore",
				nil,
			)
			unavailableRouter.ServeHTTP(unavailRec, unavailReq)
			if unavailRec.Code != http.StatusServiceUnavailable {
				t.Fatalf("restore unavailable: %d", unavailRec.Code)
			}

			item, err := svc.Move(context.Background(), "movies/clip.mp4", "")
			if err != nil {
				t.Fatalf("move: %v", err)
			}
			err = os.MkdirAll(filepath.Dir(clip), 0o750)
			if err != nil {
				t.Fatalf("mkdir dest: %v", err)
			}
			err = os.WriteFile(clip, []byte("taken"), 0o600)
			if err != nil {
				t.Fatalf("recreate dest: %v", err)
			}

			conflictRec := httptest.NewRecorder()
			conflictReq := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPost,
				"/api/admin/trash/"+item.ID+"/restore",
				nil,
			)
			router.ServeHTTP(conflictRec, conflictReq)
			if conflictRec.Code != http.StatusConflict {
				t.Fatalf("restore conflict: %d %s", conflictRec.Code, conflictRec.Body.String())
			}

			transSvc, err := transcode.NewService(t.TempDir())
			if err != nil {
				t.Fatalf("transcode: %v", err)
			}
			cleanup := trashCleanup{media: media, transcode: transSvc}
			err = cleanup.OnPermanentDelete(context.Background(), item)
			if err != nil {
				t.Fatalf("cleanup: %v", err)
			}

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodDelete,
				"/api/admin/trash/"+item.ID,
				nil,
			)
			admin.writeTrashError(ctx, mediafs.ErrDeleteReadOnly)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("readonly: %d", recorder.Code)
			}

			recorder = httptest.NewRecorder()
			ctx, _ = gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodDelete,
				"/api/admin/trash/"+item.ID,
				nil,
			)
			admin.writeTrashError(ctx, os.ErrInvalid)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("default err: %d", recorder.Code)
			}

			badJSON := httptest.NewRecorder()
			badReq := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPatch,
				"/api/admin/trash/settings",
				bytes.NewBufferString("{"),
			)
			badReq.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(badJSON, badReq)
			if badJSON.Code != http.StatusBadRequest {
				t.Fatalf("bad json: %d", badJSON.Code)
			}

			patchUnavail := httptest.NewRecorder()
			patchUnavailReq := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPatch,
				"/api/admin/trash/settings",
				bytes.NewBufferString(`{"retentionDays":1}`),
			)
			patchUnavailReq.Header.Set("Content-Type", "application/json")
			unavailableRouter.ServeHTTP(patchUnavail, patchUnavailReq)
			if patchUnavail.Code != http.StatusServiceUnavailable {
				t.Fatalf("patch unavailable: %d", patchUnavail.Code)
			}

			delUnavail := httptest.NewRecorder()
			delUnavailReq := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodDelete,
				"/api/admin/trash/"+item.ID,
				nil,
			)
			unavailableRouter.ServeHTTP(delUnavail, delUnavailReq)
			if delUnavail.Code != http.StatusServiceUnavailable {
				t.Fatalf("delete unavailable: %d", delUnavail.Code)
			}

			emptyRec := httptest.NewRecorder()
			emptyReq := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodDelete,
				"/api/admin/trash",
				nil,
			)
			unavailableRouter.ServeHTTP(emptyRec, emptyReq)
			if emptyRec.Code != http.StatusServiceUnavailable {
				t.Fatalf("empty unavailable: %d", emptyRec.Code)
			}
		},
	)
}

type trashSettingsKV struct {
	raw []byte
}

func (m *trashSettingsKV) GetSettingValue(_ context.Context, _ string) ([]byte, error) {
	if len(m.raw) == 0 {
		return nil, os.ErrNotExist
	}

	return m.raw, nil
}

func (m *trashSettingsKV) SaveSettingValue(_ context.Context, _ string, value []byte) error {
	m.raw = append([]byte(nil), value...)

	return nil
}

type trashMemoryStore struct {
	items map[string]trash.Item
}

func newTrashMemoryStore() *trashMemoryStore {
	return &trashMemoryStore{items: map[string]trash.Item{}}
}

func (m *trashMemoryStore) Insert(_ context.Context, item trash.Item) error {
	m.items[item.ID] = item

	return nil
}

func (m *trashMemoryStore) Get(_ context.Context, id string) (trash.Item, error) {
	item, ok := m.items[id]
	if !ok {
		return trash.Item{}, trash.ErrNotFound
	}

	return item, nil
}

func (m *trashMemoryStore) List(_ context.Context) ([]trash.Item, error) {
	out := make([]trash.Item, 0, len(m.items))
	for _, item := range m.items {
		out = append(out, item)
	}

	return out, nil
}

func (m *trashMemoryStore) Delete(_ context.Context, id string) error {
	delete(m.items, id)

	return nil
}

func (m *trashMemoryStore) OriginalPrefixes(_ context.Context) ([]string, error) {
	out := make([]string, 0, len(m.items))
	for _, item := range m.items {
		out = append(out, item.OriginalRelPath)
	}

	return out, nil
}
