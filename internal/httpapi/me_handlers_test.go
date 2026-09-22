package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/homeshelf"
	"sudoStream/internal/metadata"
	"sudoStream/internal/watch"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func TestHomeShelfHandlers_UnavailableAndUnauthorized(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"home shelf endpoints return 503 when unset and 401 without user",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			user := auth.PublicUser{ID: watchTestUserID, Role: auth.RoleAdmin}

			empty := &handler{}
			wired := &handler{homeShelf: homeshelf.NewService(nil, nil, nil, nil)}

			getters := []func(*handler, *gin.Context){
				(*handler).getHomeContinue,
				(*handler).getHomeFavorites,
				(*handler).getHomeWatched,
				(*handler).getHomeUnwatched,
				(*handler).getHomeStats,
			}
			paths := []string{
				"/api/me/continue",
				"/api/me/favorites",
				"/api/me/watched",
				"/api/me/unwatched",
				"/api/me/stats",
			}

			for index, getter := range getters {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequestWithContext(
					context.Background(),
					http.MethodGet,
					paths[index]+"?limit=999",
					nil,
				)
				setTestUser(ctx, user)
				getter(empty, ctx)
				if recorder.Code != http.StatusServiceUnavailable {
					t.Fatalf("%s nil: got %d", paths[index], recorder.Code)
				}

				recorder = httptest.NewRecorder()
				ctx, _ = gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequestWithContext(
					context.Background(),
					http.MethodGet,
					paths[index],
					nil,
				)
				getter(wired, ctx)
				if recorder.Code != http.StatusUnauthorized {
					t.Fatalf("%s unauth: got %d", paths[index], recorder.Code)
				}

				recorder = httptest.NewRecorder()
				ctx, _ = gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequestWithContext(
					context.Background(),
					http.MethodGet,
					paths[index],
					nil,
				)
				setTestUser(ctx, user)
				getter(wired, ctx)
				if recorder.Code != http.StatusInternalServerError {
					t.Fatalf("%s empty deps: got %d", paths[index], recorder.Code)
				}
			}
		},
	)
}

func TestHomeShelfHandlers_ContinuePaging(t *testing.T) {
	t.Parallel()

	allure.Test(t, "GET /api/me/continue returns offset and total", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		position := 40.0
		duration := 100.0
		svc := homeshelf.NewService(
			access.NewService(watchHandlerAccessStore{}),
			nil,
			meContinueWatchStore{rows: []watch.Row{
				{
					LibraryID:       watchTestLibraryID,
					RelPath:         "movies/first.mp4",
					PositionSeconds: &position,
					DurationSeconds: &duration,
				},
				{
					LibraryID:       watchTestLibraryID,
					RelPath:         "movies/second.mp4",
					PositionSeconds: &position,
					DurationSeconds: &duration,
				},
			}},
			meContinueMetaStore{items: []metadata.ShelfRow{
				{LibraryID: watchTestLibraryID, RelPath: "movies/first.mp4", Title: "First"},
				{LibraryID: watchTestLibraryID, RelPath: "movies/second.mp4", Title: "Second"},
			}},
		)
		hdl := &handler{homeShelf: svc}
		user := auth.PublicUser{ID: watchTestUserID, Role: auth.RoleAdmin}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/me/continue?limit=1&offset=1",
			nil,
		)
		setTestUser(ctx, user)
		hdl.getHomeContinue(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status: %d %s", recorder.Code, recorder.Body.String())
		}

		var body HomeItemsResponse
		err := json.Unmarshal(recorder.Body.Bytes(), &body)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Total != 2 || body.Limit != 1 || body.Offset != 1 ||
			len(body.Items) != 1 || body.Items[0].Title != "Second" {
			t.Fatalf("page: %#v", body)
		}
	})
}

type meContinueWatchStore struct {
	rows []watch.Row
}

func (w meContinueWatchStore) ListForUser(
	_ context.Context,
	_ string,
	_ []string,
	_, _ int,
) ([]watch.Row, error) {
	return nil, nil
}

func (w meContinueWatchStore) ListContinue(
	_ context.Context,
	_ string,
	_ []string,
	limit, offset int,
) ([]watch.Row, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || offset >= len(w.rows) {
		return []watch.Row{}, nil
	}
	end := min(offset+limit, len(w.rows))

	return w.rows[offset:end], nil
}

func (w meContinueWatchStore) CountForUser(_ context.Context, _ string, _ []string) (int64, error) {
	return 0, nil
}

func (w meContinueWatchStore) CountContinue(
	_ context.Context,
	_ string,
	_ []string,
) (int64, error) {
	return int64(len(w.rows)), nil
}

func (w meContinueWatchStore) CountByLibraries(
	_ context.Context,
	_ string,
	_ []string,
) (map[string]int64, error) {
	return map[string]int64{}, nil
}

type meContinueMetaStore struct {
	items []metadata.ShelfRow
}

func (m meContinueMetaStore) CountByLibraries(
	_ context.Context,
	_ []string,
) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (m meContinueMetaStore) ListUnwatched(
	_ context.Context,
	_ string,
	_ []string,
	_, _ int,
) ([]metadata.ShelfRow, error) {
	return nil, nil
}

func (m meContinueMetaStore) CountUnwatched(
	_ context.Context,
	_ string,
	_ []string,
) (int64, error) {
	return 0, nil
}

func (m meContinueMetaStore) ListShelfItems(
	_ context.Context,
	_ []string,
	_ []string,
) ([]metadata.ShelfRow, error) {
	return m.items, nil
}
