import React, { useCallback, useState } from 'react';
import { KeyRound, Plus, ShieldAlert, X } from 'lucide-react';
import { settingsApi } from '../../api/endpoints';
import { useSettingsForm } from '../../hooks/useSettingsForm';
import { Alert } from '../../components/ui/Alert';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import type { PasswordPolicySettings, SecuritySettings } from '../../types';

export interface SecurityAuthSettingsViewProps {
  canEdit: boolean;
  onDirtyChange: (dirty: boolean) => void;
  registerHandlers: (handlers: {
    save: () => Promise<void>;
    discard: () => void;
    isSaving: boolean;
  }) => void;
}

interface CombinedSecurityState {
  security: SecuritySettings;
  passwordPolicy: PasswordPolicySettings;
}

export const SecurityAuthSettingsView: React.FC<SecurityAuthSettingsViewProps> = ({
  canEdit,
  onDirtyChange,
  registerHandlers,
}) => {
  const [cidrDraft, setCidrDraft] = useState('');

  const load = useCallback(async (): Promise<CombinedSecurityState> => {
    const [secRes, pwRes] = await Promise.all([
      settingsApi.getSecurity(),
      settingsApi.getPasswordPolicy(),
    ]);
    return {
      security: secRes.data,
      passwordPolicy: pwRes.data,
    };
  }, []);

  const save = useCallback(async (val: CombinedSecurityState): Promise<CombinedSecurityState> => {
    const [secRes, pwRes] = await Promise.all([
      settingsApi.updateSecurity(val.security),
      settingsApi.updatePasswordPolicy(val.passwordPolicy),
    ]);
    return {
      security: secRes.data,
      passwordPolicy: pwRes.data,
    };
  }, []);

  const validate = useCallback((v: CombinedSecurityState): string | null => {
    if (v.security.session_timeout_minutes < 15 || v.security.session_timeout_minutes > 43200) {
      return 'Session timeout must be between 15 and 43,200 minutes.';
    }
    if (v.security.max_failed_logins < 3 || v.security.max_failed_logins > 50) {
      return 'Max failed login attempts must be between 3 and 50.';
    }
    if (v.security.lockout_duration_minutes < 1 || v.security.lockout_duration_minutes > 1440) {
      return 'Lockout duration must be between 1 and 1440 minutes.';
    }
    if (v.passwordPolicy.min_length < 8 || v.passwordPolicy.min_length > 128) {
      return 'Minimum password length must be between 8 and 128 characters.';
    }
    if (v.passwordPolicy.max_age_days < 0 || v.passwordPolicy.max_age_days > 3650) {
      return 'Password rotation max age must be between 0 and 3650 days.';
    }
    return null;
  }, []);

  const { form, setForm, isLoading, error } = useSettingsForm<CombinedSecurityState>({
    load,
    save,
    validate,
    onDirtyChange,
    registerHandlers,
    successTitle: 'Security Policy Saved',
    successMessage: 'Security, MFA, lockout, and password policy settings saved.',
    loadErrorFallback: 'Failed to load security policy settings',
    saveErrorFallback: 'Failed to save security policy settings',
  });

  if (isLoading || !form) {
    return (
      <div className="space-y-6">
        <SettingsCardSkeleton rows={5} />
        <SettingsCardSkeleton rows={4} />
      </div>
    );
  }

  const addCidr = () => {
    const cleaned = cidrDraft.trim();
    if (!cleaned || !canEdit) return;
    setForm((prev) => {
      if (!prev) return prev;
      const existing = prev.security.allowed_admin_ip_cidrs || [];
      if (existing.includes(cleaned)) return prev;
      return {
        ...prev,
        security: {
          ...prev.security,
          allowed_admin_ip_cidrs: [...existing, cleaned],
        },
      };
    });
    setCidrDraft('');
  };

  const removeCidr = (cidr: string) => {
    if (!canEdit) return;
    setForm((prev) =>
      prev
        ? {
            ...prev,
            security: {
              ...prev.security,
              allowed_admin_ip_cidrs: (prev.security.allowed_admin_ip_cidrs || []).filter(
                (item) => item !== cidr,
              ),
            },
          }
        : prev,
    );
  };

  return (
    <div className="space-y-6">
      {error && <Alert variant="error">{error}</Alert>}

      {/* Panel Security & Lockout Policy */}
      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <ShieldAlert className="w-4 h-4 text-rose-600" />
              <span>Administration Security & Lockout Policy</span>
            </span>
          }
          description="Control administrator session lifetimes, brute-force account lockout thresholds, mandatory TOTP 2FA, and network IP allowlists."
        />
        <CardContent className="space-y-5">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <Input
              label="Session Timeout (minutes)"
              type="number"
              min={15}
              max={43200}
              disabled={!canEdit}
              value={form.security.session_timeout_minutes}
              onChange={(e) =>
                setForm((prev) =>
                  prev
                    ? {
                        ...prev,
                        security: {
                          ...prev.security,
                          session_timeout_minutes: Number(e.target.value) || 1440,
                        },
                      }
                    : prev,
                )
              }
              helperText="Default 1440m (24 hours)."
            />
            <Input
              label="Max Failed Logins"
              type="number"
              min={3}
              max={50}
              disabled={!canEdit}
              value={form.security.max_failed_logins}
              onChange={(e) =>
                setForm((prev) =>
                  prev
                    ? {
                        ...prev,
                        security: {
                          ...prev.security,
                          max_failed_logins: Number(e.target.value) || 5,
                        },
                      }
                    : prev,
                )
              }
              helperText="Locks account after N invalid tries."
            />
            <Input
              label="Lockout Duration (minutes)"
              type="number"
              min={1}
              max={1440}
              disabled={!canEdit}
              value={form.security.lockout_duration_minutes}
              onChange={(e) =>
                setForm((prev) =>
                  prev
                    ? {
                        ...prev,
                        security: {
                          ...prev.security,
                          lockout_duration_minutes: Number(e.target.value) || 15,
                        },
                      }
                    : prev,
                )
              }
              helperText="Cooldown before next sign-in."
            />
          </div>

          <div className="flex items-center justify-between p-4 rounded-lg bg-slate-50 border border-slate-200/80">
            <div>
              <p className="text-xs font-semibold text-slate-800">Enforce Two-Factor Authentication (MFA)</p>
              <p className="text-[11px] text-slate-500">
                Requires all administrators to complete TOTP 2FA enrollment on next sign-in and prevents disabling MFA.
              </p>
            </div>
            <Switch
              label="Enforce Admin MFA"
              checked={form.security.enforce_admin_mfa}
              disabled={!canEdit}
              onChange={(checked) =>
                setForm((prev) =>
                  prev ? { ...prev, security: { ...prev.security, enforce_admin_mfa: checked } } : prev,
                )
              }
            />
          </div>

          <div className="space-y-2.5 pt-2 border-t border-slate-100">
            <div>
              <p className="text-xs font-semibold text-slate-800">Allowed Admin IP CIDRs</p>
              <p className="text-[11px] text-slate-500">
                Leave empty to allow access from any IP address. Single IPs normalize to /32 or /128 on save.
              </p>
            </div>
            {canEdit && (
              <div className="flex gap-2">
                <Input
                  placeholder="e.g. 10.0.0.0/8 or 203.0.113.10"
                  value={cidrDraft}
                  onChange={(e) => setCidrDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault();
                      addCidr();
                    }
                  }}
                />
                <Button type="button" variant="secondary" size="sm" onClick={addCidr}>
                  <Plus className="w-3.5 h-3.5 mr-1" />
                  Add CIDR
                </Button>
              </div>
            )}
            <div className="flex flex-wrap gap-2">
              {(form.security.allowed_admin_ip_cidrs || []).length === 0 ? (
                <span className="text-xs text-slate-400 italic">No IP restrictions configured (all IPs allowed).</span>
              ) : (
                (form.security.allowed_admin_ip_cidrs || []).map((cidr) => (
                  <Badge key={cidr} variant="info">
                    <span className="font-mono">{cidr}</span>
                    {canEdit && (
                      <button
                        type="button"
                        onClick={() => removeCidr(cidr)}
                        className="ml-1.5 hover:text-rose-700"
                        aria-label={`Remove ${cidr}`}
                      >
                        <X className="w-3 h-3" />
                      </button>
                    )}
                  </Badge>
                ))
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Password Complexity & Rotation Policy */}
      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <KeyRound className="w-4 h-4 text-amber-600" />
              <span>Password Complexity & Rotation Policy</span>
            </span>
          }
          description="Live-enforced across password creation, password changes, invitations, and end-user signups."
        />
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Minimum Password Length"
              type="number"
              min={8}
              max={128}
              disabled={!canEdit}
              value={form.passwordPolicy.min_length}
              onChange={(e) =>
                setForm((prev) =>
                  prev
                    ? {
                        ...prev,
                        passwordPolicy: {
                          ...prev.passwordPolicy,
                          min_length: Number(e.target.value) || 8,
                        },
                      }
                    : prev,
                )
              }
            />
            <Input
              label="Password Max Age (days)"
              type="number"
              min={0}
              max={3650}
              disabled={!canEdit}
              value={form.passwordPolicy.max_age_days}
              onChange={(e) =>
                setForm((prev) =>
                  prev
                    ? {
                        ...prev,
                        passwordPolicy: {
                          ...prev.passwordPolicy,
                          max_age_days: Number(e.target.value) || 0,
                        },
                      }
                    : prev,
                )
              }
              helperText="Set to 0 to disable forced password rotation."
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3 pt-2">
            {[
              {
                key: 'require_uppercase',
                label: 'Require uppercase letter (A-Z)',
                val: form.passwordPolicy.require_uppercase,
              },
              {
                key: 'require_lowercase',
                label: 'Require lowercase letter (a-z)',
                val: form.passwordPolicy.require_lowercase,
              },
              {
                key: 'require_number',
                label: 'Require numeric digit (0-9)',
                val: form.passwordPolicy.require_number,
              },
              {
                key: 'require_special',
                label: 'Require special character (!@#$...)',
                val: form.passwordPolicy.require_special,
              },
            ].map((item) => (
              <div
                key={item.key}
                className="flex items-center justify-between p-3.5 rounded-lg bg-slate-50 border border-slate-200/70"
              >
                <span className="text-xs font-medium text-slate-800">{item.label}</span>
                <Switch
                  label={item.label}
                  checked={item.val}
                  disabled={!canEdit}
                  onChange={(checked) =>
                    setForm((prev) =>
                      prev
                        ? {
                            ...prev,
                            passwordPolicy: {
                              ...prev.passwordPolicy,
                              [item.key]: checked,
                            },
                          }
                        : prev,
                    )
                  }
                />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
};
