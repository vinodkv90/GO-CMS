import React, { useCallback, useState } from 'react';
import { Plus, X } from 'lucide-react';
import { settingsApi } from '../../api/endpoints';
import { useSettingsForm } from '../../hooks/useSettingsForm';
import { Alert } from '../../components/ui/Alert';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import type { CORSSettings } from '../../types';

const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'];

export interface CorsSettingsViewProps {
  canEdit: boolean;
  onDirtyChange: (dirty: boolean) => void;
  registerHandlers: (handlers: {
    save: () => Promise<void>;
    discard: () => void;
    isSaving: boolean;
  }) => void;
}

export const CorsSettingsView: React.FC<CorsSettingsViewProps> = ({
  canEdit,
  onDirtyChange,
  registerHandlers,
}) => {
  const [originDraft, setOriginDraft] = useState('');

  const load = useCallback(async () => {
    const res = await settingsApi.getCors();
    return res.data;
  }, []);

  const save = useCallback(async (value: CORSSettings) => {
    const res = await settingsApi.updateCors(value);
    return res.data;
  }, []);

  const validate = useCallback((v: CORSSettings): string | null => {
    if (v.enabled && v.allowed_origins.length === 0) {
      return 'Add at least one allowed origin when CORS is enabled.';
    }
    if (v.allow_credentials && v.allowed_origins.includes('*')) {
      return 'Wildcard origin (*) cannot be combined with Allow Credentials.';
    }
    if (v.max_age_seconds < 0 || v.max_age_seconds > 86400) {
      return 'Preflight max age must be between 0 and 86400 seconds.';
    }
    return null;
  }, []);

  const { form, setForm, isLoading, error } = useSettingsForm<CORSSettings>({
    load,
    save,
    validate,
    successTitle: 'CORS settings applied live',
    successMessage: 'Cross-origin HTTP policies have been updated in memory and persisted.',
    loadErrorFallback: 'Failed to load CORS settings.',
    saveErrorFallback: 'Failed to save CORS settings.',
    onDirtyChange,
    registerHandlers,
  });

  if (isLoading || !form) {
    return <SettingsCardSkeleton />;
  }

  const addOrigin = () => {
    const next = originDraft.trim().replace(/\/+$/, '');
    if (!next) return;
    if (!form.allowed_origins.includes(next)) {
      setForm({ ...form, allowed_origins: [...form.allowed_origins, next] });
    }
    setOriginDraft('');
  };

  const removeOrigin = (origin: string) => {
    setForm({
      ...form,
      allowed_origins: form.allowed_origins.filter((o) => o !== origin),
    });
  };

  const toggleMethod = (method: string) => {
    const exists = form.allowed_methods.includes(method);
    const next = exists
      ? form.allowed_methods.filter((m) => m !== method)
      : [...form.allowed_methods, method];
    setForm({ ...form, allowed_methods: next });
  };

  const wildcardWithCredentials =
    form.allow_credentials && form.allowed_origins.includes('*');

  return (
    <div className="space-y-6">
      {error && (
        <Alert variant="error" title="Invalid CORS configuration">
          {error}
        </Alert>
      )}

      {wildcardWithCredentials && (
        <Alert variant="warning" title="Unsafe CORS combination detected">
          Browsers and this server reject Allow Credentials combined with wildcard origin (*). Remove * or disable Allow Credentials before saving.
        </Alert>
      )}

      <Card>
        <CardHeader
          title="Cross-Origin Resource Sharing (CORS)"
          description="Changes take effect immediately across all API endpoints without restarting the server."
        />
        <CardContent className="space-y-5">
          <div className="flex items-center justify-between p-4 border border-neutral-200 rounded-md">
            <div>
              <p className="text-sm font-medium text-neutral-900">Enable CORS headers</p>
              <p className="text-xs text-neutral-500">
                Respond to cross-origin browser requests and preflight OPTIONS checks.
              </p>
            </div>
            <Switch
              checked={form.enabled}
              disabled={!canEdit}
              onChange={(checked) => setForm({ ...form, enabled: checked })}
            />
          </div>

          <div className="space-y-2">
            <label className="block text-sm font-medium text-neutral-800">
              Allowed Origins
            </label>
            <div className="flex gap-2">
              <Input
                value={originDraft}
                disabled={!canEdit}
                onChange={(e) => setOriginDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    addOrigin();
                  }
                }}
                placeholder="https://app.example.com"
                helperText="Enter exact scheme://host[:port] origins, or * for public non-credentialed APIs."
              />
              <Button
                type="button"
                variant="outline"
                disabled={!canEdit || !originDraft.trim()}
                onClick={addOrigin}
                className="shrink-0 h-9"
              >
                <Plus className="w-4 h-4" />
                Add
              </Button>
            </div>

            <div className="flex flex-wrap gap-2 pt-1">
              {form.allowed_origins.map((origin) => (
                <span
                  key={origin}
                  className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-mono bg-neutral-100 text-neutral-800 border border-neutral-200"
                >
                  {origin}
                  {canEdit && (
                    <button
                      type="button"
                      onClick={() => removeOrigin(origin)}
                      className="text-neutral-500 hover:text-rose-600"
                      aria-label={`Remove ${origin}`}
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                  )}
                </span>
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <label className="block text-sm font-medium text-neutral-800">
              Allowed HTTP Methods
            </label>
            <div className="flex flex-wrap gap-2">
              {HTTP_METHODS.map((method) => {
                const active = form.allowed_methods.includes(method);
                return (
                  <button
                    key={method}
                    type="button"
                    disabled={!canEdit}
                    onClick={() => toggleMethod(method)}
                    className={`px-3 py-1 rounded-md text-xs font-medium border transition-colors ${
                      active
                        ? 'bg-primary-50 border-primary-300 text-primary-700'
                        : 'bg-white border-neutral-200 text-neutral-600 hover:bg-neutral-50'
                    }`}
                  >
                    {method}
                  </button>
                );
              })}
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Allowed Headers (comma-separated)"
              disabled={!canEdit}
              value={form.allowed_headers.join(', ')}
              onChange={(e) =>
                setForm({
                  ...form,
                  allowed_headers: e.target.value
                    .split(',')
                    .map((s) => s.trim())
                    .filter(Boolean),
                })
              }
            />
            <Input
              label="Exposed Headers (comma-separated)"
              disabled={!canEdit}
              value={form.exposed_headers.join(', ')}
              onChange={(e) =>
                setForm({
                  ...form,
                  exposed_headers: e.target.value
                    .split(',')
                    .map((s) => s.trim())
                    .filter(Boolean),
                })
              }
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 items-center">
            <Input
              label="Preflight Max Age (seconds)"
              type="number"
              min={0}
              max={86400}
              disabled={!canEdit}
              value={form.max_age_seconds}
              onChange={(e) =>
                setForm({ ...form, max_age_seconds: Number(e.target.value) || 0 })
              }
            />
            <div className="p-3 border border-neutral-200 rounded-md flex items-center justify-between gap-3">
              <div>
                <p className="text-sm font-medium text-neutral-900">Allow Credentials</p>
                <p className="text-xs text-neutral-500">
                  Permit cookies & auth headers from explicit origins.
                </p>
              </div>
              <Switch
                checked={form.allow_credentials}
                disabled={!canEdit}
                onChange={(checked) => setForm({ ...form, allow_credentials: checked })}
              />
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

