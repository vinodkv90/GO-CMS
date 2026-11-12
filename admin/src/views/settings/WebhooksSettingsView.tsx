import React, { useEffect, useState } from 'react';
import { Activity, Play, Plus, Send, Shield, Trash2, Webhook as WebhookIcon } from 'lucide-react';
import { webhooksApi } from '../../api/endpoints';
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
import type { Webhook, WebhookDelivery, WebhookEventDescription } from '../../types';

export interface WebhooksSettingsViewProps {
  canCreate: boolean;
  canEdit: boolean;
  canDelete: boolean;
}

const FALLBACK_EVENTS: WebhookEventDescription[] = [
  { name: 'entry.create', label: 'Entry Created', category: 'Content', description: 'Triggered when a new content entry is created.' },
  { name: 'entry.update', label: 'Entry Updated', category: 'Content', description: 'Triggered when a content entry is updated.' },
  { name: 'entry.delete', label: 'Entry Deleted', category: 'Content', description: 'Triggered when a content entry is deleted.' },
  { name: 'entry.publish', label: 'Entry Published', category: 'Content', description: 'Triggered when a content entry is published.' },
  { name: 'media.create', label: 'Media Uploaded', category: 'Media', description: 'Triggered when a media file is uploaded.' },
  { name: 'media.delete', label: 'Media Deleted', category: 'Media', description: 'Triggered when a media asset is deleted.' },
  { name: 'content_type.create', label: 'Content-Type Created', category: 'Schema', description: 'Triggered when a content-type is created.' },
  { name: 'content_type.update', label: 'Content-Type Updated', category: 'Schema', description: 'Triggered when a content-type is updated.' },
  { name: 'content_type.delete', label: 'Content-Type Deleted', category: 'Schema', description: 'Triggered when a content-type is deleted.' },
];

