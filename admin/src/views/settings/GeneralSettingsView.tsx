import React, { useEffect, useMemo, useState } from 'react';
import { Image as ImageIcon, Trash2 } from 'lucide-react';
import { settingsApi } from '../../api/endpoints';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { Textarea } from '../../components/ui/Textarea';
import { Select } from '../../components/ui/Select';
import { Button } from '../../components/ui/Button';
import { Alert } from '../../components/ui/Alert';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { MediaPickerModal } from '../../components/media/MediaPickerModal';
import { useToast } from '../../components/ui/Toast';
import type { GeneralSettings, MediaAsset, PublicGeneralSettings } from '../../types';

const COMMON_TIMEZONES = [
  'UTC',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Singapore',
  'Asia/Tokyo',
  'Australia/Sydney',
];

const DATE_FORMATS = [
  { label: 'YYYY-MM-DD (2026-11-12)', value: 'YYYY-MM-DD' },
  { label: 'MM/DD/YYYY (11/12/2026)', value: 'MM/DD/YYYY' },
  { label: 'DD/MM/YYYY (12/11/2026)', value: 'DD/MM/YYYY' },
  { label: 'MMM D, YYYY (Nov 12, 2026)', value: 'MMM D, YYYY' },
];

const TIME_FORMATS = [
  { label: '24-hour (14:30)', value: '24h' },
  { label: '12-hour (2:30 PM)', value: '12h' },
];

export interface GeneralSettingsViewProps {
  canEdit: boolean;
  onDirtyChange: (dirty: boolean) => void;
  registerHandlers: (handlers: {
    save: () => Promise<void>;
    discard: () => void;
    isSaving: boolean;
  }) => void;
  onPublicUpdated: (pub: PublicGeneralSettings) => void;
}

