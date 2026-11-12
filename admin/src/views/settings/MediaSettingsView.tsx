import React, { useCallback, useEffect, useState } from 'react';
import { Plus, X } from 'lucide-react';
import { mediaFoldersApi, settingsApi } from '../../api/endpoints';
import { useSettingsForm } from '../../hooks/useSettingsForm';
import { Alert } from '../../components/ui/Alert';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { Select } from '../../components/ui/Select';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import type { MediaFolder, MediaSettings } from '../../types';

export interface MediaSettingsViewProps {
  canEdit: boolean;
  onDirtyChange: (dirty: boolean) => void;
  registerHandlers: (handlers: {
    save: () => Promise<void>;
    discard: () => void;
    isSaving: boolean;
  }) => void;
}

const MIME_PRESETS = ['image/*', 'video/mp4', 'application/pdf', 'image/svg+xml', 'text/plain'];

export const MediaSettingsView: React.FC<MediaSettingsViewProps> = ({
  canEdit,
  onDirtyChange,
  registerHandlers,
}) => {
  const [folders, setFolders] = useState<MediaFolder[]>([]);
  const [mimeDraft, setMimeDraft] = useState('');

  useEffect(() => {
    mediaFoldersApi
      .list()
      .then((res) => setFolders(res.data || []))
      .catch(() => {
        // Non-fatal if folders list cannot be loaded
      });
  }, []);

  const load = useCallback(async () => {
    const res = await settingsApi.getMedia();
    return res.data;
  }, []);

  const save = useCallback(async (value: MediaSettings) => {
    const res = await settingsApi.updateMedia(value);
    return res.data;
  }, []);

  const validate = useCallback((v: MediaSettings): string | null => {
    if (v.max_file_size_bytes < 1024 || v.max_file_size_bytes > 512 * 1024 * 1024) {
      return 'Maximum upload file size must be between 1 KB and 512 MB.';
    }
    if (!v.allowed_mime_types || v.allowed_mime_types.length === 0) {
      return 'Add at least one allowed MIME type pattern.';
    }
    if (v.signed_url_ttl_seconds < 60 || v.signed_url_ttl_seconds > 604800) {
      return 'Signed URL lifetime must be between 60 seconds and 7 days (604,800 seconds).';
    }
    return null;
  }, []);

  const { form, setForm, isLoading, error } = useSettingsForm<MediaSettings>({
    load,
    save,
    validate,
    successTitle: 'Media library settings saved',
    successMessage: 'Upload limits, responsive thumbnails, and URL access rules are now enforced live.',
    loadErrorFallback: 'Failed to load media library settings.',
    saveErrorFallback: 'Failed to save media library settings.',
    onDirtyChange,
    registerHandlers,
  });

  if (isLoading || !form) {
    return <SettingsCardSkeleton />;
  }

  const maxMB = Math.round((form.max_file_size_bytes / (1024 * 1024)) * 100) / 100;

  const addMIMEPattern = (raw: string) => {
    const pattern = raw.trim().toLowerCase();
    if (!pattern) return;
    setForm((prev) => {
      if (!prev || prev.allowed_mime_types.includes(pattern)) return prev;
      return {
        ...prev,
        allowed_mime_types: [...prev.allowed_mime_types, pattern],
      };
    });
    setMimeDraft('');
  };

  const removeMIMEPattern = (pattern: string) => {
    setForm((prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        allowed_mime_types: prev.allowed_mime_types.filter((item) => item !== pattern),
      };
    });
  };

  return (
    <div className="space-y-6">
      {error && (
        <Alert variant="error" title="Unable to save media settings">
          {error}
        </Alert>
      )}

      <Card>
        <CardHeader
          title="Upload Limits & File Validation"
          description="Control maximum upload payload size, accepted MIME patterns, and alt-text enforcement for uploaded assets."
        />
        <CardContent className="space-y-5">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Maximum upload size (MB)"
              type="number"
              min={0.01}
              max={512}
              step="any"
              value={maxMB}
              disabled={!canEdit}
              helperText={`Enforced byte limit: ${form.max_file_size_bytes.toLocaleString()} bytes`}
              onChange={(e) => {
                const mb = Number(e.target.value);
                const bytes = Number.isFinite(mb) ? Math.round(mb * 1024 * 1024) : 0;
                setForm((prev) => (prev ? { ...prev, max_file_size_bytes: bytes } : prev));
              }}
            />

            <Select
              label="Default upload folder"
              value={form.default_folder_id ?? ''}
              disabled={!canEdit}
              helperText="Applied when an upload request does not specify a folder."
              options={[
                { label: 'Root library (/)', value: '' },
                ...folders.map((f) => ({ label: `/${f.path}`, value: f.id })),
              ]}
              onChange={(e) =>
                setForm((prev) =>
                  prev
                    ? { ...prev, default_folder_id: e.target.value === '' ? null : e.target.value }
                    : prev,
                )
              }
            />
          </div>

          <div>
            <label className="block text-xs font-semibold text-slate-700 mb-1.5">
              Allowed MIME Types
            </label>
            <div className="flex flex-wrap gap-1.5 mb-2">
              {form.allowed_mime_types.map((mime) => (
                <Badge key={mime} variant="info" className="gap-1.5 pr-1">
                  <span className="font-mono">{mime}</span>
                  {canEdit && (
                    <button
                      type="button"
                      onClick={() => removeMIMEPattern(mime)}
                      aria-label={`Remove ${mime}`}
                      className="text-blue-700 hover:text-blue-950 rounded p-0.5"
                    >
                      <X className="w-3 h-3" />
                    </button>
                  )}
                </Badge>
              ))}
            </div>
            {canEdit && (
              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  <Input
                    value={mimeDraft}
                    placeholder="e.g. image/* or application/pdf"
                    onChange={(e) => setMimeDraft(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        addMIMEPattern(mimeDraft);
                      }
                    }}
                  />
                  <Button
                    type="button"
                    variant="secondary"
                    size="sm"
                    onClick={() => addMIMEPattern(mimeDraft)}
                  >
                    <Plus className="w-3.5 h-3.5" /> Add MIME
                  </Button>
                </div>
                <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-slate-500">
                  <span>Quick presets:</span>
                  {MIME_PRESETS.filter((p) => !form.allowed_mime_types.includes(p)).map((preset) => (
                    <button
                      key={preset}
                      type="button"
                      onClick={() => addMIMEPattern(preset)}
                      className="px-2 py-0.5 rounded bg-slate-100 hover:bg-slate-200 text-slate-700 font-mono"
                    >
                      + {preset}
                    </button>
                  ))}
                </div>
              </div>
            )}
          </div>

          <div className="flex items-center justify-between gap-4 border-t border-slate-100 pt-4">
            <div>
              <p className="text-xs font-semibold text-slate-800">Require alternative text (alt_text)</p>
              <p className="text-[11px] text-slate-500">
                Reject asset uploads that omit accessibility alt text.
              </p>
            </div>
            <Switch
              checked={form.require_alt_text}
              disabled={!canEdit}
              onChange={(checked) =>
                setForm((prev) => (prev ? { ...prev, require_alt_text: checked } : prev))
              }
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader
          title="Responsive Thumbnails & Asset Access"
          description="Configure automatic image optimization sizes and signed URL access requirements."
        />
        <CardContent className="space-y-5">
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-xs font-semibold text-slate-800">Enable responsive thumbnail generation</p>
              <p className="text-[11px] text-slate-500">
                Automatically resize uploaded raster images into configured presets.
              </p>
            </div>
            <Switch
              checked={form.responsive_sizes_enabled}
              disabled={!canEdit}
              onChange={(checked) =>
                setForm((prev) => (prev ? { ...prev, responsive_sizes_enabled: checked } : prev))
              }
            />
          </div>

          {form.responsive_sizes_enabled && (
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 rounded-lg border border-slate-200 bg-slate-50/70 p-3.5">
              <div className="flex items-center justify-between gap-2">
                <div>
                  <p className="text-xs font-semibold text-slate-800">Small</p>
                  <p className="text-[11px] text-slate-500">250 px max width</p>
                </div>
                <Switch
                  checked={form.generate_small}
                  disabled={!canEdit}
                  onChange={(checked) =>
                    setForm((prev) => (prev ? { ...prev, generate_small: checked } : prev))
                  }
                />
              </div>
              <div className="flex items-center justify-between gap-2">
                <div>
                  <p className="text-xs font-semibold text-slate-800">Medium</p>
                  <p className="text-[11px] text-slate-500">500 px max width</p>
                </div>
                <Switch
                  checked={form.generate_medium}
                  disabled={!canEdit}
                  onChange={(checked) =>
                    setForm((prev) => (prev ? { ...prev, generate_medium: checked } : prev))
                  }
                />
              </div>
              <div className="flex items-center justify-between gap-2">
                <div>
                  <p className="text-xs font-semibold text-slate-800">Large</p>
                  <p className="text-[11px] text-slate-500">1000 px max width</p>
                </div>
                <Switch
                  checked={form.generate_large}
                  disabled={!canEdit}
                  onChange={(checked) =>
                    setForm((prev) => (prev ? { ...prev, generate_large: checked } : prev))
                  }
                />
              </div>
            </div>
          )}

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 border-t border-slate-100 pt-4">
            <Select
              label="Default asset URL access"
              value={form.default_access}
              disabled={!canEdit}
              helperText="Signed mode enforces HMAC signatures on all /uploads/* file downloads."
              options={[
                { label: 'Public URLs (Direct file serving)', value: 'public' },
                { label: 'Signed URLs (HMAC-SHA256 expiring links)', value: 'signed' },
              ]}
              onChange={(e) =>
                setForm((prev) =>
                  prev
                    ? { ...prev, default_access: e.target.value as MediaSettings['default_access'] }
                    : prev,
                )
              }
            />

            <Input
              label="Signed URL lifetime (seconds)"
              type="number"
              min={60}
              max={604800}
              value={form.signed_url_ttl_seconds}
              disabled={!canEdit}
              helperText={`Approx. ${Math.round(form.signed_url_ttl_seconds / 60)} minutes (60s – 7 days)`}
              onChange={(e) =>
                setForm((prev) =>
                  prev ? { ...prev, signed_url_ttl_seconds: Number(e.target.value) || 0 } : prev,
                )
              }
            />
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

