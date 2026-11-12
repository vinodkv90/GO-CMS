import React, { useCallback, useEffect, useState } from 'react';
import {
  Ban,
  Edit3,
  MailPlus,
  RefreshCw,
  Search,
  Trash2,
  Unlock,
  UserCheck,
  UserPlus,
  UserX,
} from 'lucide-react';
import { rolesApi, usersApi } from '../../api/endpoints';
import type { AdminInvitation, Role, User } from '../../types';
import { hasPermission, isSuperAdmin } from '../../utils/rbac';
import { useToast } from '../../components/ui/Toast';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Badge } from '../../components/ui/Badge';
import { Input } from '../../components/ui/Input';
import { Select } from '../../components/ui/Select';
import { DataTable, type DataTableColumn } from '../../components/ui/DataTable';
import { Pagination } from '../../components/ui/Pagination';
import { Modal } from '../../components/ui/Modal';
import { ConfirmDialog } from '../../components/ui/ConfirmDialog';
import { CopyableSecret } from '../../components/ui/CopyableSecret';
import { EmptyState } from '../../components/ui/EmptyState';

export interface AdminUsersSettingsViewProps {
  currentUser: User;
  onUserUpdated?: (user: User) => void;
  onNavigateRoles?: () => void;
}

const PAGE_LIMIT = 20;

function extractError(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback;
}

function buildInviteLink(rawToken: string): string {
  if (typeof window === 'undefined') return `?invite_token=${encodeURIComponent(rawToken)}`;
  const url = new URL(window.location.href);
  url.search = '';
  url.searchParams.set('invite_token', rawToken);
  return url.toString();
}

function statusBadgeVariant(status: string): 'success' | 'warning' | 'error' | 'neutral' {
  if (status === 'active' || status === 'accepted') return 'success';
  if (status === 'inactive' || status === 'pending') return 'warning';
  if (status === 'suspended' || status === 'revoked' || status === 'expired') return 'error';
  return 'neutral';
}

