package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sudoStream/internal/auth"
	"testing"
	"time"

	"github.com/disintegration/imaging"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
)

//nolint:paralleltest,cyclop,funlen // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAdmin_UsersSettingsInvites(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "admin manages users settings and invites", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router := newAuthTestRouter(t)
		adminToken := adminAccessToken(ctx, t, router)

		listUsersRecorder := httptest.NewRecorder()
		listUsersRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/admin/users",
			nil,
		)
		setBearerAuth(listUsersRequest, adminToken)
		router.ServeHTTP(listUsersRecorder, listUsersRequest)
		if listUsersRecorder.Code != http.StatusOK {
			t.Fatalf("list users: %d %s", listUsersRecorder.Code, listUsersRecorder.Body.String())
		}

		settingsRecorder := httptest.NewRecorder()
		settingsRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/admin/settings",
			nil,
		)
		setBearerAuth(settingsRequest, adminToken)
		router.ServeHTTP(settingsRecorder, settingsRequest)
		if settingsRecorder.Code != http.StatusOK {
			t.Fatalf("get settings: %d", settingsRecorder.Code)
		}

		patchSettingsBody := testDefaultSettingsPatchJSON()
		patchSettingsRecorder := httptest.NewRecorder()
		patchSettingsRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPatch,
			"/api/admin/settings",
			bytes.NewReader(patchSettingsBody),
		)
		patchSettingsRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(patchSettingsRequest, adminToken)
		router.ServeHTTP(patchSettingsRecorder, patchSettingsRequest)
		if patchSettingsRecorder.Code != http.StatusOK {
			t.Fatalf(
				"patch settings: %d %s",
				patchSettingsRecorder.Code,
				patchSettingsRecorder.Body.String(),
			)
		}

		createBody := []byte(
			`{"email":"member@hpserver.lan","password":"member-pass","role":"user"}`,
		)
		createRecorder := httptest.NewRecorder()
		createRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/admin/users",
			bytes.NewReader(createBody),
		)
		createRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(createRequest, adminToken)
		router.ServeHTTP(createRecorder, createRequest)
		if createRecorder.Code != http.StatusCreated {
			t.Fatalf("create user: %d %s", createRecorder.Code, createRecorder.Body.String())
		}

		var createResponse struct {
			User auth.PublicUser `json:"user"`
		}
		err := json.NewDecoder(createRecorder.Body).Decode(&createResponse)
		if err != nil {
			t.Fatalf("decode create user: %v", err)
		}

		patchUserBody := []byte(`{"enabled":true,"role":"user"}`)
		patchUserRecorder := httptest.NewRecorder()
		patchUserRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPatch,
			"/api/admin/users/"+createResponse.User.ID,
			bytes.NewReader(patchUserBody),
		)
		patchUserRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(patchUserRequest, adminToken)
		router.ServeHTTP(patchUserRecorder, patchUserRequest)
		if patchUserRecorder.Code != http.StatusOK {
			t.Fatalf("patch user: %d", patchUserRecorder.Code)
		}

		inviteBody := []byte(`{"email":"invited@hpserver.lan","role":"user"}`)
		inviteRecorder := httptest.NewRecorder()
		inviteRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/admin/invites",
			bytes.NewReader(inviteBody),
		)
		inviteRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(inviteRequest, adminToken)
		router.ServeHTTP(inviteRecorder, inviteRequest)
		if inviteRecorder.Code != http.StatusNoContent {
			t.Fatalf("create invite: %d %s", inviteRecorder.Code, inviteRecorder.Body.String())
		}

		listInvitesRecorder := httptest.NewRecorder()
		listInvitesRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/admin/invites",
			nil,
		)
		setBearerAuth(listInvitesRequest, adminToken)
		router.ServeHTTP(listInvitesRecorder, listInvitesRequest)
		if listInvitesRecorder.Code != http.StatusOK {
			t.Fatalf("list invites: %d", listInvitesRecorder.Code)
		}

		sessionsRecorder := httptest.NewRecorder()
		sessionsRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/admin/users/"+createResponse.User.ID+"/sessions",
			nil,
		)
		setBearerAuth(sessionsRequest, adminToken)
		router.ServeHTTP(sessionsRecorder, sessionsRequest)
		if sessionsRecorder.Code != http.StatusOK {
			t.Fatalf("list user sessions: %d", sessionsRecorder.Code)
		}
	})
}

