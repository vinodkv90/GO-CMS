import React, { useState } from 'react';
import {
  Puzzle,
  Power,
  Settings,
  Shield,
  Route,
  User,
  AlertTriangle,
  Code,
  CheckCircle2,
  XCircle,
} from 'lucide-react';
import { Card, CardHeader, CardContent } from '../components/ui/Card';
import { Badge } from '../components/ui/Badge';
import { Button } from '../components/ui/Button';
import { Modal } from '../components/ui/Modal';
import { pluginsApi } from '../api/endpoints';
import type { PluginEntity } from '../types';

export interface PluginsViewProps {
  plugins: PluginEntity[];
  onRefresh: () => void;
}

export const PluginsView: React.FC<PluginsViewProps> = ({ plugins, onRefresh }) => {
  const [selectedPlugin, setSelectedPlugin] = useState<PluginEntity | null>(null);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [settingsJson, setSettingsJson] = useState('{}');
  const [settingsError, setSettingsError] = useState<string | null>(null);
  const [isSavingSettings, setIsSavingSettings] = useState(false);
  const [actionLoadingId, setActionLoadingId] = useState<string | null>(null);

  const handleToggleStatus = async (plugin: PluginEntity) => {
    const isCurrentlyEnabled = plugin.status === 'enabled';
    setActionLoadingId(plugin.manifest.id);
    try {
      if (isCurrentlyEnabled) {
        await pluginsApi.disable(plugin.manifest.id);
      } else {
        await pluginsApi.enable(plugin.manifest.id);
      }
      onRefresh();
    } catch (err: any) {
      alert(`Action failed: ${err.message}`);
    } finally {
      setActionLoadingId(null);
    }
  };

  const handleOpenSettings = (plugin: PluginEntity) => {
    setSelectedPlugin(plugin);
    setSettingsJson(JSON.stringify(plugin.settings || {}, null, 2));
    setSettingsError(null);
    setIsSettingsOpen(true);
  };

  const handleSaveSettings = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedPlugin) return;

    try {
      const parsed = JSON.parse(settingsJson);
      setIsSavingSettings(true);
      await pluginsApi.updateSettings(selectedPlugin.manifest.id, parsed);
      setIsSettingsOpen(false);
      onRefresh();
    } catch (err: any) {
      setSettingsError(err.message || 'Invalid JSON syntax');
    } finally {
      setIsSavingSettings(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Top Header */}
      <div>
        <h1 className="text-2xl font-bold text-slate-900 tracking-tight">Plugin Ecosystem & SDK</h1>
        <p className="text-xs text-slate-500 mt-1">
          Extend core capabilities with sandboxed plugins, dynamic subrouter endpoints, and lifecycle event hooks.
        </p>
      </div>

      {/* Plugins Grid */}
      {plugins.length === 0 ? (
        <Card>
          <CardContent className="p-12 text-center">
            <Puzzle className="w-12 h-12 text-slate-300 mx-auto mb-3" />
            <h3 className="text-base font-bold text-slate-800">No plugins discovered</h3>
            <p className="text-xs text-slate-500 mt-1 max-w-sm mx-auto">
              Place plugins inside the <code className="font-mono text-slate-700">plugins/</code> directory with a valid{' '}
              <code className="font-mono text-slate-700">plugin.json</code> manifest.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {plugins.map((plugin) => {
            const isEnabled = plugin.status === 'enabled';
            const isLoading = actionLoadingId === plugin.manifest.id;

            return (
              <Card key={plugin.manifest.id} className="flex flex-col justify-between border-slate-200">
                <CardHeader
                  title={
                    <div className="flex items-center gap-2.5">
                      <div className="w-8 h-8 rounded-lg bg-sky-50 text-sky-600 flex items-center justify-center font-bold">
                        <Puzzle className="w-4 h-4" />
                      </div>
                      <div>
                        <div className="flex items-center gap-2">
                          <h3 className="text-base font-bold text-slate-900">{plugin.manifest.name}</h3>
                          <Badge variant={isEnabled ? 'success' : 'default'}>
                            {plugin.status}
                          </Badge>
                        </div>
                        <p className="text-[11px] font-mono text-slate-400 mt-0.5">
                          id: {plugin.manifest.id} • v{plugin.manifest.version}
                        </p>
                      </div>
                    </div>
                  }
                  action={
                    <Button
                      variant={isEnabled ? 'destructive' : 'primary'}
                      size="sm"
                      onClick={() => handleToggleStatus(plugin)}
                      isLoading={isLoading}
                    >
                      <Power className="w-3.5 h-3.5" />
                      <span>{isEnabled ? 'Disable' : 'Enable'}</span>
                    </Button>
                  }
                />
                <CardContent className="space-y-4">
                  <p className="text-xs text-slate-600 leading-relaxed">
                    {plugin.manifest.description || 'No description provided.'}
                  </p>

                  <div className="space-y-2 pt-2 border-t border-slate-100 text-xs">
                    <div className="flex items-center gap-2 text-slate-500">
                      <User className="w-3.5 h-3.5" />
                      <span>Author: {plugin.manifest.author || 'Internal'}</span>
                      <span className="text-slate-300">•</span>
                      <Code className="w-3.5 h-3.5" />
                      <span className="font-mono">Runtime: {plugin.manifest.runtime}</span>
                    </div>

                    {/* Permissions */}
                    {plugin.manifest.permissions && plugin.manifest.permissions.length > 0 && (
                      <div className="flex items-center gap-1.5 flex-wrap">
                        <Shield className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                        <span className="text-[11px] text-slate-500 font-semibold">Permissions:</span>
                        {plugin.manifest.permissions.map((perm) => (
                          <span
                            key={perm}
                            className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-slate-100 text-slate-600 border border-slate-200"
                          >
                            {perm}
                          </span>
                        ))}
                      </div>
                    )}

                    {/* Subroutes */}
                    {plugin.manifest.routes && plugin.manifest.routes.length > 0 && (
                      <div className="flex items-center gap-1.5 flex-wrap">
                        <Route className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                        <span className="text-[11px] text-slate-500 font-semibold">Routes:</span>
                        {plugin.manifest.routes.map((rt) => (
                          <span
                            key={`${rt.method}-${rt.path}`}
                            className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-sky-50 text-sky-700 border border-sky-100"
                          >
                            {rt.method} /api/v1/plugins/{plugin.manifest.id}{rt.path}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                </CardContent>

                {/* Footer Controls */}
                <div className="p-4 bg-slate-50 border-t border-slate-100 flex items-center justify-between">
                  <div className="flex items-center gap-1 text-[11px] text-slate-500">
                    {isEnabled ? (
                      <>
                        <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600" />
                        <span>Mounted & Intercepting Hooks</span>
                      </>
                    ) : (
                      <>
                        <XCircle className="w-3.5 h-3.5 text-slate-400" />
                        <span>Dormant</span>
                      </>
                    )}
                  </div>

                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handleOpenSettings(plugin)}
                  >
                    <Settings className="w-3.5 h-3.5" />
                    <span>Configuration</span>
                  </Button>
                </div>
              </Card>
            );
          })}
        </div>
      )}

      {/* Modal: Plugin Settings */}
      <Modal
        isOpen={isSettingsOpen}
        onClose={() => setIsSettingsOpen(false)}
        title={`Configure Plugin "${selectedPlugin?.manifest.name}"`}
        description="Edit runtime JSON configuration parameters passed to plugin initialization"
        maxWidth="md"
      >
        <form onSubmit={handleSaveSettings} className="space-y-4">
          {settingsError && (
            <div className="p-3 rounded-lg bg-rose-50 border border-rose-200 flex items-center gap-2 text-xs text-rose-700">
              <AlertTriangle className="w-4 h-4 shrink-0" />
              <span>{settingsError}</span>
            </div>
          )}

          <div>
            <label className="block text-xs font-semibold text-slate-700 tracking-wide uppercase mb-1.5">
              Configuration JSON
            </label>
            <textarea
              rows={8}
              value={settingsJson}
              onChange={(e) => setSettingsJson(e.target.value)}
              className="w-full px-3 py-2 bg-slate-900 text-slate-100 font-mono text-xs rounded-xl border border-slate-700 focus:outline-none focus:ring-2 focus:ring-sky-500"
            />
          </div>

          <div className="flex justify-end gap-2.5 pt-3 border-t border-slate-100">
            <Button
              type="button"
              variant="outline"
              onClick={() => setIsSettingsOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant="primary"
              isLoading={isSavingSettings}
            >
              Save Configuration
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
};

