package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/catalog"
	"sudoStream/internal/favorite"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/watch"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

const (
	catalogTestLibraryID = "lib-catalog"
	catalogParamSlug     = "librarySlug"
	catalogMissingSlug   = "missing"
)

type catalogAccessStore struct {
	library access.Library
}

func (s catalogAccessStore) ListLibraries(_ context.Context) ([]access.Library, error) {
	return []access.Library{s.library}, nil
}

func (s catalogAccessStore) GetLibrary(
	_ context.Context,
	libraryID string,
) (access.Library, error) {
	if libraryID == s.library.ID {
		return s.library, nil
	}

	return access.Library{}, access.ErrLibraryNotFound
}

func (s catalogAccessStore) UpsertLibrary(
	_ context.Context,
	library access.Library,
) (access.Library, error) {
	return library, nil
}

func (s catalogAccessStore) CreateLibrary(
	ctx context.Context,
	library access.Library,
) (access.Library, error) {
	return s.UpsertLibrary(ctx, library)
}

func (s catalogAccessStore) AddRoot(
	ctx context.Context,
	libraryID, _ string,
) (access.Library, error) {
	return s.GetLibrary(ctx, libraryID)
}

func (s catalogAccessStore) RemoveRoot(
	ctx context.Context,
	libraryID, _ string,
) (access.Library, error) {
	return s.GetLibrary(ctx, libraryID)
}

func (catalogAccessStore) DeleteLibrary(_ context.Context, _ string) error { return nil }

