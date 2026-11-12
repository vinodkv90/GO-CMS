import React, { useEffect, useState } from 'react';
import {
  Database,
  Globe,
  HardDrive,
  Info,
  Palette,
  Puzzle,
  Server,
  ShieldCheck,
  UserCheck,
} from 'lucide-react';
import { settingsApi } from '../../api/endpoints';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Badge } from '../../components/ui/Badge';
import { Alert } from '../../components/ui/Alert';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import type { SettingsOverview } from '../../types';

export interface SettingsOverviewViewProps {
  onNavigateSection: (section: string) => void;
}

export const SettingsOverviewView: React.FC<SettingsOverviewViewProps> = ({
  onNavigateSection,
}) => {
  const [overview, setOverview] = useState<SettingsOverview | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setIsLoading(true);
      setError(null);
      try {
        const res = await settingsApi.getOverview();
        if (!cancelled) setOverview(res.data);
      } catch (err: any) {
        if (!cancelled) setError(err?.message || 'Failed to load settings overview');
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  if (isLoading) {
    return (
      <div className="space-y-6">
        <SettingsCardSkeleton rows={4} />
        <SettingsCardSkeleton rows={4} />
      </div>
    );
  }

  if (error || !overview) {
    return (
      <Alert variant="error" title="Unable to load overview">
        {error || 'Settings overview data is currently unavailable.'}
      </Alert>
    );
  }

  const quickShortcuts = [
    {
      id: 'general',
      title: 'General Settings',
      description: 'Configure site name, public URL, admin path, timezone, locale, logo, and favicon.',
      icon: <Globe className="w-4 h-4 text-sky-600" />,
    },
    {
      id: 'profile',
      title: 'Account & Preferences',
      description: 'Update your display name, email, password, timezone, and console preferences.',
      icon: <UserCheck className="w-4 h-4 text-emerald-600" />,
    },
    {
      id: 'system',
      title: 'System Information',
      description: 'Inspect CMS runtime, database connection status, migrations, cache, and background workers.',
      icon: <Server className="w-4 h-4 text-indigo-600" />,
    },
  ];

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card>
          <CardContent className="p-4 space-y-1.5">
            <div className="flex items-center justify-between text-xs text-slate-500">
              <span>Application</span>
              <Badge variant="info">{overview.environment}</Badge>
            </div>
            <p className="text-base font-bold text-slate-900 truncate">{overview.site_name}</p>
            <p className="text-[11px] text-slate-500 font-mono">Go CMS v{overview.cms_version}</p>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4 space-y-1.5">
            <div className="flex items-center justify-between text-xs text-slate-500">
              <span className="flex items-center gap-1.5">
                <Database className="w-3.5 h-3.5" /> Database
              </span>
              <Badge variant={overview.database_status === 'healthy' ? 'success' : 'warning'}>
                {overview.database_status}
              </Badge>
            </div>
            <p className="text-base font-bold text-slate-900 capitalize">{overview.database_driver}</p>
            <p className="text-[11px] text-slate-500">Default locale: {overview.default_locale}</p>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4 space-y-1.5">
            <div className="flex items-center justify-between text-xs text-slate-500">
              <span className="flex items-center gap-1.5">
                <HardDrive className="w-3.5 h-3.5" /> Media &amp; Cache
              </span>
              <Badge variant={overview.signed_urls_active ? 'success' : 'neutral'}>
                {overview.signed_urls_active ? 'Signed URLs' : 'Direct'}
              </Badge>
            </div>
            <p className="text-base font-bold text-slate-900 uppercase">{overview.storage_provider}</p>
            <p className="text-[11px] text-slate-500">Cache driver: {overview.cache_driver}</p>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4 space-y-1.5">
            <div className="flex items-center justify-between text-xs text-slate-500">
              <span className="flex items-center gap-1.5">
                <Puzzle className="w-3.5 h-3.5" /> Extensions
              </span>
              <Badge variant={overview.jobs_enabled ? 'success' : 'neutral'}>
                Jobs {overview.jobs_enabled ? 'On' : 'Off'}
              </Badge>
            </div>
            <p className="text-base font-bold text-slate-900">
              {overview.plugins_enabled} / {overview.plugins_total} active
            </p>
            <p className="text-[11px] text-slate-500 flex items-center gap-1">
              <Palette className="w-3 h-3" /> Theme: {overview.active_theme} ({overview.themes_count})
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader
          title="Configuration Areas"
          description="Jump directly to persisted platform settings, account preferences, or live diagnostics."
        />
        <CardContent className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {quickShortcuts.map((item) => (
            <button
              key={item.id}
              type="button"
              onClick={() => onNavigateSection(item.id)}
              className="text-left p-4 rounded-xl border border-slate-200 hover:border-sky-300 hover:bg-sky-50/30 transition-all cursor-pointer space-y-2"
            >
              <div className="w-8 h-8 rounded-lg bg-slate-100 flex items-center justify-center">
                {item.icon}
              </div>
              <p className="text-sm font-semibold text-slate-900">{item.title}</p>
              <p className="text-xs text-slate-500 leading-relaxed">{item.description}</p>
            </button>
          ))}
        </CardContent>
      </Card>

      <Alert variant="info" title="Security & configuration boundary">
        <div className="flex items-center gap-1.5">
          <ShieldCheck className="w-3.5 h-3.5 shrink-0" />
          <span>
            Database passwords, JWT secrets, and network bind addresses remain read-only under System &rarr; Environment and are never returned to the browser.
          </span>
        </div>
      </Alert>

      <div className="text-[11px] text-slate-400 flex items-center gap-1.5">
        <Info className="w-3.5 h-3.5" />
        <span>Configured public URL: {overview.public_url || 'Not set'}</span>
      </div>
    </div>
  );
};

