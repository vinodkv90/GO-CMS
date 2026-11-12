import { describe, it, expect } from 'vitest';
import { hasPermission, matchPermission } from './rbac';
import type { User } from '../types';

describe('RBAC helpers', () => {
  it('matches wildcards and resource prefixes accurately', () => {
    expect(matchPermission('*', '*', 'update', 'settings')).toBe(true);
    expect(matchPermission('read', 'settings', 'read', 'settings')).toBe(true);
    expect(matchPermission('read', 'content:*', 'read', 'content:articles')).toBe(true);
    expect(matchPermission('read', 'content:*', 'update', 'content:articles')).toBe(false);
    expect(matchPermission('read', 'settings', 'update', 'settings')).toBe(false);
  });

  it('grants unrestricted permissions to active super_admin users only', () => {
    const superAdmin: User = {
      id: '1',
      username: 'root',
      email: 'root@example.com',
      status: 'active',
      roles: [{ id: 'r1', name: 'Super Admin', slug: 'super_admin', permissions: [] }],
    };
    expect(hasPermission(superAdmin, 'update', 'settings')).toBe(true);

    const inactive: User = { ...superAdmin, status: 'inactive' };
    expect(hasPermission(inactive, 'read', 'settings')).toBe(false);
  });

  it('enforces explicit granular permissions on non-super-admin roles', () => {
    const reader: User = {
      id: '2',
      username: 'reader',
      email: 'reader@example.com',
      status: 'active',
      roles: [
        {
          id: 'r2',
          name: 'Auditing Reader',
          slug: 'auditor',
          permissions: [{ action: 'read', resource: 'settings' }],
        },
      ],
    };
    expect(hasPermission(reader, 'read', 'settings')).toBe(true);
    expect(hasPermission(reader, 'update', 'settings')).toBe(false);
    expect(hasPermission(reader, 'read', 'system:info')).toBe(false);
  });
});

