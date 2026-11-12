import React, { useEffect, useState } from 'react';
import { Globe, Plus, Search, Trash2 } from 'lucide-react';
import { usersPermApi } from '../../api/endpoints';
import { Alert } from '../../components/ui/Alert';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { Modal } from '../../components/ui/Modal';
import { SecretInput } from '../../components/ui/SecretInput';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import { useToast } from '../../components/ui/Toast';
import type { OAuthProvider } from '../../types';

export interface UsersPermissionsProvidersViewProps {
  canEdit: boolean;
}

const BUILTIN_APP_PROVIDERS = new Set(['google', 'github', 'discord', 'oidc']);

export const UsersPermissionsProvidersView: React.FC<UsersPermissionsProvidersViewProps> = ({ canEdit }) => {
  const toast = useToast();
  const [providers, setProviders] = useState<OAuthProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<OAuthProvider | null>(null);
  const [form, setForm] = useState({
    name: '',
    display_name: '',
    provider_type: 'oidc',
    enabled: false,
    client_id: '',
    client_secret: '',
    secret_configured: false,
    issuer_url: '',
    auth_url: '',
    token_url: '',
    userinfo_url: '',
    redirect_uri: '',
    scopes: 'openid email profile',
  });
  const [discovering, setDiscovering] = useState(false);
  const [saving, setSaving] = useState(false);
  const [modalError, setModalError] = useState<string | null>(null);

  const loadProviders = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await usersPermApi.listProviders('app');
      setProviders(res.data || []);
    } catch (err: any) {
      setError(err?.message || 'Failed to load OAuth providers');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadProviders();
  }, []);

  const openModal = (p?: OAuthProvider) => {
    setModalError(null);
    if (p) {
      setEditing(p);
      setForm({
        name: p.name,
        display_name: p.display_name,
        provider_type: p.provider_type || 'oidc',
        enabled: p.enabled,
        client_id: p.client_id || '',
        client_secret: p.client_secret || '',
        secret_configured: p.secret_configured,
        issuer_url: p.issuer_url || '',
        auth_url: p.auth_url || '',
        token_url: p.token_url || '',
        userinfo_url: p.userinfo_url || '',
        redirect_uri: p.redirect_uri || '',
        scopes: p.scopes || 'openid email profile',
      });
    } else {
      setEditing(null);
      setForm({
        name: 'custom_oidc',
        display_name: 'Custom OAuth2',
        provider_type: 'oidc',
        enabled: true,
        client_id: '',
        client_secret: '',
        secret_configured: false,
        issuer_url: '',
        auth_url: '',
        token_url: '',
        userinfo_url: '',
        redirect_uri: '',
        scopes: 'openid email profile',
      });
    }
    setModalOpen(true);
  };

  const handleDiscover = async () => {
    if (!form.issuer_url.trim()) {
      setModalError('Enter an Issuer URL to run OIDC discovery.');
      return;
    }
    setDiscovering(true);
    setModalError(null);
    try {
      const res = await usersPermApi.discoverOidc(form.issuer_url.trim());
      setForm((prev) => ({
        ...prev,
        issuer_url: res.data.issuer || prev.issuer_url,
        auth_url: res.data.authorization_endpoint || prev.auth_url,
        token_url: res.data.token_endpoint || prev.token_url,
        userinfo_url: res.data.userinfo_endpoint || prev.userinfo_url,
      }));
      toast.success('OIDC configuration discovered');
    } catch (err: any) {
      setModalError(err?.message || 'Discovery failed');
    } finally {
      setDiscovering(false);
    }
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setModalError(null);
    try {
      await usersPermApi.upsertProvider(form.name, {
        ...form,
        scope: 'app',
      });
      toast.success(`Saved provider "${form.display_name || form.name}"`);
      setModalOpen(false);
      await loadProviders();
    } catch (err: any) {
      setModalError(err?.message || 'Failed to save provider');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (name: string) => {
    try {
      await usersPermApi.deleteProvider(name);
      toast.success(`Removed provider "${name}"`);
      await loadProviders();
    } catch (err: any) {
      toast.error('Delete failed', err?.message);
    }
  };

  if (loading) {
    return <SettingsCardSkeleton rows={5} />;
  }

  return (
    <div className="space-y-6">
      {error && <Alert variant="error">{error}</Alert>}

      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <Globe className="w-4 h-4 text-sky-600" />
              <span>Application Authentication Providers</span>
            </span>
          }
          description="Configure OAuth 2.0 + PKCE and OpenID Connect identity providers for end-user sign-in (/api/v1/app-auth/providers/*)."
          action={
            canEdit && (
              <Button type="button" size="sm" onClick={() => openModal()}>
                <Plus className="w-3.5 h-3.5 mr-1.5" />
                Add Custom Provider
              </Button>
            )
          }
        />
        <CardContent>
          <div className="divide-y divide-slate-100">
            {providers.map((p) => (
              <div key={p.name} className="py-3.5 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-semibold text-slate-800">{p.display_name || p.name}</span>
                    <Badge variant={p.enabled ? 'success' : 'neutral'}>
                      {p.enabled ? 'Enabled' : 'Disabled'}
                    </Badge>
                    {p.secret_configured && <Badge variant="info">Secret Stored</Badge>}
                  </div>
                  <p className="text-[11px] text-slate-500 font-mono mt-0.5">
                    Callback URL: {window.location.origin}/api/v1/app-auth/providers/{p.name}/callback
                  </p>
                </div>
                {canEdit && (
                  <div className="flex items-center gap-2">
                    <Button type="button" size="sm" variant="secondary" onClick={() => openModal(p)}>
                      Configure
                    </Button>
                    {!BUILTIN_APP_PROVIDERS.has(p.name) && (
                      <Button type="button" size="sm" variant="destructive" onClick={() => void handleDelete(p.name)}>
                        <Trash2 className="w-3.5 h-3.5" />
                      </Button>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <Modal
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        title={editing ? `Configure Provider: ${editing.display_name}` : 'Add OAuth / OIDC Provider'}
        maxWidth="xl"
      >
        <form onSubmit={(e) => void handleSave(e)} className="space-y-4">
          {modalError && <Alert variant="error">{modalError}</Alert>}

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Provider Identifier"
              value={form.name}
              disabled={Boolean(editing)}
              required
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
            <Input
              label="Display Name"
              value={form.display_name}
              required
              onChange={(e) => setForm({ ...form, display_name: e.target.value })}
            />
          </div>

          <div className="flex items-center justify-between p-3 rounded-lg bg-slate-50 border border-slate-200">
            <span className="text-xs font-semibold text-slate-800">Enable Provider for End-Users</span>
            <Switch
              label="Enable Provider"
              checked={form.enabled}
              onChange={(checked) => setForm({ ...form, enabled: checked })}
            />
          </div>

          <div className="flex gap-2 items-end">
            <div className="flex-1">
              <Input
                label="OIDC Issuer URL (optional Discovery)"
                placeholder="https://accounts.google.com"
                value={form.issuer_url}
                onChange={(e) => setForm({ ...form, issuer_url: e.target.value })}
              />
            </div>
            <Button type="button" size="sm" variant="secondary" onClick={() => void handleDiscover()} isLoading={discovering}>
              <Search className="w-3.5 h-3.5 mr-1" />
              Discover OIDC
            </Button>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Client ID"
              autoComplete="off"
              value={form.client_id}
              onChange={(e) => setForm({ ...form, client_id: e.target.value })}
            />
            <SecretInput
              label="Client Secret"
              value={form.client_secret}
              hasStoredSecret={form.secret_configured}
              onChange={(val) => setForm({ ...form, client_secret: val })}
            />
          </div>

          <Input
            label="Authorization URL"
            value={form.auth_url}
            onChange={(e) => setForm({ ...form, auth_url: e.target.value })}
          />
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Token URL"
              value={form.token_url}
              onChange={(e) => setForm({ ...form, token_url: e.target.value })}
            />
            <Input
              label="Userinfo URL"
              value={form.userinfo_url}
              onChange={(e) => setForm({ ...form, userinfo_url: e.target.value })}
            />
          </div>
          <Input
            label="Scopes"
            value={form.scopes}
            onChange={(e) => setForm({ ...form, scopes: e.target.value })}
          />

          <div className="flex justify-end gap-2 pt-2 border-t border-slate-100">
            <Button type="button" size="sm" variant="secondary" onClick={() => setModalOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" size="sm" isLoading={saving}>
              Save Provider
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
