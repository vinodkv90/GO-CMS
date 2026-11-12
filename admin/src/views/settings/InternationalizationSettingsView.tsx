import React, { useEffect, useState } from 'react';
import { Globe, Languages, Plus, Star, Trash2 } from 'lucide-react';
import { localesApi } from '../../api/endpoints';
import { Alert } from '../../components/ui/Alert';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { Modal } from '../../components/ui/Modal';
import { Select } from '../../components/ui/Select';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import { useToast } from '../../components/ui/Toast';
import type { Locale } from '../../types';

export interface InternationalizationSettingsViewProps {
  canCreate: boolean;
  canEdit: boolean;
  canDelete: boolean;
}

export const InternationalizationSettingsView: React.FC<InternationalizationSettingsViewProps> = ({
  canCreate,
  canEdit,
  canDelete,
}) => {
  const toast = useToast();
  const [locales, setLocales] = useState<Locale[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Locale | null>(null);
  const [form, setForm] = useState({
    code: '',
    name: '',
    native_name: '',
    direction: 'ltr' as 'ltr' | 'rtl',
    fallback_code: '',
    is_enabled: true,
    is_default: false,
  });
  const [saving, setSaving] = useState(false);
  const [modalError, setModalError] = useState<string | null>(null);

  const loadLocales = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await localesApi.list();
      setLocales(res.data || []);
    } catch (err: any) {
      setError(err?.message || 'Failed to load locales');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadLocales();
  }, []);

  const openModal = (loc?: Locale) => {
    setModalError(null);
    if (loc) {
      setEditing(loc);
      setForm({
        code: loc.code,
        name: loc.name,
        native_name: loc.native_name || loc.name,
        direction: loc.direction === 'rtl' ? 'rtl' : 'ltr',
        fallback_code: loc.fallback_code || '',
        is_enabled: loc.is_enabled,
        is_default: loc.is_default,
      });
    } else {
      setEditing(null);
      setForm({
        code: '',
        name: '',
        native_name: '',
        direction: 'ltr',
        fallback_code: '',
        is_enabled: true,
        is_default: false,
      });
    }
    setModalOpen(true);
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setModalError(null);
    try {
      if (editing) {
        await localesApi.update(editing.code, {
          name: form.name.trim(),
          native_name: form.native_name.trim(),
          direction: form.direction,
          fallback_code: form.fallback_code,
          is_enabled: form.is_enabled,
          is_default: form.is_default,
        });
        toast.success(`Updated locale "${editing.code}"`);
      } else {
        await localesApi.create({
          code: form.code.trim(),
          name: form.name.trim(),
          native_name: form.native_name.trim() || undefined,
          direction: form.direction,
          fallback_code: form.fallback_code || undefined,
          is_enabled: form.is_enabled,
          is_default: form.is_default,
        });
        toast.success(`Added locale "${form.code.trim()}"`);
      }
      setModalOpen(false);
      await loadLocales();
    } catch (err: any) {
      setModalError(err?.message || 'Failed to save locale');
    } finally {
      setSaving(false);
    }
  };

  const handleSetDefault = async (code: string) => {
    try {
      await localesApi.setDefault(code);
      toast.success(`Default locale switched to "${code}"`);
      await loadLocales();
    } catch (err: any) {
      toast.error('Failed to update default locale', err?.message);
    }
  };

  const handleDelete = async (loc: Locale) => {
    try {
      await localesApi.remove(loc.code);
      toast.success(`Deleted locale "${loc.code}"`);
      await loadLocales();
    } catch (err: any) {
      toast.error('Cannot delete locale', err?.message || 'Locale may be in use or default.');
    }
  };

  if (loading) {
    return <SettingsCardSkeleton rows={4} />;
  }

  return (
    <div className="space-y-6">
      {error && <Alert variant="error">{error}</Alert>}

      <Card>
        <CardHeader
          title={
            <span className="flex items-center gap-2">
              <Languages className="w-4 h-4 text-sky-600" />
              <span>Configured Locales ({locales.length})</span>
            </span>
          }
          description="Define supported content languages, fallback chains, and text layout direction (LTR / RTL). Default or referenced locales cannot be removed."
          action={
            canCreate && (
              <Button type="button" size="sm" onClick={() => openModal()}>
                <Plus className="w-3.5 h-3.5 mr-1.5" />
                Add Locale
              </Button>
            )
          }
        />
        <CardContent>
          <div className="divide-y divide-slate-100">
            {locales.map((loc) => (
              <div key={loc.code} className="py-3.5 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                <div className="space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="info">{loc.code}</Badge>
                    <span className="text-sm font-semibold text-slate-800">{loc.name}</span>
                    {loc.native_name && loc.native_name !== loc.name && (
                      <span className="text-xs text-slate-500">({loc.native_name})</span>
                    )}
                    {loc.is_default && <Badge variant="success">Default</Badge>}
                    {!loc.is_enabled && <Badge variant="warning">Disabled</Badge>}
                    <Badge variant="neutral">{String(loc.direction).toUpperCase()}</Badge>
                    {loc.fallback_code && (
                      <span className="text-[11px] text-slate-500">
                        Fallback: <code className="font-mono">{loc.fallback_code}</code>
                      </span>
                    )}
                  </div>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  {canEdit && !loc.is_default && (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => void handleSetDefault(loc.code)}
                    >
                      <Star className="w-3.5 h-3.5 mr-1" />
                      Set Default
                    </Button>
                  )}
                  {canEdit && (
                    <Button type="button" size="sm" variant="secondary" onClick={() => openModal(loc)}>
                      Edit
                    </Button>
                  )}
                  {canDelete && (
                    <Button
                      type="button"
                      size="sm"
                      variant="destructive"
                      disabled={loc.is_default}
                      title={loc.is_default ? 'Cannot delete the default locale' : `Delete ${loc.code}`}
                      onClick={() => void handleDelete(loc)}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <Modal
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        title={editing ? `Edit Locale: ${editing.code}` : 'Add Locale'}
        maxWidth="lg"
      >
        <form onSubmit={(e) => void handleSave(e)} className="space-y-4">
          {modalError && <Alert variant="error">{modalError}</Alert>}

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="ISO / BCP-47 Code"
              placeholder="en, fr, es, ar, zh-CN"
              disabled={Boolean(editing)}
              required
              value={form.code}
              onChange={(e) => setForm({ ...form, code: e.target.value })}
              helperText="2-letter language with optional region (e.g. zh-CN)."
            />
            <Select
              label="Text Direction"
              value={form.direction}
              onChange={(e) => setForm({ ...form, direction: e.target.value as 'ltr' | 'rtl' })}
              options={[
                { label: 'Left-to-Right (LTR)', value: 'ltr' },
                { label: 'Right-to-Left (RTL)', value: 'rtl' },
              ]}
            />
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Display Name"
              placeholder="French"
              required
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
            <Input
              label="Native Name"
              placeholder="Français"
              value={form.native_name}
              onChange={(e) => setForm({ ...form, native_name: e.target.value })}
            />
          </div>

          <Select
            label="Fallback Locale (optional)"
            value={form.fallback_code}
            onChange={(e) => setForm({ ...form, fallback_code: e.target.value })}
            options={[
              { label: 'System Default Locale', value: '' },
              ...locales
                .filter((l) => !editing || l.code !== editing.code)
                .map((l) => ({ label: `${l.name} (${l.code})`, value: l.code })),
            ]}
          />

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div className="flex items-center justify-between p-3 rounded-lg bg-slate-50 border border-slate-200">
              <span className="text-xs font-semibold text-slate-800 flex items-center gap-1.5">
                <Globe className="w-3.5 h-3.5 text-sky-600" /> Enabled
              </span>
              <Switch
                label="Locale Enabled"
                checked={form.is_enabled}
                disabled={form.is_default}
                onChange={(checked) => setForm({ ...form, is_enabled: checked })}
              />
            </div>
            <div className="flex items-center justify-between p-3 rounded-lg bg-slate-50 border border-slate-200">
              <span className="text-xs font-semibold text-slate-800">Set as Default</span>
              <Switch
                label="Default Locale"
                checked={form.is_default}
                onChange={(checked) =>
                  setForm({ ...form, is_default: checked, is_enabled: checked ? true : form.is_enabled })
                }
              />
            </div>
          </div>

          <div className="flex justify-end gap-2 pt-2 border-t border-slate-100">
            <Button type="button" size="sm" variant="secondary" onClick={() => setModalOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" size="sm" isLoading={saving}>
              {editing ? 'Save Locale' : 'Add Locale'}
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
};

