import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Key, Plus, RefreshCw, ShieldOff, Trash2 } from 'lucide-react';
import { rolesApi, tokensApi } from '../../api/endpoints';
import type { ApiToken, CreateTokenPayload, Permission, TokenKind, User } from '../../types';
import { hasPermission } from '../../utils/rbac';
import { useToast } from '../ui/Toast';
import { Alert } from '../ui/Alert';
import { Badge } from '../ui/Badge';
import { Button } from '../ui/Button';
import { Card } from '../ui/Card';
import { ConfirmDialog } from '../ui/ConfirmDialog';
import { CopyableSecret } from '../ui/CopyableSecret';
import { DataTable, type DataTableColumn } from '../ui/DataTable';
import { EmptyState } from '../ui/EmptyState';
import { Input } from '../ui/Input';
import { Modal } from '../ui/Modal';
import { Select } from '../ui/Select';
import { Textarea } from '../ui/Textarea';

const EXPIRATION_OPTIONS = [
  { label: '7 days', value: String(7 * 86400) },
  { label: '30 days', value: String(30 * 86400) },
  { label: '90 days', value: String(90 * 86400) },
  { label: 'Unlimited (No expiration)', value: 'unlimited' },
];

const FALLBACK_API_SCOPES = [
  { resource: 'content', action: 'read', description: 'Read published/preview entries' },
  { resource: 'content', action: 'create', description: 'Create entries' },
  { resource: 'content', action: 'update', description: 'Update entries' },
  { resource: 'content', action: 'delete', description: 'Delete entries' },
  { resource: 'content', action: 'publish', description: 'Publish entries' },
  { resource: 'content_types', action: 'read', description: 'Read content-type schemas' },
  { resource: 'media', action: 'read', description: 'Read media library assets' },
  { resource: 'media', action: 'create', description: 'Upload media files' },
  { resource: 'media', action: 'update', description: 'Update media metadata' },
  { resource: 'media', action: 'delete', description: 'Delete media files' },
  { resource: 'system:locales', action: 'read', description: 'Read configured locales' },
];

function extractError(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback;
}

function tokenStatus(token: ApiToken): { label: string; variant: 'success' | 'warning' | 'error' } {
  if (token.revoked_at) {
    return { label: 'Revoked', variant: 'error' };
  }
  if (token.expires_at && new Date(token.expires_at).getTime() <= Date.now()) {
    return { label: 'Expired', variant: 'warning' };
  }
  return { label: 'Active', variant: 'success' };
}

function formatShortDate(iso?: string): string {
  if (!iso) return 'Never';
  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) return iso;
  return parsed.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}

export interface TokenManagerPanelProps {
  mode: 'api' | 'transfer';
  currentUser: User;
}

