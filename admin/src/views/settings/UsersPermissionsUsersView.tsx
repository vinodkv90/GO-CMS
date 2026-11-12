import React, { useEffect, useState } from 'react';
import { Lock, MailCheck, Plus, Search, Trash2, Unlock, Users } from 'lucide-react';
import { usersPermApi } from '../../api/endpoints';
import { Alert } from '../../components/ui/Alert';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { Modal } from '../../components/ui/Modal';
import { Select } from '../../components/ui/Select';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import { useToast } from '../../components/ui/Toast';
import type { AppRole, AppUser } from '../../types';

export interface UsersPermissionsUsersViewProps {
  canEdit: boolean;
}

export const UsersPermissionsUsersView: React.FC<UsersPermissionsUsersViewProps> = ({ canEdit }) => {
  const toast = useToast();
  const [users, setUsers] = useState<AppUser[]>([]);
  const [roles, setRoles] = useState<AppRole[]>([]);
  const [total, setTotal] = useState(0);
  const [search, setSearch] = useState('');
  const [roleFilter, setRoleFilter] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<AppUser | null>(null);
  const [form, setForm] = useState({
    username: '',
    email: '',
    password: '',
    role_id: '',
    confirmed: true,
    blocked: false,
  });
  const [saving, setSaving] = useState(false);
  const [modalError, setModalError] = useState<string | null>(null);

  const loadData = async () => {
    setLoading(true);
    setError(null);
    try {
      const [usersRes, rolesRes] = await Promise.all([
        usersPermApi.listUsers({ search: search || undefined, role_id: roleFilter || undefined, limit: 50 }),
        usersPermApi.listRoles(),
      ]);
      setUsers(usersRes.data || []);
      setTotal(usersRes.meta?.total ?? (usersRes.data || []).length);
      setRoles(rolesRes.data || []);
    } catch (err: any) {
      setError(err?.message || 'Failed to load application users');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadData();
  }, [roleFilter]);

  const openModal = (user?: AppUser) => {
    setModalError(null);
    if (user) {
      setEditing(user);
      setForm({
        username: user.username,
        email: user.email,
        password: '',
        role_id: user.role_id || '',
        confirmed: user.confirmed,
        blocked: user.blocked,
      });
    } else {
      setEditing(null);
      const defaultRole = roles.find((r) => r.is_default) || roles[0];
      setForm({
        username: '',
        email: '',
        password: '',
        role_id: defaultRole?.id || '',
        confirmed: true,
        blocked: false,
      });
    }
    setModalOpen(true);
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setModalError(null);
    try {
      if (editing) {
        await usersPermApi.updateUser(editing.id, {
          username: form.username,
          email: form.email,
          password: form.password || undefined,
          role_id: form.role_id,
          confirmed: form.confirmed,
          blocked: form.blocked,
        });
        toast.success(`Updated user "${form.username}"`);
      } else {
        await usersPermApi.createUser(form);
        toast.success(`Created user "${form.username}"`);
      }
      setModalOpen(false);
      await loadData();
    } catch (err: any) {
      setModalError(err?.message || 'Failed to save user');
    } finally {
      setSaving(false);
    }
  };

  const toggleBlocked = async (u: AppUser) => {
    try {
      await usersPermApi.updateUser(u.id, { blocked: !u.blocked });
      toast.success(u.blocked ? `Unblocked "${u.username}"` : `Blocked "${u.username}"`);
      await loadData();
    } catch (err: any) {
      toast.error('Update failed', err?.message);
    }
  };

  const handleDelete = async (u: AppUser) => {
    try {
      await usersPermApi.deleteUser(u.id);
      toast.success(`Deleted user "${u.username}"`);
      await loadData();
    } catch (err: any) {
      toast.error('Delete failed', err?.message);
    }
  };

  if (loading && users.length === 0) {
    return <SettingsCardSkeleton rows={5} />;
  }

  return (
    <div className="space-y-6">
      {error && <Alert variant="error">{error}</Alert>}

      <Alert variant="info" title="Isolated End-User Accounts">
        Application users shown here authenticate exclusively via <code className="font-mono">/api/v1/app-auth/*</code> and never grant access to the CMS Administration Panel.
      </Alert>

      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <Users className="w-4 h-4 text-sky-600" />
              <span>Application Users ({total})</span>
            </span>
          }
          description="Manage registered frontend/mobile end-users, assigned roles, email confirmation, and account blocks."
          action={
            canEdit && (
              <Button type="button" size="sm" onClick={() => openModal()}>
                <Plus className="w-3.5 h-3.5 mr-1.5" />
                Add Application User
              </Button>
            )
          }
        />
        <CardContent className="space-y-4">
          <div className="flex flex-col sm:flex-row gap-3">
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void loadData();
              }}
              className="flex-1 flex gap-2"
            >
              <Input
                placeholder="Search by username or email..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
              <Button type="submit" size="sm" variant="secondary">
                <Search className="w-3.5 h-3.5 mr-1" />
                Search
              </Button>
            </form>
            <div className="w-full sm:w-56">
              <Select
                value={roleFilter}
                onChange={(e) => setRoleFilter(e.target.value)}
                options={[
                  { label: 'All App Roles', value: '' },
                  ...roles.map((r) => ({ label: r.name, value: r.id })),
                ]}
              />
            </div>
          </div>

          {users.length === 0 ? (
            <div className="py-10 text-center text-xs text-slate-500">
              No application users found matching the current filters.
            </div>
          ) : (
            <div className="overflow-x-auto border border-slate-200 rounded-lg">
              <table className="w-full text-left border-collapse text-xs">
                <thead className="bg-slate-50 border-b border-slate-200 text-slate-600 font-semibold">
                  <tr>
                    <th className="py-2.5 px-3">User</th>
                    <th className="py-2.5 px-3">Role</th>
                    <th className="py-2.5 px-3">Provider</th>
                    <th className="py-2.5 px-3">Status</th>
                    {canEdit && <th className="py-2.5 px-3 text-right">Actions</th>}
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {users.map((u) => (
                    <tr key={u.id} className="hover:bg-slate-50/70">
                      <td className="py-2.5 px-3">
                        <p className="font-semibold text-slate-800">{u.username}</p>
                        <p className="text-[11px] text-slate-500">{u.email}</p>
                      </td>
                      <td className="py-2.5 px-3">
                        <Badge variant="info">{u.role?.name || 'Authenticated'}</Badge>
                      </td>
                      <td className="py-2.5 px-3 font-mono text-[11px] text-slate-600">{u.provider}</td>
                      <td className="py-2.5 px-3">
                        <div className="flex flex-wrap gap-1.5">
                          <Badge variant={u.confirmed ? 'success' : 'warning'}>
                            {u.confirmed ? 'Confirmed' : 'Pending'}
                          </Badge>
                          {u.blocked && <Badge variant="error">Blocked</Badge>}
                        </div>
                      </td>
                      {canEdit && (
                        <td className="py-2.5 px-3 text-right">
                          <div className="inline-flex items-center gap-1.5">
                            <Button type="button" size="sm" variant="secondary" onClick={() => openModal(u)}>
                              Edit
                            </Button>
                            <Button
                              type="button"
                              size="sm"
                              variant="secondary"
                              onClick={() => void toggleBlocked(u)}
                              title={u.blocked ? 'Unblock user' : 'Block user'}
                            >
                              {u.blocked ? <Unlock className="w-3.5 h-3.5" /> : <Lock className="w-3.5 h-3.5" />}
                            </Button>
                            <Button
                              type="button"
                              size="sm"
                              variant="destructive"
                              onClick={() => void handleDelete(u)}
                              title="Delete user"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </Button>
                          </div>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      <Modal
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        title={editing ? `Edit Application User: ${editing.username}` : 'Create Application User'}
      >
        <form onSubmit={(e) => void handleSave(e)} className="space-y-4">
          {modalError && <Alert variant="error">{modalError}</Alert>}
          <Input
            label="Username"
            required
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.target.value })}
          />
          <Input
            label="Email"
            type="email"
            required
            value={form.email}
            onChange={(e) => setForm({ ...form, email: e.target.value })}
          />
          <Input
            label={editing ? 'New Password (leave blank to keep current)' : 'Password'}
            type="password"
            required={!editing}
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
          />
          <Select
            label="Assigned Role"
            value={form.role_id}
            onChange={(e) => setForm({ ...form, role_id: e.target.value })}
            options={roles.map((r) => ({ label: r.name, value: r.id }))}
          />
          <div className="grid grid-cols-2 gap-3">
            <div className="flex items-center justify-between p-3 rounded-lg bg-slate-50 border border-slate-200">
              <span className="text-xs font-medium text-slate-800 flex items-center gap-1.5">
                <MailCheck className="w-3.5 h-3.5 text-emerald-600" /> Confirmed
              </span>
              <Switch
                label="Confirmed"
                checked={form.confirmed}
                onChange={(checked) => setForm({ ...form, confirmed: checked })}
              />
            </div>
            <div className="flex items-center justify-between p-3 rounded-lg bg-slate-50 border border-slate-200">
              <span className="text-xs font-medium text-slate-800">Blocked</span>
              <Switch
                label="Blocked"
                checked={form.blocked}
                onChange={(checked) => setForm({ ...form, blocked: checked })}
              />
            </div>
          </div>
          <div className="flex justify-end gap-2 pt-2 border-t border-slate-100">
            <Button type="button" size="sm" variant="secondary" onClick={() => setModalOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" size="sm" isLoading={saving}>
              {editing ? 'Save Changes' : 'Create User'}
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
