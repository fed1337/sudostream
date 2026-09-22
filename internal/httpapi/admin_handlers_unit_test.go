package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func TestAdminHandler_NilAccessBranches(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"admin handlers return safe defaults when access is nil",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			admin := &adminHandler{access: nil}
			ctx := context.Background()

			cases := []struct {
				name       string
				call       func(*gin.Context)
				wantStatus int
			}{
				{
					"list libraries",
					admin.listLibraries,
					http.StatusOK,
				},
				{
					"get user libraries",
					func(c *gin.Context) {
						c.Params = gin.Params{{Key: "id", Value: "user-id"}}
						admin.getUserLibraries(c)
					},
					http.StatusOK,
				},
				{
					"sync libraries unavailable",
					admin.syncLibraries,
					http.StatusInternalServerError,
				},
				{
					"sync library unavailable",
					func(c *gin.Context) {
						c.Params = gin.Params{{Key: "id", Value: "library-id"}}
						admin.syncLibrary(c)
					},
					http.StatusInternalServerError,
				},
				{
					"patch library unavailable",
					func(c *gin.Context) {
						c.Params = gin.Params{{Key: "id", Value: "library-id"}}
						admin.patchLibrary(c)
					},
					http.StatusServiceUnavailable,
				},
				{
					"put grants unavailable",
					func(c *gin.Context) {
						c.Params = gin.Params{{Key: "id", Value: "user-id"}}
						admin.putUserLibraries(c)
					},
					http.StatusServiceUnavailable,
				},
			}

			for _, testCase := range cases {
				recorder := httptest.NewRecorder()
				ginCtx, _ := gin.CreateTestContext(recorder)
				ginCtx.Request = httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)

				testCase.call(ginCtx)

				if recorder.Code != testCase.wantStatus {
					t.Fatalf(
						"%s: got status %d want %d body=%s",
						testCase.name,
						recorder.Code,
						testCase.wantStatus,
						recorder.Body.String(),
					)
				}
			}
		},
	)
}
