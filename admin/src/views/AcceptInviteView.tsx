import React, { useEffect, useState } from 'react';
import { CheckCircle2, KeyRound, Shield } from 'lucide-react';
import { authApi } from '../api/endpoints';
import { setStoredToken } from '../api/client';
import type { InvitationPreview, PublicGeneralSettings, User } from '../types';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { Badge } from '../components/ui/Badge';
import { Alert } from '../components/ui/Alert';

export interface AcceptInviteViewProps {
  token: string;
  publicSettings: PublicGeneralSettings | null;
  onAccepted: (user: User) => void;
  onCancel: () => void;
}

export const AcceptInviteView: React.FC<AcceptInviteViewProps> = ({
  token,
  publicSettings,
  onAccepted,
  onCancel,
}) => {
  const [preview, setPreview] = useState<InvitationPreview | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const [username, setUsername] = useState('');
  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    let active = true;
    setIsLoading(true);
    setLoadError(null);
    authApi
      .previewInvitation(token)
      .then((res) => {
        if (!active) return;
        setPreview(res);
        setUsername(res.username || '');
        setFirstName(res.first_name || '');
        setLastName(res.last_name || '');
      })
      .catch((err) => {
        if (!active) return;
        setLoadError(
          err instanceof Error && err.message
            ? err.message
            : 'This invitation link is invalid, revoked, or has expired.',
        );
      })
      .finally(() => {
        if (active) setIsLoading(false);
      });
    return () => {
      active = false;
    };
  }, [token]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitError(null);

    if (password.length < 8) {
      setSubmitError('Password must be at least 8 characters long and include mixed character classes.');
      return;
    }
    if (password !== confirmPassword) {
      setSubmitError('Passwords do not match.');
      return;
    }

    setIsSubmitting(true);
    try {
      const res = await authApi.acceptInvitation({
        token,
        username: username.trim(),
        first_name: firstName.trim(),
        last_name: lastName.trim(),
        password,
      });
      if (res.token) {
        setStoredToken(res.token);
      }
      onAccepted(res.user);
    } catch (err) {
      setSubmitError(
        err instanceof Error && err.message ? err.message : 'Failed to activate account from invitation.',
      );
    } finally {
      setIsSubmitting(false);
    }
  };

  const brandName = publicSettings?.site_name || 'Go CMS';

  return (
    <div className="min-h-screen bg-slate-950 flex flex-col items-center justify-center p-4">
      <div className="w-full max-w-md rounded-2xl bg-white shadow-2xl border border-slate-200 overflow-hidden">
        <div className="px-6 py-5 bg-slate-900 text-white flex items-center justify-between">
          <div className="flex items-center gap-3">
            {publicSettings?.logo_url ? (
              <img
                src={publicSettings.logo_url}
                alt={brandName}
                className="w-8 h-8 rounded-lg object-cover bg-white/10"
              />
            ) : (
              <div className="w-8 h-8 rounded-lg bg-gradient-to-tr from-sky-500 to-indigo-600 flex items-center justify-center font-bold">
                {brandName.charAt(0).toUpperCase()}
              </div>
            )}
            <div>
              <h1 className="text-sm font-semibold">{brandName}</h1>
              <p className="text-[11px] text-slate-300">Administrator Invitation</p>
            </div>
          </div>
          <KeyRound className="w-5 h-5 text-sky-400" />
        </div>

        <div className="p-6 space-y-4">
          {isLoading ? (
            <div className="py-8 text-center text-xs text-slate-500">Validating invitation token...</div>
          ) : loadError || !preview ? (
            <div className="space-y-4">
              <Alert variant="error" title="Invitation unavailable">
                {loadError || 'Unable to verify invitation.'}
              </Alert>
              <Button type="button" variant="outline" className="w-full" onClick={onCancel}>
                Back to Sign In
              </Button>
            </div>
          ) : (
            <form onSubmit={(e) => void handleSubmit(e)} className="space-y-4">
              <div className="rounded-xl border border-sky-200 bg-sky-50/70 p-3.5 space-y-2">
                <div className="flex items-center gap-2 text-xs font-semibold text-sky-900">
                  <CheckCircle2 className="w-4 h-4 text-sky-600 shrink-0" />
                  <span>You were invited to join {brandName}</span>
                </div>
                <p className="text-xs text-sky-800 font-mono">{preview.email}</p>
                {preview.roles && preview.roles.length > 0 && (
                  <div className="flex flex-wrap items-center gap-1.5 pt-1">
                    <Shield className="w-3.5 h-3.5 text-sky-700" />
                    {preview.roles.map((r) => (
                      <Badge key={r.id} variant="info">
                        {r.name}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>

              {submitError && (
                <Alert variant="error" title="Activation failed">
                  {submitError}
                </Alert>
              )}

              <Input
                label="Username"
                required
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />

              <div className="grid grid-cols-2 gap-3">
                <Input
                  label="First name"
                  value={firstName}
                  onChange={(e) => setFirstName(e.target.value)}
                />
                <Input
                  label="Last name"
                  value={lastName}
                  onChange={(e) => setLastName(e.target.value)}
                />
              </div>

              <Input
                label="Password"
                type="password"
                required
                helperText="Minimum 8 characters with uppercase, lowercase, number, and symbol."
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />

              <Input
                label="Confirm password"
                type="password"
                required
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
              />

              <div className="flex items-center gap-2 pt-2">
                <Button type="button" variant="outline" onClick={onCancel}>
                  Cancel
                </Button>
                <Button type="submit" className="flex-1" isLoading={isSubmitting}>
                  Activate Account & Sign In
                </Button>
              </div>
            </form>
          )}
        </div>
      </div>
    </div>
  );
};
