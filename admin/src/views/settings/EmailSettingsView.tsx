import React, { useCallback, useState } from 'react';
import { Loader2, MailCheck, Send } from 'lucide-react';
import { emailApi } from '../../api/endpoints';
import { useSettingsForm } from '../../hooks/useSettingsForm';
import { Alert } from '../../components/ui/Alert';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { Modal } from '../../components/ui/Modal';
import { SecretInput } from '../../components/ui/SecretInput';
import { Select } from '../../components/ui/Select';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import type { EmailSettings, EmailTestResult } from '../../types';

export interface EmailSettingsViewProps {
  canEdit: boolean;
  onDirtyChange: (dirty: boolean) => void;
  registerHandlers: (handlers: {
    save: () => Promise<void>;
    discard: () => void;
    isSaving: boolean;
  }) => void;
}

export const EmailSettingsView: React.FC<EmailSettingsViewProps> = ({
  canEdit,
  onDirtyChange,
  registerHandlers,
}) => {
  const [testingConn, setTestingConn] = useState(false);
  const [connResult, setConnResult] = useState<EmailTestResult | null>(null);
  const [connError, setConnError] = useState<string | null>(null);

  const [sendModalOpen, setSendModalOpen] = useState(false);
  const [recipientEmail, setRecipientEmail] = useState('');
  const [sendingTest, setSendingTest] = useState(false);
  const [sendResult, setSendResult] = useState<EmailTestResult | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);

  const load = useCallback(async () => {
    const res = await emailApi.getSettings();
    return res.data;
  }, []);

  const save = useCallback(async (value: EmailSettings) => {
    const res = await emailApi.updateSettings(value);
    return res.data;
  }, []);

  const validate = useCallback((v: EmailSettings): string | null => {
    if (v.provider === 'smtp') {
      if (!v.from_email.trim()) return 'From email address is required when using SMTP.';
      if (!v.smtp_host.trim()) return 'SMTP host is required.';
      if (v.smtp_port < 1 || v.smtp_port > 65535) return 'SMTP port must be between 1 and 65535.';
    }
    return null;
  }, []);

  const { form, setForm, isLoading, error } = useSettingsForm<EmailSettings>({
    load,
    save,
    validate,
    successTitle: 'Email configuration saved',
    successMessage: 'Outbound mail transport settings have been updated.',
    loadErrorFallback: 'Failed to load email settings.',
    saveErrorFallback: 'Failed to save email settings.',
    onDirtyChange,
    registerHandlers,
  });

  const handleTestConnection = async () => {
    if (!form) return;
    setTestingConn(true);
    setConnResult(null);
    setConnError(null);
    try {
      const res = await emailApi.testConnection(form);
      setConnResult(res.data);
    } catch (err: any) {
      setConnError(err?.message || 'Failed to test email connection.');
    } finally {
      setTestingConn(false);
    }
  };

  const handleSendTest = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form) return;
    setSendingTest(true);
    setSendResult(null);
    setSendError(null);
    try {
      const res = await emailApi.testSend(recipientEmail, form);
      setSendResult(res.data);
    } catch (err: any) {
      setSendError(err?.message || 'Failed to send test email.');
    } finally {
      setSendingTest(false);
    }
  };

  if (isLoading || !form) {
    return <SettingsCardSkeleton />;
  }

  return (
    <div className="space-y-6">
      {error && (
        <Alert variant="error" title="Unable to save email settings">
          {error}
        </Alert>
      )}

      <Card>
        <CardHeader
          title="Outbound Mail Transport"
          description="Configure how transactional messages (invitations, confirmations, password resets) are delivered."
          action={
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="secondary"
                size="sm"
                disabled={testingConn}
                onClick={() => void handleTestConnection()}
              >
                {testingConn ? (
                  <>
                    <Loader2 className="w-3.5 h-3.5 animate-spin" /> Testing...
                  </>
                ) : (
                  <>
                    <MailCheck className="w-3.5 h-3.5" /> Test Connection
                  </>
                )}
              </Button>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={() => {
                  setSendResult(null);
                  setSendError(null);
                  setSendModalOpen(true);
                }}
              >
                <Send className="w-3.5 h-3.5" /> Send Test Email
              </Button>
            </div>
          }
        />
        <CardContent className="space-y-5">
          {connError && (
            <Alert variant="error" title="Connection check failed">
              {connError}
            </Alert>
          )}
          {connResult && (
            <Alert
              variant={connResult.ok ? 'success' : 'error'}
              title={connResult.ok ? 'Transport ready' : 'Transport error'}
            >
              {connResult.message} ({connResult.latency_ms}ms)
            </Alert>
          )}

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Select
              label="Email provider"
              value={form.provider}
              disabled={!canEdit}
              options={[
                { label: 'Log / Development Preview (No external SMTP)', value: 'log' },
                { label: 'SMTP Server (net/smtp)', value: 'smtp' },
                { label: 'Disabled (Do not send email)', value: 'disabled' },
              ]}
              onChange={(e) => {
                setConnResult(null);
                setConnError(null);
                setForm((prev) =>
                  prev
                    ? { ...prev, provider: e.target.value as EmailSettings['provider'] }
                    : prev,
                );
              }}
            />

            <Input
              label="Default Reply-To address"
              type="email"
              value={form.default_reply_to}
              disabled={!canEdit}
              placeholder="support@example.com"
              onChange={(e) =>
                setForm((prev) => (prev ? { ...prev, default_reply_to: e.target.value } : prev))
              }
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Sender display name"
              value={form.from_name}
              disabled={!canEdit}
              placeholder="Go CMS"
              onChange={(e) =>
                setForm((prev) => (prev ? { ...prev, from_name: e.target.value } : prev))
              }
            />
            <Input
              label="From email address"
              type="email"
              value={form.from_email}
              disabled={!canEdit}
              placeholder="no-reply@example.com"
              onChange={(e) =>
                setForm((prev) => (prev ? { ...prev, from_email: e.target.value } : prev))
              }
            />
          </div>
        </CardContent>
      </Card>

      {form.provider === 'smtp' && (
        <Card>
          <CardHeader
            title="SMTP Connection & Authentication"
            description="SMTP password is encrypted with AES-256-GCM at rest and never exposed in API responses."
          />
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="md:col-span-2">
                <Input
                  label="SMTP host"
                  value={form.smtp_host}
                  disabled={!canEdit}
                  placeholder="smtp.example.com"
                  onChange={(e) =>
                    setForm((prev) => (prev ? { ...prev, smtp_host: e.target.value } : prev))
                  }
                />
              </div>
              <Input
                label="SMTP port"
                type="number"
                min={1}
                max={65535}
                value={form.smtp_port}
                disabled={!canEdit}
                onChange={(e) =>
                  setForm((prev) =>
                    prev ? { ...prev, smtp_port: Number(e.target.value) || 587 } : prev,
                  )
                }
              />
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Select
                label="Transport encryption"
                value={form.encryption}
                disabled={!canEdit}
                options={[
                  { label: 'STARTTLS (Recommended - Port 587)', value: 'starttls' },
                  { label: 'Implicit TLS (Port 465)', value: 'tls' },
                  { label: 'Plain / Unencrypted (Local testing only)', value: 'none' },
                ]}
                onChange={(e) =>
                  setForm((prev) =>
                    prev
                      ? { ...prev, encryption: e.target.value as EmailSettings['encryption'] }
                      : prev,
                  )
                }
              />
              <Input
                label="Dial timeout (seconds)"
                type="number"
                min={1}
                max={120}
                value={form.timeout_seconds}
                disabled={!canEdit}
                onChange={(e) =>
                  setForm((prev) =>
                    prev ? { ...prev, timeout_seconds: Number(e.target.value) || 10 } : prev,
                  )
                }
              />
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Input
                label="SMTP username"
                autoComplete="off"
                value={form.smtp_username}
                disabled={!canEdit}
                onChange={(e) =>
                  setForm((prev) => (prev ? { ...prev, smtp_username: e.target.value } : prev))
                }
              />
              <SecretInput
                label="SMTP password"
                value={form.smtp_password || ''}
                hasStoredSecret={form.password_configured}
                disabled={!canEdit}
                onChange={(val) =>
                  setForm((prev) => (prev ? { ...prev, smtp_password: val } : prev))
                }
              />
            </div>

            <div className="flex items-center justify-between gap-4 border-t border-slate-100 pt-3">
              <div>
                <p className="text-xs font-semibold text-slate-800">Skip TLS certificate verification</p>
                <p className="text-[11px] text-slate-500">
                  Enable only for local or self-signed mail relays in non-production environments.
                </p>
              </div>
              <Switch
                checked={form.skip_tls_verify}
                disabled={!canEdit}
                onChange={(checked) =>
                  setForm((prev) => (prev ? { ...prev, skip_tls_verify: checked } : prev))
                }
              />
            </div>
          </CardContent>
        </Card>
      )}

      <Modal
        isOpen={sendModalOpen}
        onClose={() => setSendModalOpen(false)}
        title="Send Test Email"
        description="Dispatch a test message using the currently configured transport settings."
        maxWidth="md"
      >
        <form onSubmit={(e) => void handleSendTest(e)} className="space-y-4">
          {sendError && (
            <Alert variant="error" title="Delivery error">
              {sendError}
            </Alert>
          )}
          {sendResult && (
            <Alert
              variant={sendResult.ok ? 'success' : 'error'}
              title={sendResult.ok ? 'Test email sent' : 'Delivery failed'}
            >
              {sendResult.message} ({sendResult.latency_ms}ms)
            </Alert>
          )}

          <Input
            label="Recipient email address"
            type="email"
            required
            placeholder="you@example.com"
            value={recipientEmail}
            onChange={(e) => setRecipientEmail(e.target.value)}
          />

          <div className="flex justify-end gap-2 pt-2">
            <Button type="button" variant="secondary" onClick={() => setSendModalOpen(false)}>
              Close
            </Button>
            <Button type="submit" disabled={sendingTest || !recipientEmail.trim()}>
              {sendingTest ? 'Sending...' : 'Send Test'}
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
