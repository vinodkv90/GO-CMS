import React from 'react';
import {
  Layers,
  Image,
  Puzzle,
  Palette,
  ExternalLink,
  PlusCircle,
  Upload,
  ArrowRight,
  Database,
  Cpu,
  ShieldCheck,
  Code2,
} from 'lucide-react';
import { Card, CardHeader, CardContent } from '../components/ui/Card';
import { Badge } from '../components/ui/Badge';
import { Button } from '../components/ui/Button';
import type { ContentType, MediaAsset, PluginEntity, ThemeManifest } from '../types';

export interface DashboardViewProps {
  contentTypes: ContentType[];
  mediaAssets: MediaAsset[];
  plugins: PluginEntity[];
  themes: ThemeManifest[];
  onSelectTab: (tab: string, param?: string) => void;
}

export const DashboardView: React.FC<DashboardViewProps> = ({
  contentTypes,
  mediaAssets,
  plugins,
  themes,
  onSelectTab,
}) => {
  const managerTypes = contentTypes.filter((type) => type.kind !== 'component');
  const collectionCount = contentTypes.filter((t) => t.kind === 'collection').length;
  const singleCount = contentTypes.filter((t) => t.kind === 'single').length;
  const enabledPlugins = plugins.filter((p) => p.status === 'enabled').length;
  const activeTheme = themes.find((t) => t.is_active) || themes[0] || { name: 'Default Theme', slug: 'default' };

  // Total media size calculation
  const totalMediaBytes = mediaAssets.reduce((sum, item) => sum + (item.file_size || 0), 0);
  const formattedStorage =
    totalMediaBytes > 1024 * 1024
      ? `${(totalMediaBytes / (1024 * 1024)).toFixed(1)} MB`
      : `${(totalMediaBytes / 1024).toFixed(1)} KB`;

  return (
    <div className="space-y-8">
      {/* Welcome Banner */}
      <div className="bg-gradient-to-r from-slate-900 via-indigo-950 to-slate-900 rounded-2xl p-8 text-white relative overflow-hidden shadow-lg border border-slate-800">
        <div className="relative z-10 max-w-2xl">
          <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-sky-500/10 border border-sky-400/20 text-sky-300 text-xs font-semibold mb-4">
            <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
            <span>Go CMS Core Online</span>
          </div>
          <h1 className="text-3xl font-extrabold tracking-tight">System Overview & Metrics</h1>
          <p className="text-slate-300 text-sm mt-2 leading-relaxed">
            Welcome to the Go CMS administration console. Manage dynamic headless content types, upload media
            assets, extend platform capabilities with plugins, and view native Go <code className="text-sky-300 font-mono text-xs">html/template</code> themes.
          </p>
          <div className="flex flex-wrap items-center gap-3 mt-6">
            <Button
              variant="primary"
              size="sm"
              onClick={() => onSelectTab('content-types')}
            >
              <PlusCircle className="w-4 h-4" />
              <span>New Content Type</span>
            </Button>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => onSelectTab('media')}
            >
              <Upload className="w-4 h-4" />
              <span>Upload Media</span>
            </Button>
            <a
              href="/"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white/10 hover:bg-white/20 text-white text-xs font-semibold transition-colors border border-white/10"
            >
              <span>View Public Site</span>
              <ExternalLink className="w-3.5 h-3.5" />
            </a>
          </div>
        </div>
      </div>

      {/* Metrics Row */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
        {/* Content Types Metric */}
        <Card onClick={() => onSelectTab('content-types')}>
          <CardContent className="p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-semibold text-slate-500 uppercase tracking-wider">Content Types</p>
                <p className="text-3xl font-black text-slate-900 mt-2">{managerTypes.length}</p>
              </div>
              <div className="w-12 h-12 rounded-xl bg-sky-50 text-sky-600 flex items-center justify-center">
                <Layers className="w-6 h-6" />
              </div>
            </div>
            <div className="mt-4 pt-4 border-t border-slate-100 flex items-center justify-between text-xs text-slate-500">
              <span>{collectionCount} Collection Types</span>
              <span>{singleCount} Single Types</span>
            </div>
          </CardContent>
        </Card>

        {/* Media Assets Metric */}
        <Card onClick={() => onSelectTab('media')}>
          <CardContent className="p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-semibold text-slate-500 uppercase tracking-wider">Media Assets</p>
                <p className="text-3xl font-black text-slate-900 mt-2">{mediaAssets.length}</p>
              </div>
              <div className="w-12 h-12 rounded-xl bg-indigo-50 text-indigo-600 flex items-center justify-center">
                <Image className="w-6 h-6" />
              </div>
            </div>
            <div className="mt-4 pt-4 border-t border-slate-100 flex items-center justify-between text-xs text-slate-500">
              <span>Total Stored:</span>
              <span className="font-semibold text-slate-700">{formattedStorage}</span>
            </div>
          </CardContent>
        </Card>

        {/* Plugins Metric */}
        <Card onClick={() => onSelectTab('plugins')}>
          <CardContent className="p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-semibold text-slate-500 uppercase tracking-wider">Plugins</p>
                <p className="text-3xl font-black text-slate-900 mt-2">{plugins.length}</p>
              </div>
              <div className="w-12 h-12 rounded-xl bg-emerald-50 text-emerald-600 flex items-center justify-center">
                <Puzzle className="w-6 h-6" />
              </div>
            </div>
            <div className="mt-4 pt-4 border-t border-slate-100 flex items-center justify-between text-xs text-slate-500">
              <span className="text-emerald-600 font-semibold">{enabledPlugins} Active</span>
              <span>{plugins.length - enabledPlugins} Inactive</span>
            </div>
          </CardContent>
        </Card>

        {/* Themes Metric */}
        <Card onClick={() => onSelectTab('themes')}>
          <CardContent className="p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-semibold text-slate-500 uppercase tracking-wider">Active Theme</p>
                <p className="text-xl font-bold text-slate-900 mt-2 truncate max-w-[150px]">{activeTheme.name}</p>
              </div>
              <div className="w-12 h-12 rounded-xl bg-purple-50 text-purple-600 flex items-center justify-center">
                <Palette className="w-6 h-6" />
              </div>
            </div>
            <div className="mt-4 pt-4 border-t border-slate-100 flex items-center justify-between text-xs text-slate-500">
              <span>Engine:</span>
              <span className="font-mono font-semibold text-slate-700">html/template</span>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Main Grid: Content Types Table + Architecture Specs */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Content Types Quick Access */}
        <div className="lg:col-span-2">
          <Card>
            <CardHeader
              title="Registered Content Types"
              description="Dynamic headless schemas managed in the database"
              action={
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => onSelectTab('content-types')}
                >
                  <span>Manage All</span>
                  <ArrowRight className="w-3.5 h-3.5" />
                </Button>
              }
            />
            <div className="divide-y divide-slate-100">
              {managerTypes.length === 0 ? (
                <div className="p-8 text-center text-slate-400">
                  <Layers className="w-8 h-8 mx-auto mb-2 text-slate-300" />
                  <p className="text-sm font-medium">No content types defined yet.</p>
                  <Button
                    variant="primary"
                    size="sm"
                    className="mt-4"
                    onClick={() => onSelectTab('content-types')}
                  >
                    Build First Content Type
                  </Button>
                </div>
              ) : (
                managerTypes.map((ct) => (
                  <div
                    key={ct.uid}
                    className="p-4 flex items-center justify-between hover:bg-slate-50/80 transition-colors"
                  >
                    <div className="flex items-center gap-3">
                      <div className="w-9 h-9 rounded-lg bg-sky-50 text-sky-600 flex items-center justify-center font-bold text-xs uppercase">
                        {ct.uid.substring(0, 2)}
                      </div>
                      <div>
                        <div className="flex items-center gap-2">
                          <h4 className="text-sm font-bold text-slate-900">{ct.name}</h4>
                          <Badge variant={ct.kind}>{ct.kind}</Badge>
                        </div>
                        <p className="text-xs text-slate-400 font-mono mt-0.5">/{ct.uid}</p>
                      </div>
                    </div>

                    <div className="flex items-center gap-3">
                      <span className="text-xs text-slate-500">
                        {ct.fields?.length || 0} fields
                      </span>
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => onSelectTab('content', ct.uid)}
                      >
                        <span>Open Manager</span>
                        <ArrowRight className="w-3.5 h-3.5" />
                      </Button>
                    </div>
                  </div>
                ))
              )}
            </div>
          </Card>
        </div>

        {/* System Specs & Architecture Info */}
        <div className="space-y-6">
          <Card>
            <CardHeader
              title="Architecture Highlights"
              description="Production specifications"
            />
            <CardContent className="space-y-4 text-xs">
              <div className="flex items-start gap-3">
                <div className="p-2 rounded-lg bg-sky-50 text-sky-600 shrink-0">
                  <Database className="w-4 h-4" />
                </div>
                <div>
                  <h5 className="font-bold text-slate-800">Multi-Database Drivers</h5>
                  <p className="text-slate-500 mt-0.5">
                    Abstracted database repositories for PostgreSQL (pgx), MySQL, and MongoDB.
                  </p>
                </div>
              </div>

              <div className="flex items-start gap-3">
                <div className="p-2 rounded-lg bg-indigo-50 text-indigo-600 shrink-0">
                  <Code2 className="w-4 h-4" />
                </div>
                <div>
                  <h5 className="font-bold text-slate-800">Dual Operating Modes</h5>
                  <p className="text-slate-500 mt-0.5">
                    Headless JSON REST APIs alongside Go standard library <code className="font-mono text-slate-700">html/template</code> server rendering.
                  </p>
                </div>
              </div>

              <div className="flex items-start gap-3">
                <div className="p-2 rounded-lg bg-emerald-50 text-emerald-600 shrink-0">
                  <ShieldCheck className="w-4 h-4" />
                </div>
                <div>
                  <h5 className="font-bold text-slate-800">Security & Isolation</h5>
                  <p className="text-slate-500 mt-0.5">
                    Argon2id hashing, HMAC-SHA256 signed storage URLs, UGC Bluemonday sanitization, and path traversal defense.
                  </p>
                </div>
              </div>

              <div className="flex items-start gap-3">
                <div className="p-2 rounded-lg bg-purple-50 text-purple-600 shrink-0">
                  <Cpu className="w-4 h-4" />
                </div>
                <div>
                  <h5 className="font-bold text-slate-800">Lifecycle Hook Bus & SDK</h5>
                  <p className="text-slate-500 mt-0.5">
                    Event bus interceptors (Before/After Create/Update/Publish) and extensible Plugin SDK.
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
};
