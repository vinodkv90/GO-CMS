import React, { useEffect, useState } from 'react';
import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  Check,
  Copy,
  KeyRound,
  ShieldCheck,
  Smartphone,
} from 'lucide-react';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { authApi } from '../api/endpoints';
import { setStoredToken } from '../api/client';
import type {
  LoginResponse,
  MFAConfirmResult,
  MFAEnrollResult,
  PublicGeneralSettings,
  PublicProviderInfo,
  User,
} from '../types';

export interface LoginViewProps {
  onLoginSuccess: (user: User) => void;
  publicSettings?: PublicGeneralSettings | null;
}

type LoginStep = 'credentials' | 'mfa_verify' | 'mfa_setup' | 'mfa_recovery' | 'password_expired';

export const LoginView: React.FC<LoginViewProps> = ({ onLoginSuccess, publicSettings }) => {
  const siteName = publicSettings?.site_name || 'Go CMS Platform';
  const siteDescription =
    publicSettings?.site_description || 'Enterprise Extensible Content Management System';
  const monogram = siteName.trim().charAt(0).toUpperCase() || 'G';

  const [step, setStep] = useState<LoginStep>('credentials');
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('Admin@CMS2026!');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // MFA challenge / forced setup state
  const [mfaToken, setMfaToken] = useState<string>('');
  const [totpCode, setTotpCode] = useState<string>('');
  const [enrollData, setEnrollData] = useState<MFAEnrollResult | null>(null);
  const [confirmResult, setConfirmResult] = useState<MFAConfirmResult | null>(null);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  // Expired password rotation state
  const [newPassword, setNewPassword] = useState('');
  const [confirmNewPassword, setConfirmNewPassword] = useState('');

  // Optional Admin SSO providers
  const [ssoEnabled, setSsoEnabled] = useState(false);
  const [ssoProviders, setSsoProviders] = useState<PublicProviderInfo[]>([]);

  useEffect(() => {
    let cancelled = false;
    const loadSso = async () => {
      try {
        const res = await authApi.listSsoProviders();
        if (!cancelled && res?.data?.enabled) {
          setSsoEnabled(true);
          setSsoProviders(res.data.providers || []);
        }
      } catch {
        // Ignore when SSO endpoint is disabled or unavailable during initial boot
      }
    };
    void loadSso();
    return () => {
      cancelled = true;
    };
  }, []);

  const copyText = async (label: string, text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedKey(label);
      setTimeout(() => setCopiedKey(null), 1800);
    } catch {
      // Clipboard API may be unavailable in non-HTTPS contexts
    }
  };

  const handleLoginOutcome = async (response: LoginResponse) => {
    if (response.mfa_required && response.mfa_token) {
      setMfaToken(response.mfa_token);
      setTotpCode('');
      setStep('mfa_verify');
      return;
    }
    if (response.mfa_setup_required && response.mfa_token) {
      setMfaToken(response.mfa_token);
      setTotpCode('');
      const enrollRes = await authApi.enrollMfa(response.mfa_token);
      setEnrollData(enrollRes.data);
      setStep('mfa_setup');
      return;
    }
    if (response.token) {
      setStoredToken(response.token);
      onLoginSuccess(response.user);
      return;
    }
    setError('Unexpected authentication response from server.');
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsLoading(true);
    try {
      const response = await authApi.login({ username, password });
      await handleLoginOutcome(response);
    } catch (err: any) {
      if (err?.data?.password_expired) {
        setNewPassword('');
        setConfirmNewPassword('');
        setError(null);
        setStep('password_expired');
      } else {
        setError(err.message || 'Authentication failed. Please check your credentials.');
      }
    } finally {
      setIsLoading(false);
    }
  };

  const handleVerifyMfa = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsLoading(true);
    try {
      const response = await authApi.verifyMfa(mfaToken, totpCode.trim());
      await handleLoginOutcome(response);
    } catch (err: any) {
      setError(err.message || 'Invalid verification code.');
    } finally {
      setIsLoading(false);
    }
  };

  const handleConfirmForcedMfaSetup = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsLoading(true);
    try {
      const res = await authApi.confirmMfa(totpCode.trim(), mfaToken);
      setConfirmResult(res.data);
      setStep('mfa_recovery');
    } catch (err: any) {
      setError(err.message || 'Could not verify TOTP code.');
    } finally {
      setIsLoading(false);
    }
  };

  const handleFinishForcedMfaSetup = () => {
    if (!confirmResult) return;
    if (confirmResult.token) {
      setStoredToken(confirmResult.token);
    }
    onLoginSuccess(confirmResult.user);
  };

  const handleResetExpiredPassword = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    if (newPassword !== confirmNewPassword) {
      setError('New password and confirmation do not match.');
      return;
    }
    setIsLoading(true);
    try {
      const response = await authApi.resetExpiredPassword({
        identifier: username,
        current_password: password,
        new_password: newPassword,
      });
      await handleLoginOutcome(response);
    } catch (err: any) {
      setError(err.message || 'Failed to rotate expired password.');
    } finally {
      setIsLoading(false);
    }
  };

  const resetToCredentials = () => {
    setError(null);
    setMfaToken('');
    setTotpCode('');
    setEnrollData(null);
    setConfirmResult(null);
    setStep('credentials');
  };

  return (
    <div className="min-h-screen w-screen flex items-center justify-center bg-slate-900 px-4 py-12 relative overflow-hidden">
      <div className="absolute -top-40 -left-40 w-96 h-96 bg-sky-500/10 rounded-full blur-3xl pointer-events-none" />
      <div className="absolute -bottom-40 -right-40 w-96 h-96 bg-indigo-500/10 rounded-full blur-3xl pointer-events-none" />

      <div className="w-full max-w-md relative z-10">
        <div className="text-center mb-8">
          {publicSettings?.logo_url ? (
            <img
              src={publicSettings.logo_url}
              alt={siteName}
              className="w-14 h-14 mx-auto rounded-2xl bg-white object-contain p-1.5 shadow-xl mb-4"
            />
          ) : (
            <div className="inline-flex items-center justify-center w-14 h-14 rounded-2xl bg-gradient-to-tr from-sky-500 to-indigo-600 text-white shadow-xl shadow-sky-500/25 mb-4 font-black text-2xl">
              {monogram}
            </div>
          )}
          <h1 className="text-2xl font-extrabold text-white tracking-tight">{siteName}</h1>
          <p className="text-xs text-slate-400 mt-1">{siteDescription}</p>
        </div>

        <div className="bg-white rounded-2xl shadow-2xl border border-slate-800/10 p-8">
          {step === 'credentials' && (
            <>
              <div className="mb-6">
                <h2 className="text-lg font-bold text-slate-900">Sign in to console</h2>
                <p className="text-xs text-slate-500 mt-0.5">Enter your administrative credentials to continue</p>
              </div>

              {error && (
                <div className="mb-5 p-3.5 rounded-lg bg-rose-50 border border-rose-200 flex items-start gap-2.5 text-xs text-rose-700">
                  <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />
                  <span>{error}</span>
                </div>
              )}

              <form onSubmit={handleSubmit} className="space-y-4">
                <Input
                  id="login-username"
                  name="username"
                  label="Username or Email"
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="admin"
                  required
                  autoComplete="username"
                />

                <Input
                  id="login-password"
                  name="password"
                  label="Password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••••••"
                  required
                  autoComplete="current-password"
                />

                <Button type="submit" variant="primary" className="w-full mt-2" isLoading={isLoading}>
                  <span>Sign In</span>
                  <ArrowRight className="w-4 h-4" />
                </Button>
              </form>

              {ssoEnabled && ssoProviders.length > 0 && (
                <div className="mt-5 pt-5 border-t border-slate-200 space-y-2">
                  <p className="text-[11px] font-semibold uppercase tracking-wider text-slate-400 text-center">
                    Single Sign-On
                  </p>
                  {ssoProviders.map((prov) => (
                    <a
                      key={prov.name}
                      href={`/api/v1/auth/sso/${encodeURIComponent(prov.name)}/authorize?redirect_after=/`}
                      className="w-full inline-flex items-center justify-center gap-2 px-4 py-2 rounded-lg border border-slate-200 bg-white hover:bg-slate-50 text-xs font-semibold text-slate-700 transition-colors"
                    >
                      <ShieldCheck className="w-4 h-4 text-sky-600" />
                      <span>Continue with {prov.display_name || prov.name}</span>
                    </a>
                  ))}
                </div>
              )}

              <div className="mt-6 pt-5 border-t border-slate-100 bg-slate-50 -mx-8 -mb-8 p-6 rounded-b-2xl">
                <div className="flex items-center justify-between text-[11px] text-slate-500">
                  <span className="font-medium text-slate-600">Default Admin Account:</span>
                  <span className="font-mono bg-white px-2 py-0.5 rounded border border-slate-200">
                    admin / Admin@CMS2026!
                  </span>
                </div>
              </div>
            </>
          )}

          {step === 'mfa_verify' && (
            <>
              <div className="mb-5">
                <div className="inline-flex items-center justify-center w-10 h-10 rounded-xl bg-sky-50 text-sky-600 mb-3">
                  <Smartphone className="w-5 h-5" />
                </div>
                <h2 className="text-lg font-bold text-slate-900">Two-Factor Verification</h2>
                <p className="text-xs text-slate-500 mt-0.5">
                  Enter the 6-digit code from your authenticator app or a single-use backup recovery code.
                </p>
              </div>

              {error && (
                <div className="mb-4 p-3 rounded-lg bg-rose-50 border border-rose-200 flex items-start gap-2 text-xs text-rose-700">
                  <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />
                  <span>{error}</span>
                </div>
              )}

              <form onSubmit={handleVerifyMfa} className="space-y-4">
                <Input
                  id="login-mfa-code"
                  label="Authenticator or Recovery Code"
                  type="text"
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value)}
                  placeholder="123456 or ABCD-EFGH"
                  required
                  autoFocus
                />
                <Button type="submit" variant="primary" className="w-full" isLoading={isLoading}>
                  <span>Verify & Sign In</span>
                  <ArrowRight className="w-4 h-4" />
                </Button>
                <button
                  type="button"
                  onClick={resetToCredentials}
                  className="w-full inline-flex items-center justify-center gap-1.5 text-xs text-slate-500 hover:text-slate-700 pt-1"
                >
                  <ArrowLeft className="w-3.5 h-3.5" />
                  <span>Back to password login</span>
                </button>
              </form>
            </>
          )}

          {step === 'mfa_setup' && enrollData && (
            <>
              <div className="mb-4">
                <div className="inline-flex items-center justify-center w-10 h-10 rounded-xl bg-amber-50 text-amber-600 mb-3">
                  <ShieldCheck className="w-5 h-5" />
                </div>
                <h2 className="text-lg font-bold text-slate-900">MFA Enrollment Required</h2>
                <p className="text-xs text-slate-500 mt-0.5">
                  Your organization requires two-factor authentication for all administrators. Add this key to your authenticator app, then enter the 6-digit code.
                </p>
              </div>

              {error && (
                <div className="mb-4 p-3 rounded-lg bg-rose-50 border border-rose-200 flex items-start gap-2 text-xs text-rose-700">
                  <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />
                  <span>{error}</span>
                </div>
              )}

              <div className="mb-4 p-3 rounded-lg bg-slate-50 border border-slate-200 space-y-2 text-xs">
                <div className="flex items-center justify-between">
                  <span className="font-semibold text-slate-700">Secret Key</span>
                  <button
                    type="button"
                    onClick={() => void copyText('secret', enrollData.secret)}
                    className="inline-flex items-center gap-1 text-sky-600 hover:text-sky-700 font-medium"
                  >
                    {copiedKey === 'secret' ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
                    <span>{copiedKey === 'secret' ? 'Copied' : 'Copy'}</span>
                  </button>
                </div>
                <p className="font-mono text-[11px] break-all text-slate-800 bg-white p-2 rounded border border-slate-200">
                  {enrollData.secret}
                </p>
                <div className="flex items-center justify-between pt-1">
                  <span className="text-[11px] text-slate-500">OTPAuth Provisioning URI</span>
                  <button
                    type="button"
                    onClick={() => void copyText('uri', enrollData.otpauth_uri)}
                    className="inline-flex items-center gap-1 text-sky-600 hover:text-sky-700 font-medium text-[11px]"
                  >
                    {copiedKey === 'uri' ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
                    <span>{copiedKey === 'uri' ? 'Copied' : 'Copy URI'}</span>
                  </button>
                </div>
              </div>

              <form onSubmit={handleConfirmForcedMfaSetup} className="space-y-4">
                <Input
                  id="login-mfa-setup-code"
                  label="Initial 6-Digit Verification Code"
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value)}
                  placeholder="000000"
                  required
                />
                <Button type="submit" variant="primary" className="w-full" isLoading={isLoading}>
                  Enable MFA & Continue
                </Button>
                <button
                  type="button"
                  onClick={resetToCredentials}
                  className="w-full inline-flex items-center justify-center gap-1.5 text-xs text-slate-500 hover:text-slate-700"
                >
                  <ArrowLeft className="w-3.5 h-3.5" />
                  <span>Cancel</span>
                </button>
              </form>
            </>
          )}

          {step === 'mfa_recovery' && confirmResult && (
            <div className="space-y-4">
              <div>
                <h2 className="text-lg font-bold text-slate-900">Save Your Recovery Codes</h2>
                <p className="text-xs text-slate-500 mt-0.5">
                  Each single-use backup code allows sign-in if you lose access to your authenticator app. Store them in a secure location.
                </p>
              </div>
              <div className="grid grid-cols-2 gap-2 p-3 rounded-lg bg-slate-50 border border-slate-200 font-mono text-xs text-slate-800">
                {confirmResult.recovery_codes.map((code) => (
                  <div key={code} className="bg-white px-2.5 py-1.5 rounded border border-slate-200 text-center">
                    {code}
                  </div>
                ))}
              </div>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="secondary"
                  className="flex-1"
                  onClick={() => void copyText('codes', confirmResult.recovery_codes.join('\n'))}
                >
                  {copiedKey === 'codes' ? 'Copied Codes' : 'Copy All Codes'}
                </Button>
                <Button type="button" variant="primary" className="flex-1" onClick={handleFinishForcedMfaSetup}>
                  Continue to Console
                </Button>
              </div>
            </div>
          )}

          {step === 'password_expired' && (
            <>
              <div className="mb-5">
                <div className="inline-flex items-center justify-center w-10 h-10 rounded-xl bg-amber-50 text-amber-600 mb-3">
                  <KeyRound className="w-5 h-5" />
                </div>
                <h2 className="text-lg font-bold text-slate-900">Password Expired</h2>
                <p className="text-xs text-slate-500 mt-0.5">
                  Your password exceeded the configured rotation window. Choose a new password to continue.
                </p>
              </div>

              {error && (
                <div className="mb-4 p-3 rounded-lg bg-rose-50 border border-rose-200 flex items-start gap-2 text-xs text-rose-700">
                  <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />
                  <span>{error}</span>
                </div>
              )}

              <form onSubmit={handleResetExpiredPassword} className="space-y-4">
                <Input
                  label="New Password"
                  type="password"
                  required
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                />
                <Input
                  label="Confirm New Password"
                  type="password"
                  required
                  value={confirmNewPassword}
                  onChange={(e) => setConfirmNewPassword(e.target.value)}
                />
                <Button type="submit" variant="primary" className="w-full" isLoading={isLoading}>
                  Update Password & Sign In
                </Button>
                <button
                  type="button"
                  onClick={resetToCredentials}
                  className="w-full inline-flex items-center justify-center gap-1.5 text-xs text-slate-500 hover:text-slate-700"
                >
                  <ArrowLeft className="w-3.5 h-3.5" />
                  <span>Back to sign in</span>
                </button>
              </form>
            </>
          )}
        </div>

        <p className="text-center text-[11px] text-slate-400 mt-8">
          Powered by Go <span className="font-mono">html/template</span> & Modular REST Architecture
        </p>
      </div>
    </div>
  );
};
