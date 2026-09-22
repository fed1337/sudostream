package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sudoStream/internal/auth"
	"sudoStream/internal/usersub"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

const (
	userSubTestRel  = "movies/sample.mkv"
	userSubAltRel   = "movies/other.mkv"
	userSubTestVTT  = "WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nhello\n"
	userSubTestUser = "user-sub-1"
)

func newUserSubtitleHandler(t *testing.T) (*handler, *usersub.Service) {
	t.Helper()

	svc, err := usersub.NewService(filepath.Join(t.TempDir(), "user-subs"))
	if err != nil {
		t.Fatalf("usersub: %v", err)
	}

	return &handler{usersub: svc}, svc
}

func userSubAdmin() auth.PublicUser {
	return auth.PublicUser{
		ID:    userSubTestUser,
		Email: "sub@example.com",
		Role:  auth.RoleAdmin,
	}
}

// serveUserSubtitleRoute uses a real Gin engine so Status(204) is recorded.
func serveUserSubtitleRoute(
	hdl *handler,
	method, rel string,
	user *auth.PublicUser,
	body *bytes.Buffer,
	contentType string,
) *httptest.ResponseRecorder {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if user != nil {
			setTestUser(c, *user)
		}
		c.Next()
	})
	router.PUT("/api/user-subtitle/*path", hdl.putUserSubtitle)
	router.GET("/api/user-subtitle/*path", hdl.getUserSubtitle)
	router.DELETE("/api/user-subtitle/*path", hdl.deleteUserSubtitle)

	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body.Bytes())
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequestWithContext(
		context.Background(),
		method,
		"/api/user-subtitle/"+rel,
		reader,
	)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	return recorder
}

func multipartSubtitle(t *testing.T, filename, lang, label string, content []byte) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	_, err = part.Write(content)
	if err != nil {
		t.Fatalf("write part: %v", err)
	}
	if lang != "" {
		_ = writer.WriteField("lang", lang)
	}
	if label != "" {
		_ = writer.WriteField("label", label)
	}
	err = writer.Close()
	if err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	return body, writer.FormDataContentType()
}

func TestUserSubtitleHandlers_PutGetDeleteHappyPath(t *testing.T) {
	t.Parallel()

	allure.Test(t, "user subtitle PUT/GET/DELETE round-trip", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		hdl, _ := newUserSubtitleHandler(t)
		user := userSubAdmin()

		body, contentType := multipartSubtitle(
			t,
			"cap.vtt",
			"en",
			"English",
			[]byte(userSubTestVTT),
		)
		put := serveUserSubtitleRoute(
			hdl,
			http.MethodPut,
			userSubTestRel,
			&user,
			body,
			contentType,
		)
		if put.Code != http.StatusOK {
			t.Fatalf("put: %d %s", put.Code, put.Body.String())
		}
		var track UserSubtitleTrack
		err := json.Unmarshal(put.Body.Bytes(), &track)
		if err != nil || track.Lang != "en" || track.Label != "English" || track.URL == "" {
			t.Fatalf("track: %+v err=%v", track, err)
		}

		got := serveUserSubtitleRoute(hdl, http.MethodGet, userSubTestRel, &user, nil, "")
		if got.Code != http.StatusOK {
			t.Fatalf("get: %d %s", got.Code, got.Body.String())
		}
		if !bytes.Contains(got.Body.Bytes(), []byte("WEBVTT")) {
			t.Fatalf("get body missing WEBVTT: %q", got.Body.String())
		}

		del := serveUserSubtitleRoute(hdl, http.MethodDelete, userSubTestRel, &user, nil, "")
		if del.Code != http.StatusNoContent {
			t.Fatalf("delete: %d", del.Code)
		}

		missing := serveUserSubtitleRoute(hdl, http.MethodGet, userSubTestRel, &user, nil, "")
		if missing.Code != http.StatusNotFound {
			t.Fatalf("get after delete: %d", missing.Code)
		}
	})
}