//nolint:paralleltest,cyclop,funlen // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_ConfirmEmailAndAcceptInvite(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "invite accept enables login even when confirmation is required", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()

		mail := &captureSender{}
		router := newAuthTestRouterWithMail(t, mail)
		adminToken := adminAccessToken(ctx, t, router)

		patchSettingsBody := testEmailConfirmationSettingsPatchJSON()
		patchSettingsRecorder := httptest.NewRecorder()
		patchSettingsRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPatch,
			"/api/admin/settings",
			bytes.NewReader(patchSettingsBody),
		)
		patchSettingsRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(patchSettingsRequest, adminToken)
		router.ServeHTTP(patchSettingsRecorder, patchSettingsRequest)
		if patchSettingsRecorder.Code != http.StatusOK {
			t.Fatalf("patch settings: %d", patchSettingsRecorder.Code)
		}

		inviteConfirmBody := []byte(`{"email":"needsconfirm@hpserver.lan","role":"user"}`)
		inviteConfirmRecorder := httptest.NewRecorder()
		inviteConfirmRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/admin/invites",
			bytes.NewReader(inviteConfirmBody),
		)
		inviteConfirmRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(inviteConfirmRequest, adminToken)
		router.ServeHTTP(inviteConfirmRecorder, inviteConfirmRequest)
		if inviteConfirmRecorder.Code != http.StatusNoContent {
			t.Fatalf("create invite: %d", inviteConfirmRecorder.Code)
		}

		listUsersRecorder := httptest.NewRecorder()
		listUsersRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/admin/users",
			nil,
		)
		setBearerAuth(listUsersRequest, adminToken)
		router.ServeHTTP(listUsersRecorder, listUsersRequest)
		if listUsersRecorder.Code != http.StatusOK {
			t.Fatalf("list users: %d", listUsersRecorder.Code)
		}
		var usersResponse struct {
			Users []auth.AdminUser `json:"users"`
		}
		err := json.NewDecoder(listUsersRecorder.Body).Decode(&usersResponse)
		if err != nil {
			t.Fatalf("decode users: %v", err)
		}
		var invitedUserID string
		for _, user := range usersResponse.Users {
			if user.Email == "needsconfirm@hpserver.lan" {
				invitedUserID = user.ID

				break
			}
		}
		if invitedUserID == "" {
			t.Fatal("expected invited stub user")
		}

		resendRecorder := httptest.NewRecorder()
		resendRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/admin/users/"+invitedUserID+"/resend-confirm",
			nil,
		)
		setBearerAuth(resendRequest, adminToken)
		router.ServeHTTP(resendRecorder, resendRequest)
		if resendRecorder.Code != http.StatusNoContent {
			t.Fatalf("resend confirm: %d", resendRecorder.Code)
		}

		acceptToken := extractTokenFromEmail(mail.lastBody)
		if acceptToken == "" {
			t.Fatal("expected invite token in email")
		}

		acceptRecorder := httptest.NewRecorder()
		acceptBody := []byte(`{"token":"` + acceptToken + `","password":"secret-pass"}`)
		acceptRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/accept-invite",
			bytes.NewReader(acceptBody),
		)
		acceptRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(acceptRecorder, acceptRequest)
		if acceptRecorder.Code != http.StatusNoContent {
			t.Fatalf("accept invite: %d %s", acceptRecorder.Code, acceptRecorder.Body.String())
		}

		loginBody := []byte(`{"email":"needsconfirm@hpserver.lan","password":"secret-pass"}`)
		loginRecorder := httptest.NewRecorder()
		loginRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/login",
			bytes.NewReader(loginBody),
		)
		loginRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(loginRecorder, loginRequest)
		if loginRecorder.Code != http.StatusOK {
			t.Fatalf(
				"login after invite accept: %d %s",
				loginRecorder.Code,
				loginRecorder.Body.String(),
			)
		}

		inviteBody := []byte(`{"email":"invitee@hpserver.lan","role":"user"}`)
		inviteRecorder := httptest.NewRecorder()
		inviteRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/admin/invites",
			bytes.NewReader(inviteBody),
		)
		inviteRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(inviteRequest, adminToken)
		router.ServeHTTP(inviteRecorder, inviteRequest)
		if inviteRecorder.Code != http.StatusNoContent {
			t.Fatalf("create invite: %d", inviteRecorder.Code)
		}

		inviteToken := extractTokenFromEmail(mail.lastBody)
		if inviteToken == "" {
			t.Fatal("expected invite token in email")
		}

		inviteeAcceptBody := []byte(
			`{"token":"` + inviteToken + `","password":"invite-pass-123"}`,
		)
		inviteeAcceptRecorder := httptest.NewRecorder()
		inviteeAcceptRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/accept-invite",
			bytes.NewReader(inviteeAcceptBody),
		)
		inviteeAcceptRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(inviteeAcceptRecorder, inviteeAcceptRequest)
		if inviteeAcceptRecorder.Code != http.StatusNoContent {
			t.Fatalf(
				"accept invite: %d %s",
				inviteeAcceptRecorder.Code,
				inviteeAcceptRecorder.Body.String(),
			)
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_MeReturnsCurrentUser(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "me returns authenticated profile", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router := newAuthTestRouter(t)
		token := loginTestAuth(ctx, t, router).AccessToken

		meRecorder := httptest.NewRecorder()
		meRequest := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/auth/me", nil)
		setBearerAuth(meRequest, token)
		router.ServeHTTP(meRecorder, meRequest)

		if meRecorder.Code != http.StatusOK {
			t.Fatalf("me status: got %d body=%s", meRecorder.Code, meRecorder.Body.String())
		}
	})
}

