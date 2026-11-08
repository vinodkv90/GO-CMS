package auth

import "strings"

// MatchPermission checks if a user's granted permission satisfies the required permission.
// Supports wildcard matching on both action and resource:
// - Action "*" matches any action.
// - Resource "*" matches any resource.
// - Resource prefix with ":*" (e.g., "content:*") matches sub-resources (e.g., "content:articles").
func MatchPermission(grantedAction, grantedResource, requiredAction, requiredResource string) bool {
	// 1. Action check
	actionMatches := grantedAction == "*" || strings.EqualFold(grantedAction, requiredAction)
	if !actionMatches {
		return false
	}

	// 2. Resource check
	if grantedResource == "*" {
		return true
	}

	if strings.EqualFold(grantedResource, requiredResource) {
		return true
	}

	// Prefix matching: e.g. granted "content:*" matches required "content:article"
	if strings.HasSuffix(grantedResource, ":*") {
		prefix := strings.TrimSuffix(grantedResource, ":*")
		if strings.HasPrefix(requiredResource, prefix+":") || requiredResource == prefix {
			return true
		}
	}

	return false
}

// HasPermission checks if the user has permission to perform an action on a resource.
func HasPermission(user *User, action, resource string) bool {
	if user == nil || user.Status != StatusActive {
		return false
	}

	for _, role := range user.Roles {
		// Super Admin has unrestricted access to everything
		if role.Slug == RoleSuperAdmin {
			return true
		}

		for _, perm := range role.Permissions {
			if MatchPermission(perm.Action, perm.Resource, action, resource) {
				return true
			}
		}
	}

	return false
}

// HasAnyRole checks if the user possesses at least one of the specified roles.
func HasAnyRole(user *User, roles ...string) bool {
	if user == nil || user.Status != StatusActive {
		return false
	}

	for _, userRole := range user.Roles {
		for _, requiredRole := range roles {
			if userRole.Slug == requiredRole {
				return true
			}
		}
	}

	return false
}

// MatchTokenPermission checks if an API token's permission slice satisfies the required permission.
// Permissions in tokens are represented as "action:resource" strings (e.g. "read:content", "*:*").
func MatchTokenPermission(tokenPermissions []string, requiredAction, requiredResource string) bool {
	for _, tp := range tokenPermissions {
		parts := strings.SplitN(tp, ":", 2)
		if len(parts) == 2 {
			if MatchPermission(parts[0], parts[1], requiredAction, requiredResource) {
				return true
			}
		}
	}
	return false
}