func TestUserSubtitleHandlers_AuthAndUnavailable(t *testing.T) {
	t.Parallel()

	allure.Test(t, "user subtitle auth and unwired service responses", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		user := userSubAdmin()
		wired, _ := newUserSubtitleHandler(t)
		unwired := &handler{}

		noUser := serveUserSubtitleRoute(wired, http.MethodPut, userSubTestRel, nil, nil, "")
		if noUser.Code != http.StatusUnauthorized {
			t.Fatalf("put no user: %d", noUser.Code)
		}

		unavail := serveUserSubtitleRoute(unwired, http.MethodPut, userSubAltRel, &user, nil, "")
		if unavail.Code != http.StatusServiceUnavailable {
			t.Fatalf("put unwired: %d", unavail.Code)
		}

		getUnauth := serveUserSubtitleRoute(wired, http.MethodGet, userSubTestRel, nil, nil, "")
		if getUnauth.Code != http.StatusUnauthorized {
			t.Fatalf("get no user: %d", getUnauth.Code)
		}

		getMissingSvc := serveUserSubtitleRoute(unwired, http.MethodGet, userSubAltRel, &user, nil, "")
		if getMissingSvc.Code != http.StatusNotFound {
			t.Fatalf("get unwired: %d", getMissingSvc.Code)
		}

		delUnauth := serveUserSubtitleRoute(wired, http.MethodDelete, userSubTestRel, nil, nil, "")
		if delUnauth.Code != http.StatusUnauthorized {
			t.Fatalf("delete no user: %d", delUnauth.Code)
		}

		delUnwired := serveUserSubtitleRoute(unwired, http.MethodDelete, userSubAltRel, &user, nil, "")
		if delUnwired.Code != http.StatusNoContent {
			t.Fatalf("delete unwired: %d", delUnwired.Code)
		}
	})
}

func TestUserSubtitleHandlers_Validation(t *testing.T) {
	t.Parallel()

	allure.Test(t, "user subtitle rejects missing and invalid uploads", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		hdl, _ := newUserSubtitleHandler(t)
		user := userSubAdmin()

		missingFile := serveUserSubtitleRoute(
			hdl,
			http.MethodPut,
			userSubTestRel,
			&user,
			bytes.NewBufferString(""),
			"multipart/form-data; boundary=x",
		)
		if missingFile.Code != http.StatusBadRequest {
			t.Fatalf("missing file: %d %s", missingFile.Code, missingFile.Body.String())
		}

		body, contentType := multipartSubtitle(t, "bad.txt", "en", "", []byte("not a subtitle"))
		invalid := serveUserSubtitleRoute(hdl, http.MethodPut, userSubAltRel, &user, body, contentType)
		if invalid.Code != http.StatusBadRequest {
			t.Fatalf("invalid: %d %s", invalid.Code, invalid.Body.String())
		}

		srtBody, srtType := multipartSubtitle(
			t,
			"cap.srt",
			"",
			"",
			[]byte("1\n00:00:00,000 --> 00:00:01,000\nhi\n"),
		)
		srtOK := serveUserSubtitleRoute(hdl, http.MethodPut, userSubTestRel, &user, srtBody, srtType)
		if srtOK.Code != http.StatusOK {
			t.Fatalf("srt put: %d %s", srtOK.Code, srtOK.Body.String())
		}
	})
}

func TestUserSubtitleTrack_Helper(t *testing.T) {
	t.Parallel()

	allure.Test(t, "userSubtitleTrack returns nil or track from store", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		hdl, svc := newUserSubtitleHandler(t)
		user := userSubAdmin()

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)

		if track := (&handler{}).userSubtitleTrack(ctx, userSubTestRel); track != nil {
			t.Fatal("want nil without usersub")
		}
		if track := hdl.userSubtitleTrack(ctx, userSubTestRel); track != nil {
			t.Fatal("want nil without user")
		}

		setTestUser(ctx, user)
		if track := hdl.userSubtitleTrack(ctx, userSubAltRel); track != nil {
			t.Fatal("want nil when missing")
		}

		_, err := svc.Put(user.ID, userSubTestRel, "fr", "French", []byte(userSubTestVTT))
		if err != nil {
			t.Fatalf("put: %v", err)
		}
		track := hdl.userSubtitleTrack(ctx, userSubTestRel)
		if track == nil || track.Lang != "fr" || track.Label != "French" {
			t.Fatalf("track: %+v", track)
		}
		if got := userSubtitleURL(userSubTestRel); got == "" || got[0] != '/' {
			t.Fatalf("url: %q", got)
		}
	})
}