func (s catalogAccessStore) UpdateLibrary(
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

func (catalogAccessStore) GetUserGrantMap(
	_ context.Context,
	_ string,
) (map[string]access.LibraryPermissions, error) {
	return map[string]access.LibraryPermissions{}, nil
}

func (catalogAccessStore) ReplaceUserGrants(
	_ context.Context,
	_ string,
	_ []access.GrantInput,
) error {
	return nil
}

func newCatalogHandlerFixture(
	t *testing.T,
	libraryType access.LibraryType,
) (*handler, string, string) {
	t.Helper()

	root := t.TempDir()
	slug := playTestLibraryPath
	if libraryType == access.LibraryTypeSeries {
		slug = "series"
	}
	libDir := filepath.Join(root, slug)
	err := os.MkdirAll(libDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	var sampleRel string
	if libraryType == access.LibraryTypeFilm {
		sampleRel = slug + "/Casino.1995.mkv"
		err = os.WriteFile(filepath.Join(root, sampleRel), []byte("x"), 0o600)
	} else {
		seasonDir := filepath.Join(libDir, "Show Season 1")
		err = os.MkdirAll(seasonDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir season: %v", err)
		}
		sampleRel = slug + "/Show Season 1/Show S01E01 Pilot.mkv"
		err = os.WriteFile(filepath.Join(root, sampleRel), []byte("x"), 0o600)
	}
	if err != nil {
		t.Fatalf("write sample: %v", err)
	}

	media, err := mediafs.New(root)
	if err != nil {
		t.Fatalf("media: %v", err)
	}

	library := access.Library{
		ID:      catalogTestLibraryID,
		Slug:    slug,
		RelPath: slug,
		Name:    slug,
		Type:    libraryType,
	}
	accessSvc := access.NewService(catalogAccessStore{library: library})
	watchStore := &watchHandlerMemoryStore{rows: map[string]watch.State{}}
	watchSvc := watch.NewService(media, accessSvc, watchStore)
	favStore := &flagMemoryStore{rows: map[string]bool{}}
	favSvc := favorite.NewService(media, accessSvc, favStore)
	catalogSvc := catalog.NewService(
		catalogIndexedPaths{paths: []string{sampleRel}},
		accessSvc,
		nil,
	)

	return &handler{
		media:    media,
		access:   accessSvc,
		watch:    watchSvc,
		favorite: favSvc,
		catalog:  catalogSvc,
	}, slug, sampleRel
}

type catalogIndexedPaths struct {
	paths []string
}

func (c catalogIndexedPaths) ListIndexedPaths(
	_ context.Context,
	_ string,
) ([]string, error) {
	return append([]string(nil), c.paths...), nil
}

//nolint:cyclop,funlen // multi-scenario catalog handler coverage
func TestCatalogHandlers_FilmAndErrors(t *testing.T) {
	t.Parallel()

	allure.Test(t, "GET catalog returns movies and enrich watched", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		hdl, slug, sampleRel := newCatalogHandlerFixture(t, access.LibraryTypeFilm)
		user := auth.PublicUser{ID: watchTestUserID, Role: auth.RoleAdmin}

		_, err := hdl.watch.Set(context.Background(), user.ID, sampleRel, true)
		if err != nil {
			t.Fatalf("seed watched: %v", err)
		}
		_, err = hdl.favorite.Set(context.Background(), user.ID, sampleRel, true)
		if err != nil {
			t.Fatalf("seed favorite: %v", err)
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/catalog/"+slug,
			nil,
		)
		ctx.Params = gin.Params{{Key: catalogParamSlug, Value: slug}}
		setTestUser(ctx, user)

		hdl.getLibraryCatalog(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}

		var body catalog.LibraryCatalog
		err = json.Unmarshal(recorder.Body.Bytes(), &body)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Movies) != 1 ||
			body.Movies[0].Watched == nil ||
			!*body.Movies[0].Watched ||
			body.Movies[0].Favorited == nil ||
			!*body.Movies[0].Favorited {
			t.Fatalf("movies=%+v", body.Movies)
		}
	})

	allure.Test(t, "catalog unavailable and unauthorized", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		emptyHandler := &handler{}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/catalog/x",
			nil,
		)
		setTestUser(ctx, auth.PublicUser{Role: auth.RoleAdmin})
		emptyHandler.getLibraryCatalog(ctx)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("want 503, got %d", recorder.Code)
		}

		filmHandler, slug, _ := newCatalogHandlerFixture(t, access.LibraryTypeFilm)
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/catalog/"+slug,
			nil,
		)
		ctx.Params = gin.Params{{Key: catalogParamSlug, Value: slug}}
		filmHandler.getLibraryCatalog(ctx)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", recorder.Code)
		}
	})

	allure.Test(
		t,
		"missing library returns 404; unsupported type returns 400",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			filmHandler, _, _ := newCatalogHandlerFixture(t, access.LibraryTypeFilm)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/catalog/"+catalogMissingSlug,
				nil,
			)
			ctx.Params = gin.Params{{Key: catalogParamSlug, Value: catalogMissingSlug}}
			setTestUser(ctx, auth.PublicUser{Role: auth.RoleAdmin})
			filmHandler.getLibraryCatalog(ctx)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("want 404, got %d body=%s", recorder.Code, recorder.Body.String())
			}

			photosHandler, slug, _ := newCatalogHandlerFixture(t, access.LibraryTypePhotos)
			recorder = httptest.NewRecorder()
			ctx, _ = gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/catalog/"+slug,
				nil,
			)
			ctx.Params = gin.Params{{Key: catalogParamSlug, Value: slug}}
			setTestUser(ctx, auth.PublicUser{Role: auth.RoleAdmin})
			photosHandler.getLibraryCatalog(ctx)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d body=%s", recorder.Code, recorder.Body.String())
			}
		},
	)
}

