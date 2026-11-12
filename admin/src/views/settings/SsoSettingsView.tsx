import React, { useEffect, useState } from 'react';
import { Globe, Plus, Search, ShieldCheck, Trash2 } from 'lucide-react';
import { rolesApi, settingsApi, usersPermApi } from '../../api/endpoints';
import { Alert } from '../../components/ui/Alert';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { Modal } from '../../components/ui/Modal';
import { SecretInput } from '../../components/ui/SecretInput';
import { Select } from '../../components/ui/Select';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import { useToast } from '../../components/ui/Toast';
import type { OAuthProvider, Role, SSOSettings } from '../../types';

export interface SsoSettingsViewProps {
  canEdit: boolean;
}

export const SsoSettingsView: React.FC<SsoSettingsViewProps> = ({ canEdit }) => {
  const toast = useToast();
  const [loading, setLoading] = useState(true);
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [policy, setPolicy] = useState<SSOSettings | null>(null);
  const [domainsText, setDomainsText] = useState('');
  const [roles, setRoles] = useState<Role[]>([]);
  const [providers, setProviders] = useState<OAuthProvider[]>([]);

  // Provider modal state
  const [modalOpen, setModalOpen] = useState(false);
  const [editingProvider, setEditingProvider] = useState<OAuthProvider | null>(null);
  const [provForm, setProvForm] = useState({
    name: 'admin_oidc',
    display_name: 'Enterprise OIDC SSO',
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
  const [savingProvider, setSavingProvider] = useState(false);
  const [modalError, setModalError] = useState<string | null>(null);

  const loadAll = async () => {
    setLoading(true);
    setError(null);
    try {
      const [ssoRes, rolesRes, provRes] = await Promise.all([
        settingsApi.getSso(),
        rolesApi.list(),
        usersPermApi.listProviders('admin'),
      ]);
      setPolicy(ssoRes.data);
      setDomainsText((ssoRes.data.allowed_domains || []).join(', '));
      setRoles(rolesRes.data || []);
      setProviders(provRes.data || []);
    } catch (err: any) {
      setError(err?.message || 'Failed to load Enterprise SSO settings.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadAll();
  }, []);

  const handleSavePolicy = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!policy) return;
    setSavingPolicy(true);
    setError(null);
    try {
      const parsedDomains = domainsText
        .split(/[\n,]/)
        .map((s) => s.trim())
        .filter(Boolean);
      const res = await settingsApi.updateSso({
        ...policy,
        allowed_domains: parsedDomains,
      });
      setPolicy(res.data);
      setDomainsText((res.data.allowed_domains || []).join(', '));
      toast.success('Enterprise SSO policy updated');
    } catch (err: any) {
      const msg = err?.message || 'Failed to save SSO policy';
      setError(msg);
      toast.error('SSO save failed', msg);
    } finally {
      setSavingPolicy(false);
    }
  };

  const openEditModal = (prov?: OAuthProvider) => {
    setModalError(null);
    if (prov) {
      setEditingProvider(prov);
      setProvForm({
        name: prov.name,
        display_name: prov.display_name,
        enabled: prov.enabled,
        client_id: prov.client_id || '',
        client_secret: prov.client_secret || '',
        secret_configured: prov.secret_configured,
        issuer_url: prov.issuer_url || '',
        auth_url: prov.auth_url || '',
        token_url: prov.token_url || '',
        userinfo_url: prov.userinfo_url || '',
        redirect_uri: prov.redirect_uri || '',
        scopes: prov.scopes || 'openid email profile',
      });
    } else {
      setEditingProvider(null);
      setProvForm({
        name: 'okta_sso',
        display_name: 'Corporate Okta SSO',
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
    if (!provForm.issuer_url.trim()) {
      setModalError('Enter an OpenID Connect Issuer URL first.');
      return;
    }
    setDiscovering(true);
    setModalError(null);
    try {
      const res = await usersPermApi.discoverOidc(provForm.issuer_url.trim());
      setProvForm((prev) => ({
        ...prev,
        issuer_url: res.data.issuer || prev.issuer_url,
        auth_url: res.data.authorization_endpoint || prev.auth_url,
        token_url: res.data.token_endpoint || prev.token_url,
        userinfo_url: res.data.userinfo_endpoint || prev.userinfo_url,
      }));
      toast.success('OIDC endpoints discovered');
    } catch (err: any) {
      setModalError(err?.message || 'OIDC discovery failed');
    } finally {
      setDiscovering(false);
    }
  };

  const handleSaveProvider = async (e: React.FormEvent) => {
    e.preventDefault();
    setSavingProvider(true);
    setModalError(null);
    try {
      await usersPermApi.upsertProvider(provForm.name, {
        name: provForm.name,
        display_name: provForm.display_name,
        provider_type: 'oidc',
        scope: 'admin',
        enabled: provForm.enabled,
        client_id: provForm.client_id,
        client_secret: provForm.client_secret,
        issuer_url: provForm.issuer_url,
        auth_url: provForm.auth_url,
        token_url: provForm.token_url,
        userinfo_url: provForm.userinfo_url,
        redirect_uri: provForm.redirect_uri,
        scopes: provForm.scopes,
      });
      toast.success('SSO Identity Provider saved');
      setModalOpen(false);
      await loadAll();
    } catch (err: any) {
      setModalError(err?.message || 'Failed to save SSO provider');
    } finally {
      setSavingProvider(false);
    }
  };

  const handleDeleteProvider = async (name: string) => {
    try {
      await usersPermApi.deleteProvider(name);
      toast.success(`Provider "${name}" removed`);
      await loadAll();
    } catch (err: any) {
      toast.error('Delete failed', err?.message);
    }
  };

  if (loading || !policy) {
    return (
      <div className="space-y-6">
        <SettingsCardSkeleton rows={4} />
        <SettingsCardSkeleton rows={4} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {error && <Alert variant="error">{error}</Alert>}

      {/* Enterprise SSO Policy */}
      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <ShieldCheck className="w-4 h-4 text-sky-600" />
              <span>Enterprise Single Sign-On (Admin Panel)</span>
            </span>
          }
          description="Configure OpenID Connect SSO login, just-in-time admin provisioning, and domain enforcement for administrators."
          action={
            <Badge variant={policy.enabled ? 'success' : 'neutral'}>
              {policy.enabled ? 'SSO Enabled' : 'SSO Disabled'}
            </Badge>
          }
        />
        <CardContent>
          <form onSubmit={(e) => void handleSavePolicy(e)} className="space-y-4">
            <div className="flex items-center justify-between p-4 rounded-lg bg-slate-50 border border-slate-200">
              <div>
                <p className="text-xs font-semibold text-slate-800">Enable Admin Panel SSO</p>
                <p className="text-[11px] text-slate-500">
                  Displays configured SSO provider buttons on the Admin Login screen.
                </p>
              </div>
              <Switch
                label="Enable Admin Panel SSO"
                checked={policy.enabled}
                disabled={!canEdit}
                onChange={(checked) => setPolicy({ ...policy, enabled: checked })}
              />
            </div>

            <div className="flex items-center justify-between p-4 rounded-lg bg-slate-50 border border-slate-200">
              <div>
                <p className="text-xs font-semibold text-slate-800">Just-in-Time Admin Auto-Provisioning</p>
                <p className="text-[11px] text-slate-500">
                  Automatically create active admin accounts on first SSO sign-in when email domain is permitted.
                </p>
              </div>
              <Switch
                label="Auto-Provision Admins"
                checked={policy.auto_provision_admin}
                disabled={!canEdit}
                onChange={(checked) => setPolicy({ ...policy, auto_provision_admin: checked })}
              />
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Select
                label="Default Provisioned Role"
                disabled={!canEdit}
                value={policy.default_role_id}
                onChange={(e) => setPolicy({ ...policy, default_role_id: e.target.value })}
                options={[
                  { label: 'Viewer (Default Safe Least-Privilege)', value: '' },
                  ...roles.map((r) => ({ label: `${r.name} (${r.slug})`, value: r.id })),
                ]}
              />
              <Input
                label="Allowed Email Domains"
                disabled={!canEdit}
                placeholder="acme.com, corp.example.org"
                value={domainsText}
                onChange={(e) => setDomainsText(e.target.value)}
                helperText="Comma-separated. Leave blank to permit any verified email domain."
              />
            </div>

            <div className="flex items-center justify-between p-4 rounded-lg bg-slate-50 border border-slate-200">
              <div>
                <p className="text-xs font-semibold text-slate-800">Enforce SSO for Allowed Domains</p>
                <p className="text-[11px] text-slate-500">
                  Disables password logins on Admin Panel for users whose email matches an allowed SSO domain.
                </p>
              </div>
              <Switch
                label="Enforce SSO for Domains"
                checked={policy.enforce_sso_for_domains}
                disabled={!canEdit}
                onChange={(checked) => setPolicy({ ...policy, enforce_sso_for_domains: checked })}
              />
            </div>

            {canEdit && (
              <div className="flex justify-end pt-2 border-t border-slate-100">
                <Button type="submit" size="sm" isLoading={savingPolicy}>
                  Save SSO Policy
                </Button>
              </div>
            )}
          </form>
        </CardContent>
      </Card>

      {/* Admin OIDC Providers */}
      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <Globe className="w-4 h-4 text-indigo-600" />
              <span>Admin Identity Providers (OIDC / OAuth2 + PKCE)</span>
            </span>
          }
          description="Register external IdPs (Okta, Azure Entra ID, Google Workspace, Keycloak) for administrator SSO."
          action={
            canEdit && (
              <Button type="button" size="sm" onClick={() => openEditModal()}>
                <Plus className="w-3.5 h-3.5 mr-1.5" />
                Add SSO Provider
              </Button>
            )
          }
        />
        <CardContent>
          {providers.length === 0 ? (
            <p className="text-xs text-slate-500 py-6 text-center">No Admin SSO providers configured yet.</p>
          ) : (
            <div className="divide-y divide-slate-100">
              {providers.map((p) => (
                <div key={p.name} className="py-3.5 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-semibold text-slate-800">{p.display_name || p.name}</span>
                      <Badge variant={p.enabled ? 'success' : 'neutral'}>
                        {p.enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                      <Badge variant="info">{p.provider_type.toUpperCase()}</Badge>
                    </div>
                    <p className="text-[11px] text-slate-500 font-mono mt-0.5">
                      Callback: {window.location.origin}/api/v1/auth/sso/{p.name}/callback
                    </p>
                  </div>
                  {canEdit && (
                    <div className="flex items-center gap-2">
                      <Button type="button" size="sm" variant="secondary" onClick={() => openEditModal(p)}>
                        Configure
                      </Button>
                      {p.name !== 'admin_oidc' && (
                        <Button
                          type="button"
                          size="sm"
                          variant="destructive"
                          onClick={() => void handleDeleteProvider(p.name)}
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </Button>
                      )}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Modal
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        title={editingProvider ? `Configure SSO Provider: ${editingProvider.display_name}` : 'Add Admin SSO Provider'}
        maxWidth="xl"
      >
        <form onSubmit={(e) => void handleSaveProvider(e)} className="space-y-4">
          {modalError && <Alert variant="error">{modalError}</Alert>}

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Provider Key"
              value={provForm.name}
              disabled={Boolean(editingProvider)}
              required
              onChange={(e) => setProvForm({ ...provForm, name: e.target.value })}
            />
            <Input
              label="Display Name"
              value={provForm.display_name}
              required
              onChange={(e) => setProvForm({ ...provForm, display_name: e.target.value })}
            />
          </div>

          <div className="flex items-center justify-between p-3 rounded-lg bg-slate-50 border border-slate-200">
            <span className="text-xs font-semibold text-slate-800">Enable Provider</span>
            <Switch
              label="Enable Provider"
              checked={provForm.enabled}
              onChange={(checked) => setProvForm({ ...provForm, enabled: checked })}
            />
          </div>

          <div className="flex gap-2 items-end">
            <div className="flex-1">
              <Input
                label="OIDC Issuer URL (optional for Discovery)"
                placeholder="https://accounts.example.com"
                value={provForm.issuer_url}
                onChange={(e) => setProvForm({ ...provForm, issuer_url: e.target.value })}
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
              value={provForm.client_id}
              onChange={(e) => setProvForm({ ...provForm, client_id: e.target.value })}
            />
            <SecretInput
              label="Client Secret"
              value={provForm.client_secret}
              hasStoredSecret={provForm.secret_configured}
              onChange={(val) => setProvForm({ ...provForm, client_secret: val })}
            />
          </div>

          <Input
            label="Authorization URL"
            value={provForm.auth_url}
            onChange={(e) => setProvForm({ ...provForm, auth_url: e.target.value })}
          />
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Token URL"
              value={provForm.token_url}
              onChange={(e) => setProvForm({ ...provForm, token_url: e.target.value })}
            />
            <Input
              label="Userinfo URL"
              autoComplete="off"
              value={provForm.userinfo_url}
              onChange={(e) => setProvForm({ ...provForm, userinfo_url: e.target.value })}
            />
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Scopes"
              value={provForm.scopes}
              onChange={(e) => setProvForm({ ...provForm, scopes: e.target.value })}
            />
            <Input
              label="Custom Redirect URI (optional)"
              placeholder={`${window.location.origin}/api/v1/auth/sso/${provForm.name}/callback`}
              value={provForm.redirect_uri}
              onChange={(e) => setProvForm({ ...provForm, redirect_uri: e.target.value })}
            />
          </div>

          <div className="flex justify-end gap-2 pt-2 border-t border-slate-100">
            <Button type="button" size="sm" variant="secondary" onClick={() => setModalOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" size="sm" isLoading={savingProvider}>
              Save Provider
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
