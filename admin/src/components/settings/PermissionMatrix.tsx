import React, { useMemo, useState } from 'react';
import { Lock, ShieldCheck } from 'lucide-react';
import type { Permission } from '../../types';
import { Badge } from '../ui/Badge';
import { Button } from '../ui/Button';
import { cn } from '../../lib/cn';

export interface PermissionMatrixProps {
  permissions: Permission[];
  selectedIds: string[];
  onChange: (nextIds: string[]) => void;
  disabled?: boolean;
  isSuperAdminLocked?: boolean;
}

interface PermissionGroupDef {
  id: string;
  label: string;
  description: string;
  resources: Array<{ resource: string; label: string }>;
}

const GROUPS: PermissionGroupDef[] = [
  {
    id: 'content',
    label: 'Content & Schema',
    description: 'Manage content entries, lifecycle publishing, and schema modeling.',
    resources: [
      { resource: 'content', label: 'Content Manager Entries' },
      { resource: 'content_types', label: 'Content-Type Builder' },
    ],
  },
  {
    id: 'media',
    label: 'Media Library',
    description: 'Upload, organize folders, edit metadata, and remove media assets.',
    resources: [{ resource: 'media', label: 'Media Assets & Folders' }],
  },
  {
    id: 'administration',
    label: 'Administration',
    description: 'Manage panel administrators, roles, invitations, and permissions.',
    resources: [
      { resource: 'users', label: 'Admin Users & Invitations' },
      { resource: 'roles', label: 'Admin Roles & Matrix' },
      { resource: 'users_permissions', label: 'End-User Access Control' },
    ],
  },
  {
    id: 'integrations',
    label: 'API & Plugins',
    description: 'Control programmatic API tokens, outgoing webhooks, and CMS plugins.',
    resources: [
      { resource: 'api_tokens', label: 'API & Transfer Tokens' },
      { resource: 'webhooks', label: 'Webhooks & Deliveries' },
      { resource: 'plugins', label: 'Plugins & Extensions' },
    ],
  },
  {
    id: 'system',
    label: 'Settings & System',
    description: 'Platform settings, diagnostics, locales, tenants, jobs, and audit logs.',
    resources: [
      { resource: 'settings', label: 'Platform Settings' },
      { resource: 'system:info', label: 'System Information' },
      { resource: 'system:audit_logs', label: 'Audit Logs' },
      { resource: 'system:jobs', label: 'Background Jobs' },
      { resource: 'system:locales', label: 'Internationalization Locales' },
      { resource: 'system:tenants', label: 'Multi-Tenancy Tenants' },
    ],
  },
];

const ACTION_ORDER = ['read', 'create', 'update', 'delete', 'publish', 'manage', '*'];

function formatActionLabel(action: string): string {
  if (action === '*') return 'Full (*)';
  return action.charAt(0).toUpperCase() + action.slice(1);
}

