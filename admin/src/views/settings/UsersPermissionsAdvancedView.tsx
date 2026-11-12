import React, { useCallback, useEffect, useState } from 'react';
import { Sliders } from 'lucide-react';
import { usersPermApi } from '../../api/endpoints';
import { useSettingsForm } from '../../hooks/useSettingsForm';
import { Alert } from '../../components/ui/Alert';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { Select } from '../../components/ui/Select';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import type { AppAuthSettings, AppRole } from '../../types';

export interface UsersPermissionsAdvancedViewProps {
  canEdit: boolean;
  onDirtyChange: (dirty: boolean) => void;
  registerHandlers: (handlers: {
    save: () => Promise<void>;
    discard: () => void;
    isSaving: boolean;
  }) => void;
}

export const UsersPermissionsAdvancedView: React.FC<UsersPermissionsAdvancedViewProps> = ({
  canEdit,
  onDirtyChange,
  registerHandlers,
}) => {
  const [roles, setRoles] = useState<AppRole[]>([]);

  useEffect(() => {
    usersPermApi
      .listRoles()
      .then((res) => setRoles(res.data || []))
      .catch(() => {
        // Non-fatal fallback
      });
  }, []);

  const load = useCallback(async () => {
    const res = await usersPermApi.getSettings();
    return res.data;
  }, []);

  const save = useCallback(async (val: AppAuthSettings) => {
    const res = await usersPermApi.updateSettings(val);
    return res.data;
  }, []);

  const validate = useCallback((v: AppAuthSettings): string | null => {
    if (v.session_expiration_minutes < 15 || v.session_expiration_minutes > 43200) {
      return 'End-user JWT lifetime must be between 15 and 43,200 minutes.';
    }
    return null;
  }, []);

  const { form, setForm, isLoading, error } = useSettingsForm<AppAuthSettings>({
    load,
    save,
    validate,
    onDirtyChange,
    registerHandlers,
    successTitle: 'Settings Saved',
    successMessage: 'Users & Permissions advanced settings saved.',
    loadErrorFallback: 'Failed to load Users & Permissions settings',
    saveErrorFallback: 'Failed to save Users & Permissions settings',
  });

  if (isLoading || !form) {
    return <SettingsCardSkeleton rows={5} />;
  }

  return (
    <div className="space-y-6">
      {error && <Alert variant="error">{error}</Alert>}

      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <Sliders className="w-4 h-4 text-sky-600" />
              <span>Application Authentication Policy</span>
            </span>
          }
          description="Control public end-user registration, transactional email confirmation, default assigned role, and reset/confirmation links."
        />
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between p-4 rounded-lg bg-slate-50 border border-slate-200">
            <div>
              <p className="text-xs font-semibold text-slate-800">Enable Public End-User Sign-Ups</p>
              <p className="text-[11px] text-slate-500">
                Allows external clients to register accounts via POST /api/v1/app-auth/register.
              </p>
            </div>
            <Switch
              label="Allow Registration"
              checked={form.allow_register}
              disabled={!canEdit}
              onChange={(checked) => setForm({ ...form, allow_register: checked })}
            />
          </div>

          <div className="flex items-center justify-between p-4 rounded-lg bg-slate-50 border border-slate-200">
            <div>
              <p className="text-xs font-semibold text-slate-800">Require Email Confirmation</p>
              <p className="text-[11px] text-slate-500">
                Dispatches a confirmation link via the active Email Configuration and blocks sign-in until confirmed.
              </p>
            </div>
            <Switch
              label="Email Confirmation"
              checked={form.email_confirmation}
              disabled={!canEdit}
              onChange={(checked) => setForm({ ...form, email_confirmation: checked })}
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Select
              label="Default Role for New Users"
              disabled={!canEdit}
              value={form.default_role_id}
              onChange={(e) => setForm({ ...form, default_role_id: e.target.value })}
              options={roles
                .filter((r) => r.slug !== 'public')
                .map((r) => ({ label: r.name, value: r.id }))}
            />
            <Input
              label="App JWT Session Lifetime (minutes)"
              type="number"
              min={15}
              max={43200}
              disabled={!canEdit}
              value={form.session_expiration_minutes}
              onChange={(e) =>
                setForm({ ...form, session_expiration_minutes: Number(e.target.value) || 1440 })
              }
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Confirmation Redirection Base URL (optional)"
              placeholder="https://app.example.com/confirm-email"
              disabled={!canEdit}
              value={form.email_confirmation_redirection_url}
              onChange={(e) =>
                setForm({ ...form, email_confirmation_redirection_url: e.target.value })
              }
              helperText="Appends ?token=... when sending confirmation emails."
            />
            <Input
              label="Password Reset Page Base URL (optional)"
              placeholder="https://app.example.com/reset-password"
              disabled={!canEdit}
              value={form.reset_password_url}
              onChange={(e) => setForm({ ...form, reset_password_url: e.target.value })}
              helperText="Appends ?token=... when sending forgot-password emails."
            />
          </div>
        </CardContent>
      </Card>
    </div>
  );
};
