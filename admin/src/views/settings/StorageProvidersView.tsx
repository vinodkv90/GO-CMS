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
import type { StorageSettings, StorageTestResult } from '../../types';

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

  const capabilities = form.supported_providers || [
    { id: 'local', label: 'Local Filesystem', implemented: true },
    { id: 's3', label: 'Amazon S3', implemented: true },
    { id: 'minio', label: 'MinIO / S3-Compatible', implemented: true },
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

          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            {capabilities.map((cap) => {
              const selected = form.active_provider === cap.id;
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
                    {!cap.implemented ? (
                      <Badge variant="neutral">Not available</Badge>
                    ) : selected ? (
                      <Badge variant="success">Active</Badge>
                    ) : null}
                  </div>
                  <p className="text-[11px] text-slate-500">
                    {!cap.implemented
                      ? cap.reason || 'Storage driver is not installed.'
                      : cap.id === 'local'
                        ? 'Persist uploaded media files to local disk.'
                        : cap.id === 's3'
                          ? 'AWS S3 object storage via SigV4 HTTPS requests.'
                          : 'Self-hosted S3-compatible bucket storage.'}
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
