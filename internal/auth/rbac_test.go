package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchPermission(t *testing.T) {
	tests := []struct {
		name             string
		grantedAction    string
		grantedResource  string
		requiredAction   string
		requiredResource string
		expected         bool
	}{
		{"exact match", "read", "content", "read", "content", true},
		{"action mismatch", "write", "content", "read", "content", false},
		{"resource mismatch", "read", "media", "read", "content", false},
		{"wildcard action", "*", "content", "read", "content", true},
		{"wildcard resource", "read", "*", "read", "content", true},
		{"wildcard both", "*", "*", "delete", "users", true},
		{"resource prefix wildcard match", "read", "content:*", "read", "content:articles", true},
		{"resource prefix wildcard mismatch", "read", "content:*", "read", "users", false},
		{"case insensitive match", "READ", "Content", "read", "content", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := MatchPermission(tt.grantedAction, tt.grantedResource, tt.requiredAction, tt.requiredResource)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestHasPermission(t *testing.T) {
	superAdminUser := &User{
		ID:     "user-1",
		Status: StatusActive,
		Roles: []Role{
			{Slug: RoleSuperAdmin},
		},
	}

	assert.True(t, HasPermission(superAdminUser, "delete", "anything"))

	editorUser := &User{
		ID:     "user-2",
		Status: StatusActive,
		Roles: []Role{
			{
				Slug: RoleEditor,
				Permissions: []Permission{
					{Action: "read", Resource: "content"},
					{Action: "create", Resource: "content"},
					{Action: "update", Resource: "content"},
				},
			},
		},
	}

	assert.True(t, HasPermission(editorUser, "read", "content"))
	assert.True(t, HasPermission(editorUser, "create", "content"))
	assert.False(t, HasPermission(editorUser, "delete", "content"))
	assert.False(t, HasPermission(editorUser, "read", "users"))

	// Inactive user has no permissions
	editorUser.Status = StatusInactive
	assert.False(t, HasPermission(editorUser, "read", "content"))

	// Nil user
	assert.False(t, HasPermission(nil, "read", "content"))
}

func TestHasAnyRole(t *testing.T) {
	user := &User{
		Status: StatusActive,
		Roles: []Role{
			{Slug: RoleEditor},
		},
	}

	assert.True(t, HasAnyRole(user, RoleEditor, RoleSuperAdmin))
	assert.False(t, HasAnyRole(user, RoleSuperAdmin, RoleAuthor))
	assert.False(t, HasAnyRole(nil, RoleEditor))
}

func TestMatchTokenPermission(t *testing.T) {
	perms := []string{"read:content", "read:media", "create:media"}

	assert.True(t, MatchTokenPermission(perms, "read", "content"))
	assert.True(t, MatchTokenPermission(perms, "create", "media"))
	assert.False(t, MatchTokenPermission(perms, "delete", "content"))
	assert.False(t, MatchTokenPermission(perms, "read", "users"))

	// Wildcard token
	wildcardPerms := []string{"*:*"}
	assert.True(t, MatchTokenPermission(wildcardPerms, "delete", "users"))
}
