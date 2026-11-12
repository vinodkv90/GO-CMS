import React, { useCallback, useState } from 'react';
import { CheckCircle2, Cloud, HardDrive, Loader2, XCircle } from 'lucide-react';
import { settingsApi } from '../../api/endpoints';
import { useSettingsForm } from '../../hooks/useSettingsForm';
import { Alert } from '../../components/ui/Alert';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { SecretInput } from '../../components/ui/SecretInput';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import type { CloudinaryStorageSettings, StorageSettings, StorageTestResult } from '../../types';

const DEFAULT_CLOUDINARY: CloudinaryStorageSettings = {
  cloud_name: '',
  api_key: '',
  api_secret: '',
  secret_configured: false,
  root_folder: '',
  delivery_type: 'upload',
  default_resource_type: 'auto',
  use_secure_url: true,
  upload_preset: '',
  asset_naming_strategy: 'cms_key',
  optimization_enabled: true,
  default_format: 'auto',
  default_quality: 'auto',
  max_width: 0,
};

const isProviderConfigured = (id: string, form: StorageSettings): boolean => {
  if (id === 'local') {
    return Boolean(form.local.upload_dir?.trim());
  }
  if (id === 's3' || id === 'minio') {
    const hasBucket = Boolean(form.s3.bucket?.trim() && form.s3.access_key_id?.trim());
    const hasSecret = Boolean(form.s3.secret_configured || form.s3.secret_access_key?.trim());
    if (id === 'minio') {
      return hasBucket && hasSecret && Boolean(form.s3.endpoint?.trim());
    }
    return hasBucket && hasSecret;
  }
  if (id === 'cloudinary') {
    const c = form.cloudinary;
    return Boolean(
      c?.cloud_name?.trim() &&
        c?.api_key?.trim() &&
        (c?.secret_configured || c?.api_secret?.trim()),
    );
  }
  return false;
};

export interface StorageProvidersViewProps {
  canEdit: boolean;
  onDirtyChange: (dirty: boolean) => void;
  registerHandlers: (handlers: {
    save: () => Promise<void>;
    discard: () => void;
    isSaving: boolean;
  }) => void;
}

