import React, { useEffect, useState } from 'react';
import { Power, PowerOff, Puzzle, Settings as SettingsIcon } from 'lucide-react';
import { pluginsApi } from '../../api/endpoints';
import { Alert } from '../../components/ui/Alert';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { Modal } from '../../components/ui/Modal';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import { useToast } from '../../components/ui/Toast';
import type { PluginEntity } from '../../types';

export interface PluginSettingsViewProps {
  canEdit: boolean;
}

export const PluginSettingsView: React.FC<PluginSettingsViewProps> = ({ canEdit }) => {
  const toast = useToast();
  const [plugins, setPlugins] = useState<PluginEntity[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  // Settings modal state
  const [configuring, setConfiguring] = useState<PluginEntity | null>(null);
  const [draftSettings, setDraftSettings] = useState<Record<string, any>>({});
  const [savingSettings, setSavingSettings] = useState(false);

  const loadPlugins = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await pluginsApi.list();
      setPlugins(res.data || []);
    } catch (err: any) {
      setError(err?.message || 'Failed to load plugins');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadPlugins();
  }, []);

  const handleToggle = async (plugin: PluginEntity) => {
    setBusyId(plugin.manifest.id);
    try {
      if (plugin.status === 'enabled') {
        await pluginsApi.disable(plugin.manifest.id);
        toast.success(`Disabled plugin "${plugin.manifest.name}"`);
      } else {
        await pluginsApi.enable(plugin.manifest.id);
        toast.success(`Enabled plugin "${plugin.manifest.name}"`);
      }
      await loadPlugins();
    } catch (err: any) {
      toast.error('Plugin state change failed', err?.message);
    } finally {
      setBusyId(null);
    }
  };

  const openConfigModal = (plugin: PluginEntity) => {
    const schema = plugin.manifest.settingsSchema || {};
    const initial: Record<string, any> = {};
    for (const [key, def] of Object.entries(schema)) {
      initial[key] = plugin.settings?.[key] !== undefined ? plugin.settings[key] : def.default;
    }
    for (const [key, val] of Object.entries(plugin.settings || {})) {
      if (!(key in initial)) {
        initial[key] = val;
      }
    }
    setDraftSettings(initial);
    setConfiguring(plugin);
  };

  const handleSaveSettings = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!configuring) return;
    setSavingSettings(true);
    try {
      await pluginsApi.updateSettings(configuring.manifest.id, draftSettings);
      toast.success(`Saved settings for "${configuring.manifest.name}"`);
      setConfiguring(null);
      await loadPlugins();
    } catch (err: any) {
      toast.error('Failed to update plugin settings', err?.message);
    } finally {
      setSavingSettings(false);
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
              <Puzzle className="w-4 h-4 text-sky-600" />
              <span>Installed Extensions & Plugins ({plugins.length})</span>
            </span>
          }
          description="Enable, disable, and configure CMS plugins. State and configuration persist in database storage across restarts."
        />
        <CardContent>
          {plugins.length === 0 ? (
            <p className="text-xs text-slate-500 py-8 text-center">No plugins discovered or registered.</p>
          ) : (
            <div className="divide-y divide-slate-100">
              {plugins.map((p) => {
                const enabled = p.status === 'enabled';
                const hasSettings =
                  Object.keys(p.manifest.settingsSchema || {}).length > 0 ||
                  Object.keys(p.settings || {}).length > 0;
                return (
                  <div key={p.manifest.id} className="py-4 flex flex-col lg:flex-row lg:items-start justify-between gap-4">
                    <div className="space-y-1.5">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-sm font-semibold text-slate-800">{p.manifest.name}</span>
                        <Badge variant="neutral">v{p.manifest.version}</Badge>
                        <Badge variant={enabled ? 'success' : p.status === 'error' ? 'error' : 'neutral'}>
                          {p.status}
                        </Badge>
                        <Badge variant="info">{p.manifest.runtime}</Badge>
                      </div>
                      <p className="text-xs text-slate-600">{p.manifest.description}</p>
                      <p className="text-[11px] text-slate-400 font-mono">
                        ID: {p.manifest.id} • Author: {p.manifest.author || 'Unknown'}
                      </p>
                      {p.error_msg && <p className="text-xs text-rose-600">{p.error_msg}</p>}
                      {(p.manifest.routes?.length || 0) > 0 && (
                        <div className="flex flex-wrap gap-1.5 pt-1">
                          {p.manifest.routes?.map((rt) => (
                            <Badge key={`${rt.method}:${rt.path}`} variant="neutral">
                              {rt.method} /plugins/ext/{p.manifest.id}
                              {rt.path}
                              {rt.permission ? ` (${rt.permission})` : ''}
                            </Badge>
                          ))}
                        </div>
                      )}
                    </div>

                    {canEdit && (
                      <div className="flex items-center gap-2 shrink-0">
                        {hasSettings && (
                          <Button type="button" size="sm" variant="outline" onClick={() => openConfigModal(p)}>
                            <SettingsIcon className="w-3.5 h-3.5 mr-1" />
                            Configure
                          </Button>
                        )}
                        <Button
                          type="button"
                          size="sm"
                          variant={enabled ? 'secondary' : 'primary'}
                          isLoading={busyId === p.manifest.id}
                          onClick={() => void handleToggle(p)}
                        >
                          {enabled ? (
                            <>
                              <PowerOff className="w-3.5 h-3.5 mr-1" />
                              Disable
                            </>
                          ) : (
                            <>
                              <Power className="w-3.5 h-3.5 mr-1" />
                              Enable
                            </>
                          )}
                        </Button>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      <Modal
        isOpen={Boolean(configuring)}
        onClose={() => setConfiguring(null)}
        title={configuring ? `Plugin Settings: ${configuring.manifest.name}` : 'Plugin Settings'}
        maxWidth="lg"
      >
        {configuring && (
          <form onSubmit={(e) => void handleSaveSettings(e)} className="space-y-4">
            {Object.entries(configuring.manifest.settingsSchema || {}).map(([key, def]) => {
              const label = def.label || key;
              const value = draftSettings[key];
              if (def.type === 'boolean') {
                return (
                  <div
                    key={key}
                    className="flex items-center justify-between p-3.5 rounded-lg bg-slate-50 border border-slate-200"
                  >
                    <div>
                      <p className="text-xs font-semibold text-slate-800">{label}</p>
                      {def.description && <p className="text-[11px] text-slate-500">{def.description}</p>}
                    </div>
                    <Switch
                      label={label}
                      checked={Boolean(value)}
                      onChange={(checked) => setDraftSettings({ ...draftSettings, [key]: checked })}
                    />
                  </div>
                );
              }
              if (def.type === 'number') {
                return (
                  <Input
                    key={key}
                    label={label}
                    type="number"
                    min={def.min}
                    max={def.max}
                    value={Number(value ?? def.default ?? 0)}
                    onChange={(e) =>
                      setDraftSettings({ ...draftSettings, [key]: Number(e.target.value) })
                    }
                    helperText={def.description}
                  />
                );
              }
              return (
                <Input
                  key={key}
                  label={label}
                  value={String(value ?? def.default ?? '')}
                  onChange={(e) => setDraftSettings({ ...draftSettings, [key]: e.target.value })}
                  helperText={def.description}
                />
              );
            })}

            <div className="flex justify-end gap-2 pt-2 border-t border-slate-100">
              <Button type="button" size="sm" variant="secondary" onClick={() => setConfiguring(null)}>
                Cancel
              </Button>
              <Button type="submit" size="sm" isLoading={savingSettings}>
                Save Plugin Settings
              </Button>
            </div>
          </form>
        )}
      </Modal>
    </div>
  );
};

