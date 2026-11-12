import React, { useCallback, useEffect, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { settingsApi } from '../../api/endpoints';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Alert } from '../../components/ui/Alert';
import { DataTable, type DataTableColumn } from '../../components/ui/DataTable';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import type { MigrationItem, SystemInfo } from '../../types';

export type SystemSubSection =
  | 'system'
  | 'system-environment'
  | 'system-database'
  | 'system-cache'
  | 'system-jobs';

export interface SystemSettingsViewProps {
  subsection: SystemSubSection;
}

const KeyValueRow: React.FC<{ label: string; value: React.ReactNode }> = ({ label, value }) => (
  <div className="flex items-center justify-between gap-4 py-2.5 border-b border-slate-100 last:border-b-0 text-xs">
    <span className="font-medium text-slate-500">{label}</span>
    <span className="font-semibold text-slate-900 font-mono text-right break-all">{value}</span>
  </div>
);

export const SystemSettingsView: React.FC<SystemSettingsViewProps> = ({ subsection }) => {
  const [info, setInfo] = useState<SystemInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await settingsApi.getSystemInfo();
      setInfo(res.data);
    } catch (err: any) {
      setError(err?.message || 'Failed to load system diagnostics');
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  if (isLoading && !info) {
    return (
      <div className="space-y-6">
        <SettingsCardSkeleton rows={4} />
        <SettingsCardSkeleton rows={4} />
      </div>
    );
  }

  if (error || !info) {
    return (
      <Alert variant="error" title="System diagnostics unavailable">
        {error || 'No system information returned.'}
      </Alert>
    );
  }

  const migrationColumns: DataTableColumn<MigrationItem>[] = [
    {
      key: 'version',
      header: 'Version',
      render: (m) => <span className="font-mono text-xs font-semibold text-slate-900">{m.version}</span>,
    },
    {
      key: 'name',
      header: 'Migration Name',
      render: (m) => <span className="text-xs text-slate-700">{m.name}</span>,
    },
    {
      key: 'status',
      header: 'Status',
      render: (m) => (
        <Badge variant={m.applied ? 'success' : 'warning'}>
          {m.applied ? 'Applied' : 'Pending'}
        </Badge>
      ),
    },
    {
      key: 'applied_at',
      header: 'Applied At',
      render: (m) => (
        <span className="text-xs font-mono text-slate-500">
          {m.applied_at ? new Date(m.applied_at).toLocaleString() : '—'}
        </span>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3">
        <Alert variant="info" className="flex-1">
          Deployment-managed settings (server ports, database credentials, and cryptographic keys) are strictly read-only in the UI.
        </Alert>
        <Button type="button" variant="outline" size="sm" isLoading={isLoading} onClick={() => void load()}>
          <RefreshCw className="w-3.5 h-3.5" />
          <span>Refresh</span>
        </Button>
      </div>

      {subsection === 'system' && (
        <>
          <Card>
            <CardHeader
              title="Application Runtime"
              description="Live Go runtime and server instance metadata."
              action={<Badge variant="info">{info.application.environment}</Badge>}
            />
            <CardContent>
              <KeyValueRow label="CMS Name" value={info.application.name} />
              <KeyValueRow label="CMS Version" value={info.application.version} />
              <KeyValueRow label="Go Runtime" value={info.application.go_version} />
              <KeyValueRow label="Target Platform" value={`${info.application.os} / ${info.application.arch}`} />
              <KeyValueRow label="Bind Address" value={`${info.application.host}:${info.application.port}`} />
              <KeyValueRow label="Active Goroutines" value={info.application.goroutines} />
              <KeyValueRow label="Uptime (seconds)" value={`${info.application.uptime_seconds}s`} />
            </CardContent>
          </Card>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <Card>
              <CardHeader title="Themes" />
              <CardContent>
                <KeyValueRow label="Active Theme" value={info.themes.active_theme} />
                <KeyValueRow label="Themes Directory" value={info.themes.themes_dir} />
                <KeyValueRow label="Template Cache" value={info.themes.cache ? 'enabled' : 'disabled'} />
                <KeyValueRow
                  label="Discovered Themes"
                  value={(info.themes.installed || []).join(', ') || 'None'}
                />
              </CardContent>
            </Card>

            <Card>
              <CardHeader title="Plugins &amp; Media Storage" />
              <CardContent>
                <KeyValueRow label="Plugins Directory" value={info.plugins.plugins_dir} />
                <KeyValueRow label="Enabled / Total" value={`${info.plugins.enabled} / ${info.plugins.total}`} />
                <KeyValueRow label="Storage Provider" value={info.storage.provider} />
                <KeyValueRow label="Signed URLs" value={info.storage.signed_urls_enabled ? 'enabled' : 'disabled'} />
              </CardContent>
            </Card>
          </div>
        </>
      )}

      {subsection === 'system-environment' && (
        <Card>
          <CardHeader
            title="Environment &amp; Deployment Config"
            description="Read-only server boot configuration from config.yaml and environment variables."
            action={<Badge variant="neutral">Read-only</Badge>}
          />
          <CardContent>
            <KeyValueRow label="Environment" value={info.application.environment} />
            <KeyValueRow label="Logging Level" value={info.environment_config.logging_level} />
            <KeyValueRow label="Logging Format" value={info.environment_config.logging_format} />
            <KeyValueRow label="JWT Token Expiration" value={info.environment_config.token_expiration} />
            <KeyValueRow label="Configured CORS Origins" value={info.environment_config.cors_origins_count} />
            <KeyValueRow
              label="Trusted Proxies"
              value={(info.environment_config.trusted_proxies || []).join(', ') || 'None'}
            />
          </CardContent>
        </Card>
      )}

      {subsection === 'system-database' && (
        <>
          <Card>
            <CardHeader
              title="Database Connection"
              description={info.database.connection_note}
              action={
                <Badge variant={info.database.status === 'healthy' ? 'success' : 'error'}>
                  {info.database.status}
                </Badge>
              }
            />
            <CardContent>
              <KeyValueRow label="Driver" value={info.database.driver} />
              <KeyValueRow label="Host &amp; Port" value={`${info.database.host}:${info.database.port}`} />
              <KeyValueRow label="Database Name" value={info.database.database} />
              <KeyValueRow label="SSL Mode" value={info.database.ssl_mode} />
              <KeyValueRow
                label="Pool Limits (Open / Idle)"
                value={`${info.database.max_open_conns} / ${info.database.max_idle_conns}`}
              />
              <KeyValueRow
                label="Migrations (Applied / Pending)"
                value={`${info.database.applied_migrations} / ${info.database.pending_migrations}`}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader
              title="Schema Migrations"
              description="Tracked migration history reported directly from the database runner."
            />
            <DataTable
              columns={migrationColumns}
              rows={info.database.migrations || []}
              rowKey={(m) => m.version}
            />
          </Card>
        </>
      )}

      {subsection === 'system-cache' && (
        <Card>
          <CardHeader
            title="Cache Backend"
            description="In-memory or Redis content cache configuration."
            action={<Badge variant="success">{info.cache.status}</Badge>}
          />
          <CardContent>
            <KeyValueRow label="Driver" value={info.cache.driver} />
            <KeyValueRow label="Default TTL" value={info.cache.default_ttl} />
            <KeyValueRow label="Key Prefix" value={info.cache.prefix} />
          </CardContent>
        </Card>
      )}

      {subsection === 'system-jobs' && (
        <Card>
          <CardHeader
            title="Background Job Queue"
            description="Worker pool concurrency and in-memory queue telemetry."
            action={
              <Badge variant={info.jobs.enabled ? 'success' : 'neutral'}>
                {info.jobs.enabled ? 'Running' : 'Disabled'}
              </Badge>
            }
          />
          <CardContent>
            <KeyValueRow label="Workers" value={info.jobs.concurrency} />
            <KeyValueRow label="Max Retries" value={info.jobs.max_retries} />
            <KeyValueRow label="Poll Interval" value={info.jobs.poll_interval} />
            {info.jobs.queue_stats &&
              Object.entries(info.jobs.queue_stats).map(([k, v]) => (
                <KeyValueRow key={k} label={`Queue: ${k}`} value={v} />
              ))}
          </CardContent>
        </Card>
      )}
    </div>
  );
};

