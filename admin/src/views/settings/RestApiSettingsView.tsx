import React, { useCallback } from 'react';
import { settingsApi } from '../../api/endpoints';
import { useSettingsForm } from '../../hooks/useSettingsForm';
import { Alert } from '../../components/ui/Alert';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import type { RestAPISettings } from '../../types';

export interface RestApiSettingsViewProps {
  canEdit: boolean;
  onDirtyChange: (dirty: boolean) => void;
  registerHandlers: (handlers: {
    save: () => Promise<void>;
    discard: () => void;
    isSaving: boolean;
  }) => void;
}

export const RestApiSettingsView: React.FC<RestApiSettingsViewProps> = ({
  canEdit,
  onDirtyChange,
  registerHandlers,
}) => {
  const load = useCallback(async () => {
    const res = await settingsApi.getRestApi();
    return res.data;
  }, []);

  const save = useCallback(async (value: RestAPISettings) => {
    const res = await settingsApi.updateRestApi(value);
    return res.data;
  }, []);

  const validate = useCallback((v: RestAPISettings): string | null => {
    if (v.default_limit < 1 || v.default_limit > 500) {
      return 'Default page limit must be between 1 and 500.';
    }
    if (v.max_limit < 1 || v.max_limit > 1000) {
      return 'Maximum page limit must be between 1 and 1000.';
    }
    if (v.default_limit > v.max_limit) {
      return 'Default page limit cannot exceed maximum page limit.';
    }
    if (v.max_populate_depth < 1 || v.max_populate_depth > 8) {
      return 'Max populate depth must be between 1 and 8.';
    }
    return null;
  }, []);

  const { form, setForm, isLoading, error } = useSettingsForm<RestAPISettings>({
    load,
    save,
    validate,
    successTitle: 'REST API settings saved',
    successMessage: 'Pagination and populate rules are now active for headless API traffic.',
    loadErrorFallback: 'Failed to load REST API settings.',
    saveErrorFallback: 'Failed to save REST API settings.',
    onDirtyChange,
    registerHandlers,
  });

  if (isLoading || !form) {
    return <SettingsCardSkeleton />;
  }

  return (
    <div className="space-y-6">
      {error && (
        <Alert variant="error" title="Unable to save REST API settings">
          {error}
        </Alert>
      )}

      <Card>
        <CardHeader
          title="Pagination & Response Shaping"
          description="Control default query page sizes, maximum limits, total count metadata, and deep component nesting."
        />
        <CardContent className="space-y-5">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <Input
              label="Default limit"
              type="number"
              min={1}
              max={500}
              disabled={!canEdit}
              value={form.default_limit}
              onChange={(e) =>
                setForm({ ...form, default_limit: Number(e.target.value) || 0 })
              }
              helperText="Applied when ?limit is omitted (1–500)."
            />
            <Input
              label="Max limit"
              type="number"
              min={1}
              max={1000}
              disabled={!canEdit}
              value={form.max_limit}
              onChange={(e) =>
                setForm({ ...form, max_limit: Number(e.target.value) || 0 })
              }
              helperText="Hard cap enforced on client ?limit queries (1–1000)."
            />
            <Input
              label="Max populate depth"
              type="number"
              min={1}
              max={8}
              disabled={!canEdit}
              value={form.max_populate_depth}
              onChange={(e) =>
                setForm({ ...form, max_populate_depth: Number(e.target.value) || 1 })
              }
              helperText="Maximum nested component/dynamic zone depth (1–8)."
            />
          </div>

          <div className="divide-y divide-neutral-200 border border-neutral-200 rounded-md">
            <div className="p-4 flex items-center justify-between gap-4">
              <div>
                <p className="text-sm font-medium text-neutral-900">Include total count</p>
                <p className="text-xs text-neutral-500">
                  Return total matching entry counts alongside paginated collection responses.
                </p>
              </div>
              <Switch
                checked={form.with_count}
                disabled={!canEdit}
                onChange={(checked) => setForm({ ...form, with_count: checked })}
              />
            </div>
            <div className="p-4 flex items-center justify-between gap-4">
              <div>
                <p className="text-sm font-medium text-neutral-900">Expose draft preview parameter</p>
                <p className="text-xs text-neutral-500">
                  Allow authorized callers with read:content to inspect draft records using ?preview=true.
                </p>
              </div>
              <Switch
                checked={form.expose_draft_preview}
                disabled={!canEdit}
                onChange={(checked) => setForm({ ...form, expose_draft_preview: checked })}
              />
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

