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
	svc, _ := setupTestAuthService(t)
	ctx := context.Background()

	user, err := svc.Register(ctx, RegisterRequest{
		Username: "tokenuser",
		Email:    "tokenuser@example.com",
		Password: "Password123!",
	})
	require.NoError(t, err)

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