export const StorageProvidersView: React.FC<StorageProvidersViewProps> = ({
  canEdit,
  onDirtyChange,
  registerHandlers,
}) => {
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<StorageTestResult | null>(null);
  const [testError, setTestError] = useState<string | null>(null);

  const load = useCallback(async () => {
    const res = await settingsApi.getStorage();
    return res.data;
  }, []);

  const save = useCallback(async (value: StorageSettings) => {
    const res = await settingsApi.updateStorage(value);
    return res.data;
  }, []);

  const validate = useCallback((v: StorageSettings): string | null => {
    if (v.active_provider === 'local') {
      if (!v.local.upload_dir.trim()) return 'Local upload directory is required.';
      if (!v.local.url_prefix.startsWith('/')) return 'URL prefix must start with "/".';
      return null;
    }
    if (v.active_provider === 'cloudinary') {
      const c = { ...DEFAULT_CLOUDINARY, ...v.cloudinary };
      if (!c.cloud_name.trim()) return 'Cloudinary Cloud Name is required.';
      if (!/^[a-zA-Z0-9_-]{1,64}$/.test(c.cloud_name.trim())) {
        return 'Cloud Name may only contain letters, numbers, hyphens, and underscores.';
      }
      if (!c.api_key.trim()) return 'Cloudinary API Key is required.';
      if (!c.api_secret && !c.secret_configured) {
        return 'Cloudinary API Secret is required.';
      }
      if (c.root_folder.includes('..') || c.root_folder.includes('\\')) {
        return 'Root folder cannot contain ".." or backslashes.';
      }
      if (c.max_width && (c.max_width < 0 || c.max_width > 10000)) {
        return 'Max width must be between 0 and 10000 px.';
      }
      return null;
    }
    if (!v.s3.bucket.trim()) return 'S3 bucket name is required.';
    if (!v.s3.access_key_id.trim()) return 'Access key ID is required.';
    if (!v.s3.secret_access_key && !v.s3.secret_configured) {
      return 'Secret access key is required.';
    }
    if (v.active_provider === 'minio' && !v.s3.endpoint.trim()) {
      return 'Endpoint URL is required for MinIO.';
    }
    return null;
  }, []);

  const { form, setForm, isLoading, error } = useSettingsForm<StorageSettings>({
    load,
    save,
    validate,
    successTitle: 'Storage provider settings saved',
    successMessage: 'Media storage backend configuration updated and hot-swapped.',
    loadErrorFallback: 'Failed to load storage provider settings.',
    saveErrorFallback: 'Failed to save storage provider settings.',
    onDirtyChange,
    registerHandlers,
  });

  const handleTestConnection = async () => {
    if (!form) return;
    const validationErr = validate(form);
    if (validationErr) {
      setTestResult(null);
      setTestError(validationErr);
      return;
    }
    setTesting(true);
    setTestResult(null);
    setTestError(null);
    try {
      const res = await settingsApi.testStorage(form);
      setTestResult(res.data);
    } catch (err: any) {
      setTestError(err?.message || 'Storage connection test failed.');
    } finally {
      setTesting(false);
    }
  };

  if (isLoading || !form) {
    return <SettingsCardSkeleton />;
  }

  const cloud: CloudinaryStorageSettings = {
    ...DEFAULT_CLOUDINARY,
    ...(form.cloudinary || {}),
  };

  const updateCloud = (patch: Partial<CloudinaryStorageSettings>) => {
    setForm((prev) =>
      prev
        ? {
            ...prev,
            cloudinary: {
              ...DEFAULT_CLOUDINARY,
              ...(prev.cloudinary || {}),
              ...patch,
            },
          }
        : prev,
    );
  };

  const capabilities = form.supported_providers || [
    { id: 'local', label: 'Local Filesystem', implemented: true },
    { id: 's3', label: 'Amazon S3', implemented: true },
    { id: 'minio', label: 'MinIO / S3-Compatible', implemented: true },
    { id: 'cloudinary', label: 'Cloudinary', implemented: true },
    {
      id: 'gcs',
      label: 'Google Cloud Storage',
      implemented: false,
      reason: 'Google Cloud Storage driver is not included in this build.',
    },
    {
      id: 'azure',
      label: 'Azure Blob Storage',
      implemented: false,
      reason: 'Azure Blob Storage driver is not included in this build.',
    },
  ];

  const isS3Family = form.active_provider === 's3' || form.active_provider === 'minio';

  return (
    <div className="space-y-6">
      {error && (
        <Alert variant="error" title="Unable to save storage settings">
          {error}
        </Alert>
      )}

      <Card>
        <CardHeader
          title="Active Storage Driver"
          description="Select where uploaded media assets are persisted. Saving hot-swaps the active storage provider without restarting."
          action={
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={testing}
              onClick={() => void handleTestConnection()}
            >
              {testing ? (
                <>
                  <Loader2 className="w-3.5 h-3.5 animate-spin" /> Testing...
                </>
              ) : (
                'Test Connection'
              )}
            </Button>
          }
        />
        <CardContent className="space-y-4">
          {testError && (
            <Alert variant="error" title="Connection check failed">
              {testError}
            </Alert>
          )}
          {testResult && (
            <Alert
              variant={testResult.ok ? 'success' : 'error'}
              title={testResult.ok ? 'Connection verified' : 'Connection check failed'}
            >
              <div className="flex items-center gap-2 text-xs">
                {testResult.ok ? (
                  <CheckCircle2 className="w-4 h-4 shrink-0" />
                ) : (
                  <XCircle className="w-4 h-4 shrink-0" />
                )}
                <span>
                  {testResult.message} (provider: <code>{testResult.provider}</code>, latency:{' '}
                  {testResult.latency_ms}ms)
                </span>
              </div>
            </Alert>
          )}

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {capabilities.map((cap) => {
              const selected = form.active_provider === cap.id;
              const configured = cap.implemented && isProviderConfigured(cap.id, form);
              return (
                <button
                  key={cap.id}
                  type="button"
                  disabled={!cap.implemented || !canEdit}
                  onClick={() => {
                    if (!cap.implemented) return;
                    setTestResult(null);
                    setTestError(null);
                    setForm((prev) =>
                      prev
                        ? {
                            ...prev,
                            active_provider: cap.id as StorageSettings['active_provider'],
                            s3: {
                              ...prev.s3,
                              force_path_style: cap.id === 'minio' ? true : prev.s3.force_path_style,
                            },
                            cloudinary: prev.cloudinary || { ...DEFAULT_CLOUDINARY },
                          }
                        : prev,
                    );
                  }}
                  className={`text-left rounded-lg border p-3.5 transition-all ${
                    !cap.implemented
                      ? 'border-slate-200 bg-slate-50 opacity-65 cursor-not-allowed'
                      : selected
                        ? 'border-indigo-600 bg-indigo-50/50 ring-1 ring-indigo-600'
                        : 'border-slate-200 bg-white hover:border-slate-300'
                  }`}
                >
                  <div className="flex items-center justify-between gap-2 mb-1">
                    <span className="inline-flex items-center gap-1.5 text-xs font-semibold text-slate-900">
                      {cap.id === 'local' ? (
                        <HardDrive className="w-3.5 h-3.5 text-slate-600" />
                      ) : (
                        <Cloud className="w-3.5 h-3.5 text-indigo-600" />
                      )}
                      {cap.label}
                    </span>
                    <div className="flex items-center gap-1.5">
                      {!cap.implemented ? (
                        <Badge variant="neutral">Not available</Badge>
                      ) : (
                        <>
                          {selected && <Badge variant="success">Active</Badge>}
                          {!selected && (
                            <Badge variant={configured ? 'info' : 'neutral'}>
                              {configured ? 'Configured' : 'Not configured'}
                            </Badge>
                          )}
                        </>
                      )}
                    </div>
                  </div>
                  <p className="text-[11px] text-slate-500">
                    {!cap.implemented
                      ? cap.reason || 'Storage driver is not installed.'
                      : cap.id === 'local'
                        ? 'Local disk storage'
                        : cap.id === 's3'
                          ? 'AWS object storage via SigV4 HTTPS requests.'
                          : cap.id === 'cloudinary'
                            ? 'Cloud media platform with CDN delivery & optimization.'
                            : 'S3-compatible bucket storage.'}
                  </p>
                </button>
              );
            })}
          </div>
        </CardContent>
      </Card>

      {form.active_provider === 'local' ? (
        <Card>
          <CardHeader
            title="Local Filesystem Options"
            description="Directory path and public URL route used by the local media storage provider."
          />
          <CardContent className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Uploads directory"
              value={form.local.upload_dir}
              disabled={!canEdit}
              placeholder="./uploads"
              onChange={(e) =>
                setForm((prev) =>
                  prev ? { ...prev, local: { ...prev.local, upload_dir: e.target.value } } : prev,
                )
              }
            />
            <Input
              label="URL prefix"
              value={form.local.url_prefix}
              disabled={!canEdit}
              placeholder="/uploads"
              onChange={(e) =>
                setForm((prev) =>
                  prev ? { ...prev, local: { ...prev.local, url_prefix: e.target.value } } : prev,
                )
              }
            />
          </CardContent>
        </Card>
      ) : form.active_provider === 'cloudinary' ? (
        <Card>
          <CardHeader
            title="Cloudinary Configuration"
            description="Credentials are encrypted with AES-256-GCM at rest. Supports automatic CDN URLs, folder mapping, and responsive image transforms."
          />
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Input
                label="Cloud Name *"
                value={cloud.cloud_name}
                disabled={!canEdit}
                placeholder="your-cloud-name"
                onChange={(e) => updateCloud({ cloud_name: e.target.value })}
              />
              <Input
                label="API Key *"
                autoComplete="off"
                value={cloud.api_key}
                disabled={!canEdit}
                placeholder="Cloudinary API Key"
                onChange={(e) => updateCloud({ api_key: e.target.value })}
              />
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <SecretInput
                label="API Secret *"
                value={cloud.api_secret || ''}
                hasStoredSecret={cloud.secret_configured}
                disabled={!canEdit}
                onChange={(val) => updateCloud({ api_secret: val })}
              />
              <Input
                label="Root Folder"
                value={cloud.root_folder}
                disabled={!canEdit}
                placeholder="go-cms"
                onChange={(e) => updateCloud({ root_folder: e.target.value })}
              />
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1.5">
                  Delivery Type
                </label>
                <select
                  value={cloud.delivery_type}
                  disabled={!canEdit}
                  onChange={(e) =>
                    updateCloud({
                      delivery_type: e.target.value as CloudinaryStorageSettings['delivery_type'],
                    })
                  }
                  className="w-full px-3 py-2 text-xs bg-white border border-slate-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 disabled:bg-slate-50"
                >
                  <option value="upload">Public (upload)</option>
                  <option value="authenticated">Authenticated</option>
                  <option value="private">Private</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1.5">
                  Default Resource Type
                </label>
                <select
                  value={cloud.default_resource_type}
                  disabled={!canEdit}
                  onChange={(e) =>
                    updateCloud({
                      default_resource_type: e.target
                        .value as CloudinaryStorageSettings['default_resource_type'],
                    })
                  }
                  className="w-full px-3 py-2 text-xs bg-white border border-slate-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 disabled:bg-slate-50"
                >
                  <option value="auto">Auto (recommended)</option>
                  <option value="image">Image</option>
                  <option value="video">Video</option>
                  <option value="raw">Raw</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1.5">
                  Asset Naming Strategy
                </label>
                <select
                  value={cloud.asset_naming_strategy || 'cms_key'}
                  disabled={!canEdit}
                  onChange={(e) =>
                    updateCloud({
                      asset_naming_strategy: e.target
                        .value as CloudinaryStorageSettings['asset_naming_strategy'],
                    })
                  }
                  className="w-full px-3 py-2 text-xs bg-white border border-slate-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 disabled:bg-slate-50"
                >
                  <option value="cms_key">CMS Storage Key (deterministic)</option>
                  <option value="original_filename">Original Filename + Suffix</option>
                  <option value="random_id">Cloudinary Random Public ID</option>
                </select>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 border-t border-slate-100 pt-3">
              <Input
                label="Upload Preset (optional)"
                value={cloud.upload_preset || ''}
                disabled={!canEdit}
                placeholder="cms_media_preset"
                onChange={(e) => updateCloud({ upload_preset: e.target.value })}
              />
              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1.5">
                  Default Delivery Format
                </label>
                <select
                  value={cloud.default_format || 'auto'}
                  disabled={!canEdit}
                  onChange={(e) => updateCloud({ default_format: e.target.value })}
                  className="w-full px-3 py-2 text-xs bg-white border border-slate-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 disabled:bg-slate-50"
                >
                  <option value="auto">Auto (f_auto)</option>
                  <option value="webp">WebP</option>
                  <option value="avif">AVIF</option>
                  <option value="jpg">JPEG</option>
                  <option value="png">PNG</option>
                  <option value="">Original format</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1.5">
                  Default Quality
                </label>
                <select
                  value={cloud.default_quality || 'auto'}
                  disabled={!canEdit}
                  onChange={(e) => updateCloud({ default_quality: e.target.value })}
                  className="w-full px-3 py-2 text-xs bg-white border border-slate-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500 disabled:bg-slate-50"
                >
                  <option value="auto">Auto (q_auto)</option>
                  <option value="auto:best">Auto Best</option>
                  <option value="auto:good">Auto Good</option>
                  <option value="auto:eco">Auto Eco</option>
                  <option value="auto:low">Auto Low</option>
                  <option value="80">80%</option>
                  <option value="">Uncompressed</option>
                </select>
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-6 border-t border-slate-100 pt-3">
              <div className="flex items-center gap-3">
                <Switch
                  checked={cloud.use_secure_url}
                  disabled={!canEdit}
                  onChange={(checked) => updateCloud({ use_secure_url: checked })}
                />
                <span className="text-xs font-medium text-slate-700">Use Secure HTTPS URLs</span>
              </div>
              <div className="flex items-center gap-3">
                <Switch
                  checked={cloud.optimization_enabled}
                  disabled={!canEdit}
                  onChange={(checked) => updateCloud({ optimization_enabled: checked })}
                />
                <span className="text-xs font-medium text-slate-700">
                  Enable automatic image optimization (f_auto, q_auto)
                </span>
              </div>
            </div>
          </CardContent>
        </Card>
      ) : (
        isS3Family && (
          <Card>
            <CardHeader
              title={form.active_provider === 'minio' ? 'MinIO / Custom S3 Configuration' : 'Amazon S3 Configuration'}
              description="Credentials are encrypted with AES-256-GCM at rest and verified via AWS SigV4 requests."
            />
            <CardContent className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Input
                  label="Bucket name"
                  value={form.s3.bucket}
                  disabled={!canEdit}
                  placeholder="my-cms-media"
                  onChange={(e) =>
                    setForm((prev) =>
                      prev ? { ...prev, s3: { ...prev.s3, bucket: e.target.value } } : prev,
                    )
                  }
                />
                <Input
                  label="Region"
                  value={form.s3.region}
                  disabled={!canEdit}
                  placeholder="us-east-1"
                  onChange={(e) =>
                    setForm((prev) =>
                      prev ? { ...prev, s3: { ...prev.s3, region: e.target.value } } : prev,
                    )
                  }
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Input
                  label={form.active_provider === 'minio' ? 'MinIO Endpoint URL' : 'Custom Endpoint URL (optional)'}
                  value={form.s3.endpoint}
                  disabled={!canEdit}
                  placeholder={form.active_provider === 'minio' ? 'http://localhost:9000' : 'https://s3.us-east-1.amazonaws.com'}
                  onChange={(e) =>
                    setForm((prev) =>
                      prev ? { ...prev, s3: { ...prev.s3, endpoint: e.target.value } } : prev,
                    )
                  }
                />
                <Input
                  label="Public CDN / Base URL (optional)"
                  value={form.s3.base_url}
                  disabled={!canEdit}
                  placeholder="https://cdn.example.com"
                  onChange={(e) =>
                    setForm((prev) =>
                      prev ? { ...prev, s3: { ...prev.s3, base_url: e.target.value } } : prev,
                    )
                  }
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Input
                  label="Access Key ID"
                  autoComplete="off"
                  value={form.s3.access_key_id}
                  disabled={!canEdit}
                  onChange={(e) =>
                    setForm((prev) =>
                      prev ? { ...prev, s3: { ...prev.s3, access_key_id: e.target.value } } : prev,
                    )
                  }
                />
                <SecretInput
                  label="Secret Access Key"
                  value={form.s3.secret_access_key || ''}
                  hasStoredSecret={form.s3.secret_configured}
                  disabled={!canEdit}
                  onChange={(val) =>
                    setForm((prev) =>
                      prev ? { ...prev, s3: { ...prev.s3, secret_access_key: val } } : prev,
                    )
                  }
                />
              </div>

              <div className="flex flex-wrap items-center gap-6 border-t border-slate-100 pt-3">
                <div className="flex items-center gap-3">
                  <Switch
                    checked={form.s3.use_ssl}
                    disabled={!canEdit}
                    onChange={(checked) =>
                      setForm((prev) =>
                        prev ? { ...prev, s3: { ...prev.s3, use_ssl: checked } } : prev,
                      )
                    }
                  />
                  <span className="text-xs font-medium text-slate-700">Use TLS (HTTPS)</span>
                </div>
                <div className="flex items-center gap-3">
                  <Switch
                    checked={form.s3.force_path_style}
                    disabled={!canEdit}
                    onChange={(checked) =>
                      setForm((prev) =>
                        prev ? { ...prev, s3: { ...prev.s3, force_path_style: checked } } : prev,
                      )
                    }
                  />
                  <span className="text-xs font-medium text-slate-700">Force path-style addressing</span>
                </div>
              </div>
            </CardContent>
          </Card>
        )
      )}
    </div>
  );
};
