import React, { useCallback } from 'react';
import { settingsApi } from '../../api/endpoints';
import { useSettingsForm } from '../../hooks/useSettingsForm';
import { Alert } from '../../components/ui/Alert';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import type { RateLimitSettings } from '../../types';

export interface RateLimitingViewProps {
  canEdit: boolean;
  onDirtyChange: (dirty: boolean) => void;
  registerHandlers: (handlers: {
    save: () => Promise<void>;
    discard: () => void;
    isSaving: boolean;
  }) => void;
}

export const RateLimitingView: React.FC<RateLimitingViewProps> = ({
  canEdit,
  onDirtyChange,
  registerHandlers,
}) => {
  const load = useCallback(async () => {
    const res = await settingsApi.getRateLimit();
    return res.data;
  }, []);

  const save = useCallback(async (value: RateLimitSettings) => {
    const res = await settingsApi.updateRateLimit(value);
    return res.data;
  }, []);

  const validate = useCallback((v: RateLimitSettings): string | null => {
    if (v.global_requests_per_sec < 1 || v.global_requests_per_sec > 10000) {
      return 'Global rate must be between 1 and 10000 requests/second.';
    }
    if (v.global_burst < 1 || v.global_burst > 50000) {
      return 'Global burst must be between 1 and 50000.';
    }
    if (v.auth_requests_per_min < 1 || v.auth_requests_per_min > 1000) {
      return 'Authentication rate must be between 1 and 1000 requests/minute.';
    }
    if (v.auth_burst < 1 || v.auth_burst > 1000) {
      return 'Authentication burst must be between 1 and 1000.';
    }
    return null;
  }, []);

  const { form, setForm, isLoading, error } = useSettingsForm<RateLimitSettings>({
    load,
    save,
    validate,
    successTitle: 'Rate limits applied live',
    successMessage: 'Token buckets updated immediately for global API and auth endpoints.',
    loadErrorFallback: 'Failed to load rate limiting settings.',
    saveErrorFallback: 'Failed to save rate limiting settings.',
    onDirtyChange,
    registerHandlers,
  });

  if (isLoading || !form) {
    return <SettingsCardSkeleton />;
  }

  return (
    <div className="space-y-6">
      {error && (
        <Alert variant="error" title="Invalid rate limit settings">
          {error}
        </Alert>
      )}

      <Card>
        <CardHeader
          title="API & Authentication Rate Limiting"
          description="Live token-bucket protection applied per client IP across global routes and sensitive authentication endpoints."
        />
        <CardContent className="space-y-5">
          <div className="flex items-center justify-between p-4 border border-neutral-200 rounded-md">
            <div>
              <p className="text-sm font-medium text-neutral-900">Enable rate limiting</p>
              <p className="text-xs text-neutral-500">
                Return HTTP 429 Too Many Requests when an IP exceeds configured buckets.
              </p>
            </div>
            <Switch
              checked={form.enabled}
              disabled={!canEdit}
              onChange={(checked) => setForm({ ...form, enabled: checked })}
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Global Requests / Second"
              type="number"
              min={1}
              max={10000}
              disabled={!canEdit || !form.enabled}
              value={form.global_requests_per_sec}
              onChange={(e) =>
                setForm({
                  ...form,
                  global_requests_per_sec: Number(e.target.value) || 0,
                })
              }
              helperText="Sustained token refill rate per client IP across all HTTP endpoints."
            />
            <Input
              label="Global Burst Capacity"
              type="number"
              min={1}
              max={50000}
              disabled={!canEdit || !form.enabled}
              value={form.global_burst}
              onChange={(e) =>
                setForm({ ...form, global_burst: Number(e.target.value) || 0 })
              }
              helperText="Maximum short-term burst allowed before HTTP 429 throttling."
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-2 border-t border-neutral-100">
            <Input
              label="Auth Endpoints Requests / Minute"
              type="number"
              min={1}
              max={1000}
              disabled={!canEdit || !form.enabled}
              value={form.auth_requests_per_min}
              onChange={(e) =>
                setForm({
                  ...form,
                  auth_requests_per_min: Number(e.target.value) || 0,
                })
              }
              helperText="Dedicated limit for POST /auth/login, /auth/register, and invite acceptance."
            />
            <Input
              label="Auth Endpoints Burst Capacity"
              type="number"
              min={1}
              max={1000}
              disabled={!canEdit || !form.enabled}
              value={form.auth_burst}
              onChange={(e) =>
                setForm({ ...form, auth_burst: Number(e.target.value) || 0 })
              }
              helperText="Maximum consecutive login/register attempts allowed per IP."
            />
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