export const AdminUsersSettingsView: React.FC<AdminUsersSettingsViewProps> = ({
  currentUser,
  onUserUpdated,
  onNavigateRoles,
}) => {
  const toast = useToast();
  const canCreate = hasPermission(currentUser, 'create', 'users');
  const canUpdate = hasPermission(currentUser, 'update', 'users');
  const canDelete = hasPermission(currentUser, 'delete', 'users');
  const actorIsSuper = isSuperAdmin(currentUser);

  const [subTab, setSubTab] = useState<'users' | 'invitations'>('users');
  const [users, setUsers] = useState<User[]>([]);
  const [totalUsers, setTotalUsers] = useState(0);
  const [roles, setRoles] = useState<Role[]>([]);
  const [invitations, setInvitations] = useState<AdminInvitation[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isInvitesLoading, setIsInvitesLoading] = useState(false);

  // Filters & pagination
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [roleFilter, setRoleFilter] = useState('');
  const [offset, setOffset] = useState(0);

  // Invite / Create / Edit Modals
  const [inviteOpen, setInviteOpen] = useState(false);
  const [inviteForm, setInviteForm] = useState({
    email: '',
    username: '',
    first_name: '',
    last_name: '',
    role_ids: [] as string[],
    expires_in_hours: '72',
  });
  const [generatedInviteUrl, setGeneratedInviteUrl] = useState<string | null>(null);
  const [isSubmittingInvite, setIsSubmittingInvite] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState({
    username: '',
    email: '',
    password: '',
    first_name: '',
    last_name: '',
    status: 'active' as User['status'],
    role_ids: [] as string[],
  });
  const [isSubmittingCreate, setIsSubmittingCreate] = useState(false);

  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [editForm, setEditForm] = useState({
    email: '',
    first_name: '',
    last_name: '',
    status: 'active' as User['status'],
    role_ids: [] as string[],
  });
  const [isSubmittingEdit, setIsSubmittingEdit] = useState(false);

  const [confirmAction, setConfirmAction] = useState<{
    title: string;
    description: string;
    confirmLabel: string;
    run: () => Promise<void>;
  } | null>(null);
  const [isConfirmLoading, setIsConfirmLoading] = useState(false);

  const loadRoles = useCallback(async () => {
    try {
      const res = await rolesApi.list();
      setRoles(res.data || []);
    } catch {
      // Non-fatal if user lacks roles:read permission
    }
  }, []);

  const loadUsers = useCallback(async () => {
    setIsLoading(true);
    try {
      const res = await usersApi.list({
        search: search.trim() || undefined,
        status: statusFilter || undefined,
        role_id: roleFilter || undefined,
        limit: PAGE_LIMIT,
        offset,
      });
      setUsers(res.data || []);
      setTotalUsers(res.total ?? (res.data?.length || 0));
    } catch (err) {
      toast.error('Failed to load administrators', extractError(err, 'Unable to load user directory.'));
    } finally {
      setIsLoading(false);
    }
  }, [offset, roleFilter, search, statusFilter, toast]);

  const loadInvitations = useCallback(async () => {
    setIsInvitesLoading(true);
    try {
      const res = await usersApi.listInvitations();
      setInvitations(res.data || []);
    } catch (err) {
      toast.error('Failed to load invitations', extractError(err, 'Unable to load invitations.'));
    } finally {
      setIsInvitesLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void loadRoles();
  }, [loadRoles]);

  useEffect(() => {
    void loadUsers();
  }, [loadUsers]);

  useEffect(() => {
    if (subTab === 'invitations') {
      void loadInvitations();
    }
  }, [loadInvitations, subTab]);

  const defaultNonSuperRoleId = () => {
    const preferred = roles.find((r) => r.slug === 'editor') || roles.find((r) => r.slug !== 'super_admin') || roles[0];
    return preferred ? [preferred.id] : [];
  };

  const openInviteModal = () => {
    setInviteForm({
      email: '',
      username: '',
      first_name: '',
      last_name: '',
      role_ids: defaultNonSuperRoleId(),
      expires_in_hours: '72',
    });
    setGeneratedInviteUrl(null);
    setInviteOpen(true);
  };

  const openCreateModal = () => {
    setCreateForm({
      username: '',
      email: '',
      password: '',
      first_name: '',
      last_name: '',
      status: 'active',
      role_ids: defaultNonSuperRoleId(),
    });
    setCreateOpen(true);
  };

  const openEditModal = (u: User) => {
    setEditingUser(u);
    setEditForm({
      email: u.email,
      first_name: u.first_name || '',
      last_name: u.last_name || '',
      status: u.status,
      role_ids: (u.roles || []).map((r) => r.id),
    });
  };

  const toggleRoleSelection = (roleId: string, selected: string[], setter: (next: string[]) => void) => {
    const exists = selected.includes(roleId);
    setter(exists ? selected.filter((id) => id !== roleId) : [...selected, roleId]);
  };

  const handleSendInvite = async (e: React.FormEvent) => {
    e.preventDefault();
    if (inviteForm.role_ids.length === 0) {
      toast.error('Role required', 'Select at least one role for the invited administrator.');
      return;
    }
    setIsSubmittingInvite(true);
    try {
      const hours = parseInt(inviteForm.expires_in_hours, 10) || 72;
      const res = await usersApi.createInvitation({
        email: inviteForm.email.trim(),
        username: inviteForm.username.trim() || undefined,
        first_name: inviteForm.first_name.trim(),
        last_name: inviteForm.last_name.trim(),
        role_ids: inviteForm.role_ids,
        expires_in_hours: hours,
      });
      setGeneratedInviteUrl(buildInviteLink(res.token));
      toast.success('Invitation created', `Share the onboarding link with ${res.invitation.email}.`);
      void loadInvitations();
    } catch (err) {
      toast.error('Failed to invite user', extractError(err, 'Could not create invitation.'));
    } finally {
      setIsSubmittingInvite(false);
    }
  };

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    if (createForm.role_ids.length === 0) {
      toast.error('Role required', 'Assign at least one role.');
      return;
    }
    setIsSubmittingCreate(true);
    try {
      await usersApi.create({
        username: createForm.username.trim(),
        email: createForm.email.trim(),
        password: createForm.password,
        first_name: createForm.first_name.trim(),
        last_name: createForm.last_name.trim(),
        status: createForm.status,
        role_ids: createForm.role_ids,
      });
      toast.success('Administrator created');
      setCreateOpen(false);
      void loadUsers();
      void loadRoles();
    } catch (err) {
      toast.error('Failed to create user', extractError(err, 'Could not create administrator.'));
    } finally {
      setIsSubmittingCreate(false);
    }
  };

  const handleSaveEdit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingUser) return;
    if (editForm.role_ids.length === 0) {
      toast.error('Role required', 'Each user must keep at least one role.');
      return;
    }
    setIsSubmittingEdit(true);
    try {
      const res = await usersApi.update(editingUser.id, {
        email: editForm.email.trim(),
        first_name: editForm.first_name.trim(),
        last_name: editForm.last_name.trim(),
        status: editForm.status,
        role_ids: editForm.role_ids,
      });
      toast.success('Administrator updated');
      if (editingUser.id === currentUser.id && onUserUpdated) {
        onUserUpdated(res);
      }
      setEditingUser(null);
      void loadUsers();
      void loadRoles();
    } catch (err) {
      toast.error('Update failed', extractError(err, 'Could not update administrator.'));
    } finally {
      setIsSubmittingEdit(false);
    }
  };

  const runConfirm = async () => {
    if (!confirmAction) return;
    setIsConfirmLoading(true);
    try {
      await confirmAction.run();
      setConfirmAction(null);
    } finally {
      setIsConfirmLoading(false);
    }
  };

  const userColumns: DataTableColumn<User>[] = [
    {
      key: 'identity',
      header: 'Administrator',
      render: (u) => {
        const display = `${u.first_name || ''} ${u.last_name || ''}`.trim() || u.username;
        return (
          <div>
            <div className="flex items-center gap-2">
              <span className="font-semibold text-slate-900">{display}</span>
              {u.id === currentUser.id && <Badge variant="info">You</Badge>}
            </div>
            <p className="text-xs text-slate-500">
              @{u.username} &middot; {u.email}
            </p>
          </div>
        );
      },
    },
    {
      key: 'roles',
      header: 'Roles',
      render: (u) => (
        <div className="flex flex-wrap gap-1.5">
          {u.roles && u.roles.length > 0 ? (
            u.roles.map((r) => (
              <Badge key={r.id} variant={r.slug === 'super_admin' ? 'info' : 'neutral'}>
                {r.name}
              </Badge>
            ))
          ) : (
            <span className="text-xs text-slate-400">No role assigned</span>
          )}
        </div>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      render: (u) => {
        const isLocked = Boolean(u.locked_until && new Date(u.locked_until).getTime() > Date.now());
        return (
          <div className="flex flex-wrap items-center gap-1.5">
            <Badge variant={statusBadgeVariant(u.status)}>{u.status}</Badge>
            {isLocked && <Badge variant="error">locked</Badge>}
            {u.mfa_enabled && <Badge variant="success">2FA</Badge>}
          </div>
        );
      },
    },
    {
      key: 'last_login',
      header: 'Last Login',
      render: (u) => (
        <span className="text-xs text-slate-500">
          {u.last_login_at ? new Date(u.last_login_at).toLocaleString() : 'Never'}
        </span>
      ),
    },
    {
      key: 'actions',
      header: <span className="sr-only">Actions</span>,
      className: 'text-right',
      render: (u) => {
        const isSelf = u.id === currentUser.id;
        const targetIsSuper = isSuperAdmin(u);
        const lockedByHierarchy = !actorIsSuper && targetIsSuper;
        const isLocked = Boolean(u.locked_until && new Date(u.locked_until).getTime() > Date.now());
        return (
          <div className="flex items-center justify-end gap-1.5">
            {canUpdate && isLocked && !lockedByHierarchy && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={async () => {
                  try {
                    await usersApi.unlock(u.id);
                    toast.success('Administrator unlocked');
                    void loadUsers();
                  } catch (err) {
                    toast.error('Unlock failed', extractError(err, 'Failed to unlock administrator.'));
                  }
                }}
                title="Clear brute-force lockout"
              >
                <Unlock className="w-3.5 h-3.5" />
                <span>Unlock</span>
              </Button>
            )}
            {canUpdate && !lockedByHierarchy && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => openEditModal(u)}
                title="Edit administrator"
              >
                <Edit3 className="w-3.5 h-3.5" />
                <span>Edit</span>
              </Button>
            )}

            {canUpdate && !isSelf && !lockedByHierarchy && (
              u.status === 'active' ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    setConfirmAction({
                      title: 'Deactivate administrator?',
                      description: `${u.email} will no longer be able to sign in until reactivated.`,
                      confirmLabel: 'Deactivate',
                      run: async () => {
                        try {
                          await usersApi.deactivate(u.id);
                          toast.success('Administrator deactivated');
                          void loadUsers();
                        } catch (err) {
                          toast.error('Cannot deactivate user', extractError(err, 'Failed to deactivate account.'));
                        }
                      },
                    })
                  }
                  title="Deactivate user"
                >
                  <UserX className="w-3.5 h-3.5 text-amber-600" />
                </Button>
              ) : (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={async () => {
                    try {
                      await usersApi.activate(u.id);
                      toast.success('Administrator activated');
                      void loadUsers();
                    } catch (err) {
                      toast.error('Activation failed', extractError(err, 'Could not activate user.'));
                    }
                  }}
                  title="Activate user"
                >
                  <UserCheck className="w-3.5 h-3.5 text-emerald-600" />
                </Button>
              )
            )}

            {canDelete && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={isSelf || lockedByHierarchy}
                title={
                  isSelf
                    ? 'Cannot delete your own account'
                    : lockedByHierarchy
                      ? 'Only a Super Administrator can remove another Super Administrator'
                      : 'Delete user'
                }
                onClick={() =>
                  setConfirmAction({
                    title: 'Delete administrator?',
                    description: `Permanently delete ${u.username} (${u.email})? This action cannot be undone.`,
                    confirmLabel: 'Delete User',
                    run: async () => {
                      try {
                        await usersApi.remove(u.id);
                        toast.success('Administrator deleted');
                        void loadUsers();
                        void loadRoles();
                      } catch (err) {
                        toast.error('Delete blocked', extractError(err, 'Failed to delete user.'));
                      }
                    },
                  })
                }
              >
                <Trash2 className="w-3.5 h-3.5 text-rose-600" />
              </Button>
            )}
          </div>
        );
      },
    },
  ];

  const invitationColumns: DataTableColumn<AdminInvitation>[] = [
    {
      key: 'recipient',
      header: 'Recipient',
      render: (inv) => (
        <div>
          <p className="font-semibold text-slate-900">{inv.email}</p>
          <p className="text-xs text-slate-500">
            @{inv.username}
            {(inv.first_name || inv.last_name) && ` · ${`${inv.first_name || ''} ${inv.last_name || ''}`.trim()}`}
          </p>
        </div>
      ),
    },
    {
      key: 'roles',
      header: 'Assigned Roles',
      render: (inv) => (
        <div className="flex flex-wrap gap-1.5">
          {(inv.roles || []).map((r) => (
            <Badge key={r.id} variant="neutral">
              {r.name}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      render: (inv) => <Badge variant={statusBadgeVariant(inv.status)}>{inv.status}</Badge>,
    },
    {
      key: 'expires',
      header: 'Expires',
      render: (inv) => (
        <span className="text-xs text-slate-500">{new Date(inv.expires_at).toLocaleString()}</span>
      ),
    },
    {
      key: 'actions',
      header: <span className="sr-only">Actions</span>,
      className: 'text-right',
      render: (inv) => (
        <div className="flex items-center justify-end gap-1.5">
          {canCreate && inv.status !== 'accepted' && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={async () => {
                try {
                  const res = await usersApi.resendInvitation(inv.id, 72);
                  setInviteForm({
                    email: inv.email,
                    username: inv.username,
                    first_name: inv.first_name || '',
                    last_name: inv.last_name || '',
                    role_ids: inv.role_ids,
                    expires_in_hours: '72',
                  });
                  setGeneratedInviteUrl(buildInviteLink(res.token));
                  setInviteOpen(true);
                  void loadInvitations();
                } catch (err) {
                  toast.error('Resend failed', extractError(err, 'Could not rotate invitation link.'));
                }
              }}
            >
              <RefreshCw className="w-3.5 h-3.5" />
              <span>Resend</span>
            </Button>
          )}
          {canUpdate && inv.status === 'pending' && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              title="Revoke invitation"
              onClick={async () => {
                try {
                  await usersApi.revokeInvitation(inv.id);
                  toast.info('Invitation revoked');
                  void loadInvitations();
                } catch (err) {
                  toast.error('Revoke failed', extractError(err, 'Could not revoke invitation.'));
                }
              }}
            >
              <Ban className="w-3.5 h-3.5 text-amber-600" />
            </Button>
          )}
          {canDelete && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              title="Delete invitation"
              onClick={async () => {
                try {
                  await usersApi.deleteInvitation(inv.id);
                  toast.success('Invitation removed');
                  void loadInvitations();
                } catch (err) {
                  toast.error('Delete failed', extractError(err, 'Could not delete invitation.'));
                }
              }}
            >
              <Trash2 className="w-3.5 h-3.5 text-rose-600" />
            </Button>
          )}
        </div>
      ),
    },
  ];

  const renderRoleCheckboxes = (selectedIds: string[], onChange: (next: string[]) => void) => (
    <div className="space-y-1.5">
      <label className="block text-xs font-semibold text-slate-700">Assigned Roles</label>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-48 overflow-y-auto p-1">
        {roles.map((r) => {
          const isSuperRole = r.slug === 'super_admin';
          const disabledRole = isSuperRole && !actorIsSuper;
          const checked = selectedIds.includes(r.id);
          return (
            <label
              key={r.id}
              className={`flex items-start gap-2.5 p-2.5 rounded-lg border text-xs transition-colors ${
                checked ? 'border-sky-300 bg-sky-50/70' : 'border-slate-200 bg-white'
              } ${disabledRole ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer hover:bg-slate-50'}`}
            >
              <input
                type="checkbox"
                checked={checked}
                disabled={disabledRole}
                onChange={() => toggleRoleSelection(r.id, selectedIds, onChange)}
                className="mt-0.5 rounded border-slate-300 text-sky-600 focus:ring-sky-500"
              />
              <div>
                <p className="font-semibold text-slate-900">{r.name}</p>
                {r.description && <p className="text-[11px] text-slate-500">{r.description}</p>}
              </div>
            </label>
          );
        })}
      </div>
    </div>
  );

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="inline-flex rounded-xl bg-slate-100 p-1">
          <button
            type="button"
            onClick={() => setSubTab('users')}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-colors ${
              subTab === 'users' ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600 hover:text-slate-900'
            }`}
          >
            Administrators ({totalUsers})
          </button>
          <button
            type="button"
            onClick={() => setSubTab('invitations')}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-colors ${
              subTab === 'invitations'
                ? 'bg-white text-slate-900 shadow-sm'
                : 'text-slate-600 hover:text-slate-900'
            }`}
          >
            Invitations
          </button>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {onNavigateRoles && (
            <Button type="button" variant="outline" size="sm" onClick={onNavigateRoles}>
              Manage Roles
            </Button>
          )}
          {canCreate && (
            <>
              <Button type="button" variant="outline" size="sm" onClick={openCreateModal}>
                <UserPlus className="w-3.5 h-3.5" />
                <span>Add Directly</span>
              </Button>
              <Button type="button" size="sm" onClick={openInviteModal}>
                <MailPlus className="w-3.5 h-3.5" />
                <span>Invite Administrator</span>
              </Button>
            </>
          )}
        </div>
      </div>

      {subTab === 'users' ? (
        <Card className="overflow-hidden">
          <div className="p-4 border-b border-slate-100 grid grid-cols-1 sm:grid-cols-4 gap-3 bg-slate-50/50">
            <div className="sm:col-span-2 relative">
              <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={search}
                onChange={(e) => {
                  setOffset(0);
                  setSearch(e.target.value);
                }}
                placeholder="Search name, username, or email..."
                className="w-full pl-8 pr-3 py-2 text-sm bg-white border border-slate-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-sky-500/20 focus:border-sky-500"
              />
            </div>

            <Select
              value={statusFilter}
              onChange={(e) => {
                setOffset(0);
                setStatusFilter(e.target.value);
              }}
              options={[
                { label: 'All statuses', value: '' },
                { label: 'Active', value: 'active' },
                { label: 'Inactive', value: 'inactive' },
                { label: 'Suspended', value: 'suspended' },
              ]}
            />

            <Select
              value={roleFilter}
              onChange={(e) => {
                setOffset(0);
                setRoleFilter(e.target.value);
              }}
              options={[
                { label: 'All roles', value: '' },
                ...roles.map((r) => ({ label: r.name, value: r.id })),
              ]}
            />
          </div>

          <DataTable
            columns={userColumns}
            rows={users}
            rowKey={(u) => u.id}
            isLoading={isLoading}
            emptyState={
              <EmptyState
                title="No administrators matched"
                description="Try clearing your search filters or invite a new administrator."
              />
            }
          />
          <Pagination total={totalUsers} limit={PAGE_LIMIT} offset={offset} onPageChange={setOffset} />
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <DataTable
            columns={invitationColumns}
            rows={invitations}
            rowKey={(inv) => inv.id}
            isLoading={isInvitesLoading}
            emptyState={
              <EmptyState
                title="No invitations issued yet"
                description="Invite collaborators with an expiring onboarding link so they can choose their own password."
                action={
                  canCreate ? (
                    <Button type="button" size="sm" onClick={openInviteModal}>
                      <MailPlus className="w-3.5 h-3.5" />
                      <span>Invite Administrator</span>
                    </Button>
                  ) : undefined
                }
              />
            }
          />
        </Card>
      )}

      {/* Invite Modal */}
      <Modal
        isOpen={inviteOpen}
        onClose={() => setInviteOpen(false)}
        title={generatedInviteUrl ? 'Invitation Link Generated' : 'Invite Administrator'}
        maxWidth="lg"
      >
        {generatedInviteUrl ? (
          <div className="space-y-4">
            <CopyableSecret
              title={`Onboarding link for ${inviteForm.email}`}
              description="Share this link directly with the recipient. For security, it cannot be recovered after closing this dialog."
              label="Invitation Accept URL"
              secret={generatedInviteUrl}
            />
            <div className="flex justify-end">
              <Button type="button" onClick={() => setInviteOpen(false)}>
                Done
              </Button>
            </div>
          </div>
        ) : (
          <form onSubmit={(e) => void handleSendInvite(e)} className="space-y-4">
            <Input
              label="Email address"
              type="email"
              required
              placeholder="teammate@company.com"
              value={inviteForm.email}
              onChange={(e) => setInviteForm((f) => ({ ...f, email: e.target.value }))}
            />
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Input
                label="Suggested username"
                placeholder="Optional (defaults from email)"
                value={inviteForm.username}
                onChange={(e) => setInviteForm((f) => ({ ...f, username: e.target.value }))}
              />
              <Select
                label="Expiration"
                value={inviteForm.expires_in_hours}
                onChange={(e) => setInviteForm((f) => ({ ...f, expires_in_hours: e.target.value }))}
                options={[
                  { label: '24 hours', value: '24' },
                  { label: '72 hours (3 days)', value: '72' },
                  { label: '168 hours (7 days)', value: '168' },
                ]}
              />
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Input
                label="First name"
                value={inviteForm.first_name}
                onChange={(e) => setInviteForm((f) => ({ ...f, first_name: e.target.value }))}
              />
              <Input
                label="Last name"
                value={inviteForm.last_name}
                onChange={(e) => setInviteForm((f) => ({ ...f, last_name: e.target.value }))}
              />
            </div>

            {renderRoleCheckboxes(inviteForm.role_ids, (role_ids) =>
              setInviteForm((f) => ({ ...f, role_ids })),
            )}

            <div className="flex justify-end gap-2 pt-2 border-t border-slate-100">
              <Button type="button" variant="outline" onClick={() => setInviteOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" isLoading={isSubmittingInvite}>
                Generate Invite Link
              </Button>
            </div>
          </form>
        )}
      </Modal>

      {/* Direct Create Modal */}
      <Modal
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Add Administrator Account"
        maxWidth="lg"
      >
        <form onSubmit={(e) => void handleCreateUser(e)} className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Username"
              required
              value={createForm.username}
              onChange={(e) => setCreateForm((f) => ({ ...f, username: e.target.value }))}
            />
            <Input
              label="Email address"
              type="email"
              required
              value={createForm.email}
              onChange={(e) => setCreateForm((f) => ({ ...f, email: e.target.value }))}
            />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="First name"
              value={createForm.first_name}
              onChange={(e) => setCreateForm((f) => ({ ...f, first_name: e.target.value }))}
            />
            <Input
              label="Last name"
              value={createForm.last_name}
              onChange={(e) => setCreateForm((f) => ({ ...f, last_name: e.target.value }))}
            />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Temporary Password"
              type="password"
              required
              helperText="8+ chars with upper, lower, digit, symbol."
              value={createForm.password}
              onChange={(e) => setCreateForm((f) => ({ ...f, password: e.target.value }))}
            />
            <Select
              label="Account Status"
              value={createForm.status}
              onChange={(e) => setCreateForm((f) => ({ ...f, status: e.target.value as User['status'] }))}
              options={[
                { label: 'Active', value: 'active' },
                { label: 'Inactive', value: 'inactive' },
              ]}
            />
          </div>

          {renderRoleCheckboxes(createForm.role_ids, (role_ids) =>
            setCreateForm((f) => ({ ...f, role_ids })),
          )}

          <div className="flex justify-end gap-2 pt-2 border-t border-slate-100">
            <Button type="button" variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" isLoading={isSubmittingCreate}>
              Create Administrator
            </Button>
          </div>
        </form>
      </Modal>

      {/* Edit User Modal */}
      <Modal
        isOpen={Boolean(editingUser)}
        onClose={() => setEditingUser(null)}
        title={editingUser ? `Edit ${editingUser.username}` : 'Edit Administrator'}
        maxWidth="lg"
      >
        <form onSubmit={(e) => void handleSaveEdit(e)} className="space-y-4">
          <Input
            label="Email address"
            type="email"
            required
            value={editForm.email}
            onChange={(e) => setEditForm((f) => ({ ...f, email: e.target.value }))}
          />
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="First name"
              value={editForm.first_name}
              onChange={(e) => setEditForm((f) => ({ ...f, first_name: e.target.value }))}
            />
            <Input
              label="Last name"
              value={editForm.last_name}
              onChange={(e) => setEditForm((f) => ({ ...f, last_name: e.target.value }))}
            />
          </div>
          <Select
            label="Status"
            value={editForm.status}
            disabled={editingUser?.id === currentUser.id}
            helperText={editingUser?.id === currentUser.id ? 'You cannot deactivate your own session.' : undefined}
            onChange={(e) => setEditForm((f) => ({ ...f, status: e.target.value as User['status'] }))}
            options={[
              { label: 'Active', value: 'active' },
              { label: 'Inactive', value: 'inactive' },
              { label: 'Suspended', value: 'suspended' },
            ]}
          />

          {renderRoleCheckboxes(editForm.role_ids, (role_ids) =>
            setEditForm((f) => ({ ...f, role_ids })),
          )}

          <div className="flex justify-end gap-2 pt-2 border-t border-slate-100">
            <Button type="button" variant="outline" onClick={() => setEditingUser(null)}>
              Cancel
            </Button>
            <Button type="submit" isLoading={isSubmittingEdit}>
              Save Changes
            </Button>
          </div>
        </form>
      </Modal>

      <ConfirmDialog
        isOpen={Boolean(confirmAction)}
        onClose={() => setConfirmAction(null)}
        onConfirm={runConfirm}
        title={confirmAction?.title || ''}
        description={confirmAction?.description || ''}
        confirmLabel={confirmAction?.confirmLabel || 'Confirm'}
        isLoading={isConfirmLoading}
      />
    </div>
  );
};
