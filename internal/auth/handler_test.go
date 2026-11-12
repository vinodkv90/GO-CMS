package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRouter(svc *Service) *chi.Mux {
	r := chi.NewRouter()
	r.Use(AuthMiddleware(svc))
	handler := NewHandler(svc)
	handler.RegisterRoutes(r)
	return r
}

func TestHandler_RegisterAndLoginFlow(t *testing.T) {
	svc, _ := setupTestAuthService(t)
	router := setupTestRouter(svc)

	// 1. Register User
	registerPayload := map[string]string{
		"username":   "alice",
		"email":      "alice@example.com",
		"password":   "AliceSecret123!",
		"first_name": "Alice",
		"last_name":  "Smith",
	}
	body, _ := json.Marshal(registerPayload)
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var regResp User
	err := json.Unmarshal(w.Body.Bytes(), &regResp)
	require.NoError(t, err)
	assert.Equal(t, "alice", regResp.Username)

	// 2. Login User
	loginPayload := map[string]string{
		"identifier": "alice",
		"password":   "AliceSecret123!",
	}
	body, _ = json.Marshal(loginPayload)
	req = httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var loginResp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &loginResp)
	require.NoError(t, err)
	token := loginResp["token"].(string)
	assert.NotEmpty(t, token)

	// 3. Access Protected /auth/me with Bearer Token
	req = httptest.NewRequest("GET", "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var meResp User
	err = json.Unmarshal(w.Body.Bytes(), &meResp)
	require.NoError(t, err)
	assert.Equal(t, "alice", meResp.Username)

	// 4. Access Protected /auth/me without Token (expect 401)
	req = httptest.NewRequest("GET", "/auth/me", nil)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_APITokenFlow(t *testing.T) {
	svc, repo := setupTestAuthService(t)
	router := setupTestRouter(svc)
	_, err := svc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin@CMS2026!")
	require.NoError(t, err)

	tokenRole := &Role{ID: "role_bob_tokens", Name: "Bob Tokens", Slug: "bob_tokens"}
	require.NoError(t, repo.CreateRole(t.Context(), tokenRole))
	for _, pid := range []string{"perm_api_tokens_read", "perm_api_tokens_create", "perm_api_tokens_delete", "perm_content_read"} {
		require.NoError(t, repo.AssignPermissionToRole(t.Context(), tokenRole.ID, pid))
	}

	// Register & Login User
	user, err := svc.Register(t.Context(), RegisterRequest{
		Username: "bob",
		Email:    "bob@example.com",
		Password: "BobSecret123!",
	})
	require.NoError(t, err)
	require.NoError(t, repo.AssignRoleToUser(t.Context(), user.ID, tokenRole.ID))

	_, token, err := svc.Authenticate(t.Context(), "bob", "BobSecret123!")
	require.NoError(t, err)

	// Create API Token via endpoint
	tokenReq := map[string]any{
		"name":             "Bob Automation Key",
		"permissions":      []string{"read:content"},
		"duration_seconds": 3600,
	}
	body, _ := json.Marshal(tokenReq)
	req := httptest.NewRequest("POST", "/api-tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createResp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &createResp)
	require.NoError(t, err)
	patToken := createResp["token"].(string)
	assert.True(t, IsAPIToken(patToken))

	// Access /auth/me using Personal Access Token
	req = httptest.NewRequest("GET", "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+patToken)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var meResp User
	err = json.Unmarshal(w.Body.Bytes(), &meResp)
	require.NoError(t, err)
	assert.Equal(t, user.ID, meResp.ID)

	// List API Tokens
	req = httptest.NewRequest("GET", "/api-tokens", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_SecurityAndRBACEnforcement(t *testing.T) {
	svc, _ := setupTestAuthService(t)
	router := setupTestRouter(svc)

	// Seed roles & admin
	admin, err := svc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin@CMS2026!")
	require.NoError(t, err)

	_, adminToken, err := svc.Authenticate(t.Context(), "admin", "Admin@CMS2026!")
	require.NoError(t, err)

	// Register regular non-admin user
	nonAdmin, err := svc.Register(t.Context(), RegisterRequest{
		Username: "regularuser",
		Email:    "reg@example.com",
		Password: "RegUser2026!",
	})
	require.NoError(t, err)

	_, regToken, err := svc.Authenticate(t.Context(), "regularuser", "RegUser2026!")
	require.NoError(t, err)

	// 1. Regular user attempting to access admin-only /users endpoint (Forbidden 403)
	req := httptest.NewRequest("GET", "/users", nil)
	req.Header.Set("Authorization", "Bearer "+regToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Forbidden: insufficient permissions")

	// 2. Admin user accessing /users endpoint (OK 200)
	reqAdmin := httptest.NewRequest("GET", "/users", nil)
	reqAdmin.Header.Set("Authorization", "Bearer "+adminToken)
	wAdmin := httptest.NewRecorder()
	router.ServeHTTP(wAdmin, reqAdmin)
	assert.Equal(t, http.StatusOK, wAdmin.Code)

	// 3. Tampered token (header/signature modified)
	tamperedToken := adminToken[:len(adminToken)-5] + "XXXXX"
	reqTampered := httptest.NewRequest("GET", "/auth/me", nil)
	reqTampered.Header.Set("Authorization", "Bearer "+tamperedToken)
	wTampered := httptest.NewRecorder()
	router.ServeHTTP(wTampered, reqTampered)
	assert.Equal(t, http.StatusUnauthorized, wTampered.Code)

	// 4. Malformed Authorization headers
	for _, badHeader := range []string{"Bearer", "Basic xyz", "Bearer   ", "InvalidPrefix token"} {
		reqBad := httptest.NewRequest("GET", "/auth/me", nil)
		reqBad.Header.Set("Authorization", badHeader)
		wBad := httptest.NewRecorder()
		router.ServeHTTP(wBad, reqBad)
		assert.Equal(t, http.StatusUnauthorized, wBad.Code, "header %q should be unauthorized", badHeader)
	}

	_ = admin
	_ = nonAdmin
}

func TestHandler_AdminUsersRolesAndInvitationsFlow(t *testing.T) {
	svc, repo := setupTestAuthService(t)
	router := setupTestRouter(svc)

	_, err := svc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin@CMS2026!")
	require.NoError(t, err)

	_, adminToken, err := svc.Authenticate(t.Context(), "admin", "Admin@CMS2026!")
	require.NoError(t, err)

	// 1. Fetch Permissions catalog
	req := httptest.NewRequest("GET", "/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "system:audit_logs")

	// 2. Create custom role
	roleBody, _ := json.Marshal(map[string]any{
		"name":           "Auditors",
		"slug":           "auditors",
		"description":    "Read-only audit and users",
		"permission_ids": []string{"perm_users_read", "perm_roles_read", "perm_system_audit_read"},
	})
	req = httptest.NewRequest("POST", "/roles", bytes.NewReader(roleBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var createdRole Role
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createdRole))
	assert.Len(t, createdRole.Permissions, 3)

	// 3. Invite a user into custom role
	invBody, _ := json.Marshal(map[string]any{
		"email":            "auditor@cms.test",
		"username":         "auditor1",
		"first_name":       "Aud",
		"last_name":        "Itor",
		"role_ids":         []string{createdRole.ID},
		"expires_in_hours": 48,
	})
	req = httptest.NewRequest("POST", "/users/invitations", bytes.NewReader(invBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var invResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &invResp))
	rawInviteToken := invResp["token"].(string)

	// 4. Preview & Accept invitation publicly
	req = httptest.NewRequest("GET", "/auth/invitations/preview?token="+rawInviteToken, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "auditor@cms.test")

	acceptBody, _ := json.Marshal(map[string]any{
		"token":    rawInviteToken,
		"password": "AuditorSecret2026!",
	})
	req = httptest.NewRequest("POST", "/auth/invitations/accept", bytes.NewReader(acceptBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var acceptResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &acceptResp))
	auditorToken := acceptResp["token"].(string)

	// 5. Auditor can read users & roles, but receives 403 when trying to create/delete roles
	req = httptest.NewRequest("GET", "/users?search=auditor", nil)
	req.Header.Set("Authorization", "Bearer "+auditorToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "auditor@cms.test")

	req = httptest.NewRequest("POST", "/roles", bytes.NewReader(roleBody))
	req.Header.Set("Authorization", "Bearer "+auditorToken)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 6. Admin deactivates & reactivates auditor user
	auditorUser, err := repo.GetUserByUsername(t.Context(), "auditor1")
	require.NoError(t, err)

	req = httptest.NewRequest("POST", "/users/"+auditorUser.ID+"/deactivate", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"inactive"`)

	req = httptest.NewRequest("POST", "/users/"+auditorUser.ID+"/activate", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"active"`)
}

func TestHandler_TokenKindsAntiEscalationAndRuntimeRBAC(t *testing.T) {
	svc, repo := setupTestAuthService(t)
	r := chi.NewRouter()
	r.Use(AuthMiddleware(svc))
	NewHandler(svc).RegisterRoutes(r)

	// Dummy guarded endpoints simulating Content & Transfer routes
	r.Group(func(pr chi.Router) {
		pr.Use(RequireAuth)
		pr.With(RequirePermission("read", "content")).Get("/guard/content-read", func(w http.ResponseWriter, req *http.Request) {
			respondJSON(w, http.StatusOK, map[string]any{
				"can_write": RequestHasPermission(req.Context(), "create", "content"),
			})
		})
		pr.With(RequirePermission("create", "content")).Post("/guard/content-write", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
		})
		pr.With(RequirePermission("push", "transfer")).Post("/guard/transfer-push", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
		})
	})

	_, err := svc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin@CMS2026!")
	require.NoError(t, err)
	_, adminJWT, err := svc.Authenticate(t.Context(), "admin", "Admin@CMS2026!")
	require.NoError(t, err)

	// Create editor user with API token permissions only (not delete:users or *:*)
	tokEditorRole := &Role{ID: "role_tok_editor", Name: "Token Editor", Slug: "tok_editor"}
	require.NoError(t, repo.CreateRole(t.Context(), tokEditorRole))
	for _, pid := range []string{"perm_api_tokens_read", "perm_api_tokens_create", "perm_api_tokens_delete", "perm_content_read", "perm_content_create"} {
		require.NoError(t, repo.AssignPermissionToRole(t.Context(), tokEditorRole.ID, pid))
	}
	ed, err := svc.Register(t.Context(), RegisterRequest{Username: "ed", Email: "ed@example.com", Password: "EdPassword123!"})
	require.NoError(t, err)
	require.NoError(t, repo.AssignRoleToUser(t.Context(), ed.ID, tokEditorRole.ID))
	_, edJWT, err := svc.Authenticate(t.Context(), "ed", "EdPassword123!")
	require.NoError(t, err)

	// 1. Editor blocked from minting *:* or delete:users -> 403
	for _, payload := range []map[string]any{
		{"name": "EscalateAll", "kind": "api", "token_type": "custom", "permissions": []string{"*:*"}},
		{"name": "EscalateUsers", "kind": "admin", "token_type": "custom", "permissions": []string{"delete:users"}},
		{"name": "EscalateTransfer", "kind": "transfer", "token_type": "push"},
	} {
		b, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api-tokens", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+edJWT)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	}

	// 2. Editor creates read-only PAT -> allowed on read content, 403 on write content & RequestHasPermission(create:content) == false
	roBody, _ := json.Marshal(map[string]any{"name": "Read Content", "kind": "api", "token_type": "custom", "permissions": []string{"read:content"}})
	req := httptest.NewRequest("POST", "/api-tokens", bytes.NewReader(roBody))
	req.Header.Set("Authorization", "Bearer "+edJWT)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var roResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &roResp))
	roRaw := roResp["token"].(string)

	req = httptest.NewRequest("GET", "/guard/content-read", nil)
	req.Header.Set("Authorization", "Bearer "+roRaw)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"can_write":false`)

	req = httptest.NewRequest("POST", "/guard/content-write", nil)
	req.Header.Set("Authorization", "Bearer "+roRaw)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 3. Downgrading ed's role to viewer immediately revokes effective runtime access of existing PAT
	require.NoError(t, repo.ReplaceUserRoles(t.Context(), ed.ID, []string{"role_viewer"}))
	// Viewer keeps read:content, but let's test a PAT with create:content losing create:content on downgrade
	// First check ed cannot create a new token anymore either -> 403
	req = httptest.NewRequest("POST", "/api-tokens", bytes.NewReader(roBody))
	req.Header.Set("Authorization", "Bearer "+edJWT)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// Downgrade a token owner who held create:content to zero roles -> existing PAT blocked with 403!
	edUser2, err := svc.Register(t.Context(), RegisterRequest{Username: "ed2", Email: "ed2@example.com", Password: "EdPassword123!"})
	require.NoError(t, err)
	require.NoError(t, repo.AssignRoleToUser(t.Context(), edUser2.ID, tokEditorRole.ID))
	rawEd2, _, err := svc.CreateTokenWithOptions(t.Context(), edUser2, CreateTokenRequest{
		Name: "Write Key", Kind: TokenKindAPI, TokenType: "custom", Permissions: []string{"create:content"},
	}, AuditMeta{})
	require.NoError(t, err)
	require.NoError(t, repo.ReplaceUserRoles(t.Context(), edUser2.ID, []string{"role_viewer"}))
	req = httptest.NewRequest("POST", "/guard/content-write", nil)
	req.Header.Set("Authorization", "Bearer "+rawEd2)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 4. Super Admin creates Transfer token -> blocked on content, allowed on transfer, supports rotate/revoke/delete
	trnBody, _ := json.Marshal(map[string]any{"name": "Sync Key", "token_type": "push"})
	req = httptest.NewRequest("POST", "/transfer-tokens", bytes.NewReader(trnBody))
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var trnResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &trnResp))
	trnRaw := trnResp["token"].(string)
	trnID := trnResp["metadata"].(map[string]any)["id"].(string)
	assert.Contains(t, trnRaw, TransferTokenPrefix)

	req = httptest.NewRequest("GET", "/guard/content-read", nil)
	req.Header.Set("Authorization", "Bearer "+trnRaw)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	req = httptest.NewRequest("POST", "/guard/transfer-push", nil)
	req.Header.Set("Authorization", "Bearer "+trnRaw)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Rotate
	req = httptest.NewRequest("POST", "/transfer-tokens/"+trnID+"/rotate", nil)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var rotResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rotResp))
	rotRaw := rotResp["token"].(string)

	req = httptest.NewRequest("POST", "/guard/transfer-push", nil)
	req.Header.Set("Authorization", "Bearer "+trnRaw)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	req = httptest.NewRequest("POST", "/guard/transfer-push", nil)
	req.Header.Set("Authorization", "Bearer "+rotRaw)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Revoke
	req = httptest.NewRequest("POST", "/transfer-tokens/"+trnID+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest("POST", "/guard/transfer-push", nil)
	req.Header.Set("Authorization", "Bearer "+rotRaw)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Delete
	req = httptest.NewRequest("DELETE", "/transfer-tokens/"+trnID, nil)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