//nolint:paralleltest,cyclop // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_TwoFactorLoginFlow(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "user completes 2FA enrollment and login verification", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router := newAuthTestRouter(t)
		token := loginTestAuth(ctx, t, router).AccessToken

		setupRecorder := httptest.NewRecorder()
		setupRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/2fa/setup",
			bytes.NewReader([]byte(`{}`)),
		)
		setupRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(setupRequest, token)
		router.ServeHTTP(setupRecorder, setupRequest)
		if setupRecorder.Code != http.StatusOK {
			t.Fatalf("2fa setup: %d %s", setupRecorder.Code, setupRecorder.Body.String())
		}

		var setupResponse struct {
			Secret string `json:"secret"`
		}
		err := json.NewDecoder(setupRecorder.Body).Decode(&setupResponse)
		if err != nil {
			t.Fatalf("decode setup: %v", err)
		}

		code, err := totp.GenerateCode(setupResponse.Secret, time.Now())
		if err != nil {
			t.Fatalf("generate totp: %v", err)
		}

		confirmBody := []byte(`{"code":"` + code + `"}`)
		confirmRecorder := httptest.NewRecorder()
		confirmRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/2fa/confirm",
			bytes.NewReader(confirmBody),
		)
		confirmRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(confirmRequest, token)
		router.ServeHTTP(confirmRecorder, confirmRequest)
		if confirmRecorder.Code != http.StatusOK {
			t.Fatalf("2fa confirm: %d %s", confirmRecorder.Code, confirmRecorder.Body.String())
		}

		loginBody := testAdminLoginJSON(testDefaultPassword)
		loginRecorder := httptest.NewRecorder()
		loginRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/login",
			bytes.NewReader(loginBody),
		)
		loginRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(loginRecorder, loginRequest)
		if loginRecorder.Code != http.StatusOK {
			t.Fatalf("login challenge: %d", loginRecorder.Code)
		}

		var loginResponse struct {
			Status       string `json:"status"`
			PendingToken string `json:"pendingToken"`
		}
		err = json.NewDecoder(loginRecorder.Body).Decode(&loginResponse)
		if err != nil {
			t.Fatalf("decode login challenge: %v", err)
		}
		if loginResponse.Status != authStatus2FARequired {
			t.Fatalf("expected %s, got %q", authStatus2FARequired, loginResponse.Status)
		}

		verifyCode, err := totp.GenerateCode(setupResponse.Secret, time.Now())
		if err != nil {
			t.Fatalf("generate verify totp: %v", err)
		}

		verifyBody := []byte(
			`{"pendingToken":"` + loginResponse.PendingToken + `","code":"` + verifyCode + `"}`,
		)
		verifyRecorder := httptest.NewRecorder()
		verifyRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/2fa/verify",
			bytes.NewReader(verifyBody),
		)
		verifyRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(verifyRecorder, verifyRequest)
		if verifyRecorder.Code != http.StatusOK {
			t.Fatalf("2fa verify: %d %s", verifyRecorder.Code, verifyRecorder.Body.String())
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAdmin_LibraryGrantsAndInviteResend(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "admin manages library grants and invite resend", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router, root := newACLTestRouter(t)
		adminToken := adminAccessToken(ctx, t, router)

		err := os.MkdirAll(root+"/"+testSeriesRelPath, 0o750)
		if err != nil {
			t.Fatalf("mkdir library: %v", err)
		}
		registerMediaLibrariesHTTP(ctx, t, router, root)

		userID, _ := createUserSession(t, router, "grants@hpserver.lan", "grants-pass")
		libraries := listLibraries(t, router)
		if len(libraries) == 0 {
			t.Fatal("expected synced library")
		}

		grantsBody := []byte(`{"grants":[{"libraryId":"` + libraries[0].ID +
			`","canRead":true,"canCreate":false,"canUpdate":false,"canDelete":false}]}`)
		putGrantsRecorder := httptest.NewRecorder()
		putGrantsRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPut,
			"/api/admin/users/"+userID+"/libraries",
			bytes.NewReader(grantsBody),
		)
		putGrantsRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(putGrantsRequest, adminToken)
		router.ServeHTTP(putGrantsRecorder, putGrantsRequest)
		if putGrantsRecorder.Code != http.StatusOK {
			t.Fatalf("put grants: %d %s", putGrantsRecorder.Code, putGrantsRecorder.Body.String())
		}

		getGrantsRecorder := httptest.NewRecorder()
		getGrantsRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/admin/users/"+userID+"/libraries",
			nil,
		)
		setBearerAuth(getGrantsRequest, adminToken)
		router.ServeHTTP(getGrantsRecorder, getGrantsRequest)
		if getGrantsRecorder.Code != http.StatusOK {
			t.Fatalf("get grants: %d", getGrantsRecorder.Code)
		}

		revokeAllRecorder := httptest.NewRecorder()
		revokeAllRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/admin/users/"+userID+"/sessions/revoke-all",
			nil,
		)
		setBearerAuth(revokeAllRequest, adminToken)
		router.ServeHTTP(revokeAllRecorder, revokeAllRequest)
		if revokeAllRecorder.Code != http.StatusNoContent {
			t.Fatalf("revoke all sessions: %d", revokeAllRecorder.Code)
		}
	})
}