export const GeneralSettingsView: React.FC<GeneralSettingsViewProps> = ({
  canEdit,
  onDirtyChange,
  registerHandlers,
  onPublicUpdated,
}) => {
  const toast = useToast();
  const [baseline, setBaseline] = useState<GeneralSettings | null>(null);
  const [form, setForm] = useState<GeneralSettings | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pickerTarget, setPickerTarget] = useState<'logo' | 'favicon' | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setIsLoading(true);
      setError(null);
      try {
        const res = await settingsApi.getGeneral();
        if (!cancelled) {
          setBaseline(res.data);
          setForm(res.data);
          onDirtyChange(false);
        }
      } catch (err: any) {
        if (!cancelled) setError(err?.message || 'Failed to load general settings');
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [onDirtyChange]);

  const isDirty = useMemo(() => {
    if (!baseline || !form) return false;
    return (
      baseline.site_name !== form.site_name ||
      baseline.site_description !== form.site_description ||
      baseline.public_url !== form.public_url ||
      baseline.admin_path !== form.admin_path ||
      baseline.default_timezone !== form.default_timezone ||
      baseline.default_locale !== form.default_locale ||
      baseline.date_format !== form.date_format ||
      baseline.time_format !== form.time_format ||
      baseline.logo_url !== form.logo_url ||
      baseline.logo_media_id !== form.logo_media_id ||
      baseline.favicon_url !== form.favicon_url ||
      baseline.favicon_media_id !== form.favicon_media_id
    );
  }, [baseline, form]);

  useEffect(() => {
    onDirtyChange(isDirty);
  }, [isDirty, onDirtyChange]);

  useEffect(() => {
    registerHandlers({
      isSaving,
      discard: () => {
        if (baseline) {
          setForm(baseline);
          setError(null);
        }
      },
      save: async () => {
        if (!form || !canEdit) return;
        setIsSaving(true);
        setError(null);
        try {
          const res = await settingsApi.updateGeneral(form);
          setBaseline(res.data);
          setForm(res.data);
          onDirtyChange(false);
          onPublicUpdated({
            site_name: res.data.site_name,
            site_description: res.data.site_description,
            public_url: res.data.public_url,
            default_timezone: res.data.default_timezone,
            default_locale: res.data.default_locale,
            date_format: res.data.date_format,
            time_format: res.data.time_format,
            logo_url: res.data.logo_url,
            favicon_url: res.data.favicon_url,
          });
          toast.success('General settings saved');
        } catch (err: any) {
          const msg = err?.message || 'Failed to update general settings';
          setError(msg);
          toast.error('Save failed', msg);
        } finally {
          setIsSaving(false);
        }
      },
    });
  }, [baseline, canEdit, form, isSaving, onDirtyChange, onPublicUpdated, registerHandlers, toast]);

  if (isLoading || !form) {
    return (
      <div className="space-y-6">
        <SettingsCardSkeleton rows={4} />
        <SettingsCardSkeleton rows={4} />
      </div>
    );
  }

  const timezoneOptions = COMMON_TIMEZONES.includes(form.default_timezone)
    ? COMMON_TIMEZONES
    : [form.default_timezone, ...COMMON_TIMEZONES];

  const handleAssetConfirm = (assets: MediaAsset[]) => {
    const asset = assets[0];
    if (!asset || !pickerTarget) {
      setPickerTarget(null);
      return;
    }
    const resolvedUrl = asset.url || `/api/v1/media/${asset.id}/file`;
    if (pickerTarget === 'logo') {
      setForm((prev) =>
        prev ? { ...prev, logo_url: resolvedUrl, logo_media_id: asset.id } : prev,
      );
    } else {
      setForm((prev) =>
        prev ? { ...prev, favicon_url: resolvedUrl, favicon_media_id: asset.id } : prev,
      );
    }
    setPickerTarget(null);
  };

  return (
    <div className="space-y-6">
      {!canEdit && (
        <Alert variant="warning" title="Read-only access">
          Your role permits viewing General Settings, but editing requires the <code>update:settings</code> permission.
        </Alert>
      )}

      {error && (
        <Alert variant="error" title="Validation or save error">
          {error}
        </Alert>
      )}

      <Card>
        <CardHeader
          title="Site Identity"
          description="Controls rendered page titles, public web templates, and Admin Console branding."
        />
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Site Name"
              required
              disabled={!canEdit}
              value={form.site_name}
              onChange={(e) => setForm({ ...form, site_name: e.target.value })}
              placeholder="Go CMS"
            />
            <Input
              label="Public URL"
              disabled={!canEdit}
              value={form.public_url}
              onChange={(e) => setForm({ ...form, public_url: e.target.value })}
              placeholder="https://cms.example.com"
              helperText="Canonical base URL used in emails, links, and rendered templates."
            />
          </div>

          <Textarea
            label="Site Description"
            disabled={!canEdit}
            value={form.site_description}
            onChange={(e) => setForm({ ...form, site_description: e.target.value })}
            placeholder="Modern Extensible CMS Built in Go"
            rows={2}
          />

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Admin Path"
              disabled={!canEdit}
              value={form.admin_path}
              onChange={(e) => setForm({ ...form, admin_path: e.target.value })}
              placeholder="/admin"
              helperText="Relative mount prefix metadata for the Admin SPA."
            />
            <Input
              label="Default Locale"
              disabled={!canEdit}
              value={form.default_locale}
              onChange={(e) => setForm({ ...form, default_locale: e.target.value })}
              placeholder="en"
              helperText="ISO language or locale code (e.g. en, en-US, fr)."
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader
          title="Regional &amp; Date Formatting"
          description="System-wide defaults for schedules, display timestamps, and localization."
        />
        <CardContent className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Select
            label="Default Timezone"
            disabled={!canEdit}
            value={form.default_timezone}
            onChange={(e) => setForm({ ...form, default_timezone: e.target.value })}
            options={timezoneOptions.map((tz) => ({ label: tz, value: tz }))}
          />
          <Select
            label="Date Format"
            disabled={!canEdit}
            value={form.date_format}
            onChange={(e) => setForm({ ...form, date_format: e.target.value })}
            options={DATE_FORMATS}
          />
          <Select
            label="Time Format"
            disabled={!canEdit}
            value={form.time_format}
            onChange={(e) => setForm({ ...form, time_format: e.target.value })}
            options={TIME_FORMATS}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader
          title="Branding Assets"
          description="Select an organization logo and browser favicon from the Media Library."
        />
        <CardContent className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {/* Logo Picker */}
          <div className="space-y-3">
            <p className="text-xs font-semibold text-slate-700">Admin &amp; Site Logo</p>
            <div className="flex items-center gap-4 p-4 rounded-xl border border-slate-200 bg-slate-50/70">
              <div className="w-16 h-16 rounded-xl bg-white border border-slate-200 flex items-center justify-center overflow-hidden shrink-0">
                {form.logo_url ? (
                  <img src={form.logo_url} alt="Site logo preview" className="w-full h-full object-contain p-1" />
                ) : (
                  <ImageIcon className="w-6 h-6 text-slate-300" />
                )}
              </div>
              <div className="flex-1 min-w-0 space-y-2">
                <p className="text-xs text-slate-600 truncate">
                  {form.logo_url || 'Default Go CMS monogram'}
                </p>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={!canEdit}
                    onClick={() => setPickerTarget('logo')}
                  >
                    Select logo
                  </Button>
                  {form.logo_url && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={!canEdit}
                      onClick={() => setForm({ ...form, logo_url: '', logo_media_id: '' })}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                      <span>Clear</span>
                    </Button>
                  )}
                </div>
              </div>
            </div>
          </div>

          {/* Favicon Picker */}
          <div className="space-y-3">
            <p className="text-xs font-semibold text-slate-700">Favicon</p>
            <div className="flex items-center gap-4 p-4 rounded-xl border border-slate-200 bg-slate-50/70">
              <div className="w-16 h-16 rounded-xl bg-white border border-slate-200 flex items-center justify-center overflow-hidden shrink-0">
                {form.favicon_url ? (
                  <img src={form.favicon_url} alt="Favicon preview" className="w-10 h-10 object-contain" />
                ) : (
                  <ImageIcon className="w-6 h-6 text-slate-300" />
                )}
              </div>
              <div className="flex-1 min-w-0 space-y-2">
                <p className="text-xs text-slate-600 truncate">
                  {form.favicon_url || 'No custom favicon selected'}
                </p>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={!canEdit}
                    onClick={() => setPickerTarget('favicon')}
                  >
                    Select favicon
                  </Button>
                  {form.favicon_url && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={!canEdit}
                      onClick={() => setForm({ ...form, favicon_url: '', favicon_media_id: '' })}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                      <span>Clear</span>
                    </Button>
                  )}
                </div>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <MediaPickerModal
        isOpen={pickerTarget !== null}
        onClose={() => setPickerTarget(null)}
        multiple={false}
        allowedTypes={['image/*']}
        initialSelectedIds={
          pickerTarget === 'logo' && form.logo_media_id
            ? [form.logo_media_id]
            : pickerTarget === 'favicon' && form.favicon_media_id
              ? [form.favicon_media_id]
              : []
        }
        onConfirm={handleAssetConfirm}
      />
    </div>
  );
};
