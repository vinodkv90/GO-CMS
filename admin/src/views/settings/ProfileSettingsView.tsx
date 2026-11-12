import React, { useEffect, useState } from 'react';
import { Check, Copy, KeyRound, RefreshCw, ShieldCheck, Sliders, Smartphone, User as UserIcon } from 'lucide-react';
import { authApi, preferencesApi } from '../../api/endpoints';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { Select } from '../../components/ui/Select';
import { Switch } from '../../components/ui/Switch';
import { Button } from '../../components/ui/Button';
import { Badge } from '../../components/ui/Badge';
import { Alert } from '../../components/ui/Alert';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { useToast } from '../../components/ui/Toast';
import type { MFAEnrollResult, User, UserPreferences } from '../../types';

const PREF_TIMEZONES = [
  'UTC',
  'America/New_York',
  'America/Los_Angeles',
  'Europe/London',
  'Europe/Berlin',
  'Asia/Kolkata',
  'Asia/Singapore',
  'Asia/Tokyo',
  'Australia/Sydney',
];

export interface ProfileSettingsViewProps {
  user: User;
  onUserUpdated: (user: User) => void;
  onPreferencesUpdated?: (prefs: UserPreferences) => void;
}

export const ProfileSettingsView: React.FC<ProfileSettingsViewProps> = ({
  user,
  onUserUpdated,
  onPreferencesUpdated,
}) => {
  const toast = useToast();

  // Profile state
  const [firstName, setFirstName] = useState(user.first_name || '');
  const [lastName, setLastName] = useState(user.last_name || '');
  const [email, setEmail] = useState(user.email || '');
  const [profileSaving, setProfileSaving] = useState(false);
  const [profileError, setProfileError] = useState<string | null>(null);

  // Password state
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordSaving, setPasswordSaving] = useState(false);
  const [passwordError, setPasswordError] = useState<string | null>(null);

  // Preferences state
  const [prefs, setPrefs] = useState<UserPreferences | null>(null);
  const [prefsLoading, setPrefsLoading] = useState(true);
  const [prefsSaving, setPrefsSaving] = useState(false);
  const [prefsError, setPrefsError] = useState<string | null>(null);

  // TOTP MFA state
  const [mfaEnroll, setMfaEnroll] = useState<MFAEnrollResult | null>(null);
  const [mfaCode, setMfaCode] = useState('');
  const [mfaRecoveryCodes, setMfaRecoveryCodes] = useState<string[] | null>(null);
  const [mfaBusy, setMfaBusy] = useState(false);
  const [mfaError, setMfaError] = useState<string | null>(null);
  const [disableModalOpen, setDisableModalOpen] = useState(false);
  const [disablePassword, setDisablePassword] = useState('');
  const [disableCode, setDisableCode] = useState('');
  const [regenCode, setRegenCode] = useState('');
  const [copiedSecret, setCopiedSecret] = useState(false);

  useEffect(() => {
    setFirstName(user.first_name || '');
    setLastName(user.last_name || '');
    setEmail(user.email || '');
  }, [user]);

  useEffect(() => {
    let cancelled = false;
    const loadPrefs = async () => {
      setPrefsLoading(true);
      try {
        const res = await preferencesApi.get();
        if (!cancelled) setPrefs(res.data);
      } catch (err: any) {
        if (!cancelled) setPrefsError(err?.message || 'Failed to load preferences');
      } finally {
        if (!cancelled) setPrefsLoading(false);
      }
    };
    void loadPrefs();
    return () => {
      cancelled = true;
    };
  }, []);

  const handleSaveProfile = async (e: React.FormEvent) => {
    e.preventDefault();
    setProfileSaving(true);
    setProfileError(null);
    try {
      const updated = await authApi.updateMe({
        email,
        first_name: firstName,
        last_name: lastName,
      });
      onUserUpdated(updated);
      toast.success('Profile updated');
    } catch (err: any) {
      const msg = err?.message || 'Failed to update profile';
      setProfileError(msg);
      toast.error('Profile update failed', msg);
    } finally {
      setProfileSaving(false);
    }
  };

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordError(null);
    if (newPassword !== confirmPassword) {
      setPasswordError('New password and confirmation do not match.');
      return;
    }
    setPasswordSaving(true);
    try {
      await authApi.changePassword({
        current_password: currentPassword,
        new_password: newPassword,
      });
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      toast.success('Password changed successfully');
    } catch (err: any) {
      const msg = err?.message || 'Failed to update password';
      setPasswordError(msg);
      toast.error('Password update failed', msg);
    } finally {
      setPasswordSaving(false);
    }
  };

  const handleSavePreferences = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!prefs) return;
    setPrefsSaving(true);
    setPrefsError(null);
    try {
      const res = await preferencesApi.update(prefs);
      setPrefs(res.data);
      if (onPreferencesUpdated) onPreferencesUpdated(res.data);
      toast.success('Preferences saved');
    } catch (err: any) {
      const msg = err?.message || 'Failed to save preferences';
      setPrefsError(msg);
      toast.error('Preferences save failed', msg);
    } finally {
      setPrefsSaving(false);
    }
  };

  const handleStartMfaEnroll = async () => {
    setMfaBusy(true);
    setMfaError(null);
    try {
      const res = await authApi.enrollMfa();
      setMfaEnroll(res.data);
      setMfaCode('');
      setMfaRecoveryCodes(null);
    } catch (err: any) {
      setMfaError(err?.message || 'Failed to start MFA enrollment');
    } finally {
      setMfaBusy(false);
    }
  };

  const handleConfirmMfa = async (e: React.FormEvent) => {
    e.preventDefault();
    setMfaBusy(true);
    setMfaError(null);
    try {
      const res = await authApi.confirmMfa(mfaCode.trim());
      setMfaEnroll(null);
      setMfaCode('');
      setMfaRecoveryCodes(res.data.recovery_codes);
      onUserUpdated(res.data.user);
      toast.success('Two-factor authentication enabled');
    } catch (err: any) {
      setMfaError(err?.message || 'Invalid verification code');
    } finally {
      setMfaBusy(false);
    }
  };

  const handleRegenerateCodes = async (e: React.FormEvent) => {
    e.preventDefault();
    setMfaBusy(true);
    setMfaError(null);
    try {
      const res = await authApi.regenerateRecoveryCodes(regenCode.trim());
      setMfaRecoveryCodes(res.data.recovery_codes);
      setRegenCode('');
      toast.success('Backup recovery codes regenerated');
    } catch (err: any) {
      setMfaError(err?.message || 'Failed to regenerate recovery codes');
    } finally {
      setMfaBusy(false);
    }
  };

  const handleDisableMfa = async (e: React.FormEvent) => {
    e.preventDefault();
    setMfaBusy(true);
    setMfaError(null);
    try {
      const res = await authApi.disableMfa(disablePassword, disableCode.trim());
      setDisableModalOpen(false);
      setDisablePassword('');
      setDisableCode('');
      setMfaRecoveryCodes(null);
      onUserUpdated(res.data);
      toast.success('Two-factor authentication disabled');
    } catch (err: any) {
      setMfaError(err?.message || 'Could not disable MFA');
    } finally {
      setMfaBusy(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Identity Card */}
      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <UserIcon className="w-4 h-4 text-sky-600" />
              <span>Profile Details</span>
            </span>
          }
          description="Manage your administrator display name and email address."
          action={
            <div className="flex items-center gap-1.5">
              {(user.roles || []).map((r) => (
                <Badge key={r.id || r.slug} variant="info">
                  <ShieldCheck className="w-3 h-3 mr-1" />
                  {r.name || r.slug}
                </Badge>
              ))}
            </div>
          }
        />
        <CardContent>
          <form onSubmit={(e) => void handleSaveProfile(e)} className="space-y-4">
            {profileError && <Alert variant="error">{profileError}</Alert>}

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Input label="Username" value={user.username} disabled helperText="Usernames are immutable." />
              <Input
                label="Email Address"
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
              <Input
                label="First Name"
                value={firstName}
                onChange={(e) => setFirstName(e.target.value)}
                placeholder="Jane"
              />
              <Input
                label="Last Name"
                value={lastName}
                onChange={(e) => setLastName(e.target.value)}
                placeholder="Doe"
              />
            </div>

            <div className="flex justify-end pt-2 border-t border-slate-100">
              <Button type="submit" size="sm" isLoading={profileSaving}>
                Save profile
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {/* Change Password Card */}
      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <KeyRound className="w-4 h-4 text-amber-600" />
              <span>Password</span>
            </span>
          }
          description="Requires your current password and at least 8 characters with uppercase, lowercase, and numeric characters."
        />
        <CardContent>
          <form onSubmit={(e) => void handleChangePassword(e)} className="space-y-4">
            {passwordError && <Alert variant="error">{passwordError}</Alert>}

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <Input
                label="Current Password"
                type="password"
                required
                autoComplete="current-password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
              />
              <Input
                label="New Password"
                type="password"
                required
                autoComplete="new-password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
              />
              <Input
                label="Confirm New Password"
                type="password"
                required
                autoComplete="new-password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
              />
            </div>

            <div className="flex justify-end pt-2 border-t border-slate-100">
              <Button type="submit" size="sm" variant="secondary" isLoading={passwordSaving}>
                Update password
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {/* Two-Factor Authentication Card */}
      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <Smartphone className="w-4 h-4 text-emerald-600" />
              <span>Two-Factor Authentication (TOTP)</span>
            </span>
          }
          description="Protect your administrator account with an RFC 6238 authenticator app and single-use backup codes."
          action={
            <Badge variant={user.mfa_enabled ? 'success' : 'neutral'}>
              {user.mfa_enabled ? 'MFA Enabled' : 'Disabled'}
            </Badge>
          }
        />
        <CardContent className="space-y-4">
          {mfaError && <Alert variant="error">{mfaError}</Alert>}

          {mfaRecoveryCodes && (
            <Alert variant="warning" title="Save your backup recovery codes">
              <p className="text-xs mb-2">
                Each code works once if you lose your authenticator app. Previous recovery codes are no longer valid.
              </p>
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 font-mono text-xs text-slate-800 my-2">
                {mfaRecoveryCodes.map((rc) => (
                  <div key={rc} className="bg-white px-2 py-1 rounded border border-amber-200 text-center">
                    {rc}
                  </div>
                ))}
              </div>
              <Button
                type="button"
                size="sm"
                variant="secondary"
                onClick={() => void navigator.clipboard?.writeText(mfaRecoveryCodes.join('\n'))}
              >
                Copy All Recovery Codes
              </Button>
            </Alert>
          )}

          {!user.mfa_enabled && !mfaEnroll && (
            <div className="flex items-center justify-between p-4 rounded-lg bg-slate-50 border border-slate-200">
              <div>
                <p className="text-sm font-semibold text-slate-800">Authenticator App Not Configured</p>
                <p className="text-xs text-slate-500">
                  Enable TOTP verification using Google Authenticator, 1Password, or Authy.
                </p>
              </div>
              <Button type="button" size="sm" onClick={() => void handleStartMfaEnroll()} isLoading={mfaBusy}>
                Set Up 2FA
              </Button>
            </div>
          )}

          {!user.mfa_enabled && mfaEnroll && (
            <form onSubmit={(e) => void handleConfirmMfa(e)} className="space-y-4 p-4 rounded-lg bg-slate-50 border border-slate-200">
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-semibold text-slate-700">1. Add this setup key to your authenticator app</span>
                  <button
                    type="button"
                    onClick={() => {
                      void navigator.clipboard?.writeText(mfaEnroll.secret);
                      setCopiedSecret(true);
                      setTimeout(() => setCopiedSecret(false), 1800);
                    }}
                    className="inline-flex items-center gap-1 text-xs font-medium text-sky-600 hover:text-sky-700"
                  >
                    {copiedSecret ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
                    <span>{copiedSecret ? 'Copied' : 'Copy Secret'}</span>
                  </button>
                </div>
                <div className="font-mono text-xs bg-white px-3 py-2 rounded border border-slate-200 break-all text-slate-800">
                  {mfaEnroll.secret}
                </div>
                <p className="text-[11px] text-slate-500 break-all">
                  Provisioning URI: <span className="font-mono">{mfaEnroll.otpauth_uri}</span>
                </p>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-3 items-end">
                <Input
                  label="2. Enter 6-digit authenticator code"
                  value={mfaCode}
                  onChange={(e) => setMfaCode(e.target.value)}
                  placeholder="123456"
                  required
                />
                <div className="flex gap-2">
                  <Button type="submit" size="sm" isLoading={mfaBusy}>
                    Confirm & Enable 2FA
                  </Button>
                  <Button type="button" size="sm" variant="secondary" onClick={() => setMfaEnroll(null)}>
                    Cancel
                  </Button>
                </div>
              </div>
            </form>
          )}

          {user.mfa_enabled && (
            <div className="space-y-4">
              <form
                onSubmit={(e) => void handleRegenerateCodes(e)}
                className="flex flex-col sm:flex-row sm:items-end justify-between gap-3 p-4 rounded-lg bg-slate-50 border border-slate-200"
              >
                <div className="flex-1">
                  <p className="text-xs font-semibold text-slate-800 mb-1">Regenerate Backup Recovery Codes</p>
                  <Input
                    label="Current 6-digit TOTP code"
                    value={regenCode}
                    onChange={(e) => setRegenCode(e.target.value)}
                    placeholder="123456"
                    required
                  />
                </div>
                <Button type="submit" size="sm" variant="secondary" isLoading={mfaBusy}>
                  <RefreshCw className="w-3.5 h-3.5 mr-1.5" />
                  Regenerate Codes
                </Button>
              </form>

              {!disableModalOpen ? (
                <div className="flex justify-end">
                  <Button type="button" size="sm" variant="destructive" onClick={() => setDisableModalOpen(true)}>
                    Disable Two-Factor Authentication
                  </Button>
                </div>
              ) : (
                <form onSubmit={(e) => void handleDisableMfa(e)} className="p-4 rounded-lg bg-rose-50 border border-rose-200 space-y-3">
                  <p className="text-xs font-semibold text-rose-800">
                    Confirm your password and an active TOTP or backup code to disable MFA.
                  </p>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    {!user.sso_provider && (
                      <Input
                        label="Current Password"
                        type="password"
                        required
                        value={disablePassword}
                        onChange={(e) => setDisablePassword(e.target.value)}
                      />
                    )}
                    <Input
                      label="Authenticator / Backup Code"
                      required
                      value={disableCode}
                      onChange={(e) => setDisableCode(e.target.value)}
                      placeholder="123456 or ABCD-EFGH"
                    />
                  </div>
                  <div className="flex justify-end gap-2">
                    <Button type="button" size="sm" variant="secondary" onClick={() => setDisableModalOpen(false)}>
                      Cancel
                    </Button>
                    <Button type="submit" size="sm" variant="destructive" isLoading={mfaBusy}>
                      Confirm Disable 2FA
                    </Button>
                  </div>
                </form>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Preferences Card */}
      {prefsLoading || !prefs ? (
        <SettingsCardSkeleton rows={4} />
      ) : (
        <Card>
          <CardHeader
            title={
              <span className="flex items-center gap-2">
                <Sliders className="w-4 h-4 text-indigo-600" />
                <span>Personal Preferences</span>
              </span>
            }
            description="Persisted per administrator across sessions without affecting other operators."
          />
          <CardContent>
            <form onSubmit={(e) => void handleSavePreferences(e)} className="space-y-4">
              {prefsError && <Alert variant="error">{prefsError}</Alert>}

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Select
                  label="Interface Theme Preference"
                  value={prefs.theme}
                  onChange={(e) =>
                    setPrefs({
                      ...prefs,
                      theme: e.target.value as 'system' | 'light' | 'dark',
                    })
                  }
                  options={[
                    { label: 'System default', value: 'system' },
                    { label: 'Light', value: 'light' },
                    { label: 'Dark (stored preference)', value: 'dark' },
                  ]}
                  helperText="Stored per user; Admin Console currently renders light surface polarity."
                />
                <Input
                  label="Preferred Language"
                  value={prefs.preferred_language}
                  onChange={(e) => setPrefs({ ...prefs, preferred_language: e.target.value })}
                />
                <Select
                  label="Personal Timezone"
                  value={prefs.timezone}
                  onChange={(e) => setPrefs({ ...prefs, timezone: e.target.value })}
                  options={(PREF_TIMEZONES.includes(prefs.timezone)
                    ? PREF_TIMEZONES
                    : [prefs.timezone, ...PREF_TIMEZONES]
                  ).map((tz) => ({ label: tz, value: tz }))}
                />
                <div className="grid grid-cols-2 gap-3">
                  <Select
                    label="Date Format"
                    value={prefs.date_format}
                    onChange={(e) => setPrefs({ ...prefs, date_format: e.target.value })}
                    options={[
                      { label: 'YYYY-MM-DD', value: 'YYYY-MM-DD' },
                      { label: 'MM/DD/YYYY', value: 'MM/DD/YYYY' },
                      { label: 'DD/MM/YYYY', value: 'DD/MM/YYYY' },
                      { label: 'MMM D, YYYY', value: 'MMM D, YYYY' },
                    ]}
                  />
                  <Select
                    label="Time Format"
                    value={prefs.time_format}
                    onChange={(e) => setPrefs({ ...prefs, time_format: e.target.value })}
                    options={[
                      { label: '24-hour', value: '24h' },
                      { label: '12-hour', value: '12h' },
                    ]}
                  />
                </div>
              </div>

              <div className="flex items-center justify-between p-3.5 rounded-lg bg-slate-50 border border-slate-200/70">
                <div>
                  <p className="text-xs font-semibold text-slate-800">Compact Sidebar Rail</p>
                  <p className="text-[11px] text-slate-500">
                    Remember compact icon navigation preference on reload.
                  </p>
                </div>
                <Switch
                  checked={prefs.sidebar_collapsed}
                  onChange={(checked) => setPrefs({ ...prefs, sidebar_collapsed: checked })}
                  label="Compact Sidebar Rail"
                />
              </div>

              <div className="flex justify-end pt-2 border-t border-slate-100">
                <Button type="submit" size="sm" isLoading={prefsSaving}>
                  Save preferences
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}
    </div>
  );
};