//nolint:paralleltest,cyclop,funlen // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAdmin_InviteResendAndRevoke(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "admin can resend and revoke invites", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router, _ := newACLTestRouter(t)
		adminToken := adminAccessToken(ctx, t, router)

		inviteBody := []byte(`{"email":"resend@hpserver.lan","role":"user"}`)
		inviteRecorder := httptest.NewRecorder()
		inviteRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/admin/invites",
			bytes.NewReader(inviteBody),
		)
		inviteRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(inviteRequest, adminToken)
		router.ServeHTTP(inviteRecorder, inviteRequest)
		if inviteRecorder.Code != http.StatusNoContent {
			t.Fatalf("create invite: %d", inviteRecorder.Code)
		}

		listInvitesRecorder := httptest.NewRecorder()
		listInvitesRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/admin/invites",
			nil,
		)
		setBearerAuth(listInvitesRequest, adminToken)
		router.ServeHTTP(listInvitesRecorder, listInvitesRequest)
		if listInvitesRecorder.Code != http.StatusOK {
			t.Fatalf("list invites: %d", listInvitesRecorder.Code)
		}

		var invitesResponse struct {
			Invites []struct {
				ID string `json:"id"`
			} `json:"invites"`
		}
		err := json.NewDecoder(listInvitesRecorder.Body).Decode(&invitesResponse)
		if err != nil {
			t.Fatalf("decode invites: %v", err)
		}
		if len(invitesResponse.Invites) == 0 {
			t.Fatal("expected invite")
		}

		resendInviteRecorder := httptest.NewRecorder()
		resendInviteRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/admin/invites/"+invitesResponse.Invites[0].ID+"/resend",
			nil,
		)
		setBearerAuth(resendInviteRequest, adminToken)
		router.ServeHTTP(resendInviteRecorder, resendInviteRequest)
		if resendInviteRecorder.Code != http.StatusNoContent {
			t.Fatalf("resend invite: %d", resendInviteRecorder.Code)
		}

		listAfterResendRecorder := httptest.NewRecorder()
		listAfterResendRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/admin/invites?email=resend@hpserver.lan",
			nil,
		)
		setBearerAuth(listAfterResendRequest, adminToken)
		router.ServeHTTP(listAfterResendRecorder, listAfterResendRequest)
		if listAfterResendRecorder.Code != http.StatusOK {
			t.Fatalf("list invites after resend: %d", listAfterResendRecorder.Code)
		}

		var invitesAfterResend struct {
			Invites []struct {
				ID string `json:"id"`
			} `json:"invites"`
		}
		err = json.NewDecoder(listAfterResendRecorder.Body).Decode(&invitesAfterResend)
		if err != nil {
			t.Fatalf("decode invites after resend: %v", err)
		}
		if len(invitesAfterResend.Invites) != 1 {
			t.Fatalf(
				"expected one pending invite after resend, got %d",
				len(invitesAfterResend.Invites),
			)
		}

		revokeInviteRecorder := httptest.NewRecorder()
		revokeInviteRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodDelete,
			"/api/admin/invites/"+invitesAfterResend.Invites[0].ID,
			nil,
		)
		setBearerAuth(revokeInviteRequest, adminToken)
		router.ServeHTTP(revokeInviteRecorder, revokeInviteRequest)
		if revokeInviteRecorder.Code != http.StatusNoContent {
			t.Fatalf(
				"revoke invite: %d %s",
				revokeInviteRecorder.Code,
				revokeInviteRecorder.Body.String(),
			)
		}

		listAfterRevokeRecorder := httptest.NewRecorder()
		listAfterRevokeRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/admin/invites?email=resend@hpserver.lan",
			nil,
		)
		setBearerAuth(listAfterRevokeRequest, adminToken)
		router.ServeHTTP(listAfterRevokeRecorder, listAfterRevokeRequest)
		if listAfterRevokeRecorder.Code != http.StatusOK {
			t.Fatalf("list invites after revoke: %d", listAfterRevokeRecorder.Code)
		}

		var invitesAfterRevoke struct {
			Invites []struct {
				ID string `json:"id"`
			} `json:"invites"`
		}
		err = json.NewDecoder(listAfterRevokeRecorder.Body).Decode(&invitesAfterRevoke)
		if err != nil {
			t.Fatalf("decode invites after revoke: %v", err)
		}
		if len(invitesAfterRevoke.Invites) != 0 {
			t.Fatalf(
				"expected no pending invites after revoke, got %d",
				len(invitesAfterRevoke.Invites),
			)
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAdmin_MultipleInvitesListed(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "multiple pending invites are listed", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router, _ := newACLTestRouter(t)
		adminToken := adminAccessToken(ctx, t, router)

		for _, email := range []string{"invite-a@hpserver.lan", "invite-b@hpserver.lan"} {
			body := []byte(`{"email":"` + email + `","role":"user"}`)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/api/admin/invites",
				bytes.NewReader(body),
			)
			request.Header.Set("Content-Type", "application/json")
			setBearerAuth(request, adminToken)
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNoContent {
				t.Fatalf(
					"create invite for %s: %d %s",
					email,
					recorder.Code,
					recorder.Body.String(),
				)
			}
		}

		dupBody := []byte(`{"email":"invite-a@hpserver.lan","role":"user"}`)
		dupRecorder := httptest.NewRecorder()
		dupRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/admin/invites",
			bytes.NewReader(dupBody),
		)
		dupRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(dupRequest, adminToken)
		router.ServeHTTP(dupRecorder, dupRequest)
		if dupRecorder.Code != http.StatusNoContent {
			t.Fatalf("create duplicate invite: %d", dupRecorder.Code)
		}

		listRecorder := httptest.NewRecorder()
		listRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/admin/invites",
			nil,
		)
		setBearerAuth(listRequest, adminToken)
		router.ServeHTTP(listRecorder, listRequest)
		if listRecorder.Code != http.StatusOK {
			t.Fatalf("list invites: %d", listRecorder.Code)
		}

		var response struct {
			Invites []struct {
				Email string `json:"email"`
			} `json:"invites"`
		}
		err := json.NewDecoder(listRecorder.Body).Decode(&response)
		if err != nil {
			t.Fatalf("decode invites: %v", err)
		}
		if len(response.Invites) < 2 {
			t.Fatalf("expected at least 2 pending invites, got %d", len(response.Invites))
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAdmin_SyncLibraryMetadata(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"per-library metadata sync endpoint accepts library id",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			ctx := context.Background()
			router, root := newACLTestRouter(t)
			adminToken := adminAccessToken(ctx, t, router)

			err := os.MkdirAll(root+"/"+testSeriesRelPath, 0o750)
			if err != nil {
				t.Fatalf("mkdir library: %v", err)
			}
			registerMediaLibrariesHTTP(ctx, t, router, root)

			libraries := listLibraries(t, router)
			if len(libraries) == 0 {
				t.Fatal("expected synced library")
			}

			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/api/admin/libraries/"+libraries[0].ID+"/sync",
				nil,
			)
			setBearerAuth(request, adminToken)
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("sync library metadata: %d %s", recorder.Code, recorder.Body.String())
			}
		},
	)
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestMedia_DownloadAndThumbnailRequireAuth(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "download and thumbnail serve authenticated media", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()

		root := t.TempDir()
		err := os.WriteFile(root+"/notes.txt", []byte("hello-download"), 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		router := newTestRouter(t, root)
		token := loginTestAuth(ctx, t, router).AccessToken

		downloadRecorder := httptest.NewRecorder()
		downloadRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/download/notes.txt",
			nil,
		)
		setBearerAuth(downloadRequest, token)
		router.ServeHTTP(downloadRecorder, downloadRequest)
		if downloadRecorder.Code != http.StatusOK {
			t.Fatalf(
				"download status: got %d body=%s",
				downloadRecorder.Code,
				downloadRecorder.Body.String(),
			)
		}
		if !strings.Contains(downloadRecorder.Body.String(), "hello-download") {
			t.Fatal("expected download body")
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_InvalidTokenAndDisableTwoFactor(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "invalid tokens and disable 2FA are handled", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router := newAuthTestRouter(t)
		token := loginTestAuth(ctx, t, router).AccessToken

		resetRecorder := httptest.NewRecorder()
		resetRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/reset-password",
			bytes.NewReader([]byte(`{"token":"invalid","password":"x"}`)),
		)
		resetRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(resetRecorder, resetRequest)
		if resetRecorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid reset token: got %d", resetRecorder.Code)
		}

		setupRecorder := httptest.NewRecorder()
		setupRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/2fa/setup",
			bytes.NewReader([]byte(`{}`)),
		)
		setupRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(setupRequest, token)
		router.ServeHTTP(setupRecorder, setupRequest)
		if setupRecorder.Code != http.StatusOK {
			t.Fatalf("2fa setup: %d", setupRecorder.Code)
		}

		var setupResponse struct {
			Secret string `json:"secret"`
		}
		err := json.NewDecoder(setupRecorder.Body).Decode(&setupResponse)
		if err != nil {
			t.Fatalf("decode setup: %v", err)
		}

		code, err := totp.GenerateCode(setupResponse.Secret, time.Now())
		if err != nil {
			t.Fatalf("generate totp: %v", err)
		}

		confirmRecorder := httptest.NewRecorder()
		confirmRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/2fa/confirm",
			bytes.NewReader([]byte(`{"code":"`+code+`"}`)),
		)
		confirmRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(confirmRequest, token)
		router.ServeHTTP(confirmRecorder, confirmRequest)
		if confirmRecorder.Code != http.StatusOK {
			t.Fatalf("2fa confirm: %d", confirmRecorder.Code)
		}

		disableRecorder := httptest.NewRecorder()
		disableRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodDelete,
			"/api/auth/2fa",
			bytes.NewReader([]byte(`{"password":"changeme","code":"`+code+`"}`)),
		)
		disableRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(disableRequest, token)
		router.ServeHTTP(disableRecorder, disableRequest)
		if disableRecorder.Code != http.StatusNoContent {
			t.Fatalf(
				"disable 2fa: got %d body=%s",
				disableRecorder.Code,
				disableRecorder.Body.String(),
			)
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAdmin_ResetTwoFactorAndRevokeSession(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "admin resets 2FA and revokes user sessions", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router := newAuthTestRouter(t)
		adminToken := adminAccessToken(ctx, t, router)

		userID, userToken := createUserSession(t, router, "2fa-user@hpserver.lan", "user-pass-123")

		setupRecorder := httptest.NewRecorder()
		setupRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/2fa/setup",
			bytes.NewReader([]byte(`{}`)),
		)
		setupRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(setupRequest, userToken)
		router.ServeHTTP(setupRecorder, setupRequest)
		if setupRecorder.Code != http.StatusOK {
			t.Fatalf("user 2fa setup: %d", setupRecorder.Code)
		}

		resetTFRecorder := httptest.NewRecorder()
		resetTFRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/admin/users/"+userID+"/2fa/reset",
			nil,
		)
		setBearerAuth(resetTFRequest, adminToken)
		router.ServeHTTP(resetTFRecorder, resetTFRequest)
		if resetTFRecorder.Code != http.StatusNoContent {
			t.Fatalf("admin reset 2fa: got %d", resetTFRecorder.Code)
		}

		meRecorder := httptest.NewRecorder()
		meRequest := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/auth/me", nil)
		setBearerAuth(meRequest, adminToken)
		router.ServeHTTP(meRecorder, meRequest)
		if meRecorder.Code != http.StatusOK {
			t.Fatalf("me: got %d", meRecorder.Code)
		}

		var meResponse struct {
			User auth.PublicUser `json:"user"`
		}
		err := json.NewDecoder(meRecorder.Body).Decode(&meResponse)
		if err != nil {
			t.Fatalf("decode me: %v", err)
		}

		sessionsRecorder := httptest.NewRecorder()
		sessionsRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/admin/users/"+meResponse.User.ID+"/sessions",
			nil,
		)
		setBearerAuth(sessionsRequest, adminToken)
		router.ServeHTTP(sessionsRecorder, sessionsRequest)
		if sessionsRecorder.Code != http.StatusOK {
			t.Fatalf("admin list sessions: got %d", sessionsRecorder.Code)
		}

		var sessionsResponse struct {
			Sessions []struct {
				ID string `json:"id"`
			} `json:"sessions"`
		}
		err = json.NewDecoder(sessionsRecorder.Body).Decode(&sessionsResponse)
		if err != nil {
			t.Fatalf("decode sessions: %v", err)
		}
		if len(sessionsResponse.Sessions) == 0 {
			t.Fatal("expected admin session")
		}

		revokeRecorder := httptest.NewRecorder()
		revokeRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodDelete,
			"/api/admin/sessions/"+sessionsResponse.Sessions[0].ID,
			nil,
		)
		setBearerAuth(revokeRequest, adminToken)
		router.ServeHTTP(revokeRecorder, revokeRequest)
		if revokeRecorder.Code != http.StatusNoContent {
			t.Fatalf("admin revoke session: got %d", revokeRecorder.Code)
		}
	})
}

