import type { User } from '../types';

export function matchPermission(
  grantedAction: string,
  grantedResource: string,
  requiredAction: string,
  requiredResource: string,
): boolean {
  const actionMatches =
    grantedAction === '*' || grantedAction.toLowerCase() === requiredAction.toLowerCase();
  if (!actionMatches) return false;

  if (grantedResource === '*') return true;
  if (grantedResource.toLowerCase() === requiredResource.toLowerCase()) return true;

  if (grantedResource.endsWith(':*')) {
    const prefix = grantedResource.slice(0, -2);
    return (
      requiredResource === prefix ||
      requiredResource.startsWith(`${prefix}:`)
    );
  }
  return false;
}

export function hasPermission(
  user: User | null | undefined,
  action: string,
  resource: string,
): boolean {
  if (!user || user.status !== 'active' || !user.roles?.length) {
    return false;
  }

  for (const role of user.roles) {
    if (role.slug === 'super_admin') return true;
    for (const perm of role.permissions || []) {
      if (matchPermission(perm.action, perm.resource, action, resource)) {
        return true;
      }
    }
  }
  return false;
}

export function hasAnyPermission(
  user: User | null | undefined,
  checks: Array<[string, string]>,
): boolean {
  return checks.some(([action, resource]) => hasPermission(user, action, resource));
}

export function isSuperAdmin(user: User | null | undefined): boolean {
  return Boolean(user?.roles?.some((r) => r.slug === 'super_admin'));
}

