package observability_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sudoStream/internal/observability"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func TestRequestLogger_SetsRequestIDHeader(t *testing.T) {
	t.Parallel()

	allure.Test(t, "RequestLogger adds X-Request-Id header", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router := gin.New()
		router.Use(observability.RequestLogger())
		router.GET("/api/health", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/health",
			nil,
		)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d", recorder.Code)
		}
		if recorder.Header().Get("X-Request-Id") == "" {
			t.Fatal("expected X-Request-Id header")
		}
	})
}

func TestLogAction_DoesNotPanic(t *testing.T) {
	t.Parallel()

	allure.Test(t, "LogAction emits structured operation log", func(_ *allure.Context) {
		gin.SetMode(gin.TestMode)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/playback/movies/foo.mp4",
			nil,
		)
		ctx.Set(observability.RequestIDKey, "req-123")
		ctx.Set(observability.UserIDKey, "user-1")

		observability.LogAction(ctx, "media.playback")
	})
}

func TestCountingResponseWriter_TracksBytes(t *testing.T) {
	t.Parallel()

	allure.Test(t, "CountingResponseWriter accumulates payload bytes", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		writer := &observability.CountingResponseWriter{ResponseWriter: ctx.Writer}

		_, err := writer.Write([]byte("hello"))
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		if writer.BytesWritten() != 5 {
			t.Fatalf("bytes written: got %d want 5", writer.BytesWritten())
		}
	})
}