export const WebhooksSettingsView: React.FC<WebhooksSettingsViewProps> = ({
  canCreate,
  canEdit,
  canDelete,
}) => {
  const toast = useToast();
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [eventsCatalog, setEventsCatalog] = useState<WebhookEventDescription[]>(FALLBACK_EVENTS);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Create / Edit modal
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Webhook | null>(null);
  const [form, setForm] = useState({
    name: '',
    url: '',
    secret: '',
    secret_configured: false,
    is_enabled: true,
    max_retries: 3,
    events: ['entry.publish'] as string[],
    headers: [] as Array<{ key: string; value: string }>,
  });
  const [saving, setSaving] = useState(false);
  const [modalError, setModalError] = useState<string | null>(null);

  // Delivery logs modal
  const [logsWebhook, setLogsWebhook] = useState<Webhook | null>(null);
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [testingId, setTestingId] = useState<string | null>(null);

  const loadData = async () => {
    setLoading(true);
    setError(null);
    try {
      const [whRes, evRes] = await Promise.all([
        webhooksApi.list(),
        webhooksApi.listEvents().catch(() => ({ data: FALLBACK_EVENTS })),
      ]);
      setWebhooks(whRes.data || []);
      if (evRes.data?.length) {
        setEventsCatalog(evRes.data);
      }
    } catch (err: any) {
      setError(err?.message || 'Failed to load webhooks');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadData();
  }, []);

  const openModal = (wh?: Webhook) => {
    setModalError(null);
    if (wh) {
      setEditing(wh);
      const headerPairs = Object.entries(wh.headers || {}).map(([key, value]) => ({ key, value }));
      setForm({
        name: wh.name,
        url: wh.url,
        secret: wh.secret || '',
        secret_configured: Boolean(wh.secret_configured),
        is_enabled: wh.is_enabled,
        max_retries: wh.max_retries,
        events: wh.events || [],
        headers: headerPairs,
      });
    } else {
      setEditing(null);
      setForm({
        name: '',
        url: '',
        secret: '',
        secret_configured: false,
        is_enabled: true,
        max_retries: 3,
        events: ['entry.publish', 'entry.create'],
        headers: [],
      });
    }
    setModalOpen(true);
  };

  const toggleEvent = (evName: string) => {
    setForm((prev) => {
      const exists = prev.events.includes(evName);
      return {
        ...prev,
        events: exists ? prev.events.filter((e) => e !== evName) : [...prev.events, evName],
      };
    });
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setModalError(null);
    const headersMap: Record<string, string> = {};
    for (const pair of form.headers) {
      const k = pair.key.trim();
      if (k) {
        headersMap[k] = pair.value.trim();
      }
    }
    try {
      if (editing) {
        await webhooksApi.update(editing.id, {
          name: form.name.trim(),
          url: form.url.trim(),
          headers: headersMap,
          events: form.events,
          secret: form.secret,
          is_enabled: form.is_enabled,
          max_retries: Number(form.max_retries) || 0,
        });
        toast.success(`Webhook "${form.name.trim()}" updated`);
      } else {
        await webhooksApi.create({
          name: form.name.trim(),
          url: form.url.trim(),
          headers: headersMap,
          events: form.events,
          secret: form.secret.trim() || undefined,
          is_enabled: form.is_enabled,
          max_retries: Number(form.max_retries) || 0,
        });
        toast.success(`Webhook "${form.name.trim()}" created`);
      }
      setModalOpen(false);
      await loadData();
    } catch (err: any) {
      setModalError(err?.message || 'Failed to save webhook');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (wh: Webhook) => {
    try {
      await webhooksApi.remove(wh.id);
      toast.success(`Deleted webhook "${wh.name}"`);
      await loadData();
    } catch (err: any) {
      toast.error('Delete failed', err?.message);
    }
  };

  const openDeliveryLogs = async (wh: Webhook) => {
    setLogsWebhook(wh);
    setLogsLoading(true);
    try {
      const res = await webhooksApi.listDeliveries(wh.id, 25, 0);
      setDeliveries(res.data || []);
    } catch (err: any) {
      toast.error('Failed to load delivery logs', err?.message);
      setDeliveries([]);
    } finally {
      setLogsLoading(false);
    }
  };

  const handleTriggerTest = async (wh: Webhook) => {
    setTestingId(wh.id);
    try {
      const res = await webhooksApi.triggerTest(wh.id);
      const d = res.data;
      if (d.success) {
        toast.success(`Test delivery succeeded (HTTP ${d.status_code} in ${d.duration_ms}ms)`);
      } else {
        toast.error('Test delivery returned failure', d.error_message || `HTTP ${d.status_code}`);
      }
      await openDeliveryLogs(wh);
    } catch (err: any) {
      toast.error('Test trigger failed', err?.message);
    } finally {
      setTestingId(null);
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
              <WebhookIcon className="w-4 h-4 text-sky-600" />
              <span>Outgoing Webhooks ({webhooks.length})</span>
            </span>
          }
          description="Send SSRF-protected, HMAC-SHA256 signed HTTP POST payloads when content entries or schemas change."
          action={
            canCreate && (
              <Button type="button" size="sm" onClick={() => openModal()}>
                <Plus className="w-3.5 h-3.5 mr-1.5" />
                Create Webhook
              </Button>
            )
          }
        />
        <CardContent>
          {webhooks.length === 0 ? (
            <div className="py-10 text-center text-xs text-slate-500">
              No outgoing webhooks configured yet. Create a webhook to integrate external build pipelines, search indexes, or notifications.
            </div>
          ) : (
            <div className="divide-y divide-slate-100">
              {webhooks.map((wh) => (
                <div key={wh.id} className="py-4 flex flex-col lg:flex-row lg:items-center justify-between gap-4">
                  <div className="space-y-1.5">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-semibold text-slate-800">{wh.name}</span>
                      <Badge variant={wh.is_enabled ? 'success' : 'neutral'}>
                        {wh.is_enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                      {wh.secret_configured && (
                        <Badge variant="info">
                          <Shield className="w-3 h-3 mr-1 inline" />
                          HMAC Signed
                        </Badge>
                      )}
                      <span className="text-[11px] text-slate-500">Max retries: {wh.max_retries}</span>
                    </div>
                    <p className="text-xs font-mono text-slate-600 break-all">{wh.url}</p>
                    <div className="flex flex-wrap gap-1.5 pt-0.5">
                      {(wh.events || []).map((ev) => (
                        <Badge key={ev} variant="neutral">
                          {ev}
                        </Badge>
                      ))}
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => void openDeliveryLogs(wh)}
                    >
                      <Activity className="w-3.5 h-3.5 mr-1" />
                      Deliveries
                    </Button>
                    {canEdit && (
                      <>
                        <Button
                          type="button"
                          size="sm"
                          variant="secondary"
                          isLoading={testingId === wh.id}
                          onClick={() => void handleTriggerTest(wh)}
                        >
                          <Send className="w-3.5 h-3.5 mr-1" />
                          Trigger Test
                        </Button>
                        <Button type="button" size="sm" variant="secondary" onClick={() => openModal(wh)}>
                          Edit
                        </Button>
                      </>
                    )}
                    {canDelete && (
                      <Button
                        type="button"
                        size="sm"
                        variant="destructive"
                        onClick={() => void handleDelete(wh)}
                        title="Delete webhook"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </Button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Create / Edit Webhook Modal */}
      <Modal
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        title={editing ? `Edit Webhook: ${editing.name}` : 'Create Webhook'}
        maxWidth="xl"
      >
        <form onSubmit={(e) => void handleSave(e)} className="space-y-4">
          {modalError && <Alert variant="error">{modalError}</Alert>}

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Webhook Name"
              placeholder="Production Netlify Rebuild"
              required
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
            <Input
              label="Max Retries (0 - 5)"
              type="number"
              min={0}
              max={5}
              value={form.max_retries}
              onChange={(e) => setForm({ ...form, max_retries: Number(e.target.value) || 0 })}
            />
          </div>

          <Input
            label="Target URL"
            placeholder="https://api.example.com/webhooks/cms"
            autoComplete="off"
            required
            value={form.url}
            onChange={(e) => setForm({ ...form, url: e.target.value })}
            helperText="Private, loopback, and cloud metadata IPs are blocked for SSRF protection."
          />

          <div className="flex items-center justify-between p-3 rounded-lg bg-slate-50 border border-slate-200">
            <span className="text-xs font-semibold text-slate-800">Enable Webhook Dispatch</span>
            <Switch
              label="Webhook Enabled"
              checked={form.is_enabled}
              onChange={(checked) => setForm({ ...form, is_enabled: checked })}
            />
          </div>

          <SecretInput
            label="HMAC Signing Secret (X-CMS-Webhook-Signature)"
            placeholder="Optional shared secret for sha256 payload signatures"
            value={form.secret}
            hasStoredSecret={form.secret_configured}
            onChange={(val) => setForm({ ...form, secret: val })}
          />

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-slate-700">Custom HTTP Headers</span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() =>
                  setForm((prev) => ({
                    ...prev,
                    headers: [...prev.headers, { key: '', value: '' }],
                  }))
                }
              >
                <Plus className="w-3 h-3 mr-1" />
                Add Header
              </Button>
            </div>
            {form.headers.length === 0 ? (
              <p className="text-[11px] text-slate-400">No custom headers configured.</p>
            ) : (
              <div className="space-y-2">
                {form.headers.map((pair, idx) => (
                  <div key={idx} className="flex items-center gap-2">
                    <Input
                      placeholder="Header Name"
                      value={pair.key}
                      onChange={(e) =>
                        setForm((prev) => ({
                          ...prev,
                          headers: prev.headers.map((h, i) => (i === idx ? { ...h, key: e.target.value } : h)),
                        }))
                      }
                    />
                    <Input
                      placeholder="Header Value"
                      value={pair.value}
                      onChange={(e) =>
                        setForm((prev) => ({
                          ...prev,
                          headers: prev.headers.map((h, i) => (i === idx ? { ...h, value: e.target.value } : h)),
                        }))
                      }
                    />
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() =>
                        setForm((prev) => ({
                          ...prev,
                          headers: prev.headers.filter((_, i) => i !== idx),
                        }))
                      }
                    >
                      <Trash2 className="w-3.5 h-3.5 text-rose-500" />
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="space-y-2">
            <span className="text-xs font-semibold text-slate-700">Subscribed Events</span>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-48 overflow-y-auto p-2 rounded-lg border border-slate-200 bg-slate-50/70">
              {eventsCatalog.map((ev) => {
                const checked = form.events.includes(ev.name);
                return (
                  <label
                    key={ev.name}
                    className={`flex items-start gap-2 p-2 rounded border text-xs cursor-pointer ${
                      checked ? 'bg-sky-50 border-sky-200 text-sky-900' : 'bg-white border-slate-200 text-slate-700'
                    }`}
                  >
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={() => toggleEvent(ev.name)}
                      className="mt-0.5 rounded border-slate-300 text-sky-600 focus:ring-sky-500"
                    />
                    <div>
                      <p className="font-semibold">{ev.label}</p>
                      <p className="text-[10px] font-mono text-slate-500">{ev.name}</p>
                    </div>
                  </label>
                );
              })}
            </div>
          </div>

          <div className="flex justify-end gap-2 pt-2 border-t border-slate-100">
            <Button type="button" size="sm" variant="secondary" onClick={() => setModalOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" size="sm" isLoading={saving}>
              {editing ? 'Save Changes' : 'Create Webhook'}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delivery Logs Modal */}
      <Modal
        isOpen={Boolean(logsWebhook)}
        onClose={() => setLogsWebhook(null)}
        title={logsWebhook ? `Delivery History: ${logsWebhook.name}` : 'Delivery History'}
        maxWidth="2xl"
      >
        <div className="space-y-4">
          {logsWebhook && canEdit && (
            <div className="flex items-center justify-between bg-slate-50 border border-slate-200 rounded-lg px-3 py-2">
              <span className="text-xs font-mono text-slate-600 truncate">{logsWebhook.url}</span>
              <Button
                type="button"
                size="sm"
                variant="secondary"
                isLoading={testingId === logsWebhook.id}
                onClick={() => void handleTriggerTest(logsWebhook)}
              >
                <Play className="w-3.5 h-3.5 mr-1" />
                Send Test Event
              </Button>
            </div>
          )}

          {logsLoading ? (
            <SettingsCardSkeleton rows={3} />
          ) : deliveries.length === 0 ? (
            <p className="text-xs text-slate-500 py-8 text-center">No deliveries recorded for this webhook yet.</p>
          ) : (
            <div className="space-y-2 max-h-96 overflow-y-auto">
              {deliveries.map((d) => (
                <div key={d.id} className="p-3 rounded-lg border border-slate-200 bg-white space-y-1.5 text-xs">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <Badge variant={d.success ? 'success' : 'error'}>
                        {d.status_code > 0 ? `HTTP ${d.status_code}` : 'Network Error'}
                      </Badge>
                      <span className="font-mono font-semibold text-slate-800">{d.event}</span>
                      <span className="text-[11px] text-slate-500">Attempt #{d.attempt}</span>
                    </div>
                    <span className="text-[11px] text-slate-500">
                      {d.duration_ms}ms • {new Date(d.created_at).toLocaleString()}
                    </span>
                  </div>
                  {d.error_message && <p className="text-[11px] text-rose-600">{d.error_message}</p>}
                  {d.response_snippet && (
                    <pre className="text-[11px] font-mono bg-slate-50 border border-slate-200 rounded p-2 overflow-x-auto text-slate-700">
                      {d.response_snippet}
                    </pre>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </Modal>
    </div>
  );
};