export const TokenManagerPanel: React.FC<TokenManagerPanelProps> = ({ mode, currentUser }) => {
  const toast = useToast();
  const canCreate = hasPermission(currentUser, 'create', 'api_tokens');
  const canDelete = hasPermission(currentUser, 'delete', 'api_tokens');
  const canReadRoles = hasPermission(currentUser, 'read', 'roles');

  const [activeKind, setActiveKind] = useState<'api' | 'admin'>('api');
  const effectiveKind: TokenKind = mode === 'transfer' ? 'transfer' : activeKind;

  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [catalogPerms, setCatalogPerms] = useState<Permission[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [tokenType, setTokenType] = useState<string>(mode === 'transfer' ? 'push_pull' : 'read_only');
  const [durationVal, setDurationVal] = useState<string>(String(30 * 86400));
  const [selectedScopes, setSelectedScopes] = useState<string[]>([]);

  const [revealedSecret, setRevealedSecret] = useState<{ title: string; token: string } | null>(null);
  const [confirmAction, setConfirmAction] = useState<{
    type: 'revoke' | 'delete';
    token: ApiToken;
  } | null>(null);
  const [isActionRunning, setIsActionRunning] = useState(false);

  const loadTokens = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res =
        mode === 'transfer'
          ? await tokensApi.listTransfer()
          : await tokensApi.list(activeKind);
      setTokens(Array.isArray(res.data) ? res.data : []);
    } catch (err) {
      setError(extractError(err, 'Failed to load tokens.'));
    } finally {
      setIsLoading(false);
    }
  }, [activeKind, mode]);

  useEffect(() => {
    void loadTokens();
  }, [loadTokens]);

  useEffect(() => {
    if (mode !== 'api' || !canReadRoles) return;
    let cancelled = false;
    rolesApi
      .listPermissions()
      .then((res) => {
        if (!cancelled && Array.isArray(res.data)) {
          setCatalogPerms(res.data);
        }
      })
      .catch(() => {
        // Non-super-admins without read:roles fall back to built-in content/media scope catalog
      });
    return () => {
      cancelled = true;
    };
  }, [canReadRoles, mode]);

  const availableCustomScopes = useMemo(() => {
    const rawList =
      catalogPerms.length > 0
        ? catalogPerms
        : FALLBACK_API_SCOPES;
    const filtered = rawList.filter((p) => {
      if (effectiveKind === 'api') {
        const res = p.resource.toLowerCase();
        return (
          res === 'content' ||
          res === 'content_types' ||
          res === 'media' ||
          res === 'system:locales'
        );
      }
      return p.resource.toLowerCase() !== 'transfer';
    });
    const byResource = new Map<string, Array<{ scope: string; action: string; description: string; allowed: boolean }>>();
    for (const p of filtered) {
      const scope = `${p.action}:${p.resource}`;
      const list = byResource.get(p.resource) ?? [];
      list.push({
        scope,
        action: p.action,
        description: p.description || scope,
        allowed: hasPermission(currentUser, p.action, p.resource),
      });
      byResource.set(p.resource, list);
    }
    return Array.from(byResource.entries());
  }, [catalogPerms, currentUser, effectiveKind]);

  const openCreateModal = () => {
    setName('');
    setDescription('');
    setTokenType(mode === 'transfer' ? 'push_pull' : 'read_only');
    setDurationVal(String(30 * 86400));
    setSelectedScopes([]);
    setIsCreateOpen(true);
  };

  const toggleScope = (scope: string) => {
    setSelectedScopes((prev) =>
      prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]
    );
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      toast.error('Validation error', 'Token name is required.');
      return;
    }
    if (tokenType === 'custom' && selectedScopes.length === 0) {
      toast.error('Validation error', 'Select at least one permission scope for a custom token.');
      return;
    }

    const payload: CreateTokenPayload = {
      name: name.trim(),
      kind: effectiveKind,
      description: description.trim() || undefined,
      token_type: tokenType,
      permissions: tokenType === 'custom' ? selectedScopes : undefined,
      never_expires: durationVal === 'unlimited',
      duration_seconds: durationVal === 'unlimited' ? undefined : Number(durationVal),
    };

    setIsSubmitting(true);
    try {
      const res =
        mode === 'transfer'
          ? await tokensApi.createTransfer(payload)
          : await tokensApi.create(payload);
      setIsCreateOpen(false);
      setRevealedSecret({
        title: `${res.metadata.name} created`,
        token: res.token,
      });
      toast.success('Token created', 'Copy your token now; it will not be displayed again.');
      await loadTokens();
    } catch (err) {
      toast.error('Could not create token', extractError(err, 'Failed to create token.'));
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleRotate = async (tok: ApiToken) => {
    try {
      const res = await tokensApi.rotate(tok.id);
      setRevealedSecret({
        title: `${tok.name} rotated`,
        token: res.token,
      });
      toast.success('Token rotated', 'The previous token has been immediately invalidated.');
      await loadTokens();
    } catch (err) {
      toast.error('Rotation failed', extractError(err, 'Failed to rotate token.'));
    }
  };

  const handleConfirmAction = async () => {
    if (!confirmAction) return;
    setIsActionRunning(true);
    try {
      if (confirmAction.type === 'revoke') {
        await tokensApi.revoke(confirmAction.token.id);
        toast.success('Token revoked', `${confirmAction.token.name} can no longer authenticate.`);
      } else {
        await tokensApi.remove(confirmAction.token.id);
        toast.success('Token deleted', `${confirmAction.token.name} has been permanently removed.`);
      }
      setConfirmAction(null);
      await loadTokens();
    } catch (err) {
      toast.error('Action failed', extractError(err, 'Failed to update token.'));
    } finally {
      setIsActionRunning(false);
    }
  };

  const typeOptions =
    mode === 'transfer'
      ? [
          { label: 'Push & Pull (Full transfer access)', value: 'push_pull' },
          { label: 'Push only (Import data into CMS)', value: 'push' },
          { label: 'Pull only (Export data from CMS)', value: 'pull' },
        ]
      : [
          { label: 'Read-only', value: 'read_only' },
          { label: 'Full access', value: 'full_access' },
          { label: 'Custom scopes', value: 'custom' },
        ];

  const columns: DataTableColumn<ApiToken>[] = [
    {
      key: 'name',
      header: 'Name',
      render: (tok) => (
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-medium text-neutral-900">{tok.name}</span>
            <Badge variant={tokenStatus(tok).variant}>{tokenStatus(tok).label}</Badge>
          </div>
          {tok.description && (
            <p className="text-xs text-neutral-500 mt-0.5 truncate max-w-xs">{tok.description}</p>
          )}
        </div>
      ),
    },
    {
      key: 'prefix',
      header: 'Prefix / Type',
      render: (tok) => (
        <div className="space-y-1">
          <code className="px-2 py-0.5 rounded bg-neutral-100 text-neutral-700 text-xs font-mono">
            {tok.token_prefix || (tok.kind === 'transfer' ? 'cms_trn_***' : 'cms_pat_***')}...
          </code>
          <div className="text-[11px] text-neutral-500 capitalize">
            {(tok.token_type || 'custom').replace(/_/g, ' ')}
          </div>
        </div>
      ),
    },
    {
      key: 'permissions',
      header: 'Scopes',
      render: (tok) => (
        <div className="flex flex-wrap gap-1 max-w-xs">
          {(tok.permissions || []).slice(0, 3).map((perm) => (
            <span
              key={perm}
              className="px-1.5 py-0.5 text-[11px] rounded bg-neutral-100 text-neutral-700 font-mono"
            >
              {perm}
            </span>
          ))}
          {(tok.permissions || []).length > 3 && (
            <span className="px-1.5 py-0.5 text-[11px] rounded bg-neutral-100 text-neutral-500">
              +{tok.permissions.length - 3} more
            </span>
          )}
        </div>
      ),
    },
    {
      key: 'last_used_at',
      header: 'Last used',
      render: (tok) => <span className="text-xs text-neutral-600">{formatShortDate(tok.last_used_at)}</span>,
    },
    {
      key: 'expires_at',
      header: 'Expires',
      render: (tok) => (
        <span className="text-xs text-neutral-600">
          {tok.expires_at ? formatShortDate(tok.expires_at) : 'Never'}
        </span>
      ),
    },
    {
      key: 'actions',
      header: <span className="sr-only">Actions</span>,
      className: 'text-right',
      render: (tok) => {
        const isRevoked = Boolean(tok.revoked_at);
        return (
          <div className="inline-flex items-center gap-1 justify-end">
            {canCreate && !isRevoked && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                title="Rotate token secret"
                onClick={() => void handleRotate(tok)}
              >
                <RefreshCw className="w-3.5 h-3.5" />
                <span className="hidden xl:inline">Rotate</span>
              </Button>
            )}
            {canDelete && !isRevoked && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                title="Revoke token"
                onClick={() => setConfirmAction({ type: 'revoke', token: tok })}
              >
                <ShieldOff className="w-3.5 h-3.5 text-amber-600" />
              </Button>
            )}
            {canDelete && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                title="Delete token"
                onClick={() => setConfirmAction({ type: 'delete', token: tok })}
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
        {mode === 'api' ? (
          <div className="inline-flex rounded-md border border-neutral-200 bg-neutral-100 p-0.5">
            <button
              type="button"
              onClick={() => setActiveKind('api')}
              className={`px-3 py-1.5 text-xs font-medium rounded transition-colors ${
                activeKind === 'api'
                  ? 'bg-white text-neutral-900 shadow-sm'
                  : 'text-neutral-600 hover:text-neutral-900'
              }`}
            >
              Content API Tokens
            </button>
            <button
              type="button"
              onClick={() => setActiveKind('admin')}
              className={`px-3 py-1.5 text-xs font-medium rounded transition-colors ${
                activeKind === 'admin'
                  ? 'bg-white text-neutral-900 shadow-sm'
                  : 'text-neutral-600 hover:text-neutral-900'
              }`}
            >
              Admin Personal Access Tokens
            </button>
          </div>
        ) : (
          <div className="text-xs text-neutral-600">
            Transfer tokens use the <code className="font-mono text-neutral-800">cms_trn_</code> prefix and are restricted strictly to data migration scopes.
          </div>
        )}

        {canCreate && (
          <Button type="button" onClick={openCreateModal}>
            <Plus className="w-4 h-4" />
            {mode === 'transfer'
              ? 'Create new Transfer Token'
              : activeKind === 'admin'
                ? 'Create Admin PAT'
                : 'Create new API Token'}
          </Button>
        )}
      </div>

      <Alert variant="info" title="Least-privilege enforcement">
        Tokens can only carry permissions held by their creator, are stored exclusively as SHA-256 hashes, and are evaluated against live RBAC roles on every request.
      </Alert>

      {error && (
        <Alert variant="error" title="Could not load tokens">
          {error}
        </Alert>
      )}

      <Card className="overflow-hidden">
        <DataTable
          columns={columns}
          rows={tokens}
          rowKey={(t) => t.id}
          isLoading={isLoading}
          emptyState={
            <EmptyState
              icon={<Key className="w-6 h-6 text-neutral-400" />}
              title={
                mode === 'transfer'
                  ? 'No transfer tokens yet'
                  : activeKind === 'admin'
                    ? 'No admin personal access tokens yet'
                    : 'No Content API tokens yet'
              }
              description="Generate a scoped programmatic token for headless integrations or automation pipelines."
              action={
                canCreate ? (
                  <Button type="button" size="sm" onClick={openCreateModal}>
                    <Plus className="w-4 h-4" />
                    Create token
                  </Button>
                ) : undefined
              }
            />
          }
        />
      </Card>

      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title={
          mode === 'transfer'
            ? 'Create Transfer Token'
            : effectiveKind === 'admin'
              ? 'Create Admin Personal Access Token'
              : 'Create API Token'
        }
        description="The plaintext token value is displayed once upon creation. Copy and store it securely."
        maxWidth="lg"
      >
        <form onSubmit={(e) => void handleCreate(e)} className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Input
              label="Token name"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={mode === 'transfer' ? 'Staging Sync Runner' : 'Next.js Storefront'}
            />
            <Select
              label="Token duration"
              options={EXPIRATION_OPTIONS}
              value={durationVal}
              onChange={(e) => setDurationVal(e.target.value)}
            />
          </div>

          <Textarea
            label="Description"
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Describe what system or pipeline uses this token..."
          />

          <Select
            label="Token type"
            options={typeOptions}
            value={tokenType}
            onChange={(e) => setTokenType(e.target.value)}
            helperText={
              effectiveKind === 'admin' && tokenType === 'full_access'
                ? 'Full-access Admin PAT (*:*) requires the Super Admin role.'
                : undefined
            }
          />

          {mode === 'api' && tokenType === 'custom' && (
            <div className="space-y-2 border border-neutral-200 rounded-md p-3 bg-neutral-50 max-h-64 overflow-y-auto">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold uppercase tracking-wide text-neutral-600">
                  Custom Permission Scopes ({selectedScopes.length} selected)
                </span>
                {selectedScopes.length > 0 && (
                  <button
                    type="button"
                    className="text-xs text-primary-600 hover:underline"
                    onClick={() => setSelectedScopes([])}
                  >
                    Clear all
                  </button>
                )}
              </div>
              <div className="space-y-3">
                {availableCustomScopes.map(([resource, items]) => (
                  <div key={resource} className="bg-white border border-neutral-200 rounded p-2.5">
                    <div className="text-xs font-semibold text-neutral-800 mb-1.5 font-mono">
                      {resource}
                    </div>
                    <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
                      {items.map((item) => {
                        const checked = selectedScopes.includes(item.scope);
                        return (
                          <label
                            key={item.scope}
                            className={`flex items-center gap-2 text-xs rounded px-2 py-1 border ${
                              item.allowed
                                ? 'border-neutral-200 hover:bg-neutral-50 cursor-pointer text-neutral-800'
                                : 'border-neutral-100 bg-neutral-50 text-neutral-400 cursor-not-allowed'
                            }`}
                            title={
                              item.allowed
                                ? item.description
                                : 'Requires this permission on your own admin role'
                            }
                          >
                            <input
                              type="checkbox"
                              disabled={!item.allowed}
                              checked={checked}
                              onChange={() => toggleScope(item.scope)}
                              className="rounded border-neutral-300 text-primary-600 focus:ring-primary-500"
                            />
                            <span className="truncate font-medium">{item.action}</span>
                          </label>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {effectiveKind === 'admin' && tokenType === 'full_access' && !hasPermission(currentUser, '*', '*') && (
            <Alert variant="warning" title="Super Admin required">
              Only Super Administrators can mint Full Access Admin Personal Access Tokens (*:*).
            </Alert>
          )}

          <div className="flex justify-end gap-2 pt-2 border-t border-neutral-200">
            <Button type="button" variant="outline" onClick={() => setIsCreateOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" isLoading={isSubmitting}>
              Generate token
            </Button>
          </div>
        </form>
      </Modal>

      <Modal
        isOpen={Boolean(revealedSecret)}
        onClose={() => setRevealedSecret(null)}
        title={revealedSecret?.title || 'Save your token'}
        description="Copy this token into your secrets manager before closing this dialog."
        maxWidth="md"
      >
        {revealedSecret && (
          <div className="space-y-4">
            <CopyableSecret
              title="One-time plaintext token"
              description="We never store plaintext tokens. If lost, rotate or regenerate a new token."
              secret={revealedSecret.token}
              label="Bearer Token"
            />
            <div className="flex justify-end">
              <Button type="button" onClick={() => setRevealedSecret(null)}>
                I have stored this token
              </Button>
            </div>
          </div>
        )}
      </Modal>

      <ConfirmDialog
        isOpen={Boolean(confirmAction)}
        onClose={() => setConfirmAction(null)}
        onConfirm={handleConfirmAction}
        isLoading={isActionRunning}
        variant="destructive"
        title={confirmAction?.type === 'revoke' ? 'Revoke API token?' : 'Delete API token?'}
        description={
          confirmAction?.type === 'revoke'
            ? `Revoking "${confirmAction.token.name}" immediately rejects any requests authenticating with it while keeping a disabled audit record.`
            : `Permanently delete "${confirmAction?.token.name}"? Any client using this token will lose access immediately.`
        }
        confirmLabel={confirmAction?.type === 'revoke' ? 'Revoke token' : 'Delete permanently'}
      />
    </div>
  );
};
