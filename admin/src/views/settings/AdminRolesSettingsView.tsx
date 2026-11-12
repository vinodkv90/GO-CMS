import React, { useCallback, useEffect, useState } from 'react';
import { ArrowLeft, Copy, Plus, Save, Shield, Trash2 } from 'lucide-react';
import { rolesApi } from '../../api/endpoints';
import type { Permission, Role, User } from '../../types';
import { hasPermission } from '../../utils/rbac';
import { useToast } from '../../components/ui/Toast';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Badge } from '../../components/ui/Badge';
import { Input } from '../../components/ui/Input';
import { Textarea } from '../../components/ui/Textarea';
import { DataTable, type DataTableColumn } from '../../components/ui/DataTable';
import { Modal } from '../../components/ui/Modal';
import { ConfirmDialog } from '../../components/ui/ConfirmDialog';
import { PermissionMatrix } from '../../components/settings/PermissionMatrix';

export interface AdminRolesSettingsViewProps {
  currentUser: User;
}

function extractError(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback;
}

export const AdminRolesSettingsView: React.FC<AdminRolesSettingsViewProps> = ({ currentUser }) => {
  const toast = useToast();
  const canCreate = hasPermission(currentUser, 'create', 'roles');
  const canUpdate = hasPermission(currentUser, 'update', 'roles');
  const canDelete = hasPermission(currentUser, 'delete', 'roles');

  const [roles, setRoles] = useState<Role[]>([]);
  const [permissions, setPermissions] = useState<Permission[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  const [mode, setMode] = useState<'list' | 'edit' | 'create'>('list');
  const [activeRole, setActiveRole] = useState<Role | null>(null);
  const [form, setForm] = useState({
    name: '',
    slug: '',
    description: '',
    permission_ids: [] as string[],
  });
  const [isSaving, setIsSaving] = useState(false);

  const [dupTarget, setDupTarget] = useState<Role | null>(null);
  const [dupForm, setDupForm] = useState({ name: '', slug: '', description: '' });
  const [isDuplicating, setIsDuplicating] = useState(false);

  const [deleteTarget, setDeleteTarget] = useState<Role | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  const loadData = useCallback(async () => {
    setIsLoading(true);
    try {
      const [rolesRes, permsRes] = await Promise.all([rolesApi.list(), rolesApi.listPermissions()]);
      setRoles(rolesRes.data || []);
      setPermissions(permsRes.data || []);
    } catch (err) {
      toast.error('Failed to load roles', extractError(err, 'Could not fetch roles and permissions.'));
    } finally {
      setIsLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const openCreate = () => {
    setActiveRole(null);
    setForm({
      name: '',
      slug: '',
      description: '',
      permission_ids: [],
    });
    setMode('create');
  };

  const openEdit = (role: Role) => {
    setActiveRole(role);
    setForm({
      name: role.name,
      slug: role.slug,
      description: role.description || '',
      permission_ids: (role.permissions || []).map((p) => p.id || '').filter(Boolean),
    });
    setMode('edit');
  };

  const openDuplicate = (role: Role) => {
    setDupTarget(role);
    setDupForm({
      name: `${role.name} Copy`,
      slug: `${role.slug}_copy`,
      description: role.description || '',
    });
  };

  const handleSaveRole = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSaving(true);
    try {
      if (mode === 'create') {
        await rolesApi.create({
          name: form.name.trim(),
          slug: form.slug.trim() || undefined,
          description: form.description.trim(),
          permission_ids: form.permission_ids,
        });
        toast.success('Role created', `${form.name} has been saved.`);
      } else if (activeRole) {
        await rolesApi.update(activeRole.id, {
          name: form.name.trim(),
          slug: activeRole.is_system ? undefined : form.slug.trim(),
          description: form.description.trim(),
          permission_ids: form.permission_ids,
        });
        toast.success('Role updated', `Permissions for ${form.name} were updated.`);
      }
      setMode('list');
      await loadData();
    } catch (err) {
      toast.error('Save failed', extractError(err, 'Could not save role configuration.'));
    } finally {
      setIsSaving(false);
    }
  };

  const handleDuplicate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!dupTarget) return;
    setIsDuplicating(true);
    try {
      const res = await rolesApi.duplicate(dupTarget.id, {
        name: dupForm.name.trim(),
        slug: dupForm.slug.trim(),
        description: dupForm.description.trim(),
      });
      toast.success('Role duplicated', `Created custom role ${res.name}.`);
      setDupTarget(null);
      await loadData();
    } catch (err) {
      toast.error('Duplicate failed', extractError(err, 'Could not duplicate role.'));
    } finally {
      setIsDuplicating(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setIsDeleting(true);
    try {
      await rolesApi.remove(deleteTarget.id);
      toast.success('Role deleted', `${deleteTarget.name} was removed.`);
      setDeleteTarget(null);
      await loadData();
    } catch (err) {
      toast.error('Cannot delete role', extractError(err, 'Role could not be deleted.'));
    } finally {
      setIsDeleting(false);
    }
  };

  if (mode === 'create' || mode === 'edit') {
    const isSuperLocked = activeRole?.slug === 'super_admin';
    const isSystemRole = Boolean(activeRole?.is_system);

    return (
      <form onSubmit={(e) => void handleSaveRole(e)} className="space-y-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <Button type="button" variant="outline" size="sm" onClick={() => setMode('list')}>
              <ArrowLeft className="w-3.5 h-3.5" />
              <span>All Roles</span>
            </Button>
            <div>
              <h3 className="text-base font-bold text-slate-900">
                {mode === 'create' ? 'Create New Role' : activeRole?.name}
              </h3>
              <p className="text-xs text-slate-500">
                {mode === 'create'
                  ? 'Define role details and choose allowed actions per resource.'
                  : 'Configure RBAC permissions assigned to users holding this role.'}
              </p>
            </div>
          </div>

          {!isSuperLocked && (mode === 'create' ? canCreate : canUpdate) && (
            <Button type="submit" size="sm" isLoading={isSaving}>
              <Save className="w-3.5 h-3.5" />
              <span>{mode === 'create' ? 'Create Role' : 'Save Role'}</span>
            </Button>
          )}
        </div>

        <Card className="p-5 space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Input
              label="Role Name"
              required
              disabled={isSuperLocked}
              placeholder="e.g. Managing Editor"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            />
            <Input
              label="Role Slug"
              disabled={isSystemRole}
              helperText={
                isSystemRole
                  ? 'Built-in system role slugs cannot be altered.'
                  : 'Lowercase identifier used by RBAC policies (auto-generated if blank).'
              }
              placeholder="e.g. managing_editor"
              value={form.slug}
              onChange={(e) => setForm((f) => ({ ...f, slug: e.target.value }))}
            />
          </div>
          <Textarea
            label="Description"
            rows={2}
            disabled={isSuperLocked}
            placeholder="Describe who holds this role and what capabilities it grants..."
            value={form.description}
            onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
          />
        </Card>

        <PermissionMatrix
          permissions={permissions}
          selectedIds={form.permission_ids}
          disabled={mode === 'edit' && !canUpdate}
          isSuperAdminLocked={isSuperLocked}
          onChange={(permission_ids) => setForm((f) => ({ ...f, permission_ids }))}
        />
      </form>
    );
  }

  const columns: DataTableColumn<Role>[] = [
    {
      key: 'name',
      header: 'Role',
      render: (r) => (
        <div>
          <div className="flex items-center gap-2">
            <Shield className="w-3.5 h-3.5 text-slate-400" />
            <span className="font-semibold text-slate-900">{r.name}</span>
            {r.is_system && <Badge variant="neutral">System</Badge>}
          </div>
          {r.description && <p className="text-xs text-slate-500 mt-0.5">{r.description}</p>}
        </div>
      ),
    },
    {
      key: 'slug',
      header: 'Identifier',
      render: (r) => <code className="text-xs font-mono text-slate-600">{r.slug}</code>,
    },
    {
      key: 'users',
      header: 'Users',
      render: (r) => <Badge variant="info">{r.user_count ?? 0} users</Badge>,
    },
    {
      key: 'perms',
      header: 'Permissions',
      render: (r) => (
        <span className="text-xs text-slate-600 font-medium">
          {r.slug === 'super_admin' ? 'All (*:*)' : `${r.permissions?.length ?? 0} assigned`}
        </span>
      ),
    },
    {
      key: 'actions',
      header: <span className="sr-only">Actions</span>,
      className: 'text-right',
      render: (r) => {
        const blockedDelete = Boolean(r.is_system) || (r.user_count ?? 0) > 0;
        const deleteReason = r.is_system
          ? 'Built-in system roles cannot be deleted'
          : (r.user_count ?? 0) > 0
            ? 'Reassign users from this role before deleting'
            : 'Delete role';

        return (
          <div className="flex items-center justify-end gap-1.5">
            <Button type="button" variant="outline" size="sm" onClick={() => openEdit(r)}>
              {r.slug === 'super_admin' ? 'Inspect' : 'Permissions'}
            </Button>
            {canCreate && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                title="Duplicate role"
                onClick={() => openDuplicate(r)}
              >
                <Copy className="w-3.5 h-3.5 text-slate-600" />
              </Button>
            )}
            {canDelete && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={blockedDelete}
                title={deleteReason}
                onClick={() => setDeleteTarget(r)}
              >
                <Trash2 className="w-3.5 h-3.5 text-rose-600" />
              </Button>
            )}
          </div>
        );
      },
    },
  ];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-xs text-slate-500">
          Every role bundles fine-grained RBAC rules across content, media, administration, and system modules.
        </p>
        {canCreate && (
          <Button type="button" size="sm" onClick={openCreate}>
            <Plus className="w-3.5 h-3.5" />
            <span>Add New Role</span>
          </Button>
        )}
      </div>

      <Card className="overflow-hidden">
        <DataTable columns={columns} rows={roles} rowKey={(r) => r.id} isLoading={isLoading} />
      </Card>

      <Modal
        isOpen={Boolean(dupTarget)}
        onClose={() => setDupTarget(null)}
        title={dupTarget ? `Duplicate ${dupTarget.name}` : 'Duplicate Role'}
        maxWidth="md"
      >
        <form onSubmit={(e) => void handleDuplicate(e)} className="space-y-4">
          <Input
            label="New Role Name"
            required
            value={dupForm.name}
            onChange={(e) => setDupForm((f) => ({ ...f, name: e.target.value }))}
          />
          <Input
            label="Slug"
            value={dupForm.slug}
            onChange={(e) => setDupForm((f) => ({ ...f, slug: e.target.value }))}
          />
          <Textarea
            label="Description"
            rows={2}
            value={dupForm.description}
            onChange={(e) => setDupForm((f) => ({ ...f, description: e.target.value }))}
          />
          <div className="flex justify-end gap-2 pt-2 border-t border-slate-100">
            <Button type="button" variant="outline" onClick={() => setDupTarget(null)}>
              Cancel
            </Button>
            <Button type="submit" isLoading={isDuplicating}>
              Create Copy
            </Button>
          </div>
        </form>
      </Modal>

      <ConfirmDialog
        isOpen={Boolean(deleteTarget)}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Delete custom role?"
        description={`Delete "${deleteTarget?.name || ''}" permanently? This only succeeds when zero users are assigned.`}
        confirmLabel="Delete Role"
        isLoading={isDeleting}
      />
    </div>
  );
};