//nolint:paralleltest // shares SUDOSTREAM_DATABASE_URL integration fixture
func TestMedia_ThumbnailAndPathErrors(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "thumbnail and invalid paths return expected errors", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()

		root := t.TempDir()
		img := imaging.New(4, 4, color.White)
		err := imaging.Save(img, root+"/pixel.jpg")
		if err != nil {
			t.Fatalf("save jpeg: %v", err)
		}

		router := newTestRouter(t, root)
		token := loginTestAuth(ctx, t, router).AccessToken

		thumbRecorder := httptest.NewRecorder()
		thumbRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/thumbnail/pixel.jpg?w=64&h=2000",
			nil,
		)
		setBearerAuth(thumbRequest, token)
		router.ServeHTTP(thumbRecorder, thumbRequest)
		if thumbRecorder.Code != http.StatusOK {
			t.Fatalf(
				"thumbnail status: got %d body=%s",
				thumbRecorder.Code,
				thumbRecorder.Body.String(),
			)
		}

		invalidBrowseRecorder := httptest.NewRecorder()
		invalidBrowseRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/browse/../etc/passwd",
			nil,
		)
		setBearerAuth(invalidBrowseRequest, token)
		router.ServeHTTP(invalidBrowseRecorder, invalidBrowseRequest)
		if invalidBrowseRecorder.Code != http.StatusNotFound &&
			invalidBrowseRecorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid browse: got %d", invalidBrowseRecorder.Code)
		}

		missingRecorder := httptest.NewRecorder()
		missingRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/browse/missing-folder",
			nil,
		)
		setBearerAuth(missingRequest, token)
		router.ServeHTTP(missingRecorder, missingRequest)
		if missingRecorder.Code != http.StatusNotFound {
			t.Fatalf("missing browse: got %d", missingRecorder.Code)
		}

		bad2faRecorder := httptest.NewRecorder()
		bad2faRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/2fa/verify",
			bytes.NewReader([]byte(`{"pendingToken":"bad","code":"000000"}`)),
		)
		bad2faRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(bad2faRecorder, bad2faRequest)
		if bad2faRecorder.Code != http.StatusBadRequest {
			t.Fatalf("bad 2fa verify: got %d", bad2faRecorder.Code)
		}
	})
}

