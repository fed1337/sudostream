package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func adminJSONRequest(
	ctx context.Context,
	t *testing.T,
	router *gin.Engine,
	method, path, token, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader([]byte(body)))
	request.Header.Set("Content-Type", "application/json")
	setBearerAuth(request, token)
	router.ServeHTTP(recorder, request)

	return recorder
}

func adminUserID(ctx context.Context, t *testing.T, router *gin.Engine, token string) string {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/admin/users", nil)
	setBearerAuth(request, token)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list users: %d %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Users []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"users"`
	}
	err := json.NewDecoder(recorder.Body).Decode(&payload)
	if err != nil {
		t.Fatalf("decode users: %v", err)
	}
	for _, user := range payload.Users {
		if user.Role == "admin" {
			return user.ID
		}
	}
	t.Fatal("no admin user found")

	return ""
}

const malformedJSONBody = `{bad`

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAdmin_ErrorBranches(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "admin endpoints return typed errors for bad input", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router, _ := newACLTestRouter(t)
		token := adminAccessToken(ctx, t, router)

		const missingUUID = "11111111-1111-1111-1111-111111111111"

		created := adminJSONRequest(ctx, t, router, http.MethodPost, "/api/admin/users", token,
			`{"email":"dup@hpserver.lan","password":"changeme123","role":"user"}`)
		if created.Code != http.StatusCreated {
			t.Fatalf("create user: %d %s", created.Code, created.Body.String())
		}

		assertAdminDeleteUserFlow(ctx, t, router, token)

		adminID := adminUserID(ctx, t, router, token)

		cases := []struct {
			name   string
			method string
			path   string
			body   string
			want   int
		}{
			{
				"create user bad body", http.MethodPost, "/api/admin/users",
				malformedJSONBody, http.StatusBadRequest,
			},
			{
				"create duplicate user", http.MethodPost, "/api/admin/users",
				`{"email":"dup@hpserver.lan","password":"changeme123","role":"user"}`,
				http.StatusConflict,
			},
			{
				"update user bad body", http.MethodPatch, "/api/admin/users/" + missingUUID,
				malformedJSONBody, http.StatusBadRequest,
			},
			{
				"disable last admin", http.MethodPatch, "/api/admin/users/" + adminID,
				`{"enabled":false}`, http.StatusForbidden,
			},
			{
				"delete missing user", http.MethodDelete, "/api/admin/users/" + missingUUID,
				"", http.StatusNotFound,
			},
			{
				"patch settings bad body", http.MethodPatch, "/api/admin/settings",
				malformedJSONBody, http.StatusBadRequest,
			},
			{
				"create invite duplicate", http.MethodPost, "/api/admin/invites",
				`{"email":"dup@hpserver.lan","role":"user"}`, http.StatusConflict,
			},
			{
				"patch library bad body", http.MethodPatch, "/api/admin/libraries/" + missingUUID,
				malformedJSONBody, http.StatusBadRequest,
			},
			{
				"patch library bad type", http.MethodPatch, "/api/admin/libraries/" + missingUUID,
				`{"type":"bogus"}`, http.StatusBadRequest,
			},
			{
				"patch library not found", http.MethodPatch, "/api/admin/libraries/" + missingUUID,
				`{"type":"series"}`, http.StatusNotFound,
			},
			{
				"put grants unknown user", http.MethodPut,
				"/api/admin/users/" + missingUUID + "/libraries",
				`{"grants":[]}`, http.StatusNotFound,
			},
		}

		for _, testCase := range cases {
			recorder := adminJSONRequest(
				ctx, t, router, testCase.method, testCase.path, token, testCase.body,
			)
			if recorder.Code != testCase.want {
				t.Fatalf("%s: expected %d, got %d %s",
					testCase.name, testCase.want, recorder.Code, recorder.Body.String())
			}
		}
	})
}

func assertAdminDeleteUserFlow(
	ctx context.Context,
	t *testing.T,
	router *gin.Engine,
	token string,
) {
	t.Helper()

	toDelete := adminJSONRequest(ctx, t, router, http.MethodPost, "/api/admin/users", token,
		`{"email":"delete-me@hpserver.lan","password":"changeme123","role":"user"}`)
	if toDelete.Code != http.StatusCreated {
		t.Fatalf("create deletable user: %d %s", toDelete.Code, toDelete.Body.String())
	}

	var deleteTarget struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	err := json.NewDecoder(toDelete.Body).Decode(&deleteTarget)
	if err != nil || deleteTarget.User.ID == "" {
		t.Fatalf("decode deletable user: %v body=%s", err, toDelete.Body.String())
	}

	deleteEnabled := adminJSONRequest(
		ctx, t, router, http.MethodDelete, "/api/admin/users/"+deleteTarget.User.ID, token, "",
	)
	if deleteEnabled.Code != http.StatusBadRequest {
		t.Fatalf(
			"delete enabled user: expected 400, got %d %s",
			deleteEnabled.Code,
			deleteEnabled.Body.String(),
		)
	}

	disabled := adminJSONRequest(
		ctx,
		t,
		router,
		http.MethodPatch,
		"/api/admin/users/"+deleteTarget.User.ID,
		token,
		`{"enabled":false}`,
	)
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable user: %d %s", disabled.Code, disabled.Body.String())
	}

	deleteDisabled := adminJSONRequest(
		ctx, t, router, http.MethodDelete, "/api/admin/users/"+deleteTarget.User.ID, token, "",
	)
	if deleteDisabled.Code != http.StatusNoContent {
		t.Fatalf(
			"delete disabled user: expected 204, got %d %s",
			deleteDisabled.Code,
			deleteDisabled.Body.String(),
		)
	}
}
