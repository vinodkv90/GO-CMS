import React, { useEffect, useState } from 'react';
import { Plus, Shield, Trash2 } from 'lucide-react';
import { usersPermApi } from '../../api/endpoints';
import { Alert } from '../../components/ui/Alert';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { Modal } from '../../components/ui/Modal';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import { useToast } from '../../components/ui/Toast';
import type { AppRole } from '../../types';

export interface UsersPermissionsRolesViewProps {
  canEdit: boolean;
}

const PERMISSION_GROUPS: { resource: string; label: string; actions: string[] }[] = [
  { resource: 'content', label: 'Content Entries (API)', actions: ['find', 'findOne', 'create', 'update', 'delete'] },
  { resource: 'media', label: 'Media Assets (Upload)', actions: ['find', 'findOne', 'upload'] },
  { resource: 'app-users', label: 'End-User Profile', actions: ['me'] },
];

export const UsersPermissionsRolesView: React.FC<UsersPermissionsRolesViewProps> = ({ canEdit }) => {
  const toast = useToast();
  const [roles, setRoles] = useState<AppRole[]>([]);
  const [selectedId, setSelectedId] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Selected role draft
  const [draftName, setDraftName] = useState('');
  const [draftDesc, setDraftDesc] = useState('');
  const [draftDefault, setDraftDefault] = useState(false);
  const [draftPerms, setDraftPerms] = useState<Record<string, Record<string, boolean>>>({});

  // Create modal
  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState('');
  const [newSlug, setNewSlug] = useState('');
  const [newDesc, setNewDesc] = useState('');
  const [creating, setCreating] = useState(false);

  const loadRoles = async (selectTarget?: string) => {
    setLoading(true);
    setError(null);
    try {
      const res = await usersPermApi.listRoles();
      const items = res.data || [];
      setRoles(items);
      const next =
        items.find((r) => r.id === (selectTarget || selectedId)) ||
        items.find((r) => r.slug === 'authenticated') ||
        items[0];
      if (next) {
        selectRole(next);
      }
    } catch (err: any) {
      setError(err?.message || 'Failed to load application roles');
    } finally {
      setLoading(false);
    }
  };

  const selectRole = (role: AppRole) => {
    setSelectedId(role.id);
    setDraftName(role.name);
    setDraftDesc(role.description || '');
    setDraftDefault(role.is_default);
    setDraftPerms(JSON.parse(JSON.stringify(role.permissions || {})));
  };

  useEffect(() => {
    void loadRoles();
  }, []);

  const togglePerm = (resource: string, action: string) => {
    if (!canEdit) return;
    setDraftPerms((prev) => ({
      ...prev,
      [resource]: {
        ...(prev[resource] || {}),
        [action]: !(prev[resource]?.[action] ?? false),
      },
    }));
  };

  const handleSaveRole = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedId) return;
    setSaving(true);
    setError(null);
    try {
      await usersPermApi.updateRole(selectedId, {
        name: draftName,
        description: draftDesc,
        is_default: draftDefault,
        permissions: draftPerms,
      });
      toast.success('Application role permissions saved');
      await loadRoles(selectedId);
    } catch (err: any) {
      setError(err?.message || 'Failed to save role permissions');
    } finally {
      setSaving(false);
    }
  };

  const handleCreateRole = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreating(true);
    try {
      const res = await usersPermApi.createRole({
        name: newName,
        slug: newSlug || undefined,
        description: newDesc,
        permissions: { content: { find: true, findOne: true } },
      });
      toast.success(`Created role "${res.data.name}"`);
      setCreateOpen(false);
      setNewName('');
      setNewSlug('');
      setNewDesc('');
      await loadRoles(res.data.id);
    } catch (err: any) {
      toast.error('Create failed', err?.message);
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteRole = async (role: AppRole) => {
    try {
      await usersPermApi.deleteRole(role.id);
      toast.success(`Deleted role "${role.name}"`);
      await loadRoles();
    } catch (err: any) {
      toast.error('Cannot delete role', err?.message);
    }
  };

  if (loading) {
    return <SettingsCardSkeleton rows={6} />;
  }

  const selectedRole = roles.find((r) => r.id === selectedId);
  const isBuiltIn = selectedRole?.slug === 'public' || selectedRole?.slug === 'authenticated';

  return (
    <div className="space-y-6">
      {error && <Alert variant="error">{error}</Alert>}

      <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
        {/* Role Sidebar List */}
        <Card className="lg:col-span-4">
          <CardHeader
            title="End-User Roles"
            description="Permission profiles for cms_app_users."
            action={
              canEdit && (
                <Button type="button" size="sm" onClick={() => setCreateOpen(true)}>
                  <Plus className="w-3.5 h-3.5 mr-1" />
                  Add Role
                </Button>
              )
            }
          />
          <CardContent className="p-2 space-y-1">
            {roles.map((r) => {
              const active = r.id === selectedId;
              return (
                <button
                  key={r.id}
                  type="button"
                  onClick={() => selectRole(r)}
                  className={`w-full text-left px-3 py-2.5 rounded-lg border transition-colors ${
                    active
                      ? 'bg-sky-50 border-sky-200 text-sky-900'
                      : 'bg-white border-transparent hover:bg-slate-50 text-slate-700'
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-xs font-semibold flex items-center gap-1.5">
                      <Shield className="w-3.5 h-3.5 text-sky-600" />
                      {r.name}
                    </span>
                    <div className="flex items-center gap-1">
                      {r.is_default && <Badge variant="info">Default</Badge>}
                      <Badge variant="neutral">{r.user_count ?? 0}</Badge>
                    </div>
                  </div>
                  <p className="text-[11px] text-slate-500 mt-0.5 line-clamp-1">{r.description}</p>
                </button>
              );
            })}
          </CardContent>
        </Card>

        {/* Permission Editor */}
        <Card className="lg:col-span-8">
          {selectedRole ? (
            <form onSubmit={(e) => void handleSaveRole(e)}>
              <CardHeader
                title={`Permissions: ${selectedRole.name}`}
                description={`Slug: ${selectedRole.slug} • Type: ${selectedRole.type}`}
                action={
                  canEdit && !isBuiltIn && !selectedRole.is_default && (
                    <Button
                      type="button"
                      size="sm"
                      variant="destructive"
                      onClick={() => void handleDeleteRole(selectedRole)}
                    >
                      <Trash2 className="w-3.5 h-3.5 mr-1" />
                      Delete
                    </Button>
                  )
                }
              />
              <CardContent className="space-y-5">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <Input
                    label="Role Name"
                    value={draftName}
                    disabled={!canEdit}
                    required
                    onChange={(e) => setDraftName(e.target.value)}
                  />
                  <Input
                    label="Description"
                    value={draftDesc}
                    disabled={!canEdit}
                    onChange={(e) => setDraftDesc(e.target.value)}
                  />
                </div>

                {selectedRole.slug !== 'public' && (
                  <div className="flex items-center justify-between p-3 rounded-lg bg-slate-50 border border-slate-200">
                    <div>
                      <p className="text-xs font-semibold text-slate-800">Default Registration Role</p>
                      <p className="text-[11px] text-slate-500">Assigned automatically to newly registered application users.</p>
                    </div>
                    <Switch
                      label="Default Role"
                      checked={draftDefault}
                      disabled={!canEdit}
                      onChange={setDraftDefault}
                    />
                  </div>
                )}

                <div className="space-y-3">
                  <p className="text-xs font-bold uppercase tracking-wider text-slate-500">Resource Capabilities</p>
                  {PERMISSION_GROUPS.map((group) => (
                    <div key={group.resource} className="p-3.5 rounded-lg border border-slate-200 bg-slate-50/60">
                      <p className="text-xs font-semibold text-slate-800 mb-2.5">{group.label}</p>
                      <div className="flex flex-wrap gap-3">
                        {group.actions.map((action) => {
                          const enabled = Boolean(draftPerms[group.resource]?.[action]);
                          return (
                            <label
                              key={action}
                              className={`inline-flex items-center gap-2 px-2.5 py-1.5 rounded-md border text-xs cursor-pointer transition-colors ${
                                enabled
                                  ? 'bg-sky-50 border-sky-200 text-sky-800 font-semibold'
                                  : 'bg-white border-slate-200 text-slate-600'
                              }`}
                            >
                              <input
                                type="checkbox"
                                checked={enabled}
                                disabled={!canEdit}
                                onChange={() => togglePerm(group.resource, action)}
                                className="rounded border-slate-300 text-sky-600 focus:ring-sky-500"
                              />
                              <span className="font-mono">{action}</span>
                            </label>
                          );
                        })}
                      </div>
                    </div>
                  ))}
                </div>

                {canEdit && (
                  <div className="flex justify-end pt-3 border-t border-slate-100">
                    <Button type="submit" size="sm" isLoading={saving}>
                      Save Role Permissions
                    </Button>
                  </div>
                )}
              </CardContent>
            </form>
          ) : (
            <CardContent className="py-10 text-center text-xs text-slate-500">
              Select an application role to configure permissions.
            </CardContent>
          )}
        </Card>
      </div>

      <Modal isOpen={createOpen} onClose={() => setCreateOpen(false)} title="Create End-User Role">
        <form onSubmit={(e) => void handleCreateRole(e)} className="space-y-4">
          <Input label="Role Name" required value={newName} onChange={(e) => setNewName(e.target.value)} />
          <Input
            label="Slug (optional)"
            placeholder="premium_member"
            value={newSlug}
            onChange={(e) => setNewSlug(e.target.value)}
          />
          <Input
            label="Description"
            value={newDesc}
            onChange={(e) => setNewDesc(e.target.value)}
          />
          <div className="flex justify-end gap-2 pt-2 border-t border-slate-100">
            <Button type="button" size="sm" variant="secondary" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" size="sm" isLoading={creating}>
              Create Role
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