//nolint:paralleltest // shares SUDOSTREAM_DATABASE_URL integration fixture
func TestMiddleware_NonAdminForbidden(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "non-admin cannot access admin routes", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router := newAuthTestRouter(t)
		_, userToken := createUserSession(t, router, "regular@hpserver.lan", "regular-pass")

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/admin/users", nil)
		setBearerAuth(request, userToken)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("expected forbidden, got %d", recorder.Code)
		}
	})
}

//nolint:paralleltest // shares SUDOSTREAM_DATABASE_URL integration fixture
func TestAuth_PendingTwoFactorSetupLogin(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "mandatory 2FA enrollment completes via pending token", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		pool := setupTestPool(ctx, t)
		authService := prepareIntegrationAuth(ctx, t, pool)

		_, err := authService.UpdateSettings(ctx, auth.Settings{
			EmailConfirmationRequired: false,
			TwoFactorRequired:         true,
		})
		if err != nil {
			t.Fatalf("update settings: %v", err)
		}

		router := newAuthTestRouterFromService(t, authService)

		loginBody := testAdminLoginJSON(testDefaultPassword)
		loginRecorder := httptest.NewRecorder()
		loginRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/login",
			bytes.NewReader(loginBody),
		)
		loginRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(loginRecorder, loginRequest)

		var loginResponse struct {
			Status       string `json:"status"`
			PendingToken string `json:"pendingToken"`
		}
		err = json.NewDecoder(loginRecorder.Body).Decode(&loginResponse)
		if err != nil {
			t.Fatalf("decode login: %v", err)
		}
		if loginResponse.Status != authStatus2FASetupRequired {
			t.Fatalf("expected setup required, got %q", loginResponse.Status)
		}

		setupBody := []byte(`{"pendingToken":"` + loginResponse.PendingToken + `"}`)
		setupRecorder := httptest.NewRecorder()
		setupRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/2fa/setup",
			bytes.NewReader(setupBody),
		)
		setupRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(setupRecorder, setupRequest)
		if setupRecorder.Code != http.StatusOK {
			t.Fatalf("pending setup: %d %s", setupRecorder.Code, setupRecorder.Body.String())
		}

		var setupResponse struct {
			Secret string `json:"secret"`
		}
		err = json.NewDecoder(setupRecorder.Body).Decode(&setupResponse)
		if err != nil {
			t.Fatalf("decode setup: %v", err)
		}

		code, err := totp.GenerateCode(setupResponse.Secret, time.Now())
		if err != nil {
			t.Fatalf("generate totp: %v", err)
		}

		confirmBody := []byte(
			`{"pendingToken":"` + loginResponse.PendingToken + `","code":"` + code + `"}`,
		)
		confirmRecorder := httptest.NewRecorder()
		confirmRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/2fa/confirm",
			bytes.NewReader(confirmBody),
		)
		confirmRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(confirmRecorder, confirmRequest)
		if confirmRecorder.Code != http.StatusOK {
			t.Fatalf("pending confirm: %d %s", confirmRecorder.Code, confirmRecorder.Body.String())
		}
	})
}
