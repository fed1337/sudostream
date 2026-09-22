package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/favorite"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

var errTestFavoriteHandler = errors.New("favorite handler error")

func newFavoriteHandlerFixture(t *testing.T) (*handler, string) {
	t.Helper()

	root := t.TempDir()
	rel := playTestSampleRel
	abs := filepath.Join(root, rel)
	err := os.MkdirAll(filepath.Dir(abs), 0o750)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err = os.WriteFile(abs, []byte("fake-video"), playTestFileMode)
	if err != nil {
		t.Fatalf("write video: %v", err)
	}

	media, err := mediafs.New(root)
	if err != nil {
		t.Fatalf("media service: %v", err)
	}

	accessSvc := access.NewService(watchHandlerAccessStore{})
	store := &flagMemoryStore{rows: map[string]bool{}}
	favSvc := favorite.NewService(media, accessSvc, store)

	return &handler{media: media, favorite: favSvc}, rel
}

func favoriteAdminUser() auth.PublicUser {
	return auth.PublicUser{
		ID:    watchTestUserID,
		Email: watchTestUserEmail,
		Role:  auth.RoleAdmin,
	}
}

func serveFavorite(
	hdl *handler,
	method, rel string,
	body []byte,
	user *auth.PublicUser,
	call func(*handler, *gin.Context),
) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	ctx.Request = httptest.NewRequestWithContext(
		context.Background(),
		method,
		"/api/favorite/"+rel,
		reader,
	)
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: rel}}
	if user != nil {
		setTestUser(ctx, *user)
	}
	call(hdl, ctx)

	return recorder
}

func TestFavoriteHandlers_GetPatchAuthAndErrors(t *testing.T) { //nolint:cyclop,funlen
	t.Parallel()

	allure.Test(
		t,
		"favorite GET/PATCH, auth, unavailable, and error mapping",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			user := favoriteAdminUser()
			hdl, rel := newFavoriteHandlerFixture(t)

			got := serveFavorite(hdl, http.MethodGet, rel, nil, &user, (*handler).getFavorite)
			if got.Code != http.StatusOK {
				t.Fatalf("get: %d %s", got.Code, got.Body.String())
			}
			var state FavoriteResponse
			err := json.Unmarshal(got.Body.Bytes(), &state)
			if err != nil || state.Favorited {
				t.Fatalf("initial: %+v err=%v", state, err)
			}

			payload, err := json.Marshal(FavoritePatchRequest{Favorited: true})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			patched := serveFavorite(
				hdl,
				http.MethodPatch,
				rel,
				payload,
				&user,
				(*handler).patchFavorite,
			)
			if patched.Code != http.StatusOK {
				t.Fatalf("patch: %d %s", patched.Code, patched.Body.String())
			}
			err = json.Unmarshal(patched.Body.Bytes(), &state)
			if err != nil || !state.Favorited {
				t.Fatalf("patched: %+v err=%v", state, err)
			}

			unauth := serveFavorite(hdl, http.MethodGet, rel, nil, nil, (*handler).getFavorite)
			if unauth.Code != http.StatusUnauthorized {
				t.Fatalf("get unauth: %d", unauth.Code)
			}
			unauthPatch := serveFavorite(
				hdl,
				http.MethodPatch,
				rel,
				payload,
				nil,
				(*handler).patchFavorite,
			)
			if unauthPatch.Code != http.StatusUnauthorized {
				t.Fatalf("patch unauth: %d", unauthPatch.Code)
			}

			nilHdl := &handler{}
			if serveFavorite(
				nilHdl,
				http.MethodGet,
				rel,
				nil,
				&user,
				(*handler).getFavorite,
			).Code !=
				http.StatusServiceUnavailable {
				t.Fatal("expected get 503")
			}
			if serveFavorite(
				nilHdl,
				http.MethodPatch,
				rel,
				payload,
				&user,
				(*handler).patchFavorite,
			).Code !=
				http.StatusServiceUnavailable {
				t.Fatal("expected patch 503")
			}

			bad := serveFavorite(
				hdl,
				http.MethodPatch,
				rel,
				[]byte("{"),
				&user,
				(*handler).patchFavorite,
			)
			if bad.Code != http.StatusBadRequest {
				t.Fatalf("bad json: %d", bad.Code)
			}

			for _, domainErr := range []error{
				favorite.ErrUnknownLibrary,
				mediafs.ErrInvalidPath,
				mediafs.ErrPathOutsideRoot,
			} {
				rec := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(rec)
				ctx.Request = httptest.NewRequestWithContext(
					context.Background(),
					http.MethodGet,
					"/api/favorite/"+rel,
					nil,
				)
				nilHdl.handleFavoriteError(ctx, rel, domainErr)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("%v: %d", domainErr, rec.Code)
				}
			}

			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPatch,
				"/api/favorite/"+rel,
				nil,
			)
			nilHdl.handleFavoriteError(ctx, rel, errTestFavoriteHandler)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("unexpected: %d", rec.Code)
			}
		},
	)
}