export const PermissionMatrix: React.FC<PermissionMatrixProps> = ({
  permissions,
  selectedIds,
  onChange,
  disabled = false,
  isSuperAdminLocked = false,
}) => {
  const [activeGroup, setActiveGroup] = useState<string>('all');
  const selectedSet = useMemo(() => new Set(selectedIds), [selectedIds]);

  const byResource = useMemo(() => {
    const map = new Map<string, Permission[]>();
    for (const perm of permissions) {
      const list = map.get(perm.resource) || [];
      list.push(perm);
      map.set(perm.resource, list);
    }
    for (const list of map.values()) {
      list.sort((a, b) => {
        const ia = ACTION_ORDER.indexOf(a.action);
        const ib = ACTION_ORDER.indexOf(b.action);
        const wa = ia === -1 ? 99 : ia;
        const wb = ib === -1 ? 99 : ib;
        if (wa !== wb) return wa - wb;
        return a.action.localeCompare(b.action);
      });
    }
    return map;
  }, [permissions]);

  const wildcardAllPerm = useMemo(
    () => permissions.find((p) => p.action === '*' && p.resource === '*'),
    [permissions],
  );

  const knownResources = useMemo(() => {
    const s = new Set<string>(['*']);
    for (const g of GROUPS) {
      for (const r of g.resources) {
        s.add(r.resource);
      }
    }
    return s;
  }, []);

  const extraResources = useMemo(() => {
    const extras: Array<{ resource: string; label: string }> = [];
    for (const res of byResource.keys()) {
      if (!knownResources.has(res)) {
        extras.push({ resource: res, label: res });
      }
    }
    return extras;
  }, [byResource, knownResources]);

  const visibleGroups = useMemo(() => {
    const groups = [...GROUPS];
    if (extraResources.length > 0) {
      groups.push({
        id: 'other',
        label: 'Other Resources',
        description: 'Additional registered permissions.',
        resources: extraResources,
      });
    }
    if (activeGroup === 'all') return groups;
    return groups.filter((g) => g.id === activeGroup);
  }, [activeGroup, extraResources]);

  const readOnly = disabled || isSuperAdminLocked;

  const togglePermission = (permId?: string) => {
    if (readOnly || !permId) return;
    const next = new Set(selectedSet);
    if (next.has(permId)) {
      next.delete(permId);
    } else {
      next.add(permId);
    }
    onChange(Array.from(next));
  };

  const toggleResourceAll = (perms: Permission[]) => {
    if (readOnly || perms.length === 0) return;
    const permIds = perms.map((p) => p.id || '').filter(Boolean);
    const allChecked = permIds.every((pid) => selectedSet.has(pid));
    const next = new Set(selectedSet);
    for (const pid of permIds) {
      if (allChecked) next.delete(pid);
      else next.add(pid);
    }
    onChange(Array.from(next));
  };

  const toggleGroupAll = (group: PermissionGroupDef) => {
    if (readOnly) return;
    const permIds = group.resources
      .flatMap((r) => byResource.get(r.resource) || [])
      .map((p) => p.id || '')
      .filter(Boolean);
    if (permIds.length === 0) return;
    const allChecked = permIds.every((pid) => selectedSet.has(pid));
    const next = new Set(selectedSet);
    for (const pid of permIds) {
      if (allChecked) next.delete(pid);
      else next.add(pid);
    }
    onChange(Array.from(next));
  };

  return (
    <div className="space-y-4">
      {isSuperAdminLocked && (
        <div className="flex items-center gap-2.5 rounded-xl border border-amber-200 bg-amber-50/70 px-4 py-3 text-xs text-amber-900">
          <Lock className="w-4 h-4 shrink-0 text-amber-600" />
          <span>
            <strong>Super Administrator</strong> has unrestricted access across every module and cannot have permissions removed. Duplicate this role to customize a restricted variant.
          </span>
        </div>
      )}

      {wildcardAllPerm && wildcardAllPerm.id && (
        <div className="flex items-center justify-between gap-3 rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
          <div className="flex items-center gap-2.5">
            <ShieldCheck className="w-4 h-4 text-sky-600 shrink-0" />
            <div>
              <p className="text-xs font-semibold text-slate-900">Unrestricted Wildcard Access (*:*)</p>
              <p className="text-[11px] text-slate-500">
                Grants full administrative authority across all modules, settings, and security boundaries.
              </p>
            </div>
          </div>
          <label className="inline-flex items-center gap-2 text-xs font-medium text-slate-700 cursor-pointer">
            <input
              type="checkbox"
              checked={selectedSet.has(wildcardAllPerm.id)}
              disabled={readOnly}
              onChange={() => togglePermission(wildcardAllPerm.id)}
              className="rounded border-slate-300 text-sky-600 focus:ring-sky-500"
            />
            <span>Enabled</span>
          </label>
        </div>
      )}

      <div className="flex flex-wrap items-center gap-1.5 border-b border-slate-200 pb-2.5">
        <button
          type="button"
          onClick={() => setActiveGroup('all')}
          className={cn(
            'px-3 py-1.5 rounded-lg text-xs font-medium transition-colors',
            activeGroup === 'all'
              ? 'bg-slate-900 text-white'
              : 'text-slate-600 hover:bg-slate-100',
          )}
        >
          All Categories
        </button>
        {GROUPS.map((g) => {
          const groupPerms = g.resources.flatMap((r) => byResource.get(r.resource) || []);
          const checkedCount = groupPerms.filter((p) => p.id && selectedSet.has(p.id)).length;
          return (
            <button
              key={g.id}
              type="button"
              onClick={() => setActiveGroup(g.id)}
              className={cn(
                'px-3 py-1.5 rounded-lg text-xs font-medium transition-colors inline-flex items-center gap-1.5',
                activeGroup === g.id
                  ? 'bg-slate-900 text-white'
                  : 'text-slate-600 hover:bg-slate-100',
              )}
            >
              <span>{g.label}</span>
              <Badge variant={checkedCount > 0 ? 'info' : 'neutral'} className="text-[10px]">
                {checkedCount}/{groupPerms.length}
              </Badge>
            </button>
          );
        })}
      </div>

      <div className="space-y-4">
        {visibleGroups.map((group) => {
          const groupPerms = group.resources.flatMap((r) => byResource.get(r.resource) || []);
          if (groupPerms.length === 0) return null;
          const allGroupSelected = groupPerms.every((p) => Boolean(p.id) && selectedSet.has(p.id!));

          return (
            <div key={group.id} className="rounded-xl border border-slate-200 bg-white overflow-hidden">
              <div className="flex items-center justify-between gap-3 px-4 py-3 bg-slate-50/80 border-b border-slate-200">
                <div>
                  <h4 className="text-xs font-semibold text-slate-900">{group.label}</h4>
                  <p className="text-[11px] text-slate-500">{group.description}</p>
                </div>
                {!readOnly && (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => toggleGroupAll(group)}
                  >
                    {allGroupSelected ? 'Clear section' : 'Select all'}
                  </Button>
                )}
              </div>

              <div className="divide-y divide-slate-100">
                {group.resources.map((resDef) => {
                  const resPerms = byResource.get(resDef.resource) || [];
                  if (resPerms.length === 0) return null;
                  const allRowChecked = resPerms.every((p) => Boolean(p.id) && selectedSet.has(p.id!));

                  return (
                    <div
                      key={resDef.resource}
                      className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 px-4 py-3"
                    >
                      <div className="min-w-[200px]">
                        <p className="text-xs font-semibold text-slate-800">{resDef.label}</p>
                        <p className="text-[11px] font-mono text-slate-400">{resDef.resource}</p>
                      </div>

                      <div className="flex flex-wrap items-center gap-2">
                        {resPerms.map((perm) => {
                          const pid = perm.id || `${perm.action}:${perm.resource}`;
                          const checked = selectedSet.has(pid);
                          return (
                            <label
                              key={pid}
                              title={perm.description || `${perm.action}:${perm.resource}`}
                              className={cn(
                                'inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border text-xs font-medium transition-colors select-none',
                                checked
                                  ? 'border-sky-300 bg-sky-50 text-sky-800'
                                  : 'border-slate-200 bg-white text-slate-600 hover:bg-slate-50',
                                readOnly ? 'opacity-70 cursor-not-allowed' : 'cursor-pointer',
                              )}
                            >
                              <input
                                type="checkbox"
                                checked={checked}
                                disabled={readOnly}
                                onChange={() => togglePermission(pid)}
                                className="rounded border-slate-300 text-sky-600 focus:ring-sky-500"
                              />
                              <span>{formatActionLabel(perm.action)}</span>
                            </label>
                          );
                        })}

                        {resPerms.length > 1 && !readOnly && (
                          <button
                            type="button"
                            onClick={() => toggleResourceAll(resPerms)}
                            className="text-[11px] font-medium text-slate-500 hover:text-sky-700 px-1.5 py-1"
                          >
                            {allRowChecked ? 'None' : 'All'}
                          </button>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
