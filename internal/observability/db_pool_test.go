package observability_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sudoStream/internal/db"
	"sudoStream/internal/observability"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func TestDBPoolMetrics_UpdatesPrometheusGauges(t *testing.T) {
	t.Parallel()

	allure.Test(t, "UpdateDBPoolMetrics exposes sql pool stats", func(a *allure.Context) {
		t := a.T()
		databaseURL := os.Getenv("SUDOSTREAM_DATABASE_URL")
		if databaseURL == "" {
			t.Skip("SUDOSTREAM_DATABASE_URL not set")
		}

		ctx := context.Background()
		database, err := db.Open(ctx, databaseURL, db.PoolOptionsFromEnv())
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer database.Close()

		observability.RegisterDBSQL(database.SQLDB())
		observability.UpdateDBPoolMetrics()

		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.GET("/metrics", observability.MetricsHandler())

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/metrics",
			nil,
		)
		router.ServeHTTP(recorder, request)

		body := recorder.Body.String()
		for _, metric := range []string{
			"db_sql_in_use_conns",
			"db_sql_idle_conns",
			"db_sql_open_conns",
		} {
			if !strings.Contains(body, metric) {
				t.Fatalf("expected %q in metrics output", metric)
			}
		}
	})
}
