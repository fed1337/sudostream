package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func TestParsePageOpts(t *testing.T) {
	t.Parallel()

	allure.Test(t, "parsePageOpts reads and ignores invalid limit/offset", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		cases := []struct {
			name       string
			query      string
			wantLimit  int
			wantOffset int
		}{
			{name: "defaults", query: "", wantLimit: mediafs.DefaultPageLimit, wantOffset: 0},
			{name: "valid both", query: "limit=12&offset=24", wantLimit: 12, wantOffset: 24},
			{
				name: "invalid limit ignored", query: "limit=abc&offset=3",
				wantLimit: mediafs.DefaultPageLimit, wantOffset: 3,
			},
			{
				name:       "invalid offset ignored",
				query:      "limit=10&offset=xyz",
				wantLimit:  10,
				wantOffset: 0,
			},
			{
				name: "both invalid", query: "limit=nope&offset=bad",
				wantLimit: mediafs.DefaultPageLimit, wantOffset: 0,
			},
			{
				name:       "clamps max limit",
				query:      "limit=999",
				wantLimit:  mediafs.MaxPageLimit,
				wantOffset: 0,
			},
		}

		for _, testCase := range cases {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			request := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/?"+testCase.query,
				nil,
			)
			ctx.Request = request

			got := parsePageOpts(ctx)
			if got.Limit != testCase.wantLimit || got.Offset != testCase.wantOffset {
				t.Fatalf("%s: got limit=%d offset=%d, want limit=%d offset=%d",
					testCase.name, got.Limit, got.Offset, testCase.wantLimit, testCase.wantOffset)
			}
		}
	})
}

func TestParseRawPageOpts(t *testing.T) {
	t.Parallel()

	allure.Test(t, "parseRawPageOpts leaves limit unset when omitted", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/",
			nil,
		)
		ctx.Request = request

		got := parseRawPageOpts(ctx)
		if got.Limit != 0 || got.Offset != 0 {
			t.Fatalf("got limit=%d offset=%d, want 0/0", got.Limit, got.Offset)
		}
	})
}
