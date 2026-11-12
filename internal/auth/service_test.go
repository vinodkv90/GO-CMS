package auth

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestAuthService(t *testing.T) (*Service, *MemoryAuthRepository) {
	repo := NewMemoryAuthRepository()
	cfg := config.DefaultConfig()
	cfg.Auth.JWTSecret = "test-jwt-secret-key-for-unit-tests-123456"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := NewService(repo, repo, repo, repo, cfg, logger)
	return svc, repo
}

func TestService_RegisterAndAuthenticate(t *testing.T) {
	svc, _ := setupTestAuthService(t)
	ctx := context.Background()

	// 1. Register User
	user, err := svc.Register(ctx, RegisterRequest{
		Username:  "johndoe",
		Email:     "john@example.com",
		Password:  "SecurePassword1!",
		FirstName: "John",
		LastName:  "Doe",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, user.ID)
	assert.Equal(t, "johndoe", user.Username)

	// 2. Authenticate with Username
	authUser, token, err := svc.Authenticate(ctx, "johndoe", "SecurePassword1!")
	require.NoError(t, err)
	assert.Equal(t, user.ID, authUser.ID)
	assert.NotEmpty(t, token)

	// 3. Authenticate with Email
	authUser, token, err = svc.Authenticate(ctx, "john@example.com", "SecurePassword1!")
	require.NoError(t, err)
	assert.Equal(t, user.ID, authUser.ID)
	assert.NotEmpty(t, token)

	// 4. Invalid Password
	_, _, err = svc.Authenticate(ctx, "johndoe", "WrongPass123!")
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	// 5. Unknown User
	_, _, err = svc.Authenticate(ctx, "unknown_user", "AnyPassword1!")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestService_SeedDefaultRolesAndAdmin(t *testing.T) {
	svc, repo := setupTestAuthService(t)
	ctx := context.Background()

	admin, err := svc.SeedDefaultRolesAndAdmin(ctx, "superadmin", "superadmin@example.com", "AdminSecret123!")
	require.NoError(t, err)
	require.NotNil(t, admin)

	assert.Equal(t, "superadmin", admin.Username)

	// Verify roles exist
	roles, err := repo.ListRoles(ctx)
	require.NoError(t, err)
	assert.Len(t, roles, 4) // super_admin, editor, author, viewer

	// Calling seed again does not duplicate admin
	admin2, err := svc.SeedDefaultRolesAndAdmin(ctx, "admin2", "admin2@example.com", "AdminSecret123!")
	require.NoError(t, err)
	assert.Nil(t, admin2)
}

func TestService_APITokens(t *testing.T) {
	svc, repo := setupTestAuthService(t)
	ctx := context.Background()
	_, err := svc.SeedDefaultRolesAndAdmin(ctx, "admin", "admin@example.com", "AdminSecret123!")
	require.NoError(t, err)

	tokenRole := &Role{ID: "role_token_tester", Name: "Token Tester", Slug: "token_tester", Description: "Token test role"}
	require.NoError(t, repo.CreateRole(ctx, tokenRole))
	for _, permID := range []string{"perm_api_tokens_read", "perm_api_tokens_create", "perm_api_tokens_delete", "perm_content_read", "perm_media_create"} {
		require.NoError(t, repo.AssignPermissionToRole(ctx, tokenRole.ID, permID))
	}

	user, err := svc.Register(ctx, RegisterRequest{
		Username: "tokenuser",
		Email:    "tokenuser@example.com",
		Password: "Password123!",
	})
	require.NoError(t, err)
	require.NoError(t, repo.AssignRoleToUser(ctx, user.ID, tokenRole.ID))

	// Create Token
	duration := 24 * time.Hour
	rawToken, tokenMeta, err := svc.CreateAPIToken(ctx, user.ID, "Test API Key", []string{"read:content", "create:media"}, &duration)
	require.NoError(t, err)
	assert.True(t, IsAPIToken(rawToken))
	assert.Equal(t, "Test API Key", tokenMeta.Name)

	// Validate Token
	validatedToken, validatedUser, err := svc.ValidateAPIToken(ctx, rawToken)
	require.NoError(t, err)
	assert.Equal(t, tokenMeta.ID, validatedToken.ID)
	assert.Equal(t, user.ID, validatedUser.ID)

	// List Tokens
	tokens, err := svc.ListAPITokens(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, tokens, 1)

	// Revoke Token
	err = svc.RevokeAPIToken(ctx, tokenMeta.ID, user.ID)
	require.NoError(t, err)

	// Validation after revoke fails
	_, _, err = svc.ValidateAPIToken(ctx, rawToken)
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestService_UpdateAndDeleteUser(t *testing.T) {
	svc, _ := setupTestAuthService(t)
	ctx := context.Background()

	user, err := svc.Register(ctx, RegisterRequest{
		Username: "updateuser",
		Email:    "update@example.com",
		Password: "Password123!",
	})
	require.NoError(t, err)

	// Update User
	updated, err := svc.UpdateUser(ctx, user.ID, UpdateUserRequest{
		FirstName: "UpdatedFirst",
		LastName:  "UpdatedLast",
		Status:    StatusSuspended,
	})
	require.NoError(t, err)
	assert.Equal(t, "UpdatedFirst", updated.FirstName)
	assert.Equal(t, StatusSuspended, updated.Status)

	// Authentication fails for suspended user
	_, _, err = svc.Authenticate(ctx, "updateuser", "Password123!")
	assert.ErrorIs(t, err, ErrUserSuspended)

	// Delete User
	err = svc.DeleteUser(ctx, user.ID)
	require.NoError(t, err)

	_, err = svc.GetUserProfile(ctx, user.ID)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestService_PasswordChangeAndEmailReindex(t *testing.T) {
	svc, _ := setupTestAuthService(t)
	ctx := context.Background()

	user, err := svc.Register(ctx, RegisterRequest{
		Username:  "pwuser",
		Email:     "old@example.com",
		Password:  "InitialPass1!",
		FirstName: "Old",
		LastName:  "Name",
	})
	require.NoError(t, err)

	err = svc.ChangePassword(ctx, user.ID, ChangePasswordRequest{
		CurrentPassword: "InitialPass1!",
		NewPassword:     "NewStrongPass2!",
	})
	require.NoError(t, err)

	_, _, err = svc.Authenticate(ctx, "pwuser", "InitialPass1!")
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	_, _, err = svc.Authenticate(ctx, "pwuser", "NewStrongPass2!")
	require.NoError(t, err)

	updated, err := svc.UpdateProfile(ctx, user.ID, UpdateProfileRequest{
		Email:     "new@example.com",
		FirstName: "New",
		LastName:  "Name",
	})
	require.NoError(t, err)
	assert.Equal(t, "new@example.com", updated.Email)

	_, _, err = svc.Authenticate(ctx, "old@example.com", "NewStrongPass2!")
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	authUser, _, err := svc.Authenticate(ctx, "new@example.com", "NewStrongPass2!")
	require.NoError(t, err)
	assert.NotNil(t, authUser.LastLoginAt)
}

func TestService_LastSuperAdminAndPrivilegeEscalationGuards(t *testing.T) {
	svc, repo := setupTestAuthService(t)
	ctx := context.Background()

	admin, err := svc.SeedDefaultRolesAndAdmin(ctx, "superadmin", "super@example.com", "AdminSecret123!")
	require.NoError(t, err)
	admin, err = svc.GetUserProfile(ctx, admin.ID)
	require.NoError(t, err)

	editorRole, err := repo.GetRoleBySlug(ctx, RoleEditor)
	require.NoError(t, err)

	// Cannot delete self
	err = svc.DeleteUserByActor(ctx, admin, admin.ID, AuditMeta{})
	assert.ErrorIs(t, err, ErrCannotDeleteSelf)

	// Cannot delete last active super admin
	err = svc.DeleteUserByActor(ctx, nil, admin.ID, AuditMeta{})
	assert.ErrorIs(t, err, ErrLastSuperAdmin)

	// Cannot deactivate or strip role from last active super admin
	_, err = svc.SetUserStatus(ctx, nil, admin.ID, StatusInactive, AuditMeta{})
	assert.ErrorIs(t, err, ErrLastSuperAdmin)

	_, err = svc.UpdateUserByActor(ctx, admin, admin.ID, UpdateUserRequest{
		RoleIDs: []string{editorRole.ID},
	}, AuditMeta{})
	assert.ErrorIs(t, err, ErrLastSuperAdmin)

	// Non-super-admin actor cannot escalate or edit a super admin
	userAdminRole, err := svc.CreateRole(ctx, RoleMutationRequest{
		Name:          "User Manager",
		Slug:          "user_manager",
		PermissionIDs: []string{"perm_users_read", "perm_users_create", "perm_users_update", "perm_users_delete"},
	}, AuditMeta{})
	require.NoError(t, err)

	manager, err := svc.CreateAdminUser(ctx, admin, CreateAdminUserRequest{
		Username: "usermanager",
		Email:    "manager@example.com",
		Password: "ManagerPass123!",
		RoleIDs:  []string{userAdminRole.ID},
	}, AuditMeta{})
	require.NoError(t, err)

	superRole, err := repo.GetRoleBySlug(ctx, RoleSuperAdmin)
	require.NoError(t, err)

	_, err = svc.CreateAdminUser(ctx, manager, CreateAdminUserRequest{
		Username: "rogueadmin",
		Email:    "rogue@example.com",
		Password: "RoguePass123!",
		RoleIDs:  []string{superRole.ID},
	}, AuditMeta{})
	assert.ErrorIs(t, err, ErrForbidden)

	_, err = svc.UpdateUserByActor(ctx, manager, admin.ID, UpdateUserRequest{FirstName: "Hacked"}, AuditMeta{})
	assert.ErrorIs(t, err, ErrForbidden)
}

func TestService_RoleCRUDAndProtection(t *testing.T) {
	svc, repo := setupTestAuthService(t)
	ctx := context.Background()

	_, err := svc.SeedDefaultRolesAndAdmin(ctx, "admin", "admin@example.com", "AdminSecret123!")
	require.NoError(t, err)

	superRole, err := repo.GetRoleBySlug(ctx, RoleSuperAdmin)
	require.NoError(t, err)
	assert.True(t, superRole.IsSystem)

	// super_admin cannot be modified or deleted
	_, err = svc.UpdateRole(ctx, superRole.ID, RoleMutationRequest{Name: "Modified"}, AuditMeta{})
	assert.ErrorIs(t, err, ErrSystemRoleProtected)

	err = svc.DeleteRole(ctx, superRole.ID, AuditMeta{})
	assert.ErrorIs(t, err, ErrSystemRoleProtected)

	// Duplicate super_admin into a custom role
	dup, err := svc.DuplicateRole(ctx, superRole.ID, RoleMutationRequest{Name: "Custom Ops", Slug: "custom_ops"}, AuditMeta{})
	require.NoError(t, err)
	assert.False(t, dup.IsSystem)
	assert.Len(t, dup.Permissions, 1)

	// Update custom role permissions
	updated, err := svc.UpdateRole(ctx, dup.ID, RoleMutationRequest{
		Name:          "Content Reviewer",
		Description:   "Read only review",
		PermissionIDs: []string{"perm_content_read", "perm_media_read"},
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Equal(t, "Content Reviewer", updated.Name)
	assert.Len(t, updated.Permissions, 2)

	// Assign user -> cannot delete role in use
	reviewer, err := svc.CreateAdminUser(ctx, nil, CreateAdminUserRequest{
		Username: "reviewer1",
		Email:    "reviewer1@example.com",
		Password: "ReviewerPass1!",
		RoleIDs:  []string{dup.ID},
	}, AuditMeta{})
	require.NoError(t, err)

	err = svc.DeleteRole(ctx, dup.ID, AuditMeta{})
	assert.ErrorIs(t, err, ErrRoleInUse)

	// Remove role & delete custom role
	editorRole, err := repo.GetRoleBySlug(ctx, RoleEditor)
	require.NoError(t, err)
	_, err = svc.UpdateUserByActor(ctx, nil, reviewer.ID, UpdateUserRequest{RoleIDs: []string{editorRole.ID}}, AuditMeta{})
	require.NoError(t, err)

	err = svc.DeleteRole(ctx, dup.ID, AuditMeta{})
	require.NoError(t, err)
}

func TestService_AdminInvitationLifecycle(t *testing.T) {
	svc, repo := setupTestAuthService(t)
	ctx := context.Background()

	admin, err := svc.SeedDefaultRolesAndAdmin(ctx, "admin", "admin@example.com", "AdminSecret123!")
	require.NoError(t, err)
	admin, err = svc.GetUserProfile(ctx, admin.ID)
	require.NoError(t, err)

	editorRole, err := repo.GetRoleBySlug(ctx, RoleEditor)
	require.NoError(t, err)

	// Create invitation
	raw1, inv, err := svc.CreateInvitation(ctx, admin, CreateInvitationRequest{
		Email:          "invitee@example.com",
		Username:       "invitee",
		FirstName:      "Jane",
		LastName:       "Invite",
		RoleIDs:        []string{editorRole.ID},
		ExpiresInHours: 24,
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Equal(t, InvitationPending, inv.Status)
	assert.Contains(t, raw1, InviteTokenPrefix)

	// Preview works
	preview, err := svc.PreviewInvitation(ctx, raw1)
	require.NoError(t, err)
	assert.Equal(t, "invitee@example.com", preview.Email)

	// Resend rotates token and invalidates old token
	raw2, _, err := svc.ResendInvitation(ctx, admin, inv.ID, 48, AuditMeta{})
	require.NoError(t, err)
	assert.NotEqual(t, raw1, raw2)
	_, err = svc.PreviewInvitation(ctx, raw1)
	assert.ErrorIs(t, err, ErrInvitationNotFound)

	// Accept creates active user with assigned roles & returns JWT
	acceptedUser, jwtTok, err := svc.AcceptInvitation(ctx, AcceptInvitationRequest{
		Token:    raw2,
		Password: "InviteePass123!",
	}, AuditMeta{})
	require.NoError(t, err)
	assert.NotEmpty(t, jwtTok)
	assert.Equal(t, StatusActive, acceptedUser.Status)
	require.Len(t, acceptedUser.Roles, 1)
	assert.Equal(t, editorRole.Slug, acceptedUser.Roles[0].Slug)

	// Cannot reuse accepted token
	_, err = svc.PreviewInvitation(ctx, raw2)
	assert.ErrorIs(t, err, ErrInvitationRevoked)
}

func TestService_TokenSecurityAndKinds(t *testing.T) {
	svc, repo := setupTestAuthService(t)
	ctx := context.Background()
	admin, err := svc.SeedDefaultRolesAndAdmin(ctx, "admin", "admin@example.com", "AdminSecret123!")
	require.NoError(t, err)
	admin, err = repo.GetUserByID(ctx, admin.ID)
	require.NoError(t, err)

	editorTokenRole := &Role{ID: "role_editor_tokens", Name: "Editor Tokens", Slug: "editor_tokens"}
	require.NoError(t, repo.CreateRole(ctx, editorTokenRole))
	for _, pid := range []string{"perm_api_tokens_read", "perm_api_tokens_create", "perm_api_tokens_delete", "perm_content_read", "perm_content_create"} {
		require.NoError(t, repo.AssignPermissionToRole(ctx, editorTokenRole.ID, pid))
	}

	editorUser, err := svc.Register(ctx, RegisterRequest{
		Username: "editortok",
		Email:    "editortok@example.com",
		Password: "Password123!",
	})
	require.NoError(t, err)
	require.NoError(t, repo.AssignRoleToUser(ctx, editorUser.ID, editorTokenRole.ID))

	// Editor cannot escalate to *:* or delete:users or admin full_access or transfer tokens
	_, _, err = svc.CreateTokenWithOptions(ctx, editorUser, CreateTokenRequest{
		Name:        "Escalation",
		Kind:        TokenKindAPI,
		TokenType:   "custom",
		Permissions: []string{"*:*"},
	}, AuditMeta{})
	assert.ErrorIs(t, err, ErrForbidden)

	_, _, err = svc.CreateTokenWithOptions(ctx, editorUser, CreateTokenRequest{
		Name:        "Delete Users",
		Kind:        TokenKindAdmin,
		TokenType:   "custom",
		Permissions: []string{"delete:users"},
	}, AuditMeta{})
	assert.ErrorIs(t, err, ErrForbidden)

	_, _, err = svc.CreateTokenWithOptions(ctx, editorUser, CreateTokenRequest{
		Name:      "Admin Full",
		Kind:      TokenKindAdmin,
		TokenType: "full_access",
	}, AuditMeta{})
	assert.ErrorIs(t, err, ErrForbidden)

	_, _, err = svc.CreateTokenWithOptions(ctx, editorUser, CreateTokenRequest{
		Name:      "Transfer Push",
		Kind:      TokenKindTransfer,
		TokenType: "push",
	}, AuditMeta{})
	assert.ErrorIs(t, err, ErrForbidden)

	// Read-only preset filters to caller's allowed read scopes
	roRaw, roMeta, err := svc.CreateTokenWithOptions(ctx, editorUser, CreateTokenRequest{
		Name:      "Editor RO",
		Kind:      TokenKindAPI,
		TokenType: "read_only",
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Equal(t, []string{"read:content"}, roMeta.Permissions)
	assert.True(t, len(roMeta.TokenPrefix) > 8)

	// Token context with only read:content cannot mint create:content even with create:api_tokens
	roCtx := context.WithValue(ctx, APITokenContextKey, &APIToken{
		ID:          roMeta.ID,
		Permissions: []string{"create:api_tokens", "read:content"},
	})
	_, _, err = svc.CreateTokenWithOptions(roCtx, editorUser, CreateTokenRequest{
		Name:        "Chained Escalation",
		Kind:        TokenKindAPI,
		TokenType:   "custom",
		Permissions: []string{"create:content"},
	}, AuditMeta{})
	assert.ErrorIs(t, err, ErrForbidden)
	assert.NotEmpty(t, roRaw)

	// Super Admin can mint Admin PAT (cms_adm_) and Transfer token (cms_trn_)
	admRaw, admMeta, err := svc.CreateTokenWithOptions(ctx, admin, CreateTokenRequest{
		Name:        "Super Admin PAT",
		Kind:        TokenKindAdmin,
		TokenType:   "full_access",
		Description: "Automation admin key",
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Contains(t, admRaw, AdminTokenPrefix)
	assert.Equal(t, TokenKindAdmin, admMeta.Kind)
	assert.Equal(t, []string{"*:*"}, admMeta.Permissions)

	trnRaw, trnMeta, err := svc.CreateTokenWithOptions(ctx, admin, CreateTokenRequest{
		Name:      "Nightly Transfer",
		Kind:      TokenKindTransfer,
		TokenType: "push_pull",
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Contains(t, trnRaw, TransferTokenPrefix)
	assert.Equal(t, TokenKindTransfer, trnMeta.Kind)
	assert.Equal(t, []string{"push:transfer", "pull:transfer"}, trnMeta.Permissions)

	// Rotate invalidates old raw token and preserves active state
	rotatedRaw, rotatedMeta, err := svc.RotateAPIToken(ctx, admin, trnMeta.ID, AuditMeta{})
	require.NoError(t, err)
	assert.NotEqual(t, trnRaw, rotatedRaw)
	assert.Contains(t, rotatedRaw, TransferTokenPrefix)
	assert.Equal(t, rotatedRaw[:16], rotatedMeta.TokenPrefix)

	_, _, err = svc.ValidateAPIToken(ctx, trnRaw)
	assert.ErrorIs(t, err, ErrUnauthorized)
	_, _, err = svc.ValidateAPIToken(ctx, rotatedRaw)
	require.NoError(t, err)

	// Revoke keeps history record with RevokedAt and rejects authentication & rotation
	require.NoError(t, svc.RevokeAPITokenByActor(ctx, admin, trnMeta.ID, AuditMeta{}))
	_, _, err = svc.ValidateAPIToken(ctx, rotatedRaw)
	assert.ErrorIs(t, err, ErrUnauthorized)
	_, _, err = svc.RotateAPIToken(ctx, admin, trnMeta.ID, AuditMeta{})
	assert.Error(t, err)

	listed, err := svc.ListTokensByKind(ctx, admin, TokenKindTransfer)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.NotNil(t, listed[0].RevokedAt)

	require.NoError(t, svc.DeleteAPITokenByActor(ctx, admin, trnMeta.ID, AuditMeta{}))
	listedAfterDel, err := svc.ListTokensByKind(ctx, admin, TokenKindTransfer)
	require.NoError(t, err)
	assert.Empty(t, listedAfterDel)
}