//nolint:cyclop,funlen,gocognit // multi-scenario show catalog coverage
func TestCatalogHandlers_SeriesShow(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"GET show catalog returns seasons; episodes endpoint enriches watched",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			hdl, slug, sampleRel := newCatalogHandlerFixture(t, access.LibraryTypeSeries)
			user := auth.PublicUser{ID: watchTestUserID, Role: auth.RoleAdmin}

			_, err := hdl.watch.Set(context.Background(), user.ID, sampleRel, true)
			if err != nil {
				t.Fatalf("seed watched: %v", err)
			}
			_, err = hdl.favorite.Set(context.Background(), user.ID, sampleRel, true)
			if err != nil {
				t.Fatalf("seed favorite: %v", err)
			}

			listRecorder := httptest.NewRecorder()
			listCtx, _ := gin.CreateTestContext(listRecorder)
			listCtx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/catalog/"+slug,
				nil,
			)
			listCtx.Params = gin.Params{{Key: catalogParamSlug, Value: slug}}
			setTestUser(listCtx, user)
			hdl.getLibraryCatalog(listCtx)
			if listRecorder.Code != http.StatusOK {
				t.Fatalf("list status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
			}
			var list catalog.LibraryCatalog
			err = json.Unmarshal(listRecorder.Body.Bytes(), &list)
			if err != nil || len(list.Shows) != 1 {
				t.Fatalf("list=%+v err=%v", list, err)
			}
			showKey := list.Shows[0].ShowKey

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/catalog/"+slug+"/shows/"+showKey,
				nil,
			)
			ctx.Params = gin.Params{
				{Key: catalogParamSlug, Value: slug},
				{Key: catalogParamShowKey, Value: showKey},
			}
			setTestUser(ctx, user)
			hdl.getShowCatalog(ctx)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var detail catalog.ShowDetail
			err = json.Unmarshal(recorder.Body.Bytes(), &detail)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(detail.Seasons) == 0 || detail.Seasons[0].EpisodeCount < 1 {
				t.Fatalf("detail=%+v", detail)
			}

			epRecorder := httptest.NewRecorder()
			epCtx, _ := gin.CreateTestContext(epRecorder)
			season := strconv.Itoa(detail.Seasons[0].Season)
			epCtx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/catalog/"+slug+"/shows/"+showKey+"/seasons/"+season+"/episodes",
				nil,
			)
			epCtx.Params = gin.Params{
				{Key: catalogParamSlug, Value: slug},
				{Key: catalogParamShowKey, Value: showKey},
				{Key: "season", Value: season},
			}
			setTestUser(epCtx, user)
			hdl.getShowSeasonEpisodes(epCtx)
			if epRecorder.Code != http.StatusOK {
				t.Fatalf("episodes status=%d body=%s", epRecorder.Code, epRecorder.Body.String())
			}
			var episodes catalog.SeasonEpisodes
			err = json.Unmarshal(epRecorder.Body.Bytes(), &episodes)
			if err != nil {
				t.Fatalf("decode episodes: %v", err)
			}
			if len(episodes.Episodes) == 0 ||
				episodes.Episodes[0].Watched == nil ||
				!*episodes.Episodes[0].Watched ||
				episodes.Episodes[0].Favorited == nil ||
				!*episodes.Episodes[0].Favorited {
				t.Fatalf("episodes=%+v", episodes)
			}
		},
	)

	allure.Test(t, "show catalog unavailable and missing show", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		emptyHandler := &handler{}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/catalog/x/shows/y",
			nil,
		)
		setTestUser(ctx, auth.PublicUser{Role: auth.RoleAdmin})
		emptyHandler.getShowCatalog(ctx)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("want 503, got %d", recorder.Code)
		}

		seriesHandler, slug, _ := newCatalogHandlerFixture(t, access.LibraryTypeSeries)
		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/catalog/"+slug+"/shows/"+catalogMissingSlug,
			nil,
		)
		ctx.Params = gin.Params{
			{Key: catalogParamSlug, Value: slug},
			{Key: catalogParamShowKey, Value: catalogMissingSlug},
		}
		setTestUser(ctx, auth.PublicUser{Role: auth.RoleAdmin})
		seriesHandler.getShowCatalog(ctx)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", recorder.Code)
		}

		seasonRec := httptest.NewRecorder()
		seasonCtx, _ := gin.CreateTestContext(seasonRec)
		seasonCtx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/catalog/x/shows/y/seasons/1/episodes",
			nil,
		)
		setTestUser(seasonCtx, auth.PublicUser{Role: auth.RoleAdmin})
		emptyHandler.getShowSeasonEpisodes(seasonCtx)
		if seasonRec.Code != http.StatusServiceUnavailable {
			t.Fatalf("season 503: got %d", seasonRec.Code)
		}

		badSeason := httptest.NewRecorder()
		badCtx, _ := gin.CreateTestContext(badSeason)
		badCtx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/catalog/"+slug+"/shows/show/seasons/x/episodes",
			nil,
		)
		badCtx.Params = gin.Params{
			{Key: catalogParamSlug, Value: slug},
			{Key: catalogParamShowKey, Value: "show"},
			{Key: "season", Value: "x"},
		}
		setTestUser(badCtx, auth.PublicUser{Role: auth.RoleAdmin})
		seriesHandler.getShowSeasonEpisodes(badCtx)
		if badSeason.Code != http.StatusBadRequest {
			t.Fatalf("invalid season: got %d", badSeason.Code)
		}
	})
}
